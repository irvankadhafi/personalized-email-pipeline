package campaign

import (
	"context"
	"runtime"
	"sync"
	"time"
)

func Run(ctx context.Context, next NextFunc, cfg RunConfig) RunReport {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.GOMAXPROCS(0)
	}
	if cfg.Settlement <= 0 {
		cfg.Settlement = DefaultSettlement
	}
	if cfg.ResponseBound <= 0 {
		cfg.ResponseBound = DefaultResponseBound
	}
	if cfg.Sink == nil {
		digest := NewDigestSink()
		cfg.Sink = digest.Accept
	}
	start := time.Now()
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	ready := make(chan workerReady, cfg.Workers)
	results := make(chan workerResult, cfg.Workers)
	var workers sync.WaitGroup
	for range cfg.Workers {
		workers.Add(1)
		go runWorker(workerCtx, &workers, ready, results, cfg.Sink)
	}
	peak := startHeapSampler(workerCtx)

	report := RunReport{
		AccountingScope: "full",
		InvalidReasons:  make(map[Reason]ReasonSummary),
		FailedReasons:   make(map[Reason]ReasonSummary),
		Workers:         cfg.Workers,
		Settlement:      cfg.Settlement,
		ResponseBound:   cfg.ResponseBound,
	}
	seen := make(map[string]struct{})
	queue := make([]job, 0, cfg.Workers*2)
	active := make(map[int64]struct{})
	inputDone := false
	admissionOpen := true
	var cancelAt, deadline time.Time
	lastCancellationCheck := time.Now()

	for !inputDone || len(queue) > 0 || len(active) > 0 {
		if ctx.Err() != nil && !report.Cancelled {
			report.Cancelled = true
			cancelAt = time.Now()
			admissionOpen = false
			deadline = cancelAt.Add(cfg.Settlement)
			report.MeasuredResponse = time.Since(lastCancellationCheck)
			report.Counts.Unprocessed += int64(len(queue))
			queue = queue[:0]
		} else {
			lastCancellationCheck = time.Now()
		}
		if !deadline.IsZero() && time.Now().After(deadline) && len(active) > 0 {
			stopWorkers()
			for ordinal := range active {
				report.Counts.Failed++
				addReason(report.FailedReasons, ReasonInterrupted, ordinal, 3)
				delete(active, ordinal)
			}
		}

		progressed := false
		if !inputDone && (len(queue) < cap(queue) || !admissionOpen) {
			record, ok, err := next()
			progressed = true
			if err != nil {
				report.Fatal = true
				report.AccountingScope = "prefix_only"
				inputDone = true
				admissionOpen = false
				report.Counts.Unprocessed += int64(len(queue))
				queue = queue[:0]
				deadline = time.Now().Add(cfg.Settlement)
			} else if !ok {
				inputDone = true
			} else {
				classifyRecord(&report, seen, &queue, record, admissionOpen)
			}
		}

		for {
			select {
			case result := <-results:
				progressed = true
				if _, ok := active[result.ordinal]; !ok {
					continue
				}
				delete(active, result.ordinal)
				if result.reason == "" && (deadline.IsZero() || !result.accepted.After(deadline)) {
					report.Counts.Completed++
					report.Samples.Add(Sample{Ordinal: result.ordinal, Category: result.category, Text: SyntheticSample(result.ordinal, result.category)}, 2)
				} else {
					reason := result.reason
					if reason == "" {
						reason = ReasonInterrupted
					}
					report.Counts.Failed++
					addReason(report.FailedReasons, reason, result.ordinal, 3)
				}
			default:
				goto drained
			}
		}
	drained:
		if admissionOpen && len(queue) > 0 {
			select {
			case request := <-ready:
				granted := queue[0]
				copy(queue, queue[1:])
				queue = queue[:len(queue)-1]
				active[granted.recipient.Ordinal] = struct{}{}
				report.Started++
				request.grant <- granted
				progressed = true
			default:
			}
		}
		if inputDone && len(queue) == 0 && len(active) == 0 {
			break
		}
		if !progressed {
			var readyForWork <-chan workerReady
			if admissionOpen && len(queue) > 0 {
				readyForWork = ready
			}
			select {
			case request := <-readyForWork:
				granted := queue[0]
				copy(queue, queue[1:])
				queue = queue[:len(queue)-1]
				active[granted.recipient.Ordinal] = struct{}{}
				report.Started++
				request.grant <- granted
			case result := <-results:
				if _, ok := active[result.ordinal]; ok {
					delete(active, result.ordinal)
					if result.reason == "" && (deadline.IsZero() || !result.accepted.After(deadline)) {
						report.Counts.Completed++
						report.Samples.Add(Sample{Ordinal: result.ordinal, Category: result.category, Text: SyntheticSample(result.ordinal, result.category)}, 2)
					} else {
						reason := result.reason
						if reason == "" {
							reason = ReasonInterrupted
						}
						report.Counts.Failed++
						addReason(report.FailedReasons, reason, result.ordinal, 3)
					}
				}
			case <-ctx.Done():
			case <-time.After(time.Millisecond):
			}
		}
	}

	stopWorkers()
	workers.Wait()
	report.PeakHeapInuseBytes = <-peak
	report.ProcessingElapsed = time.Since(start)
	report.Outcome = DeriveOutcome(report.Counts, report.Cancelled, report.Fatal)
	return report
}

func classifyRecord(report *RunReport, seen map[string]struct{}, queue *[]job, record SourceRecord, admissionOpen bool) {
	report.Counts.Examined++
	if record.ParserReason != "" {
		report.Counts.Invalid++
		addReason(report.InvalidReasons, record.ParserReason, record.Ordinal, 3)
		return
	}
	identity, valid := NormalizeEmail(record.Email)
	if !valid {
		report.Counts.Invalid++
		addReason(report.InvalidReasons, ReasonInvalidEmail, record.Ordinal, 3)
		return
	}
	if _, duplicate := seen[identity]; duplicate {
		report.Counts.Duplicate++
		return
	}
	seen[identity] = struct{}{}
	report.Counts.Eligible++
	name, named := UsableName(record.Name)
	category := CategoryFallback
	if named {
		category = CategoryNamed
	}
	if !admissionOpen {
		report.Counts.Unprocessed++
		return
	}
	*queue = append(*queue, job{recipient: Recipient{Ordinal: record.Ordinal, Email: record.Email, Identity: identity, Name: name, Category: category}})
}
