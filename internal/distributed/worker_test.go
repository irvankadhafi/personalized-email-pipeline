package distributed

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	campaignpkg "github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/testinbox"
)

func TestWorkerDeliversOldPayloadAsExactText(t *testing.T) {
	// Given
	ledger, _, distributedCampaign, now := newDeliveryTestLedger(t, 1)
	requireNoError(t, ledger.Acknowledge(context.Background(), distributedCampaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	var delivered campaignpkg.RenderedMessage
	worker, err := NewWorker(WorkerConfig{Ledger: ledger, Deliver: func(_ context.Context, message campaignpkg.RenderedMessage) testinbox.DeliveryResult {
		delivered = message
		return testinbox.DeliveryResult{Status: testinbox.DeliveryConfirmed}
	}})
	requireNoError(t, err)
	task := asynq.NewTask(TaskTypeTestInbox, []byte(`{"version":1,"campaign_id":"`+distributedCampaign.ID+`","sink":"test-inbox","algorithm":"v1","seed":7,"count":1,"first":1,"last":1}`))

	// When
	requireNoError(t, worker.ProcessTask(context.Background(), task))

	// Then
	want := "Subject: Your exclusive offer\n\nHello Customer 000001,\n\nExclusive offer: save 20% on your next purchase.\n"
	if delivered.Format() != campaignpkg.TextFormat || string(delivered.Bytes()) != want {
		t.Fatalf("format=%q body=%q", delivered.Format().String(), delivered.Bytes())
	}
}

func TestWorkerDeliversExplicitHTML(t *testing.T) {
	// Given
	ledger, _, distributedCampaign, now := newDeliveryTestLedger(t, 1)
	requireNoError(t, ledger.Acknowledge(context.Background(), distributedCampaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	var delivered campaignpkg.RenderedMessage
	worker, err := NewWorker(WorkerConfig{Ledger: ledger, Deliver: func(_ context.Context, message campaignpkg.RenderedMessage) testinbox.DeliveryResult {
		delivered = message
		return testinbox.DeliveryResult{Status: testinbox.DeliveryConfirmed}
	}})
	requireNoError(t, err)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: distributedCampaign.ID, Sink: TaskSinkTestInbox,
		Algorithm: distributedCampaign.Algorithm, Seed: distributedCampaign.Seed, Count: distributedCampaign.Total,
		First: 1, Last: 1, Format: campaignpkg.HTMLFormat,
	})
	requireNoError(t, err)

	// When
	requireNoError(t, worker.ProcessTask(context.Background(), task))

	// Then
	if delivered.Format() != campaignpkg.HTMLFormat || !bytes.Contains(delivered.Bytes(), []byte("<!doctype html>")) {
		t.Fatalf("format=%q body=%q", delivered.Format().String(), delivered.Bytes())
	}
}

func TestWorkerCompletesDryRunRange(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 2)
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 2, Now: now}))
	worker, err := NewWorker(WorkerConfig{Ledger: ledger})
	requireNoError(t, err)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkDryRun,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 2,
	})
	requireNoError(t, err)

	requireNoError(t, worker.ProcessTask(context.Background(), task))

	snapshot, err := ledger.Snapshot(context.Background(), campaign.ID)
	requireNoError(t, err)
	if snapshot.Completed != 2 || snapshot.Started != 0 || snapshot.Attempts != 2 {
		t.Fatalf("unexpected dry-run snapshot: %+v", snapshot)
	}
}

func TestWorkerOneShotRedeliveryDoesNotDeliverAgain(t *testing.T) {
	ledger, _, campaign, now := newDeliveryTestLedger(t, 1)
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	_, err := ledger.Begin(context.Background(), campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	var deliveries atomic.Int64
	worker, err := NewWorker(WorkerConfig{
		Ledger: ledger,
		Deliver: func(context.Context, campaignpkg.RenderedMessage) testinbox.DeliveryResult {
			deliveries.Add(1)
			return testinbox.DeliveryResult{Status: testinbox.DeliveryConfirmed}
		},
	})
	requireNoError(t, err)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkTestInbox,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 1,
	})
	requireNoError(t, err)

	requireNoError(t, worker.ProcessTask(context.Background(), task))

	if deliveries.Load() != 0 {
		t.Fatalf("redelivery attempted SMTP %d times", deliveries.Load())
	}
}

func TestWorkerCommitsIndeterminateDeliveryWithoutRetry(t *testing.T) {
	ledger, client, campaign, now := newDeliveryTestLedger(t, 1)
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	var deliveries atomic.Int64
	worker, err := NewWorker(WorkerConfig{
		Ledger: ledger,
		Deliver: func(context.Context, campaignpkg.RenderedMessage) testinbox.DeliveryResult {
			deliveries.Add(1)
			return testinbox.DeliveryResult{Status: testinbox.DeliveryIndeterminate, Err: testinbox.ErrIndeterminate}
		},
	})
	requireNoError(t, err)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkTestInbox,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 1,
	})
	requireNoError(t, err)

	requireNoError(t, worker.ProcessTask(context.Background(), task))
	requireNoError(t, worker.ProcessTask(context.Background(), task))

	if deliveries.Load() != 1 {
		t.Fatalf("expected one delivery attempt, got %d", deliveries.Load())
	}
	reason, err := client.HGet(context.Background(), campaignKeys(campaign.ID)[7], "1").Result()
	requireNoError(t, err)
	if reason != string(FailureDeliveryIndeterminate) {
		t.Fatalf("unexpected failure reason %q", reason)
	}
}

func TestWorkerRetriesTaskDeliveredBeforeAcknowledgement(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 1)
	worker, err := NewWorker(WorkerConfig{Ledger: ledger, Now: func() time.Time { return now }})
	requireNoError(t, err)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkDryRun,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 1,
	})
	requireNoError(t, err)

	if err := worker.ProcessTask(context.Background(), task); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected retryable pre-acknowledgement error, got %v", err)
	}
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	requireNoError(t, worker.ProcessTask(context.Background(), task))

	snapshot, err := ledger.Snapshot(context.Background(), campaign.ID)
	requireNoError(t, err)
	if !snapshot.IsTerminal() || snapshot.Completed != 1 {
		t.Fatalf("unexpected recovered snapshot: %+v", snapshot)
	}
}
