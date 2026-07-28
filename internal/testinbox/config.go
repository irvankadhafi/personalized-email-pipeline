package testinbox

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidConfig         = errors.New("invalid test inbox configuration")
	ErrDestinationNotAllowed = errors.New("test inbox destination is not allowlisted")
	ErrConfiguration         = errors.New("test inbox configuration failure")
	ErrTransport             = errors.New("test inbox transport failure")
	ErrRejected              = errors.New("test inbox delivery rejected")
	ErrIndeterminate         = errors.New("test inbox delivery indeterminate")
)

type DeliveryStatus uint8

const (
	DeliveryConfirmed DeliveryStatus = iota + 1
	DeliveryRejected
	DeliveryTransport
	DeliveryIndeterminate
)

type DeliveryResult struct {
	Status DeliveryStatus
	Err    error
}

type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	From        string
	Destination string
	Allowlist   []string
	Timeout     time.Duration
}

const defaultTimeout = 15 * time.Second

func ParseConfig(values map[string]string) (Config, error) {
	port, err := strconv.Atoi(strings.TrimSpace(values["EMAIL_PIPELINE_SMTP_PORT"]))
	if err != nil {
		return Config{}, ErrInvalidConfig
	}
	timeout := defaultTimeout
	if raw := strings.TrimSpace(values["EMAIL_PIPELINE_SMTP_TIMEOUT"]); raw != "" {
		timeout, err = time.ParseDuration(raw)
		if err != nil {
			return Config{}, ErrInvalidConfig
		}
	}
	cfg := Config{
		Host: strings.TrimSpace(values["EMAIL_PIPELINE_SMTP_HOST"]), Port: port,
		Username: values["EMAIL_PIPELINE_SMTP_USERNAME"], Password: values["EMAIL_PIPELINE_SMTP_PASSWORD"],
		From:        strings.TrimSpace(values["EMAIL_PIPELINE_SMTP_FROM"]),
		Destination: strings.TrimSpace(values["EMAIL_PIPELINE_TEST_DESTINATION"]), Timeout: timeout,
	}
	for _, raw := range strings.Split(values["EMAIL_PIPELINE_TEST_ALLOWLIST"], ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			cfg.Allowlist = append(cfg.Allowlist, raw)
		}
	}
	return validateConfig(cfg)
}

func (c Config) Validate() error { _, err := validateConfig(c); return err }

func validateConfig(c Config) (Config, error) {
	if c.Host == "" || c.Port < 1 || c.Port > 65535 || c.Username == "" || c.Password == "" || c.Timeout <= 0 {
		return Config{}, ErrInvalidConfig
	}
	from, ok := normalizeAddress(c.From)
	if !ok {
		return Config{}, ErrInvalidConfig
	}
	destination, ok := normalizeAddress(c.Destination)
	if !ok {
		return Config{}, ErrInvalidConfig
	}
	if len(c.Allowlist) == 0 {
		return Config{}, ErrInvalidConfig
	}
	allowed := make([]string, 0, len(c.Allowlist))
	for _, value := range c.Allowlist {
		item, ok := normalizeAddress(value)
		if !ok {
			return Config{}, ErrInvalidConfig
		}
		allowed = append(allowed, item)
	}
	found := false
	for _, item := range allowed {
		if item == destination {
			found = true
			break
		}
	}
	if !found {
		return Config{}, ErrDestinationNotAllowed
	}
	c.From, c.Destination, c.Allowlist = from, destination, allowed
	return c, nil
}

func normalizeAddress(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if len(v) == 0 || len(v) > 254 || strings.Count(v, "@") != 1 {
		return "", false
	}
	parts := strings.Split(v, "@")
	if len(parts[0]) == 0 || len(parts[0]) > 64 || len(parts[1]) < 3 || !strings.Contains(parts[1], ".") {
		return "", false
	}
	if strings.Contains(v, " ") || strings.Contains(v, "\t") || strings.Contains(v, "\r") || strings.Contains(v, "\n") || strings.Contains(v, "..") {
		return "", false
	}
	return v, true
}
