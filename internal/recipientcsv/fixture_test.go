package recipientcsv

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func TestGenerateIsDeterministicAndRoundTrips(t *testing.T) {
	opts := FixtureOptions{Algorithm: FixtureAlgorithmV1, Seed: 42, Count: 6}
	var first, second bytes.Buffer
	s1, err := Generate(&first, opts)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Generate(&second, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) || !reflect.DeepEqual(s1, s2) {
		t.Fatal("same options were not deterministic")
	}
	r, err := NewReader(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var count, named, fallback int64
	for {
		record, ok, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		count++
		if record.Name == "" {
			fallback++
		} else {
			named++
		}
	}
	if count != s1.Count || named != s1.Named || fallback != s1.Fallback {
		t.Fatalf("oracle counts = %d,%d,%d summary=%#v", count, named, fallback, s1)
	}
}

func TestGenerateGoldenV1(t *testing.T) {
	var got bytes.Buffer
	summary, err := Generate(&got, FixtureOptions{Algorithm: FixtureAlgorithmV1, Seed: 7, Count: 4})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile("testdata/fixture-v1.csv", got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/fixture-v1.csv")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("golden mismatch\ngot:\n%s\nwant:\n%s", got.Bytes(), want)
	}
	wantSummary := FixtureSummary{Algorithm: FixtureAlgorithmV1, Seed: 7, Count: 4, Named: 2, Fallback: 2}
	if !reflect.DeepEqual(summary, wantSummary) {
		t.Fatalf("summary=%#v want=%#v", summary, wantSummary)
	}
}

func TestGeneratedSourceMatchesGeneratedCSV(t *testing.T) {
	opts := FixtureOptions{Algorithm: FixtureAlgorithmV1, Seed: 7, Count: 11}
	var csvOutput bytes.Buffer
	wantSummary, err := Generate(&csvOutput, opts)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewReader(bytes.NewReader(csvOutput.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewGeneratedSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	for {
		want, wantOK, err := reader.Next()
		if err != nil {
			t.Fatal(err)
		}
		got, gotOK := source.Next()
		if gotOK != wantOK {
			t.Fatalf("source availability = %v, want %v", gotOK, wantOK)
		}
		if !gotOK {
			break
		}
		if got.Ordinal != want.Ordinal || got.Email != want.Email || got.Name != want.Name {
			t.Fatalf("source record = %#v, want %#v", got, want)
		}
	}
	if got := source.Summary(); !reflect.DeepEqual(got, wantSummary) {
		t.Fatalf("source summary = %#v, want %#v", got, wantSummary)
	}
}

func TestGenerateZeroAndInvalid(t *testing.T) {
	var got bytes.Buffer
	summary, err := Generate(&got, FixtureOptions{Seed: 1, Count: 0})
	if err != nil || summary.Count != 0 || got.String() != "email,name\n" {
		t.Fatalf("zero = %#v %q %v", summary, got.String(), err)
	}
	if _, err := Generate(&got, FixtureOptions{Algorithm: "v2", Count: 1}); err == nil {
		t.Fatal("unsupported algorithm accepted")
	}
}
