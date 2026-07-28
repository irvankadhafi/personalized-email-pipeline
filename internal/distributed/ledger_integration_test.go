package distributed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLedgerCompletesOnceAndTracksDuplicateAttempts(t *testing.T) {
	ledger, client, campaign, now := newTestLedger(t, 2)
	ctx := context.Background()

	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 2, Now: now}))
	first, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	if first.Result != BeginGranted {
		t.Fatalf("expected initial grant, got %q", first.Result)
	}
	duplicate, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	if duplicate.Result != BeginNotAcknowledged || duplicate.Duplicates != 1 {
		t.Fatalf("expected one inert duplicate, got %+v", duplicate)
	}
	commit := CommitRequest{Ordinal: 1, State: TerminalCompleted, Digest: validDigest(), Work: time.Millisecond, Now: now}
	requireNoError(t, ledger.Commit(ctx, campaign.ID, commit))
	requireNoError(t, ledger.Commit(ctx, campaign.ID, commit))

	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.Completed != 1 || snapshot.Acknowledged != 1 || snapshot.Attempts != 1 || snapshot.Duplicates != 1 || snapshot.Work != time.Millisecond {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	effect, err := client.HGet(ctx, campaignKeys(campaign.ID)[6], "1").Result()
	requireNoError(t, err)
	if effect != validDigest() {
		t.Fatalf("unexpected effect digest %q", effect)
	}
}

func TestLedgerRetryThenFailureIsTerminalOnce(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 1)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	_, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	retry, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptRetry, Now: now})
	requireNoError(t, err)
	if retry.Result != BeginRetryGranted || retry.Attempts != 2 || retry.Retries != 1 {
		t.Fatalf("unexpected retry: %+v", retry)
	}
	failure := CommitRequest{Ordinal: 1, State: TerminalFailed, Reason: FailureRetryExhausted, Now: now}
	requireNoError(t, ledger.Commit(ctx, campaign.ID, failure))
	requireNoError(t, ledger.Commit(ctx, campaign.ID, failure))
	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.Failed != 1 || snapshot.Completed != 0 || snapshot.Attempts != 2 || snapshot.Retries != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestLedgerFinalCommitClosesNaturallyWithoutFailureReason(t *testing.T) {
	ledger, client, campaign, now := newTestLedger(t, 2)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 2, Now: now}))
	for ordinal := int64(1); ordinal <= 2; ordinal++ {
		_, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: ordinal, Kind: AttemptInitial, Now: now})
		requireNoError(t, err)
		requireNoError(t, ledger.Commit(ctx, campaign.ID, CommitRequest{
			Ordinal: ordinal, State: TerminalCompleted, Digest: validDigest(), Now: now,
		}))
		if ordinal == 1 {
			snapshot, err := ledger.Snapshot(ctx, campaign.ID)
			requireNoError(t, err)
			if snapshot.State != StateOpen || snapshot.IsTerminal() {
				t.Fatalf("campaign closed before final commit: %+v", snapshot)
			}
		}
	}

	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.State != StateClosed || snapshot.Completed != 2 || !snapshot.IsTerminal() {
		t.Fatalf("expected naturally closed campaign, got %+v", snapshot)
	}
	if client.HExists(ctx, campaignKeys(campaign.ID)[0], "close_reason").Val() {
		t.Fatal("natural closure stored a campaign failure reason")
	}
	requireNoError(t, ledger.Commit(ctx, campaign.ID, CommitRequest{
		Ordinal: 2, State: TerminalCompleted, Digest: validDigest(), Now: now,
	}))
}

func TestLedgerFinalFailureClosesNaturallyWithoutCampaignFailureReason(t *testing.T) {
	ledger, client, campaign, now := newTestLedger(t, 1)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	_, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	requireNoError(t, ledger.Commit(ctx, campaign.ID, CommitRequest{
		Ordinal: 1, State: TerminalFailed, Reason: FailureRetryExhausted, Now: now,
	}))

	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.State != StateClosed || snapshot.Failed != 1 || !snapshot.IsTerminal() {
		t.Fatalf("expected naturally closed failed campaign, got %+v", snapshot)
	}
	if client.HExists(ctx, campaignKeys(campaign.ID)[0], "close_reason").Val() {
		t.Fatal("natural closure stored a campaign failure reason")
	}
}

func TestLedgerCloseAndSettleReconcileCampaign(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 3)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 3, Now: now}))
	_, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	requireNoError(t, ledger.Close(ctx, campaign.ID, CloseRequest{Now: now, Reason: FailureInterrupted}))
	if err := ledger.Settle(ctx, campaign.ID, SettlementRequest{Now: campaign.Deadline.Add(-time.Millisecond), Reason: FailureDeadline}); !errors.Is(err, ErrNotDue) {
		t.Fatalf("expected settlement not due, got %v", err)
	}
	late, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 2, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	if late.Result != BeginUnprocessed {
		t.Fatalf("expected inert unprocessed begin, got %+v", late)
	}
	requireNoError(t, ledger.Settle(ctx, campaign.ID, SettlementRequest{Now: campaign.Deadline, Reason: FailureDeadline}))
	requireNoError(t, ledger.Settle(ctx, campaign.ID, SettlementRequest{Now: campaign.Deadline, Reason: FailureDeadline}))
	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.Failed != 1 || snapshot.Unprocessed != 2 || !snapshot.IsTerminal() {
		t.Fatalf("expected reconciled terminal snapshot, got %+v", snapshot)
	}
}

func TestLedgerConcurrentBeginsGrantOnce(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 1)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))

	var granted atomic.Int64
	var failures atomic.Int64
	var group sync.WaitGroup
	for range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			transition, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
			if err != nil {
				failures.Add(1)
				return
			}
			if transition.Result == BeginGranted {
				granted.Add(1)
			}
		}()
	}
	group.Wait()
	if granted.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("expected one grant and no errors, got grants=%d errors=%d", granted.Load(), failures.Load())
	}
	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.Started != 1 || snapshot.Attempts != 1 || snapshot.Duplicates != 31 {
		t.Fatalf("unexpected concurrent snapshot: %+v", snapshot)
	}
}

func TestLedgerRedisStateExcludesPrivacyCanaries(t *testing.T) {
	ledger, client, campaign, now := newTestLedger(t, 1)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	_, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	requireNoError(t, ledger.Commit(ctx, campaign.ID, CommitRequest{Ordinal: 1, State: TerminalCompleted, Digest: validDigest(), Now: now}))

	var persisted strings.Builder
	for _, key := range campaignKeys(campaign.ID) {
		persisted.WriteString(key)
		persisted.WriteString(fmt.Sprint(client.Do(ctx, "DUMP", key).Val()))
	}
	for _, canary := range []string{"recipient@example.test", "Customer 000001", "rendered body", "redis-password", "smtp-password"} {
		if strings.Contains(persisted.String(), canary) {
			t.Fatalf("privacy canary persisted: %q", canary)
		}
	}
}

func TestLedgerSnapshotReturnsUnknownWhenRedisUnavailable(t *testing.T) {
	address := os.Getenv("REDIS_TEST_ADDR")
	if address == "" {
		t.Skip("REDIS_TEST_ADDR is not set")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	ledger, err := NewLedger(client)
	requireNoError(t, err)
	requireNoError(t, client.Close())

	snapshot, err := ledger.Snapshot(context.Background(), "campaign-unavailable")
	if !errors.Is(err, ErrUnavailable) || snapshot.Trust != TrustUnknown {
		t.Fatalf("expected unknown unavailable snapshot, got snapshot=%+v err=%v", snapshot, err)
	}
}

func TestLedgerOneShotDeliveryReservationIsNeverGrantedTwice(t *testing.T) {
	ledger, _, campaign, now := newDeliveryTestLedger(t, 1)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))

	first, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	second, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptRetry, Now: now})
	requireNoError(t, err)

	if first.Result != BeginGranted || second.Result != BeginReserved {
		t.Fatalf("unexpected reservation results: first=%+v second=%+v", first, second)
	}
	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.Started != 1 || snapshot.DeliveryReserved != 1 || snapshot.Attempts != 1 || snapshot.Duplicates != 1 {
		t.Fatalf("unexpected reservation snapshot: %+v", snapshot)
	}
}

func TestLedgerOneShotDeliveryReservationLimitIsAtomic(t *testing.T) {
	ledger, _, campaign, now := newDeliveryTestLedger(t, MaxDeliveryLimit)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: MaxDeliveryLimit, Now: now}))

	var granted atomic.Int64
	var group sync.WaitGroup
	for ordinal := int64(1); ordinal <= MaxDeliveryLimit; ordinal++ {
		for range 4 {
			group.Add(1)
			go func() {
				defer group.Done()
				transition, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: ordinal, Kind: AttemptInitial, Now: now})
				if err == nil && transition.Result == BeginGranted {
					granted.Add(1)
				}
			}()
		}
	}
	group.Wait()

	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if granted.Load() != MaxDeliveryLimit || snapshot.Started != MaxDeliveryLimit ||
		snapshot.DeliveryReserved != MaxDeliveryLimit || snapshot.Attempts != MaxDeliveryLimit ||
		snapshot.Duplicates != MaxDeliveryLimit*3 {
		t.Fatalf("unexpected concurrent reservation snapshot: grants=%d snapshot=%+v", granted.Load(), snapshot)
	}
}

func TestLedgerOneShotReservationSettlesAsIndeterminate(t *testing.T) {
	ledger, _, campaign, now := newDeliveryTestLedger(t, 1)
	ctx := context.Background()
	requireNoError(t, ledger.Acknowledge(ctx, campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	_, err := ledger.Begin(ctx, campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	requireNoError(t, ledger.Close(ctx, campaign.ID, CloseRequest{Now: now, Reason: FailureInterrupted}))
	requireNoError(t, ledger.Settle(ctx, campaign.ID, SettlementRequest{Now: campaign.Deadline, Reason: FailureDeliveryIndeterminate}))

	snapshot, err := ledger.Snapshot(ctx, campaign.ID)
	requireNoError(t, err)
	if snapshot.Failed != 1 || snapshot.Started != 0 || snapshot.DeliveryReserved != 1 || !snapshot.IsTerminal() {
		t.Fatalf("unexpected settled delivery snapshot: %+v", snapshot)
	}
}

func newTestLedger(t *testing.T, total int64) (*Ledger, *redis.Client, Campaign, time.Time) {
	t.Helper()
	address := os.Getenv("REDIS_TEST_ADDR")
	if address == "" {
		t.Skip("REDIS_TEST_ADDR is not set")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	ledger, err := NewLedger(client)
	requireNoError(t, err)
	now := time.Unix(1_700_000_000, 0).UTC()
	campaign := Campaign{
		ID: fmt.Sprintf("test-%d", time.Now().UnixNano()), Algorithm: "v1", Seed: 7,
		Total: total, Created: now, Deadline: now.Add(time.Minute), Version: LedgerVersion, Policy: PolicyRetryable,
	}
	keys := campaignKeys(campaign.ID)
	t.Cleanup(func() { _ = client.Del(context.Background(), keys[:]...).Err() })
	requireNoError(t, ledger.Create(context.Background(), campaign))
	return ledger, client, campaign, now
}

func newDeliveryTestLedger(t *testing.T, total int64) (*Ledger, *redis.Client, Campaign, time.Time) {
	t.Helper()
	ledger, client, campaign, now := newTestLedger(t, total)
	keys := campaignKeys(campaign.ID)
	requireNoError(t, client.Del(context.Background(), keys[:]...).Err())
	campaign.Policy = PolicyOneShotDelivery
	campaign.DeliveryLimit = MaxDeliveryLimit
	requireNoError(t, ledger.Create(context.Background(), campaign))
	return ledger, client, campaign, now
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
