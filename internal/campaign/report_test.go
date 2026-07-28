package campaign

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMarshalReportIncludesPerformanceAndSafeSamples(t *testing.T) {
	report := RunReport{
		Outcome: Success, AccountingScope: "full",
		Counts:            Counts{Examined: 1, Eligible: 1, Completed: 1},
		Started:           1,
		ProcessingElapsed: time.Second, PeakHeapInuseBytes: 1024, Workers: 2,
		Settlement: DefaultSettlement, ResponseBound: DefaultResponseBound,
	}
	report.Samples.Add(Sample{Ordinal: 1, Category: CategoryNamed, Text: SyntheticSample(1, CategoryNamed)}, 2)
	data, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "@") || !strings.Contains(string(data), `"completed_renderings_per_second":1`) {
		t.Fatalf("report=%s", data)
	}
	for _, optional := range []string{`"mode"`, `"test_delivery"`, `"distributed"`, `"format"`, `"page_duration"`} {
		if strings.Contains(string(data), optional) {
			t.Fatalf("local report contains optional section %s: %s", optional, data)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestMarshalReportIncludesTestDeliveryEvidence(t *testing.T) {
	data, err := MarshalReport(RunReport{
		Outcome: Success, AccountingScope: "full",
		Counts: Counts{Examined: 1, Eligible: 1, Completed: 1}, Started: 1,
		Mode:         &ModeEvidence{Backend: BackendLocal, Sink: SinkTestInbox},
		TestDelivery: &TestDeliveryEvidence{Confirmed: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"test_delivery":{"confirmed":1,"rejected":0,"transport":0,"indeterminate":0}`) {
		t.Fatalf("report=%s", data)
	}
}

func TestMarshalReportIncludesDeliveryTransportEvidence(t *testing.T) {
	data, err := MarshalReport(RunReport{
		Outcome: PartialSuccess, AccountingScope: "full",
		Counts: Counts{Examined: 1, Eligible: 1, Failed: 1}, Started: 1,
		Mode:         &ModeEvidence{Backend: BackendLocal, Sink: SinkTestInbox},
		TestDelivery: &TestDeliveryEvidence{Transport: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"test_delivery":{"confirmed":0,"rejected":0,"transport":1,"indeterminate":0}`) {
		t.Fatalf("report=%s", data)
	}
}

func TestMarshalReportIncludesDistributedTestDeliveryEvidence(t *testing.T) {
	data, err := MarshalReport(RunReport{
		Outcome: PartialSuccess, AccountingScope: "full",
		Counts: Counts{Examined: 2, Eligible: 2, Completed: 1, Failed: 1}, Started: 2,
		Mode:         &ModeEvidence{Backend: BackendAsynq, Sink: SinkTestInbox},
		TestDelivery: &TestDeliveryEvidence{Confirmed: 1, Indeterminate: 1},
		Distributed:  &DistributedEvidence{KnownEnqueued: 2, KnownTerminal: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"mode":{"backend":"asynq","sink":"test-inbox"}`) ||
		!strings.Contains(string(data), `"test_delivery":{"confirmed":1,"rejected":0,"transport":0,"indeterminate":1}`) {
		t.Fatalf("report=%s", data)
	}
	for _, localOnly := range []string{`"workers"`, `"queue_capacity"`, `"peak_heap_inuse_bytes"`, `"heap_sample_interval_milliseconds"`} {
		if strings.Contains(string(data), localOnly) {
			t.Fatalf("distributed report contains local-only field %s: %s", localOnly, data)
		}
	}
}

func TestMarshalReportUnknownDistributedOmitsReconciliationClaims(t *testing.T) {
	data, err := MarshalReport(RunReport{
		Outcome: Failure, AccountingScope: "unknown", Fatal: true,
		Counts: Counts{Examined: 4, Eligible: 4, Completed: 2, Failed: 1, Unprocessed: 1}, Started: 3,
		Mode:        &ModeEvidence{Backend: BackendAsynq, Sink: SinkDryRun},
		Distributed: &DistributedEvidence{KnownEnqueued: 4, KnownTerminal: 4, Unknown: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"processing_elapsed_seconds", "records_per_second", "proportions"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("unknown report contains %q: %s", forbidden, data)
		}
	}
	if !strings.Contains(string(data), `"distributed":{"known_enqueued":4,"known_terminal":4,"unknown":2}`) {
		t.Fatalf("report=%s", data)
	}
}

func TestMarshalReportRejectsInvalidOptionalEvidence(t *testing.T) {
	tests := []RunReport{
		{
			Outcome: Success, AccountingScope: "full",
			Counts: Counts{Examined: 1, Eligible: 1, Completed: 1}, Started: 1,
			TestDelivery: &TestDeliveryEvidence{Confirmed: 1},
		},
		{
			Outcome: Failure, AccountingScope: "unknown", Fatal: true,
			Mode:        &ModeEvidence{Backend: BackendAsynq, Sink: SinkDryRun},
			Distributed: &DistributedEvidence{KnownEnqueued: 1, KnownTerminal: 2, Unknown: 1},
		},
	}
	for _, report := range tests {
		if _, err := MarshalReport(report); err != ErrReportInvariant {
			t.Fatalf("report=%#v error=%v", report, err)
		}
	}
}

func TestMarshalReportRejectsUnbalancedCounts(t *testing.T) {
	_, err := MarshalReport(RunReport{Counts: Counts{Examined: 2, Eligible: 1, Completed: 1}})
	if err == nil || err.Error() != "report invariant failed" {
		t.Fatalf("error=%v", err)
	}
}

func TestMarshalReportRejectsInconsistentTerminalState(t *testing.T) {
	tests := []RunReport{
		{Outcome: Success, AccountingScope: "full", Fatal: true},
		{Outcome: Interrupted, AccountingScope: "full", Cancelled: false},
		{Outcome: Failure, AccountingScope: "prefix_only", Fatal: false},
		{Outcome: Success, AccountingScope: "unknown"},
	}
	for _, report := range tests {
		if _, err := MarshalReport(report); err != ErrReportInvariant {
			t.Fatalf("report=%#v error=%v", report, err)
		}
	}
}

func TestMarshalFatalReportOmitsPerformanceClaims(t *testing.T) {
	data, err := MarshalReport(RunReport{
		Outcome: Failure, AccountingScope: "prefix_only", Fatal: true,
		Counts: Counts{Examined: 1, Eligible: 1, Failed: 1}, Started: 1,
		ProcessingElapsed: time.Second, PeakHeapInuseBytes: 1024, Workers: 2,
		Settlement: DefaultSettlement, ResponseBound: DefaultResponseBound,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"processing_elapsed_seconds", "records_per_second", "peak_heap_inuse_bytes", "proportions"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("fatal report contains %q: %s", forbidden, data)
		}
	}
	if !strings.Contains(string(data), `"fatal":true`) || !strings.Contains(string(data), `"accounting_scope":"prefix_only"`) {
		t.Fatalf("fatal report=%s", data)
	}
}
