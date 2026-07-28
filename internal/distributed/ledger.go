package distributed

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Ledger struct {
	client redis.UniversalClient
}

func NewLedger(client redis.UniversalClient) (*Ledger, error) {
	if client == nil {
		return nil, ErrInvalidRequest
	}
	return &Ledger{client: client}, nil
}

func (l *Ledger) Create(ctx context.Context, campaign Campaign) error {
	if err := campaign.Validate(); err != nil {
		return err
	}
	values, err := l.run(ctx, createScript, campaign.ID, campaign.Version, campaign.Algorithm,
		campaign.Seed, campaign.Total, millis(campaign.Created), millis(campaign.Deadline),
		string(campaign.Policy), campaign.DeliveryLimit)
	if err != nil {
		return err
	}
	if code(values) == 1 {
		return ErrAlreadyExists
	}
	return nil
}

func (l *Ledger) Acknowledge(ctx context.Context, id string, request AcknowledgeRequest) error {
	if !campaignIDPattern.MatchString(id) || request.Start < 1 || request.End < request.Start ||
		request.End-request.Start+1 > MaxAcknowledgeRange || request.Now.IsZero() {
		return ErrInvalidRequest
	}
	values, err := l.run(ctx, acknowledgeScript, id, request.Start, request.End, millis(request.Now))
	if err != nil {
		return err
	}
	switch code(values) {
	case 0:
		return nil
	case 1:
		return ErrNotFound
	case 2:
		return ErrClosed
	default:
		return ErrInvalidRequest
	}
}

func (l *Ledger) Begin(ctx context.Context, id string, request BeginRequest) (Transition, error) {
	if !campaignIDPattern.MatchString(id) || request.Ordinal < 1 ||
		(request.Kind != AttemptInitial && request.Kind != AttemptRetry) || request.Now.IsZero() {
		return Transition{}, ErrInvalidRequest
	}
	values, err := l.run(ctx, beginScript, id, request.Ordinal, string(request.Kind), millis(request.Now))
	if err != nil {
		return Transition{}, err
	}
	result := mapBeginResult(code(values))
	if result == "" {
		if code(values) == 9 {
			return Transition{}, ErrNotFound
		}
		return Transition{}, ErrInvalidRequest
	}
	return Transition{
		Result: result, Ordinal: request.Ordinal, Attempts: integer(values, 1),
		Retries: integer(values, 2), Duplicates: integer(values, 3),
	}, nil
}

func (l *Ledger) Commit(ctx context.Context, id string, request CommitRequest) error {
	validCompletion := request.State == TerminalCompleted && request.Reason == "" && digestPattern.MatchString(request.Digest)
	validFailure := request.State == TerminalFailed && validFailureReason(request.Reason) && request.Digest == ""
	if !campaignIDPattern.MatchString(id) || request.Ordinal < 1 || (!validCompletion && !validFailure) ||
		request.Work < 0 || request.Work > MaxWorkDuration || request.Now.IsZero() {
		return ErrInvalidRequest
	}
	values, err := l.run(ctx, commitScript, id, request.Ordinal, string(request.State), string(request.Reason),
		request.Digest, request.Work.Nanoseconds(), millis(request.Now))
	if err != nil {
		return err
	}
	switch code(values) {
	case 0, 1:
		return nil
	case 2:
		return ErrInvalidRequest
	case 3:
		return ErrNotFound
	default:
		return ErrInvalidRequest
	}
}

func (l *Ledger) Close(ctx context.Context, id string, request CloseRequest) error {
	if !campaignIDPattern.MatchString(id) || request.Validate() != nil {
		return ErrInvalidRequest
	}
	values, err := l.run(ctx, closeScript, id, millis(request.Now), string(request.Reason))
	if err != nil {
		return err
	}
	if code(values) == 2 {
		return ErrNotFound
	}
	return nil
}

func (l *Ledger) Settle(ctx context.Context, id string, request SettlementRequest) error {
	if !campaignIDPattern.MatchString(id) || request.Validate() != nil {
		return ErrInvalidRequest
	}
	values, err := l.run(ctx, settleScript, id, millis(request.Now), string(request.Reason))
	if err != nil {
		return err
	}
	switch code(values) {
	case 0, 2:
		return nil
	case 1:
		return ErrNotDue
	case 3:
		return ErrClosed
	case 4:
		return ErrNotFound
	default:
		return ErrInvariant
	}
}

func (l *Ledger) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	if !campaignIDPattern.MatchString(id) {
		return Snapshot{}, ErrInvalidRequest
	}
	values, err := l.run(ctx, snapshotScript, id, LedgerVersion)
	if err != nil {
		return Snapshot{CampaignID: id, Trust: TrustUnknown}, ErrUnavailable
	}
	switch code(values) {
	case 1:
		return Snapshot{}, ErrNotFound
	case 2:
		return Snapshot{}, ErrInvariant
	}
	snapshot, err := decodeSnapshot(id, values)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, snapshot.Validate()
}

func (l *Ledger) run(ctx context.Context, script *redis.Script, id string, args ...any) ([]any, error) {
	keys := campaignKeys(id)
	result, err := script.Run(ctx, l.client, keys[:], args...).Slice()
	if err != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}

func campaignKeys(id string) [8]string {
	prefix := "dist:{" + id + "}:"
	return [8]string{prefix + "meta", prefix + "ack", prefix + "started", prefix + "completed",
		prefix + "failed", prefix + "unprocessed", prefix + "effects", prefix + "reasons"}
}

func decodeSnapshot(id string, values []any) (Snapshot, error) {
	seed, err := strconv.ParseUint(text(values, 3), 10, 64)
	if err != nil || len(values) != 23 {
		return Snapshot{}, ErrInvariant
	}
	return Snapshot{
		CampaignID: id, State: CampaignState(text(values, 1)), Algorithm: text(values, 2), Seed: seed,
		Total: integer(values, 4), Created: fromMillis(integer(values, 5)), Deadline: fromMillis(integer(values, 6)),
		Acknowledged: integer(values, 7), Started: integer(values, 8), Completed: integer(values, 9),
		Failed: integer(values, 10), Unprocessed: integer(values, 11), Attempts: integer(values, 12),
		Retries: integer(values, 13), Duplicates: integer(values, 14), Work: time.Duration(integer(values, 15)),
		Updated: fromMillis(integer(values, 16)), Trust: TrustFull,
		Policy: AttemptPolicy(text(values, 17)), DeliveryLimit: integer(values, 18), DeliveryReserved: integer(values, 19),
		DeliveryRejected: integer(values, 20), DeliveryUnknown: integer(values, 21), DeliveryTransport: integer(values, 22),
	}, nil
}

func code(values []any) int64 { return integer(values, 0) }

func integer(values []any, index int) int64 {
	if index >= len(values) {
		return -1
	}
	switch value := values[index].(type) {
	case int64:
		return value
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return -1
}

func text(values []any, index int) string {
	if index >= len(values) {
		return ""
	}
	value, _ := values[index].(string)
	return value
}

func mapBeginResult(value int64) BeginResult {
	return map[int64]BeginResult{0: BeginGranted, 1: BeginRetryGranted, 2: BeginInertCompleted,
		3: BeginInertFailed, 4: BeginUnprocessed, 5: BeginNotAcknowledged, 6: BeginExpired,
		7: BeginReserved, 8: BeginLimitExceeded}[value]
}

func millis(value time.Time) int64 { return value.UnixMilli() }

func fromMillis(value int64) time.Time { return time.UnixMilli(value) }
