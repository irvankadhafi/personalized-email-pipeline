package distributed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/testinbox"
)

type DeliveryFunc func(context.Context, campaign.RenderedMessage) testinbox.DeliveryResult

type WorkerConfig struct {
	Ledger  *Ledger
	Deliver DeliveryFunc
	Now     func() time.Time
}

type Worker struct {
	ledger  *Ledger
	deliver DeliveryFunc
	now     func() time.Time
}

type recordTaskError struct {
	ordinal int64
	err     error
}

func (e *recordTaskError) Error() string { return e.err.Error() }

func (e *recordTaskError) Unwrap() error { return e.err }

func NewWorker(config WorkerConfig) (*Worker, error) {
	if config.Ledger == nil {
		return nil, ErrInvalidRequest
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Worker{ledger: config.Ledger, deliver: config.Deliver, now: config.Now}, nil
}

func (w *Worker) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := DecodeTask(task)
	if err != nil {
		return permanentWorkerError()
	}
	snapshot, err := w.ledger.Snapshot(ctx, payload.CampaignID)
	if err != nil {
		return err
	}
	if !payloadMatchesSnapshot(payload, snapshot) || payload.Sink == TaskSinkTestInbox && w.deliver == nil {
		return permanentWorkerError()
	}
	source, err := recipientcsv.NewGeneratedSource(recipientcsv.FixtureOptions{
		Algorithm: payload.Algorithm,
		Seed:      payload.Seed,
		Count:     payload.Count,
	})
	if err != nil {
		return permanentWorkerError()
	}
	retryCount, _ := asynq.GetRetryCount(ctx)
	kind := AttemptInitial
	if retryCount > 0 {
		kind = AttemptRetry
	}
	for record, ok := source.Next(); ok && record.Ordinal <= payload.Last; record, ok = source.Next() {
		if record.Ordinal < payload.First {
			continue
		}
		if err := w.processRecord(ctx, payload, record, kind); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) HandleError(ctx context.Context, task *asynq.Task, taskErr error) {
	if taskErr == nil || errors.Is(taskErr, asynq.SkipRetry) {
		return
	}
	retryCount, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxRetryOK := asynq.GetMaxRetry(ctx)
	if !retryOK || !maxRetryOK || retryCount < maxRetry {
		return
	}
	var recordErr *recordTaskError
	if !errors.As(taskErr, &recordErr) {
		return
	}
	payload, err := DecodeTask(task)
	if err != nil || payload.Sink != TaskSinkDryRun || recordErr.ordinal < payload.First || recordErr.ordinal > payload.Last {
		return
	}
	_ = w.ledger.Commit(ctx, payload.CampaignID, CommitRequest{
		Ordinal: recordErr.ordinal,
		State:   TerminalFailed,
		Reason:  FailureRetryExhausted,
		Now:     w.now(),
	})
}

func (w *Worker) processRecord(ctx context.Context, payload TaskPayload, record recipientcsv.Record, kind AttemptKind) error {
	transition, err := w.ledger.Begin(ctx, payload.CampaignID, BeginRequest{
		Ordinal: record.Ordinal,
		Kind:    kind,
		Now:     w.now(),
	})
	if err != nil {
		return err
	}
	if transition.Result == BeginNotAcknowledged {
		return ErrUnavailable
	}
	if transition.Result != BeginGranted && transition.Result != BeginRetryGranted {
		return nil
	}
	started := w.now()
	recipient := campaign.Recipient{Ordinal: record.Ordinal, Email: record.Email, Name: record.Name}
	if payload.Sink == TaskSinkDryRun {
		sink := campaign.NewDigestSink()
		message, _ := campaign.RenderMessage(recipient, payload.Format)
		if err := sink.AcceptMessage(ctx, message); err != nil {
			return &recordTaskError{ordinal: record.Ordinal, err: ErrUnavailable}
		}
		digest := sink.Digest()
		if err := w.ledger.Commit(ctx, payload.CampaignID, CommitRequest{
			Ordinal: record.Ordinal,
			State:   TerminalCompleted,
			Digest:  hex.EncodeToString(digest[:]),
			Work:    w.now().Sub(started),
			Now:     w.now(),
		}); err != nil {
			return &recordTaskError{ordinal: record.Ordinal, err: err}
		}
		return nil
	}
	var delivery testinbox.DeliveryResult
	var digest [sha256.Size]byte
	message, _ := campaign.RenderMessage(recipient, payload.Format)
	digest = sha256.Sum256(message.Bytes())
	delivery = w.deliver(ctx, message)
	commit := CommitRequest{Ordinal: record.Ordinal, Work: w.now().Sub(started), Now: w.now()}
	if delivery.Err == nil && delivery.Status == testinbox.DeliveryConfirmed {
		commit.State = TerminalCompleted
		commit.Digest = hex.EncodeToString(digest[:])
	} else {
		commit.State = TerminalFailed
		commit.Reason = deliveryFailureReason(delivery)
		commit.Work = 0
	}
	return w.ledger.Commit(ctx, payload.CampaignID, commit)
}

func payloadMatchesSnapshot(payload TaskPayload, snapshot Snapshot) bool {
	policy := PolicyRetryable
	if payload.Sink == TaskSinkTestInbox {
		policy = PolicyOneShotDelivery
	}
	return snapshot.Trust == TrustFull && snapshot.Algorithm == payload.Algorithm &&
		snapshot.Seed == payload.Seed && snapshot.Total == payload.Count && snapshot.Policy == policy
}

func deliveryFailureReason(result testinbox.DeliveryResult) FailureReason {
	switch result.Status {
	case testinbox.DeliveryIndeterminate:
		return FailureDeliveryIndeterminate
	case testinbox.DeliveryTransport:
		return FailureDeliveryTransport
	case testinbox.DeliveryRejected:
		return FailureDeliveryRejected
	default:
		return FailureDeliveryTransport
	}
}

func permanentWorkerError() error {
	return fmt.Errorf("distributed task refused: %w", asynq.SkipRetry)
}
