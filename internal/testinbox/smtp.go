package testinbox

import (
	"context"
	"crypto/tls"
	"errors"

	mail "github.com/wneessen/go-mail"
)

type Sink struct {
	cfg  Config
	dial mail.DialContextFunc
	tls  *tls.Config
}

func NewSink(cfg Config) (*Sink, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Sink{cfg: cfg}, nil
}

func NewSinkWithTLSConfig(cfg Config, tlsConfig *tls.Config) (*Sink, error) {
	if tlsConfig == nil {
		return nil, ErrInvalidConfig
	}
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Sink{cfg: cfg, tls: tlsConfig}, nil
}

func NewSinkWithDial(cfg Config, dial mail.DialContextFunc) (*Sink, error) {
	return newSinkWithTransport(cfg, dial, nil)
}

func newSinkWithTransport(cfg Config, dial mail.DialContextFunc, tlsConfig *tls.Config) (*Sink, error) {
	if dial == nil {
		return nil, ErrInvalidConfig
	}
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Sink{cfg: cfg, dial: dial, tls: tlsConfig}, nil
}

func (s *Sink) Accept(ctx context.Context, body []byte) error {
	r := s.Deliver(ctx, body)
	if r.Status == DeliveryConfirmed {
		return nil
	}
	return r.Err
}

func (s *Sink) Deliver(ctx context.Context, body []byte) DeliveryResult {
	if err := ctx.Err(); err != nil {
		return DeliveryResult{Status: DeliveryTransport, Err: ErrTransport}
	}
	opts := []mail.Option{mail.WithPort(s.cfg.Port), mail.WithTimeout(s.cfg.Timeout), mail.WithTLSPolicy(mail.TLSMandatory), mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(s.cfg.Username), mail.WithPassword(s.cfg.Password)}
	if s.dial != nil {
		opts = append(opts, mail.WithDialContextFunc(s.dial))
	}
	if s.tls != nil {
		opts = append(opts, mail.WithTLSConfig(s.tls))
	}
	client, err := mail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return DeliveryResult{Status: DeliveryRejected, Err: ErrConfiguration}
	}
	msg := mail.NewMsg()
	if err = msg.From(s.cfg.From); err != nil {
		return DeliveryResult{Status: DeliveryRejected, Err: ErrConfiguration}
	}
	if err = msg.To(s.cfg.Destination); err != nil {
		return DeliveryResult{Status: DeliveryRejected, Err: ErrConfiguration}
	}
	msg.Subject("Test inbox delivery")
	msg.SetBodyString(mail.TypeTextPlain, string(body))
	smtpClient, err := client.DialToSMTPClientWithContext(ctx)
	if err != nil {
		return DeliveryResult{Status: DeliveryTransport, Err: ErrTransport}
	}
	err = client.SendWithSMTPClient(smtpClient, msg)
	_ = client.CloseWithSMTPClient(smtpClient)
	if err == nil {
		return DeliveryResult{Status: DeliveryConfirmed}
	}
	return classifySendError(err)
}

func classifySendError(err error) DeliveryResult {
	var sendErr *mail.SendError
	if errors.As(err, &sendErr) {
		switch sendErr.Reason {
		case mail.ErrSMTPRcptTo, mail.ErrSMTPData:
			return DeliveryResult{Status: DeliveryRejected, Err: ErrRejected}
		case mail.ErrSMTPDataClose, mail.ErrWriteContent, mail.ErrConnCheck, mail.ErrAmbiguous:
			return DeliveryResult{Status: DeliveryIndeterminate, Err: ErrIndeterminate}
		default:
			return DeliveryResult{Status: DeliveryTransport, Err: ErrTransport}
		}
	}
	return DeliveryResult{Status: DeliveryIndeterminate, Err: ErrIndeterminate}
}
