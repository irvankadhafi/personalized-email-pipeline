package campaign

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRenderNamedAndFallback(t *testing.T) {
	for _, tt := range []struct {
		name      string
		recipient Recipient
		contains  string
		category  Category
	}{
		{"named", Recipient{Name: "Ada", Email: "ada@example.com"}, "Hello Ada,", CategoryNamed},
		{"fallback", Recipient{Email: "guest@example.com"}, "Hello there,", CategoryFallback},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var rendered string
			result := Render(context.Background(), tt.recipient, func(_ context.Context, message []byte) error {
				rendered = string(message)
				return nil
			})
			if result.Reason != "" || result.Category != tt.category || !strings.Contains(rendered, tt.contains) || !strings.Contains(rendered, "Exclusive offer") {
				t.Fatalf("result=%#v rendered=%q", result, rendered)
			}
		})
	}
}

func TestRenderRequiresSinkAcceptance(t *testing.T) {
	result := Render(context.Background(), Recipient{Name: "Ada"}, func(context.Context, []byte) error {
		return errors.New("SECRET sink detail")
	})
	if result.Reason != ReasonSink {
		t.Fatalf("reason=%q", result.Reason)
	}
}

func TestDigestSinkConsumesWithoutRetainingMessage(t *testing.T) {
	sink := NewDigestSink()
	message := []byte("private body")
	if err := sink.Accept(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if sink.Bytes() != int64(len(message)) || sink.Digest() == ([32]byte{}) {
		t.Fatalf("bytes=%d digest=%x", sink.Bytes(), sink.Digest())
	}
}
