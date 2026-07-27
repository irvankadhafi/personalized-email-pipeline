package recipientcsv

import (
	"errors"
	"strings"
	"testing"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestReaderContinuesAfterRecoverableRows(t *testing.T) {
	input := "\ufeffemail,name\nvalid@example.com,Ada\n\"broken,quote\nnext@example.com,\n\n"
	r, err := NewReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	var records []Record
	for {
		record, ok, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		records = append(records, record)
	}
	if len(records) != 4 {
		t.Fatalf("got %d records", len(records))
	}
	if records[0].Email != "valid@example.com" || records[0].Reason != "" {
		t.Fatalf("first = %#v", records[0])
	}
	if records[1].Reason != campaign.ReasonInvalidCSV {
		t.Fatalf("second reason = %q", records[1].Reason)
	}
	if records[2].Email != "next@example.com" || records[2].Name != "" {
		t.Fatalf("third = %#v", records[2])
	}
	if records[3].Reason != campaign.ReasonBlankRecord {
		t.Fatalf("fourth reason = %q", records[3].Reason)
	}
}

func TestReaderRejectsWrongHeader(t *testing.T) {
	_, err := NewReader(strings.NewReader("address,name\na@example.com,A\n"))
	if !errors.Is(err, ErrFatalInput) {
		t.Fatalf("error = %v", err)
	}
}

func TestReaderDrainsOversizedRecord(t *testing.T) {
	input := "email,name\n" + strings.Repeat("a", MaxRecordBytes+10) + "\nnext@example.com,Ada\n"
	r, err := NewReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	first, ok, err := r.Next()
	if err != nil || !ok || first.Reason != campaign.ReasonOversizedRecord {
		t.Fatalf("oversized = %#v,%v,%v", first, ok, err)
	}
	second, ok, err := r.Next()
	if err != nil || !ok || second.Email != "next@example.com" {
		t.Fatalf("next = %#v,%v,%v", second, ok, err)
	}
}

func TestReaderDoesNotLeakInvalidContent(t *testing.T) {
	secret := "SECRET_PERSON@example.com"
	r, err := NewReader(strings.NewReader("email,name\n\"" + secret + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := r.Next()
	if err != nil || strings.Contains(string(record.Reason), secret) {
		t.Fatalf("leaked: %#v %v", record, err)
	}
}
