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
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
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
