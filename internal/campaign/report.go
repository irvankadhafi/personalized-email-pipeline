package campaign

import (
	"encoding/json"
	"errors"
	"slices"
)

var ErrReportInvariant = errors.New("report invariant failed")

type reportCounts struct {
	Examined    int64 `json:"examined"`
	Invalid     int64 `json:"invalid"`
	Duplicate   int64 `json:"duplicate"`
	Eligible    int64 `json:"eligible"`
	Started     int64 `json:"started"`
	Completed   int64 `json:"completed"`
	Failed      int64 `json:"failed"`
	Unprocessed int64 `json:"unprocessed"`
}

type reportSample struct {
	Ordinal  int64    `json:"ordinal"`
	Category Category `json:"category"`
	Text     string   `json:"text"`
}

type reportProportions struct {
	Invalid     float64 `json:"invalid"`
	Duplicate   float64 `json:"duplicate"`
	Failed      float64 `json:"failed"`
	Unprocessed float64 `json:"unprocessed"`
}

type reportDocument struct {
	Outcome                        Outcome                  `json:"outcome"`
	AccountingScope                string                   `json:"accounting_scope"`
	Counts                         reportCounts             `json:"counts"`
	InvalidReasons                 map[Reason]ReasonSummary `json:"invalid_reasons,omitempty"`
	FailedReasons                  map[Reason]ReasonSummary `json:"failed_reasons,omitempty"`
	Samples                        []reportSample           `json:"samples,omitempty"`
	ProcessingElapsedSeconds       *float64                 `json:"processing_elapsed_seconds,omitempty"`
	ExaminedRecordsPerSecond       *float64                 `json:"examined_records_per_second,omitempty"`
	CompletedRenderingsPerSecond   *float64                 `json:"completed_renderings_per_second,omitempty"`
	PeakHeapInuseBytes             *uint64                  `json:"peak_heap_inuse_bytes,omitempty"`
	HeapSampleIntervalMilliseconds *int64                   `json:"heap_sample_interval_milliseconds,omitempty"`
	Workers                        *int                     `json:"workers,omitempty"`
	QueueCapacity                  *int                     `json:"queue_capacity,omitempty"`
	ResponseBoundMilliseconds      *int64                   `json:"response_bound_milliseconds,omitempty"`
	MeasuredResponseMilliseconds   *int64                   `json:"measured_response_milliseconds,omitempty"`
	SettlementMilliseconds         *int64                   `json:"settlement_milliseconds,omitempty"`
	Cancelled                      bool                     `json:"cancelled"`
	Fatal                          bool                     `json:"fatal"`
	Proportions                    *reportProportions       `json:"proportions,omitempty"`
	Mode                           *ModeEvidence            `json:"mode,omitempty"`
	TestDelivery                   *TestDeliveryEvidence    `json:"test_delivery,omitempty"`
	Distributed                    *DistributedEvidence     `json:"distributed,omitempty"`
}

func MarshalReport(report RunReport) ([]byte, error) {
	if !validOptionalEvidence(report) || report.Counts.Validate() != nil || report.Started != report.Counts.Completed+report.Counts.Failed ||
		report.Outcome != DeriveOutcome(report.Counts, report.Cancelled, report.Fatal) ||
		(report.Fatal && report.AccountingScope != "prefix_only" && report.AccountingScope != "unknown") ||
		(!report.Fatal && report.AccountingScope != "full") {
		return nil, ErrReportInvariant
	}
	elapsed := report.ProcessingElapsed.Seconds()
	document := reportDocument{
		Outcome: report.Outcome, AccountingScope: report.AccountingScope,
		Counts: reportCounts{
			Examined: report.Counts.Examined, Invalid: report.Counts.Invalid,
			Duplicate: report.Counts.Duplicate, Eligible: report.Counts.Eligible,
			Started: report.Started, Completed: report.Counts.Completed,
			Failed: report.Counts.Failed, Unprocessed: report.Counts.Unprocessed,
		},
		InvalidReasons: report.InvalidReasons, FailedReasons: report.FailedReasons,
		Samples: reportSamples(report.Samples), Cancelled: report.Cancelled, Fatal: report.Fatal,
		Mode: report.Mode, TestDelivery: report.TestDelivery, Distributed: report.Distributed,
	}
	if report.Fatal {
		return json.Marshal(document)
	}
	document.ProcessingElapsedSeconds = &elapsed
	document.PeakHeapInuseBytes = &report.PeakHeapInuseBytes
	heapInterval := HeapSampleInterval.Milliseconds()
	document.HeapSampleIntervalMilliseconds = &heapInterval
	document.Workers = &report.Workers
	queueCapacity := report.Workers * 2
	document.QueueCapacity = &queueCapacity
	responseBound := report.ResponseBound.Milliseconds()
	document.ResponseBoundMilliseconds = &responseBound
	measuredResponse := report.MeasuredResponse.Milliseconds()
	document.MeasuredResponseMilliseconds = &measuredResponse
	settlement := report.Settlement.Milliseconds()
	document.SettlementMilliseconds = &settlement
	document.Proportions = &reportProportions{
		Invalid: ratio(report.Counts.Invalid, report.Counts.Examined), Duplicate: ratio(report.Counts.Duplicate, report.Counts.Examined),
		Failed: ratio(report.Counts.Failed, report.Counts.Eligible), Unprocessed: ratio(report.Counts.Unprocessed, report.Counts.Eligible),
	}
	if report.Mode != nil && report.Mode.Backend == BackendAsynq {
		document.PeakHeapInuseBytes = nil
		document.HeapSampleIntervalMilliseconds = nil
		document.Workers = nil
		document.QueueCapacity = nil
		document.ResponseBoundMilliseconds = nil
		document.MeasuredResponseMilliseconds = nil
		document.SettlementMilliseconds = nil
	}
	if elapsed > 0 {
		examinedRate := float64(report.Counts.Examined) / elapsed
		completedRate := float64(report.Counts.Completed) / elapsed
		document.ExaminedRecordsPerSecond = &examinedRate
		document.CompletedRenderingsPerSecond = &completedRate
	}
	return json.Marshal(document)
}

func validOptionalEvidence(report RunReport) bool {
	if report.Mode == nil {
		return report.TestDelivery == nil && report.Distributed == nil && report.AccountingScope != "unknown"
	}
	mode := *report.Mode
	if mode.Backend == BackendLocal && mode.Sink == SinkTestInbox {
		evidence := report.TestDelivery
		return evidence != nil && report.Distributed == nil && evidence.Confirmed >= 0 && evidence.Rejected >= 0 &&
			evidence.Transport >= 0 && evidence.Indeterminate >= 0 && evidence.Confirmed == report.Counts.Completed &&
			evidence.Rejected+evidence.Transport+evidence.Indeterminate == report.Counts.Failed && report.AccountingScope != "unknown"
	}
	if mode.Backend == BackendAsynq {
		evidence := report.Distributed
		if evidence == nil || evidence.KnownEnqueued < 0 || evidence.KnownTerminal < 0 ||
			evidence.Unknown < 0 || evidence.KnownTerminal > evidence.KnownEnqueued {
			return false
		}
		if mode.Sink == SinkDryRun && report.TestDelivery != nil {
			return false
		}
		if mode.Sink == SinkTestInbox {
			delivery := report.TestDelivery
			if delivery == nil || delivery.Confirmed < 0 || delivery.Rejected < 0 || delivery.Transport < 0 || delivery.Indeterminate < 0 ||
				delivery.Confirmed != report.Counts.Completed || delivery.Rejected+delivery.Transport+delivery.Indeterminate != report.Counts.Failed {
				return false
			}
		} else if mode.Sink != SinkDryRun {
			return false
		}
		if report.AccountingScope == "unknown" {
			return report.Fatal && report.Outcome == Failure && evidence.Unknown > 0
		}
		return evidence.Unknown == 0
	}
	return mode.Backend == BackendLocal && mode.Sink == SinkDryRun && report.TestDelivery == nil &&
		report.Distributed == nil && report.AccountingScope != "unknown"
}

func reportSamples(samples Samples) []reportSample {
	result := make([]reportSample, 0, 4)
	for _, category := range []Category{CategoryNamed, CategoryFallback} {
		for _, sample := range samples.For(category) {
			result = append(result, reportSample{Ordinal: sample.Ordinal, Category: sample.Category, Text: sample.Text})
		}
	}
	slices.SortFunc(result, func(a, b reportSample) int { return int(a.Ordinal - b.Ordinal) })
	return result
}

func ratio(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
