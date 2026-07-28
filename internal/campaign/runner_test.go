package campaign

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunnerReconcilesValidationDedupAndRendering(t *testing.T) {
	records := []SourceRecord{
		{Ordinal: 1, Email: " Ada@Example.com ", Name: "Ada"},
		{Ordinal: 2, Email: "ada@example.com", Name: "Duplicate"},
		{Ordinal: 3, ParserReason: ReasonInvalidCSV},
		{Ordinal: 4, Email: "plus+tag@example.com"},
	}
	report := Run(context.Background(), SliceSource(records), RunConfig{Workers: 2})
	want := Counts{Examined: 4, Invalid: 1, Duplicate: 1, Eligible: 2, Completed: 2}
	if report.Counts != want || report.Outcome != PartialSuccess {
		t.Fatalf("report=%#v want counts=%#v partial", report, want)
	}
	if err := report.Counts.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerSinkFailureIsPartial(t *testing.T) {
	report := Run(context.Background(), SliceSource([]SourceRecord{{Ordinal: 1, Email: "a@example.com"}}), RunConfig{
		Workers: 1,
		Sink:    func(context.Context, []byte) error { return errors.New("secret") },
	})
	if report.Counts.Failed != 1 || report.Counts.Completed != 0 || report.Outcome != PartialSuccess {
		t.Fatalf("report=%#v", report)
	}
}

func TestRunnerCompletesConsecutiveEligibleRecords(t *testing.T) {
	records := make([]SourceRecord, 4)
	for i := range records {
		records[i] = SourceRecord{Ordinal: int64(i + 1), Email: uniqueEmail(i)}
	}
	report := Run(context.Background(), SliceSource(records), RunConfig{Workers: 2})
	if report.Counts.Completed != 4 || report.Started != 4 || report.Counts.Validate() != nil {
		t.Fatalf("report=%#v", report)
	}
}

func TestRunnerCancellationDrainsAsUnprocessed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int64
	sink := func(ctx context.Context, _ []byte) error {
		if calls.Add(1) == 1 {
			cancel()
		}
		<-ctx.Done()
		return ctx.Err()
	}
	records := make([]SourceRecord, 100)
	for i := range records {
		records[i] = SourceRecord{Ordinal: int64(i + 1), Email: uniqueEmail(i)}
	}
	report := Run(ctx, SliceSource(records), RunConfig{Workers: 2, Settlement: 20 * time.Millisecond, Sink: sink})
	if report.Outcome != Interrupted || report.Counts.Examined != 100 || report.Counts.Eligible != 100 {
		t.Fatalf("report=%#v", report)
	}
	if report.Counts.Unprocessed == 0 || report.Counts.Failed == 0 || report.Counts.Validate() != nil {
		t.Fatalf("cancellation accounting=%#v", report.Counts)
	}
}

func TestRunnerCancellationResponseIncludesBlockedSourceRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const blocked = 25 * time.Millisecond
	reads := 0
	next := func() (SourceRecord, bool, error) {
		reads++
		if reads > 1 {
			return SourceRecord{}, false, nil
		}
		cancel()
		<-time.After(blocked)
		return SourceRecord{Ordinal: 1, Email: "a@example.com"}, true, nil
	}

	report := Run(ctx, next, RunConfig{Workers: 1})

	if !report.Cancelled || report.MeasuredResponse < blocked {
		t.Fatalf("cancelled=%t measured response=%s want at least %s", report.Cancelled, report.MeasuredResponse, blocked)
	}
}

func TestRunnerEmptyFails(t *testing.T) {
	report := Run(context.Background(), SliceSource(nil), RunConfig{Workers: 1})
	if report.Outcome != Failure || report.Counts != (Counts{}) {
		t.Fatalf("report=%#v", report)
	}
}

func TestRunnerFatalInputSettlesActiveWork(t *testing.T) {
	ctx := context.Background()
	records := 0
	started := make(chan struct{})
	next := func() (SourceRecord, bool, error) {
		records++
		switch records {
		case 1:
			return SourceRecord{Ordinal: 1, Email: "a@example.com"}, true, nil
		case 2:
			return SourceRecord{Ordinal: 2, Email: "b@example.com"}, true, nil
		}
		<-started
		return SourceRecord{}, false, errors.New("private source detail")
	}
	sink := func(ctx context.Context, _ []byte) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	report := Run(ctx, next, RunConfig{Workers: 1, Settlement: 5 * time.Millisecond, Sink: sink})
	if report.Outcome != Failure || !report.Fatal || report.AccountingScope != "prefix_only" {
		t.Fatalf("report=%#v", report)
	}
	if report.Counts != (Counts{Examined: 2, Eligible: 2, Failed: 1, Unprocessed: 1}) || report.Started != 1 {
		t.Fatalf("counts=%#v started=%d", report.Counts, report.Started)
	}
}

func TestRunnerFatalInputRestartsCancellationSettlement(t *testing.T) {
	for _, tc := range []struct {
		name         string
		releaseAfter time.Duration
		want         Counts
		wantOutcome  Outcome
		wantAccepted bool
	}{
		{
			name:         "after original deadline before restarted deadline",
			releaseAfter: 125 * time.Millisecond,
			want:         Counts{Examined: 3, Eligible: 3, Completed: 1, Unprocessed: 2},
			wantOutcome:  Failure,
			wantAccepted: true,
		},
		{
			name:         "after restarted deadline",
			releaseAfter: 200 * time.Millisecond,
			want:         Counts{Examined: 3, Eligible: 3, Failed: 1, Unprocessed: 2},
			wantOutcome:  Failure,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				inputCtx := cancellationErrContext{Context: ctx}
				started := make(chan struct{})
				cancelled := make(chan struct{})
				release := make(chan struct{})
				accepted := make(chan struct{})
				interrupted := make(chan struct{})
				helperDone := make(chan struct{})
				reads := 0
				next := func() (SourceRecord, bool, error) {
					reads++
					switch reads {
					case 1:
						return SourceRecord{Ordinal: 1, Email: "a@example.com"}, true, nil
					case 2:
						return SourceRecord{Ordinal: 2, Email: "b@example.com"}, true, nil
					case 3:
						<-started
						cancel()
						close(cancelled)
						return SourceRecord{Ordinal: 3, Email: "c@example.com"}, true, nil
					default:
						time.Sleep(50 * time.Millisecond)
						return SourceRecord{}, false, errors.New("private source detail")
					}
				}
				sink := func(ctx context.Context, _ []byte) error {
					close(started)
					select {
					case <-release:
						close(accepted)
						return nil
					case <-ctx.Done():
						close(interrupted)
						return ctx.Err()
					}
				}
				go func() {
					defer close(helperDone)
					<-cancelled
					time.Sleep(tc.releaseAfter)
					close(release)
				}()

				report := Run(inputCtx, next, RunConfig{Workers: 1, Settlement: 100 * time.Millisecond, Sink: sink})
				<-helperDone
				if report.Outcome != tc.wantOutcome || !report.Fatal || !report.Cancelled || report.AccountingScope != "prefix_only" {
					t.Fatalf("report=%#v", report)
				}
				if report.Counts != tc.want || report.Started != 1 || report.Counts.Validate() != nil {
					t.Fatalf("counts=%#v want=%#v started=%d", report.Counts, tc.want, report.Started)
				}
				if tc.wantAccepted {
					select {
					case <-accepted:
					default:
						t.Fatal("sink was not accepted before settlement")
					}
					select {
					case <-interrupted:
						t.Fatal("accepted sink was interrupted")
					default:
					}
				} else {
					select {
					case <-interrupted:
					case <-accepted:
						t.Fatal("sink was accepted after restarted deadline")
					default:
						t.Fatal("sink was neither accepted nor interrupted")
					}
				}
			})
		})
	}
}

type cancellationErrContext struct {
	context.Context
}

func (cancellationErrContext) Done() <-chan struct{} {
	return nil
}

func uniqueEmail(i int) string {
	return "user" + leftPadOrdinal(int64(i)) + "@example.com"
}
