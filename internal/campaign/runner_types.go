package campaign

import "time"

const (
	DefaultResponseBound = 100 * time.Millisecond
	DefaultSettlement    = 5 * time.Second
	HeapSampleInterval   = 10 * time.Millisecond
)

type SourceRecord struct {
	Ordinal      int64
	Email        string
	Name         string
	ParserReason Reason
}

type NextFunc func() (SourceRecord, bool, error)

func SliceSource(records []SourceRecord) NextFunc {
	index := 0
	return func() (SourceRecord, bool, error) {
		if index == len(records) {
			return SourceRecord{}, false, nil
		}
		record := records[index]
		index++
		return record, true, nil
	}
}

type RunConfig struct {
	Workers       int
	Settlement    time.Duration
	ResponseBound time.Duration
	Sink          SinkFunc
}

type ReasonSummary struct {
	Count    int64   `json:"count"`
	Ordinals []int64 `json:"sample_ordinals,omitempty"`
	Omitted  int64   `json:"omitted"`
}

type Backend string

const (
	BackendLocal Backend = "local"
	BackendAsynq Backend = "asynq"
)

type Sink string

const (
	SinkDryRun    Sink = "dry-run"
	SinkTestInbox Sink = "test-inbox"
)

type ModeEvidence struct {
	Backend Backend `json:"backend"`
	Sink    Sink    `json:"sink"`
}

type TestDeliveryEvidence struct {
	Confirmed     int64 `json:"confirmed"`
	Rejected      int64 `json:"rejected"`
	Transport     int64 `json:"transport"`
	Indeterminate int64 `json:"indeterminate"`
}

type DistributedEvidence struct {
	KnownEnqueued int64 `json:"known_enqueued"`
	KnownTerminal int64 `json:"known_terminal"`
	Unknown       int64 `json:"unknown"`
}

type RunReport struct {
	Outcome            Outcome
	AccountingScope    string
	Counts             Counts
	InvalidReasons     map[Reason]ReasonSummary
	FailedReasons      map[Reason]ReasonSummary
	Samples            Samples
	ProcessingElapsed  time.Duration
	PeakHeapInuseBytes uint64
	Workers            int
	Settlement         time.Duration
	ResponseBound      time.Duration
	MeasuredResponse   time.Duration
	Cancelled          bool
	Fatal              bool
	Started            int64
	Mode               *ModeEvidence
	TestDelivery       *TestDeliveryEvidence
	Distributed        *DistributedEvidence
}

type job struct {
	recipient Recipient
}

type workerReady struct {
	grant chan job
}

type workerResult struct {
	ordinal  int64
	category Category
	reason   Reason
	accepted time.Time
}
