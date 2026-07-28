package distributed

import (
	"errors"
	"testing"
	"time"
)

func TestCampaignValidateRejectsUnsafeIdentifier(t *testing.T) {
	created := time.Unix(1_700_000_000, 0)
	campaign := Campaign{
		ID:        "recipient@example.test",
		Algorithm: "v1",
		Total:     1,
		Created:   created,
		Deadline:  created.Add(time.Minute),
		Version:   1,
	}

	err := campaign.Validate()

	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}

func TestSnapshotValidateAllowsTrustworthyAcknowledgedPrefix(t *testing.T) {
	snapshot := Snapshot{
		CampaignID:   "campaign-1",
		State:        StateOpen,
		Trust:        TrustFull,
		Policy:       PolicyRetryable,
		Total:        10,
		Acknowledged: 3,
	}

	if err := snapshot.Validate(); err != nil {
		t.Fatalf("expected valid prefix snapshot, got %v", err)
	}
	if snapshot.KnownEnqueued() != 3 {
		t.Fatalf("expected three known enqueued records, got %d", snapshot.KnownEnqueued())
	}
}

func TestSnapshotValidateRejectsLifecycleCountAboveTotal(t *testing.T) {
	snapshot := Snapshot{
		CampaignID: "campaign-1",
		State:      StateOpen,
		Trust:      TrustFull,
		Policy:     PolicyRetryable,
		Total:      1,
		Completed:  2,
	}

	if !errors.Is(snapshot.Validate(), ErrInvariant) {
		t.Fatal("expected invariant error")
	}
}

func TestSnapshotIsTerminalRequiresFullReconciliation(t *testing.T) {
	snapshot := Snapshot{
		CampaignID: "campaign-1",
		State:      StateClosed,
		Trust:      TrustFull,
		Policy:     PolicyRetryable,
		Total:      2,
		Completed:  1,
		Failed:     1,
	}

	if !snapshot.IsTerminal() {
		t.Fatal("expected terminal snapshot")
	}
	snapshot.Total = 3
	if snapshot.IsTerminal() {
		t.Fatal("partial prefix must not be terminal")
	}
}

func TestCommitRequestValidateRequiresMatchingTerminalEvidence(t *testing.T) {
	tests := []struct {
		name string
		req  CommitRequest
	}{
		{name: "completion without digest", req: CommitRequest{Ordinal: 1, State: TerminalCompleted}},
		{name: "completion with reason", req: CommitRequest{Ordinal: 1, State: TerminalCompleted, Digest: validDigest(), Reason: FailureInterrupted}},
		{name: "failure without reason", req: CommitRequest{Ordinal: 1, State: TerminalFailed}},
		{name: "failure with digest", req: CommitRequest{Ordinal: 1, State: TerminalFailed, Digest: validDigest(), Reason: FailureInterrupted}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !errors.Is(test.req.Validate(1), ErrInvalidRequest) {
				t.Fatal("expected invalid request")
			}
		})
	}
}

func TestCampaignValidateRequiresBoundedDeliveryPolicy(t *testing.T) {
	created := time.Unix(1_700_000_000, 0)
	tests := []Campaign{
		{ID: "delivery-1", Algorithm: "v1", Total: 11, Created: created, Deadline: created.Add(time.Minute), Version: LedgerVersion, Policy: PolicyOneShotDelivery, DeliveryLimit: MaxDeliveryLimit},
		{ID: "delivery-2", Algorithm: "v1", Total: 1, Created: created, Deadline: created.Add(time.Minute), Version: LedgerVersion, Policy: PolicyOneShotDelivery},
		{ID: "delivery-3", Algorithm: "v1", Total: 1, Created: created, Deadline: created.Add(time.Minute), Version: LedgerVersion, Policy: AttemptPolicy("unknown")},
	}
	for _, campaign := range tests {
		if !errors.Is(campaign.Validate(), ErrInvalidRequest) {
			t.Fatalf("expected invalid campaign: %+v", campaign)
		}
	}
	valid := Campaign{ID: "delivery-4", Algorithm: "v1", Total: 10, Created: created, Deadline: created.Add(time.Minute), Version: LedgerVersion, Policy: PolicyOneShotDelivery, DeliveryLimit: MaxDeliveryLimit}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid delivery campaign, got %v", err)
	}
}

func TestDeliveryIndeterminateIsValidTerminalFailure(t *testing.T) {
	req := CommitRequest{Ordinal: 1, State: TerminalFailed, Reason: FailureDeliveryIndeterminate, Now: time.Now()}
	if err := req.Validate(1); err != nil {
		t.Fatalf("expected valid delivery failure, got %v", err)
	}
}

func validDigest() string {
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
