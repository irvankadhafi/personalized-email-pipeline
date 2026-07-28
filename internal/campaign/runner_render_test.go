package campaign

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunner_passes_complete_selected_html_to_sink_before_completion(t *testing.T) {
	// Given
	records := []SourceRecord{
		{Ordinal: 1, Email: "ada@example.com", Name: "Ada"},
		{Ordinal: 2, Email: "guest@example.com"},
	}
	accepted := make(chan RenderedMessage, len(records))

	// When
	report := Run(context.Background(), SliceSource(records), RunConfig{
		Workers: 2,
		Format:  HTMLFormat,
		MessageSink: func(_ context.Context, message RenderedMessage) error {
			accepted <- message
			return nil
		},
	})
	close(accepted)

	// Then
	if report.Counts.Completed != int64(len(records)) || report.Started != int64(len(records)) {
		t.Fatalf("report=%#v", report)
	}
	acceptedCount := 0
	for message := range accepted {
		acceptedCount++
		if message.Format() != HTMLFormat || !strings.Contains(string(message.Bytes()), promotion) {
			t.Fatalf("format=%q body=%q", message.Format(), message.Bytes())
		}
	}
	if acceptedCount != len(records) {
		t.Fatalf("accepted=%d want=%d", acceptedCount, len(records))
	}
}

func TestRunner_typed_sink_rejection_preserves_failure_accounting_for_each_format(t *testing.T) {
	for _, format := range []Format{TextFormat, HTMLFormat} {
		t.Run(format.String(), func(t *testing.T) {
			// Given
			records := []SourceRecord{{Ordinal: 1, Email: "a@example.com", Name: "Ada"}}

			// When
			report := Run(context.Background(), SliceSource(records), RunConfig{
				Workers: 1,
				Format:  format,
				MessageSink: func(context.Context, RenderedMessage) error {
					return errors.New("private sink detail")
				},
			})

			// Then
			if report.Counts != (Counts{Examined: 1, Eligible: 1, Failed: 1}) || report.FailedReasons[ReasonSink].Count != 1 {
				t.Fatalf("report=%#v", report)
			}
		})
	}
}
