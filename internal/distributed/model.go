package distributed

import (
	"errors"
	"regexp"
	"time"
)

const (
	LedgerVersion       = int64(1)
	MaxAcknowledgeRange = int64(10_000)
	MaxDeliveryLimit    = int64(10)
	MaxWorkDuration     = 24 * time.Hour
)

var (
	ErrInvalidRequest = errors.New("invalid distributed ledger request")
	ErrNotFound       = errors.New("distributed campaign not found")
	ErrAlreadyExists  = errors.New("distributed campaign already exists")
	ErrClosed         = errors.New("distributed campaign closed")
	ErrNotDue         = errors.New("distributed settlement not due")
	ErrUnavailable    = errors.New("distributed ledger unavailable")
	ErrInvariant      = errors.New("distributed ledger invariant failed")
	campaignIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	digestPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type CampaignState string

const (
	StateOpen   CampaignState = "open"
	StateClosed CampaignState = "closed"
)

type TerminalState string

const (
	TerminalCompleted TerminalState = "completed"
	TerminalFailed    TerminalState = "failed"
)

type FailureReason string

const (
	FailureInterrupted           FailureReason = "interrupted"
	FailureRetryExhausted        FailureReason = "distributed_retry_exhausted"
	FailureDeadline              FailureReason = "completion_deadline"
	FailureDeliveryRejected      FailureReason = "delivery_rejected"
	FailureDeliveryIndeterminate FailureReason = "delivery_indeterminate"
	FailureDeliveryTransport     FailureReason = "delivery_transport"
)

type AttemptPolicy string

const (
	PolicyRetryable       AttemptPolicy = "retryable"
	PolicyOneShotDelivery AttemptPolicy = "one_shot_delivery"
)

type AttemptKind string

const (
	AttemptInitial AttemptKind = "initial"
	AttemptRetry   AttemptKind = "retry"
)

type BeginResult string

const (
	BeginGranted         BeginResult = "granted"
	BeginRetryGranted    BeginResult = "retry_granted"
	BeginInertCompleted  BeginResult = "inert_completed"
	BeginInertFailed     BeginResult = "inert_failed"
	BeginUnprocessed     BeginResult = "unprocessed"
	BeginNotAcknowledged BeginResult = "not_acknowledged"
	BeginExpired         BeginResult = "expired"
	BeginReserved        BeginResult = "reserved"
	BeginLimitExceeded   BeginResult = "limit_exceeded"
)

type SnapshotTrust string

const (
	TrustFull    SnapshotTrust = "full"
	TrustUnknown SnapshotTrust = "unknown"
)

type Campaign struct {
	ID            string
	Algorithm     string
	Seed          uint64
	Total         int64
	Deadline      time.Time
	Created       time.Time
	Version       int64
	Policy        AttemptPolicy
	DeliveryLimit int64
}

func (c Campaign) Validate() error {
	validPolicy := c.Policy == PolicyRetryable && c.DeliveryLimit == 0 ||
		c.Policy == PolicyOneShotDelivery && c.Total >= 1 && c.Total <= c.DeliveryLimit && c.DeliveryLimit == MaxDeliveryLimit
	if !campaignIDPattern.MatchString(c.ID) || c.Algorithm == "" || c.Total < 0 || !validPolicy ||
		c.Version != LedgerVersion || c.Created.IsZero() || !c.Deadline.After(c.Created) {
		return ErrInvalidRequest
	}
	return nil
}

type Snapshot struct {
	CampaignID        string
	Algorithm         string
	Seed              uint64
	State             CampaignState
	Trust             SnapshotTrust
	Total             int64
	Acknowledged      int64
	Started           int64
	Completed         int64
	Failed            int64
	Unprocessed       int64
	Attempts          int64
	Retries           int64
	Duplicates        int64
	Work              time.Duration
	Created           time.Time
	Deadline          time.Time
	Updated           time.Time
	Policy            AttemptPolicy
	DeliveryLimit     int64
	DeliveryReserved  int64
	DeliveryRejected  int64
	DeliveryUnknown   int64
	DeliveryTransport int64
}

func (s Snapshot) KnownTerminal() int64 { return s.Completed + s.Failed }

func (s Snapshot) KnownEnqueued() int64 {
	return s.Acknowledged + s.Started + s.Completed + s.Failed + s.Unprocessed
}

func (s Snapshot) IsTerminal() bool {
	return s.Trust == TrustFull && s.State == StateClosed && s.Acknowledged == 0 &&
		s.Started == 0 && s.KnownTerminal()+s.Unprocessed == s.Total
}

func (s Snapshot) Validate() error {
	if s.Trust == TrustUnknown {
		return nil
	}
	if s.Trust != TrustFull || !campaignIDPattern.MatchString(s.CampaignID) ||
		(s.State != StateOpen && s.State != StateClosed) || s.Total < 0 ||
		s.Acknowledged < 0 || s.Started < 0 || s.Completed < 0 || s.Failed < 0 ||
		s.Unprocessed < 0 || s.Attempts < 0 || s.Retries < 0 || s.Duplicates < 0 ||
		s.DeliveryLimit < 0 || s.DeliveryReserved < 0 || s.DeliveryReserved > s.DeliveryLimit ||
		s.DeliveryRejected < 0 || s.DeliveryUnknown < 0 || s.DeliveryTransport < 0 ||
		(s.Policy != PolicyRetryable && s.Policy != PolicyOneShotDelivery) ||
		s.Work < 0 || s.KnownEnqueued() > s.Total {
		return ErrInvariant
	}
	return nil
}

type AcknowledgeRequest struct {
	Start int64
	End   int64
	Now   time.Time
}

func (r AcknowledgeRequest) Validate(total int64) error {
	if r.Start < 1 || r.End < r.Start || r.End > total || r.End-r.Start+1 > MaxAcknowledgeRange || r.Now.IsZero() {
		return ErrInvalidRequest
	}
	return nil
}

type BeginRequest struct {
	Ordinal int64
	Kind    AttemptKind
	Now     time.Time
}

func (r BeginRequest) Validate(total int64) error {
	if r.Ordinal < 1 || r.Ordinal > total || (r.Kind != AttemptInitial && r.Kind != AttemptRetry) || r.Now.IsZero() {
		return ErrInvalidRequest
	}
	return nil
}

type CommitRequest struct {
	Ordinal int64
	State   TerminalState
	Reason  FailureReason
	Digest  string
	Work    time.Duration
	Now     time.Time
}

func (r CommitRequest) Validate(total int64) error {
	validCompletion := r.State == TerminalCompleted && r.Reason == "" && digestPattern.MatchString(r.Digest)
	validFailure := r.State == TerminalFailed && validFailureReason(r.Reason) && r.Digest == ""
	if r.Ordinal < 1 || r.Ordinal > total || (!validCompletion && !validFailure) ||
		r.Work < 0 || r.Work > MaxWorkDuration || r.Now.IsZero() {
		return ErrInvalidRequest
	}
	return nil
}

type CloseRequest struct {
	Now    time.Time
	Reason FailureReason
}

func (r CloseRequest) Validate() error {
	if r.Now.IsZero() || !validFailureReason(r.Reason) {
		return ErrInvalidRequest
	}
	return nil
}

type SettlementRequest struct {
	Now    time.Time
	Reason FailureReason
}

func (r SettlementRequest) Validate() error { return CloseRequest(r).Validate() }

type Transition struct {
	Result     BeginResult
	Ordinal    int64
	Attempts   int64
	Retries    int64
	Duplicates int64
}

func validFailureReason(reason FailureReason) bool {
	return reason == FailureInterrupted || reason == FailureRetryExhausted || reason == FailureDeadline ||
		reason == FailureDeliveryRejected || reason == FailureDeliveryIndeterminate || reason == FailureDeliveryTransport
}
