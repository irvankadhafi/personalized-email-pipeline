package testinbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestDeliverBytesUsesTextPlainMIME(t *testing.T) {
	// Given
	sink, wire := newCapturingSink(t, 1)

	// When
	result := sink.Deliver(context.Background(), []byte("synthetic body"))

	// Then
	if result.Status != DeliveryConfirmed {
		t.Fatalf("result=%#v", result)
	}
	if contentType := parseWireContentType(t, <-wire); contentType != "text/plain" {
		t.Fatalf("content-type=%q", contentType)
	}
}

func TestDeliverMessageUsesPerMessageMIME(t *testing.T) {
	// Given
	sink, wire := newCapturingSink(t, 2)
	textMessage, _ := campaign.RenderMessage(campaign.Recipient{Name: "Text"}, campaign.TextFormat)
	htmlMessage, _ := campaign.RenderMessage(campaign.Recipient{Name: "HTML"}, campaign.HTMLFormat)

	// When
	textResult := sink.DeliverMessage(context.Background(), textMessage)
	htmlResult := sink.DeliverMessage(context.Background(), htmlMessage)

	// Then
	if textResult.Status != DeliveryConfirmed || htmlResult.Status != DeliveryConfirmed {
		t.Fatalf("text=%#v html=%#v", textResult, htmlResult)
	}
	if contentType := parseWireContentType(t, <-wire); contentType != "text/plain" {
		t.Fatalf("text content-type=%q", contentType)
	}
	if contentType := parseWireContentType(t, <-wire); contentType != "text/html" {
		t.Fatalf("html content-type=%q", contentType)
	}
}

func newCapturingSink(t *testing.T, deliveries int) (*Sink, <-chan []byte) {
	t.Helper()
	tlsConfig, serverTLS := testTLSConfigs(t)
	connections := make(chan net.Conn, deliveries)
	wire := make(chan []byte, deliveries)
	done := make(chan error, deliveries)
	for range deliveries {
		clientConn, serverConn := net.Pipe()
		connections <- clientConn
		go func() { done <- serveCapturedSMTP(serverConn, serverTLS, wire) }()
	}
	t.Cleanup(func() {
		for range deliveries {
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
	})
	cfg := Config{Host: "smtp.invalid", Port: 587, Username: "u", Password: "p", From: "from@example.test", Destination: "to@example.test", Allowlist: []string{"to@example.test"}, Timeout: time.Second}
	sink, err := newSinkWithTransport(cfg, func(context.Context, string, string) (net.Conn, error) { return <-connections, nil }, tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	return sink, wire
}

func parseWireContentType(t *testing.T, wire []byte) string {
	t.Helper()
	message, err := mail.ReadMessage(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	contentType, _, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	return contentType
}

func serveCapturedSMTP(conn net.Conn, tlsConfig *tls.Config, wire chan<- []byte) error {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(text string) error { _, err := io.WriteString(conn, text); return err }
	read := func(prefix string) error {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if !strings.HasPrefix(line, prefix) {
			return fmt.Errorf("unexpected smtp command: want %q, got %q", prefix, strings.TrimSpace(line))
		}
		return nil
	}
	steps := []struct {
		command string
		reply   string
	}{
		{"", "220 smtp.invalid ESMTP\r\n"},
		{"EHLO", "250-smtp.invalid\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n"},
		{"STARTTLS", "220 Ready to start TLS\r\n"},
	}
	for _, step := range steps {
		if step.command != "" {
			if err := read(step.command); err != nil {
				return err
			}
		}
		if err := write(step.reply); err != nil {
			return err
		}
	}
	tlsConn := tls.Server(conn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return err
	}
	conn, reader = tlsConn, bufio.NewReader(tlsConn)
	for _, step := range []struct{ command, reply string }{
		{"EHLO", "250-smtp.invalid\r\n250 AUTH PLAIN\r\n"},
		{"AUTH PLAIN", "235 authenticated\r\n"},
		{"NOOP", "250 connection ok\r\n"},
		{"MAIL FROM:", "250 sender ok\r\n"},
		{"RCPT TO:", "250 recipient ok\r\n"},
		{"DATA", "354 end with dot\r\n"},
	} {
		if err := read(step.command); err != nil {
			return err
		}
		if err := write(step.reply); err != nil {
			return err
		}
	}
	var message bytes.Buffer
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		if bytes.Equal(line, []byte(".\r\n")) {
			break
		}
		message.Write(line)
	}
	wire <- message.Bytes()
	if err := write("250 accepted\r\n"); err != nil {
		return err
	}
	return finishSMTP(reader, write)
}
