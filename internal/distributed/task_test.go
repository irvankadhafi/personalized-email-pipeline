package distributed

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
)

func TestTaskPayloadRoundTripsGeneratedRange(t *testing.T) {
	payload := validTaskPayload()

	task, err := NewTask(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded, err := DecodeTask(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded != payload {
		t.Fatalf("decoded payload mismatch: got %+v want %+v", decoded, payload)
	}
	if task.Type() != TaskTypeDryRun {
		t.Fatalf("unexpected task type %q", task.Type())
	}
}

func TestTaskPayloadRejectsInvalidBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TaskPayload)
	}{
		{name: "unsupported version", mutate: func(p *TaskPayload) { p.Version++ }},
		{name: "unsafe campaign", mutate: func(p *TaskPayload) { p.CampaignID = "email@example.test" }},
		{name: "unsupported algorithm", mutate: func(p *TaskPayload) { p.Algorithm = "v2" }},
		{name: "unsupported sink", mutate: func(p *TaskPayload) { p.Sink = "unknown" }},
		{name: "zero count", mutate: func(p *TaskPayload) { p.Count = 0 }},
		{name: "zero first ordinal", mutate: func(p *TaskPayload) { p.First = 0 }},
		{name: "reversed range", mutate: func(p *TaskPayload) { p.Last = p.First - 1 }},
		{name: "range beyond count", mutate: func(p *TaskPayload) { p.Last = p.Count + 1 }},
		{name: "oversized range", mutate: func(p *TaskPayload) { p.Count = MaxTaskRange + 1; p.Last = MaxTaskRange + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validTaskPayload()
			test.mutate(&payload)

			if !errors.Is(payload.Validate(), ErrInvalidRequest) {
				t.Fatal("expected invalid request")
			}
		})
	}
}

func TestDecodeTaskRejectsWrongTypeAndUnknownFields(t *testing.T) {
	wrongType := asynq.NewTask("other:type", []byte(`{}`))
	if _, err := DecodeTask(wrongType); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request for wrong type, got %v", err)
	}
	unknown := asynq.NewTask(TaskTypeDryRun, []byte(`{"version":1,"campaign_id":"campaign-1","sink":"dry-run","algorithm":"v1","seed":7,"count":1,"first":1,"last":1,"body":"secret"}`))
	if _, err := DecodeTask(unknown); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request for unknown field, got %v", err)
	}
}

func TestTaskPayloadContainsNoRecipientOrCredentialFields(t *testing.T) {
	task, err := NewTask(validTaskPayload())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, canary := range []string{"email", "name", "body", "password", "recipient-"} {
		if bytes.Contains(task.Payload(), []byte(canary)) {
			t.Fatalf("payload contains forbidden field or value %q", canary)
		}
	}
}

func TestTaskIDIsStableAndRangeSpecific(t *testing.T) {
	first := TaskID("campaign-1", 1, 10)
	if first != TaskID("campaign-1", 1, 10) {
		t.Fatal("task ID is not stable")
	}
	if first == TaskID("campaign-1", 11, 20) || first == TaskID("campaign-2", 1, 10) {
		t.Fatal("task ID does not distinguish campaign and range")
	}
	if strings.Contains(first, "@") {
		t.Fatal("task ID contains recipient-like data")
	}
}

func TestTaskTypeSeparatesSinkQueues(t *testing.T) {
	payload := validTaskPayload()
	dryRun, err := NewTask(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload.Sink = TaskSinkTestInbox
	testInbox, err := NewTask(payload)
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.Type() != TaskTypeDryRun || testInbox.Type() != TaskTypeTestInbox || dryRun.Type() == testInbox.Type() {
		t.Fatalf("unexpected task types: dry-run=%q test-inbox=%q", dryRun.Type(), testInbox.Type())
	}
}

func validTaskPayload() TaskPayload {
	return TaskPayload{
		Version: PayloadVersion, CampaignID: "campaign-1", Sink: TaskSinkDryRun, Algorithm: "v1",
		Seed: 7, Count: 10, First: 1, Last: 10,
	}
}
