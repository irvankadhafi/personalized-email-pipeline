package campaign

import (
	"errors"
	"slices"
)

type Outcome string

const (
	Success        Outcome = "success"
	PartialSuccess Outcome = "partial_success"
	Failure        Outcome = "failure"
	Interrupted    Outcome = "interrupted"
)

type Category string

const (
	CategoryNamed    Category = "named"
	CategoryFallback Category = "fallback"
)

type Reason string

const (
	ReasonBlankRecord     Reason = "blank_record"
	ReasonFieldCount      Reason = "field_count"
	ReasonInvalidCSV      Reason = "invalid_csv"
	ReasonInvalidUTF8     Reason = "invalid_utf8"
	ReasonOversizedRecord Reason = "oversized_record"
	ReasonMissingEmail    Reason = "missing_email"
	ReasonInvalidEmail    Reason = "invalid_email"
	ReasonRender          Reason = "render_failed"
	ReasonSink            Reason = "sink_rejected"
	ReasonInterrupted     Reason = "interrupted"
	ReasonFatalInput      Reason = "fatal_input"
	ReasonUntrustworthy   Reason = "untrustworthy_accounting"
)

type Recipient struct {
	Ordinal  int64
	Email    string
	Identity string
	Name     string
	Category Category
}

type Counts struct {
	Examined    int64
	Invalid     int64
	Duplicate   int64
	Eligible    int64
	Completed   int64
	Failed      int64
	Unprocessed int64
}

func (c Counts) Validate() error {
	if c.Examined < 0 || c.Invalid < 0 || c.Duplicate < 0 || c.Eligible < 0 ||
		c.Completed < 0 || c.Failed < 0 || c.Unprocessed < 0 {
		return errors.New("accounting invariant failed")
	}
	if c.Examined != c.Invalid+c.Duplicate+c.Eligible {
		return errors.New("accounting invariant failed")
	}
	if c.Eligible != c.Completed+c.Failed+c.Unprocessed {
		return errors.New("accounting invariant failed")
	}
	return nil
}

func DeriveOutcome(c Counts, cancelled, fatal bool) Outcome {
	if fatal || c.Validate() != nil {
		return Failure
	}
	if cancelled {
		return Interrupted
	}
	if c.Eligible == 0 {
		return Failure
	}
	if c.Invalid > 0 || c.Failed > 0 || c.Unprocessed > 0 {
		return PartialSuccess
	}
	return Success
}

type Sample struct {
	Ordinal  int64
	Category Category
	Reason   Reason
	Text     string
}

type Samples struct {
	items map[Category][]Sample
}

func (s *Samples) Add(sample Sample, limit int) {
	if limit <= 0 {
		return
	}
	if s.items == nil {
		s.items = make(map[Category][]Sample)
	}
	items := append(s.items[sample.Category], sample)
	slices.SortFunc(items, func(a, b Sample) int {
		if a.Ordinal < b.Ordinal {
			return -1
		}
		if a.Ordinal > b.Ordinal {
			return 1
		}
		return 0
	})
	if len(items) > limit {
		items = items[:limit]
	}
	s.items[sample.Category] = items
}

func (s Samples) For(category Category) []Sample {
	return slices.Clone(s.items[category])
}
