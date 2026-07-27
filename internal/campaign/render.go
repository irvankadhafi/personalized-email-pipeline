package campaign

import (
	"context"
	"crypto/sha256"
	"hash"
	"strconv"
	"strings"
	"sync"
)

const promotion = "Exclusive offer: save 20% on your next purchase."

type SinkFunc func(context.Context, []byte) error

type RenderResult struct {
	Category Category
	Reason   Reason
}

func Render(ctx context.Context, recipient Recipient, sink SinkFunc) RenderResult {
	name, named := UsableName(recipient.Name)
	category := CategoryFallback
	greeting := "Hello there,"
	if named {
		category = CategoryNamed
		greeting = "Hello " + name + ","
	}
	message := []byte("Subject: Your exclusive offer\n\n" + greeting + "\n\n" + promotion + "\n")
	if err := sink(ctx, message); err != nil {
		return RenderResult{Category: category, Reason: ReasonSink}
	}
	return RenderResult{Category: category}
}

func SyntheticSample(ordinal int64, category Category) string {
	label := "recipient-" + leftPadOrdinal(ordinal)
	if category == CategoryNamed {
		return "Hello " + label + ", [promotion rendered]"
	}
	return "Hello there, [promotion rendered for " + label + "]"
}

func leftPadOrdinal(ordinal int64) string {
	const width = 6
	s := strconv.FormatInt(ordinal, 10)
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}

type DigestSink struct {
	mu    sync.Mutex
	hash  hash.Hash
	bytes int64
}

func NewDigestSink() *DigestSink {
	return &DigestSink{hash: sha256.New()}
}

func (s *DigestSink) Accept(ctx context.Context, message []byte) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.hash.Write(message)
	s.bytes += int64(len(message))
	return nil
}

func (s *DigestSink) Bytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytes
}

func (s *DigestSink) Digest() [32]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var digest [32]byte
	copy(digest[:], s.hash.Sum(nil))
	return digest
}
