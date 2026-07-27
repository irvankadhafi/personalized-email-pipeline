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

func Generate(w io.Writer, opts FixtureOptions) (FixtureSummary, error) {
	if opts.Algorithm == "" {
		opts.Algorithm = FixtureAlgorithmV1
	}
	if opts.Algorithm != FixtureAlgorithmV1 || opts.Count < 0 {
		return FixtureSummary{}, errors.New("invalid fixture configuration")
	}
	summary := FixtureSummary{Algorithm: opts.Algorithm, Seed: opts.Seed, Count: opts.Count}
	csvw := csv.NewWriter(w)
	if err := csvw.Write([]string{"email", "name"}); err != nil {
		return FixtureSummary{}, errors.New("fixture write failed")
	}
	rng := rand.New(rand.NewPCG(opts.Seed, fixtureStateTwo))
	for i := int64(1); i <= opts.Count; i++ {
		token := rng.Uint64()
		name := ""
		if i%2 == 1 {
			name = fmt.Sprintf("Customer %06d", i)
			summary.Named++
		} else {
			summary.Fallback++
		}
		email := fmt.Sprintf("recipient-%06d-%016x@example.test", i, token)
		if err := csvw.Write([]string{email, name}); err != nil {
			return FixtureSummary{}, errors.New("fixture write failed")
		}
	}
	csvw.Flush()
	if csvw.Error() != nil {
		return FixtureSummary{}, errors.New("fixture write failed")
	}
	return summary, nil
}
