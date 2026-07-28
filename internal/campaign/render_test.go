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

func TestRenderMessage_preserves_exact_text_bytes_for_named_recipient(t *testing.T) {
	// Given
	recipient := Recipient{Name: "Ada", Email: "ada@example.com"}
	want := []byte("Subject: Your exclusive offer\n\nHello Ada,\n\nExclusive offer: save 20% on your next purchase.\n")

	// When
	message, category := RenderMessage(recipient, TextFormat)

	// Then
	if category != CategoryNamed || message.Format() != TextFormat || string(message.Bytes()) != string(want) {
		t.Fatalf("category=%q format=%q bytes=%q", category, message.Format(), message.Bytes())
	}
}

func TestRenderMessage_preserves_exact_text_bytes_for_fallback_recipient(t *testing.T) {
	// Given
	recipient := Recipient{Email: "guest@example.com"}
	want := []byte("Subject: Your exclusive offer\n\nHello there,\n\nExclusive offer: save 20% on your next purchase.\n")

	// When
	message, category := RenderMessage(recipient, Format{})

	// Then
	if category != CategoryFallback || message.Format() != TextFormat || string(message.Bytes()) != string(want) {
		t.Fatalf("category=%q format=%q bytes=%q", category, message.Format(), message.Bytes())
	}
}

func TestParseFormat_accepts_closed_values_and_rejects_unsupported_input(t *testing.T) {
	// Given
	tests := []struct {
		raw  string
		want Format
	}{
		{raw: "", want: TextFormat},
		{raw: "text", want: TextFormat},
		{raw: "html", want: HTMLFormat},
	}

	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			// When
			got, err := ParseFormat(test.raw)

			// Then
			if err != nil || got != test.want {
				t.Fatalf("ParseFormat(%q)=(%q, %v) want %q", test.raw, got, err, test.want)
			}
		})
	}

	// When
	_, err := ParseFormat("markdown")

	// Then
	if !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("error=%v want %v", err, ErrInvalidFormat)
	}
}

func TestRenderMessage_html_has_text_semantics_and_escapes_personalization(t *testing.T) {
	// Given
	recipient := Recipient{Name: `<Ada onmouseover="alert(1)">& Co`, Email: "ada@example.com"}

	// When
	message, category := RenderMessage(recipient, HTMLFormat)
	body := string(message.Bytes())

	// Then
	if category != CategoryNamed || message.Format() != HTMLFormat {
		t.Fatalf("category=%q format=%q", category, message.Format())
	}
	for _, semantic := range []string{"Your exclusive offer", "Hello &lt;Ada onmouseover=&#34;alert(1)&#34;&gt;&amp; Co,", promotion} {
		if !strings.Contains(body, semantic) {
			t.Fatalf("HTML missing %q: %q", semantic, body)
		}
	}
	for _, unsafe := range []string{"<script", "<form", "onmouseover=\"", "onerror=", "javascript:", "http://", "https://", "<a ", "<img", "<link", "<iframe"} {
		if strings.Contains(strings.ToLower(body), unsafe) {
			t.Fatalf("HTML contains active content %q: %q", unsafe, body)
		}
	}
}

func TestRenderMessage_html_preserves_fallback_semantics(t *testing.T) {
	// Given
	recipient := Recipient{Email: "guest@example.com"}

	// When
	message, category := RenderMessage(recipient, HTMLFormat)
	body := string(message.Bytes())

	// Then
	if category != CategoryFallback || !strings.Contains(body, "Hello there,") || !strings.Contains(body, promotion) {
		t.Fatalf("category=%q body=%q", category, body)
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
