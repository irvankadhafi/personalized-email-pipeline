package distributed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestProducerRejectsSameCampaignRestartWithOppositeFormatBeforeEnqueue(t *testing.T) {
	tests := []struct {
		name         string
		firstFormat  campaign.Format
		secondFormat campaign.Format
	}{
		{name: "text first html second", firstFormat: campaign.TextFormat, secondFormat: campaign.HTMLFormat},
		{name: "html first text second", firstFormat: campaign.HTMLFormat, secondFormat: campaign.TextFormat},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			ledger, _, distributedCampaign, now := newTestLedger(t, 2)
			keys := campaignKeys(distributedCampaign.ID)
			requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
			firstEnqueuer := &fakeEnqueuer{}
			firstProducer, err := NewProducer(ProducerConfig{
				Ledger: ledger, Enqueuer: firstEnqueuer, Campaign: distributedCampaign, Sink: TaskSinkDryRun,
				Format: test.firstFormat, TaskSize: 2, MaxRetry: 3, Retention: time.Hour,
				EnqueueTimeout: time.Second, Now: func() time.Time { return now },
			})
			requireNoError(t, err)
			_, err = firstProducer.Enqueue(context.Background())
			requireNoError(t, err)
			before, err := ledger.Snapshot(context.Background(), distributedCampaign.ID)
			requireNoError(t, err)

			secondEnqueuer := &fakeEnqueuer{}
			secondProducer, err := NewProducer(ProducerConfig{
				Ledger: ledger, Enqueuer: secondEnqueuer, Campaign: distributedCampaign, Sink: TaskSinkDryRun,
				Format: test.secondFormat, TaskSize: 2, MaxRetry: 3, Retention: time.Hour,
				EnqueueTimeout: time.Second, Now: func() time.Time { return now },
			})
			requireNoError(t, err)

			// When
			_, err = secondProducer.Enqueue(context.Background())

			// Then
			if !errors.Is(err, ErrAlreadyExists) {
				t.Fatalf("expected ErrAlreadyExists, got %v", err)
			}
			if len(secondEnqueuer.calls) != 0 {
				t.Fatalf("expected no enqueue calls, got %d", len(secondEnqueuer.calls))
			}
			after, snapshotErr := ledger.Snapshot(context.Background(), distributedCampaign.ID)
			requireNoError(t, snapshotErr)
			if after != before {
				t.Fatalf("ledger snapshot changed: before=%+v after=%+v", before, after)
			}
		})
	}
}
