package distributed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
)

const (
	TaskTypeDryRun    = "email:dry-run"
	TaskTypeTestInbox = "email:test-inbox"
	PayloadVersion    = int64(1)
	MaxTaskRange      = MaxAcknowledgeRange
)

type TaskSink string

const (
	TaskSinkDryRun    TaskSink = "dry-run"
	TaskSinkTestInbox TaskSink = "test-inbox"
)

type TaskPayload struct {
	Version    int64    `json:"version"`
	CampaignID string   `json:"campaign_id"`
	Sink       TaskSink `json:"sink"`
	Algorithm  string   `json:"algorithm"`
	Seed       uint64   `json:"seed"`
	Count      int64    `json:"count"`
	First      int64    `json:"first"`
	Last       int64    `json:"last"`
}

func (p TaskPayload) Validate() error {
	if p.Version != PayloadVersion || !campaignIDPattern.MatchString(p.CampaignID) ||
		(p.Sink != TaskSinkDryRun && p.Sink != TaskSinkTestInbox) ||
		p.Algorithm != recipientcsv.FixtureAlgorithmV1 || p.Count < 1 || p.First < 1 ||
		p.Last < p.First || p.Last > p.Count || p.Last-p.First+1 > MaxTaskRange {
		return ErrInvalidRequest
	}
	return nil
}

func NewTask(payload TaskPayload) (*asynq.Task, error) {
	if err := payload.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	return asynq.NewTask(taskType(payload.Sink), encoded), nil
}

func DecodeTask(task *asynq.Task) (TaskPayload, error) {
	if task == nil || (task.Type() != TaskTypeDryRun && task.Type() != TaskTypeTestInbox) {
		return TaskPayload{}, ErrInvalidRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(task.Payload()))
	decoder.DisallowUnknownFields()
	var payload TaskPayload
	if err := decoder.Decode(&payload); err != nil {
		return TaskPayload{}, ErrInvalidRequest
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return TaskPayload{}, ErrInvalidRequest
	}
	if err := payload.Validate(); err != nil {
		return TaskPayload{}, err
	}
	if task.Type() != taskType(payload.Sink) {
		return TaskPayload{}, ErrInvalidRequest
	}
	return payload, nil
}

func taskType(sink TaskSink) string {
	if sink == TaskSinkTestInbox {
		return TaskTypeTestInbox
	}
	return TaskTypeDryRun
}

func TaskID(campaignID string, first, last int64) string {
	return fmt.Sprintf("%s:%d-%d", campaignID, first, last)
}
