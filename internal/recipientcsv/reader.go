package recipientcsv

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

const MaxRecordBytes = 1 << 20

var ErrFatalInput = errors.New("fatal input error")

type Record struct {
	Ordinal int64
	Email   string
	Name    string
	Reason  campaign.Reason
}

type Reader struct {
	input   *bufio.Reader
	ordinal int64
}

func NewReader(input io.Reader) (*Reader, error) {
	r := &Reader{input: bufio.NewReader(input)}
	header, oversized, ok, err := r.readLine()
	if err != nil || !ok || oversized {
		return nil, ErrFatalInput
	}
	header = bytes.TrimPrefix(header, []byte{0xef, 0xbb, 0xbf})
	fields, reason := parseLine(header)
	if reason != "" || len(fields) != 2 || fields[0] != "email" || fields[1] != "name" {
		return nil, ErrFatalInput
	}
	return r, nil
}

func (r *Reader) Next() (Record, bool, error) {
	line, oversized, ok, err := r.readLine()
	if err != nil {
		return Record{}, false, ErrFatalInput
	}
	if !ok {
		return Record{}, false, nil
	}
	r.ordinal++
	record := Record{Ordinal: r.ordinal}
	if oversized {
		record.Reason = campaign.ReasonOversizedRecord
		return record, true, nil
	}
	fields, reason := parseLine(line)
	if reason != "" {
		record.Reason = reason
		return record, true, nil
	}
	record.Email, record.Name = fields[0], fields[1]
	if strings.TrimSpace(record.Email) == "" {
		record.Email = ""
		record.Name = ""
		record.Reason = campaign.ReasonMissingEmail
	}
	return record, true, nil
}

func parseLine(line []byte) ([]string, campaign.Reason) {
	if len(line) == 0 {
		return nil, campaign.ReasonBlankRecord
	}
	if !utf8.Valid(line) || bytes.HasPrefix(line, []byte{0xef, 0xbb, 0xbf}) {
		return nil, campaign.ReasonInvalidUTF8
	}
	r := csv.NewReader(bytes.NewReader(line))
	r.FieldsPerRecord = 2
	fields, err := r.Read()
	if err != nil {
		if errors.Is(err, csv.ErrFieldCount) {
			return nil, campaign.ReasonFieldCount
		}
		return nil, campaign.ReasonInvalidCSV
	}
	if _, err := r.Read(); err != io.EOF {
		return nil, campaign.ReasonInvalidCSV
	}
	return fields, ""
}

func (r *Reader) readLine() ([]byte, bool, bool, error) {
	line := make([]byte, 0, min(MaxRecordBytes, r.input.Size()))
	oversized := false
	readAny := false
	for {
		fragment, err := r.input.ReadSlice('\n')
		if len(fragment) > 0 {
			readAny = true
			payload := fragment
			if fragment[len(fragment)-1] == '\n' {
				payload = fragment[:len(fragment)-1]
				if len(payload) > 0 && payload[len(payload)-1] == '\r' {
					payload = payload[:len(payload)-1]
				}
			}
			if !oversized {
				if len(line)+len(payload) > MaxRecordBytes {
					oversized = true
					line = nil
				} else {
					line = append(line, payload...)
				}
			}
		}
		switch {
		case err == nil:
			return line, oversized, true, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && readAny:
			return line, oversized, true, nil
		case errors.Is(err, io.EOF):
			return nil, false, false, nil
		default:
			return nil, false, false, err
		}
	}
}
