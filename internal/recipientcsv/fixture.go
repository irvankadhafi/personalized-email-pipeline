package recipientcsv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
)

const (
	FixtureAlgorithmV1 = "v1"
	fixtureStateTwo    = uint64(0x9e3779b97f4a7c15)
)

type FixtureOptions struct {
	Algorithm string `json:"algorithm"`
	Seed      uint64 `json:"seed"`
	Count     int64  `json:"count"`
}

type FixtureSummary struct {
	Algorithm string `json:"algorithm"`
	Seed      uint64 `json:"seed"`
	Count     int64  `json:"count"`
	Named     int64  `json:"named"`
	Fallback  int64  `json:"fallback"`
}

type GeneratedSource struct {
	rng     *rand.Rand
	summary FixtureSummary
	next    int64
}

func NewGeneratedSource(opts FixtureOptions) (*GeneratedSource, error) {
	if opts.Algorithm == "" {
		opts.Algorithm = FixtureAlgorithmV1
	}
	if opts.Algorithm != FixtureAlgorithmV1 || opts.Count < 0 {
		return nil, errors.New("invalid fixture configuration")
	}
	return &GeneratedSource{
		rng: rand.New(rand.NewPCG(opts.Seed, fixtureStateTwo)),
		summary: FixtureSummary{
			Algorithm: opts.Algorithm,
			Seed:      opts.Seed,
			Count:     opts.Count,
		},
		next: 1,
	}, nil
}

func (s *GeneratedSource) Next() (Record, bool) {
	if s.next > s.summary.Count {
		return Record{}, false
	}
	ordinal := s.next
	s.next++
	name := ""
	if ordinal%2 == 1 {
		name = fmt.Sprintf("Customer %06d", ordinal)
		s.summary.Named++
	} else {
		s.summary.Fallback++
	}
	return Record{
		Ordinal: ordinal,
		Email:   fmt.Sprintf("recipient-%06d-%016x@example.test", ordinal, s.rng.Uint64()),
		Name:    name,
	}, true
}

func (s *GeneratedSource) Summary() FixtureSummary {
	return s.summary
}

func Generate(w io.Writer, opts FixtureOptions) (FixtureSummary, error) {
	source, err := NewGeneratedSource(opts)
	if err != nil {
		return FixtureSummary{}, err
	}
	csvw := csv.NewWriter(w)
	if err := csvw.Write([]string{"email", "name"}); err != nil {
		return FixtureSummary{}, errors.New("fixture write failed")
	}
	for record, ok := source.Next(); ok; record, ok = source.Next() {
		if err := csvw.Write([]string{record.Email, record.Name}); err != nil {
			return FixtureSummary{}, errors.New("fixture write failed")
		}
	}
	csvw.Flush()
	if csvw.Error() != nil {
		return FixtureSummary{}, errors.New("fixture write failed")
	}
	return source.Summary(), nil
}
