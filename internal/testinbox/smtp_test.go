package testinbox

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	mail "github.com/wneessen/go-mail"
)

func TestSinkRefusesDisallowedDestinationBeforeDial(t *testing.T) {
	cfg := Config{Host: "smtp.invalid", Port: 587, Username: "u", Password: "p", From: "from@example.test", Destination: "to@example.test", Allowlist: []string{"other@example.test"}, Timeout: time.Second}
	called := false
	_, err := NewSinkWithDial(cfg, func(context.Context, string, string) (net.Conn, error) { called = true; return nil, nil })
	if err != ErrDestinationNotAllowed || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestNewSinkWithTLSConfigRequiresTLSConfig(t *testing.T) {
	cfg := Config{Host: "smtp.invalid", Port: 587, Username: "u", Password: "p", From: "from@example.test", Destination: "to@example.test", Allowlist: []string{"to@example.test"}, Timeout: time.Second}
	if _, err := NewSinkWithTLSConfig(cfg, nil); err != ErrInvalidConfig {
		t.Fatalf("error=%v", err)
	}
}

func TestDeliveryStatusValuesAreClosed(t *testing.T) {
	statuses := map[DeliveryStatus]struct{}{
		DeliveryConfirmed: {}, DeliveryRejected: {}, DeliveryTransport: {}, DeliveryIndeterminate: {},
	}
	if len(statuses) != 4 {
		t.Fatal("statuses overlap")
	}
}

func TestClassifySendErrorDistinguishesDeliveryStages(t *testing.T) {
	tests := []struct {
		name   string
		reason mail.SendErrReason
		status DeliveryStatus
		err    error
	}{
		{"recipient rejected", mail.ErrSMTPRcptTo, DeliveryRejected, ErrRejected},
		{"data rejected", mail.ErrSMTPData, DeliveryRejected, ErrRejected},
		{"sender transport failure", mail.ErrSMTPMailFrom, DeliveryTransport, ErrTransport},
		{"data close uncertain", mail.ErrSMTPDataClose, DeliveryIndeterminate, ErrIndeterminate},
		{"body write uncertain", mail.ErrWriteContent, DeliveryIndeterminate, ErrIndeterminate},
		{"connection check uncertain", mail.ErrConnCheck, DeliveryIndeterminate, ErrIndeterminate},
		{"ambiguous", mail.ErrAmbiguous, DeliveryIndeterminate, ErrIndeterminate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifySendError(&mail.SendError{Reason: tt.reason})
			if result.Status != tt.status || !errors.Is(result.Err, tt.err) {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func TestDeliverCancelledBeforeDialIsDefinitiveTransportFailure(t *testing.T) {
	cfg := Config{Host: "smtp.invalid", Port: 587, Username: "u", Password: "p", From: "from@example.test", Destination: "to@example.test", Allowlist: []string{"to@example.test"}, Timeout: time.Second}
	called := false
	sink, err := NewSinkWithDial(cfg, func(context.Context, string, string) (net.Conn, error) {
		called = true
		return nil, errors.New("unexpected dial")
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := sink.Deliver(ctx, []byte("synthetic body"))
	if result.Status != DeliveryTransport || !errors.Is(result.Err, ErrTransport) || called {
		t.Fatalf("result=%#v called=%v", result, called)
	}
}

func TestDeliverClassifiesSMTPWireOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		status DeliveryStatus
		err    error
	}{
		{"accepted", "accepted", DeliveryConfirmed, nil},
		{"rejected", "rejected", DeliveryRejected, ErrRejected},
		{"connection lost after data", "lost", DeliveryIndeterminate, ErrIndeterminate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			tlsConfig, serverTLS := testTLSConfigs(t)
			done := make(chan error, 1)
			go func() { done <- serveSMTP(serverConn, serverTLS, tt.mode) }()
			cfg := Config{Host: "smtp.invalid", Port: 587, Username: "u", Password: "p", From: "from@example.test", Destination: "to@example.test", Allowlist: []string{"to@example.test"}, Timeout: time.Second}
			sink, err := newSinkWithTransport(cfg, func(context.Context, string, string) (net.Conn, error) { return clientConn, nil }, tlsConfig)
			if err != nil {
				t.Fatal(err)
			}
			result := sink.Deliver(context.Background(), []byte("synthetic body"))
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if result.Status != tt.status || !errors.Is(result.Err, tt.err) {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func testTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smtp.invalid"}, DNSNames: []string{"smtp.invalid"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	roots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots.AddCert(parsed)
	return &tls.Config{ServerName: "smtp.invalid", RootCAs: roots, MinVersion: tls.VersionTLS12}, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
}

func serveSMTP(conn net.Conn, tlsConfig *tls.Config, mode string) error {
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
	if err := write("220 smtp.invalid ESMTP\r\n"); err != nil {
		return err
	}
	if err := read("EHLO"); err != nil {
		return err
	}
	if err := write("250-smtp.invalid\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n"); err != nil {
		return err
	}
	if err := read("STARTTLS"); err != nil {
		return err
	}
	if err := write("220 Ready to start TLS\r\n"); err != nil {
		return err
	}
	tlsConn := tls.Server(conn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return err
	}
	conn, reader = tlsConn, bufio.NewReader(tlsConn)
	if err := read("EHLO"); err != nil {
		return err
	}
	if err := write("250-smtp.invalid\r\n250 AUTH PLAIN\r\n"); err != nil {
		return err
	}
	if err := read("AUTH PLAIN"); err != nil {
		return err
	}
	if err := write("235 authenticated\r\n"); err != nil {
		return err
	}
	if err := read("NOOP"); err != nil {
		return err
	}
	if err := write("250 connection ok\r\n"); err != nil {
		return err
	}
	if err := read("MAIL FROM:"); err != nil {
		return err
	}
	if err := write("250 sender ok\r\n"); err != nil {
		return err
	}
	if err := read("RCPT TO:"); err != nil {
		return err
	}
	if mode == "rejected" {
		if err := write("550 recipient rejected\r\n"); err != nil {
			return err
		}
		return finishSMTP(reader, write)
	}
	if err := write("250 recipient ok\r\n"); err != nil {
		return err
	}
	if err := read("DATA"); err != nil {
		return err
	}
	if err := write("354 end with dot\r\n"); err != nil {
		return err
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == ".\r\n" {
			break
		}
	}
	if mode == "lost" {
		return nil
	}
	if err := write("250 accepted\r\n"); err != nil {
		return err
	}
	return finishSMTP(reader, write)
}

func finishSMTP(reader *bufio.Reader, write func(string) error) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch {
		case strings.HasPrefix(line, "NOOP"):
			if err := write("250 connection ok\r\n"); err != nil {
				return err
			}
		case strings.HasPrefix(line, "RSET"):
			if err := write("250 reset ok\r\n"); err != nil {
				return err
			}
		case strings.HasPrefix(line, "QUIT"):
			return write("221 bye\r\n")
		default:
			return fmt.Errorf("unexpected smtp cleanup command: %q", strings.TrimSpace(line))
		}
	}
}
