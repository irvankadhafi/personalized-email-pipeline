package testinbox

import (
	"errors"
	"testing"
	"time"
)

func TestParseConfigNormalizesAndChecksIndependentAllowlist(t *testing.T) {
	cfg, err := ParseConfig(map[string]string{
		"EMAIL_PIPELINE_SMTP_HOST": "smtp.invalid", "EMAIL_PIPELINE_SMTP_PORT": "587",
		"EMAIL_PIPELINE_SMTP_USERNAME": "user", "EMAIL_PIPELINE_SMTP_PASSWORD": "secret",
		"EMAIL_PIPELINE_SMTP_FROM": " Sender@Example.test ", "EMAIL_PIPELINE_TEST_DESTINATION": " DEST@Example.test ",
		"EMAIL_PIPELINE_TEST_ALLOWLIST": "other@example.test, dest@example.test",
	})
	if err != nil || cfg.Destination != "dest@example.test" || cfg.From != "sender@example.test" {
		t.Fatalf("unexpected config: %#v, %v", cfg, err)
	}
}

func TestParseConfigRejectsInvalidBoundaryValues(t *testing.T) {
	values := map[string]string{"EMAIL_PIPELINE_SMTP_HOST": "h", "EMAIL_PIPELINE_SMTP_PORT": "1", "EMAIL_PIPELINE_SMTP_USERNAME": "u", "EMAIL_PIPELINE_SMTP_PASSWORD": "p", "EMAIL_PIPELINE_SMTP_FROM": "a@example.test", "EMAIL_PIPELINE_TEST_DESTINATION": "b@example.test", "EMAIL_PIPELINE_TEST_ALLOWLIST": "b@example.test", "EMAIL_PIPELINE_SMTP_TIMEOUT": "0s"}
	if _, err := ParseConfig(values); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v", err)
	}
	values["EMAIL_PIPELINE_SMTP_TIMEOUT"] = (time.Second).String()
	values["EMAIL_PIPELINE_TEST_ALLOWLIST"] = "a@example.test"
	if _, err := ParseConfig(values); !errors.Is(err, ErrDestinationNotAllowed) {
		t.Fatalf("error = %v", err)
	}
}
