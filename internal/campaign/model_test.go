package campaign

import "testing"

func TestCountsValidateAndOutcome(t *testing.T) {
	tests := []struct {
		name    string
		counts  Counts
		cancel  bool
		fatal   bool
		want    Outcome
		wantErr bool
	}{
		{"success", Counts{Examined: 2, Eligible: 2, Completed: 2}, false, false, Success, false},
		{"duplicates do not downgrade", Counts{Examined: 2, Duplicate: 1, Eligible: 1, Completed: 1}, false, false, Success, false},
		{"invalid is partial", Counts{Examined: 2, Invalid: 1, Eligible: 1, Completed: 1}, false, false, PartialSuccess, false},
		{"all eligible failed is partial", Counts{Examined: 1, Eligible: 1, Failed: 1}, false, false, PartialSuccess, false},
		{"empty is failure", Counts{}, false, false, Failure, false},
		{"cancelled", Counts{Examined: 1, Eligible: 1, Unprocessed: 1}, true, false, Interrupted, false},
		{"fatal wins", Counts{Examined: 1, Eligible: 1, Failed: 1}, true, true, Failure, false},
		{"unbalanced examined", Counts{Examined: 2, Eligible: 1, Completed: 1}, false, false, Failure, true},
		{"unbalanced eligible", Counts{Examined: 1, Eligible: 1}, false, false, Failure, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.counts.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := DeriveOutcome(tt.counts, tt.cancel, tt.fatal); got != tt.want {
				t.Fatalf("DeriveOutcome() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBoundedSamplesKeepLowestOrdinals(t *testing.T) {
	var samples Samples
	for _, ordinal := range []int64{9, 2, 5, 1} {
		samples.Add(Sample{Ordinal: ordinal, Category: CategoryNamed}, 2)
	}
	got := samples.For(CategoryNamed)
	if len(got) != 2 || got[0].Ordinal != 1 || got[1].Ordinal != 2 {
		t.Fatalf("samples = %#v, want ordinals 1,2", got)
	}
}
