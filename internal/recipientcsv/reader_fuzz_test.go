package recipientcsv

import (
	"bytes"
	"testing"
)

func FuzzReaderNeverPanicsOrLeaksRawErrors(f *testing.F) {
	f.Add([]byte("email,name\na@example.com,Ada\n"))
	f.Add([]byte("email,name\n\xff\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := NewReader(bytes.NewReader(data))
		if err != nil {
			return
		}
		for i := 0; i < 1000; i++ {
			_, ok, err := r.Next()
			if err != nil || !ok {
				return
			}
		}
	})
}
