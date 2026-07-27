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
