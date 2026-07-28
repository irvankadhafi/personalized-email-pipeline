package campaign

import (
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"html"
	"strconv"
	"strings"
	"sync"
)

const (
	promotion = "Exclusive offer: save 20% on your next purchase."
	subject   = "Your exclusive offer"
)

var ErrInvalidFormat = errors.New("invalid message format")

type Format struct {
	kind formatKind
}

type formatKind uint8

const (
	formatText formatKind = iota
	formatHTML
)

var (
	TextFormat = Format{kind: formatText}
	HTMLFormat = Format{kind: formatHTML}
)

func ParseFormat(raw string) (Format, error) {
	switch raw {
	case "", "text":
		return TextFormat, nil
	case "html":
		return HTMLFormat, nil
	default:
		return Format{}, ErrInvalidFormat
	}
}

func (f Format) String() string {
	if f.kind == formatHTML {
		return "html"
	}
	return "text"
}

type RenderedMessage struct {
	format Format
	body   []byte
}

func (m RenderedMessage) Format() Format {
	return m.format
}

func (m RenderedMessage) Bytes() []byte {
	return m.body
}

type SinkFunc func(context.Context, []byte) error

type MessageSinkFunc func(context.Context, RenderedMessage) error

type RenderResult struct {
	Category Category
	Reason   Reason
}

func Render(ctx context.Context, recipient Recipient, sink SinkFunc) RenderResult {
	message, category := RenderMessage(recipient, TextFormat)
	if err := sink(ctx, message.Bytes()); err != nil {
		return RenderResult{Category: category, Reason: ReasonSink}
	}
	return RenderResult{Category: category}
}

func RenderMessage(recipient Recipient, format Format) (RenderedMessage, Category) {
	name, named := UsableName(recipient.Name)
	category := CategoryFallback
	greeting := "Hello there,"
	if named {
		category = CategoryNamed
		greeting = "Hello " + name + ","
	}
	if format.kind == formatHTML {
		body := []byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>" + subject + "</title></head><body><h1>" + subject + "</h1><p>" + html.EscapeString(greeting) + "</p><p>" + promotion + "</p></body></html>\n")
		return RenderedMessage{format: HTMLFormat, body: body}, category
	}
	body := []byte("Subject: " + subject + "\n\n" + greeting + "\n\n" + promotion + "\n")
	return RenderedMessage{format: TextFormat, body: body}, category
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

func (s *DigestSink) AcceptMessage(ctx context.Context, message RenderedMessage) error {
	return s.Accept(ctx, message.Bytes())
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
