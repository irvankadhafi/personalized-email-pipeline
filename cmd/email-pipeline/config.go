package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/recipientcsv"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/testinbox"
	"github.com/redis/go-redis/v9"
)

const testInboxConfirmation = "SEND_SYNTHETIC_TEST"

type runOptions struct {
	input              string
	backend            campaign.Backend
	sink               campaign.Sink
	count              int64
	seed               uint64
	algorithm          string
	workers            int
	settlement         time.Duration
	confirmation       string
	taskSize           int64
	maxRetry           int
	completionDeadline time.Duration
	exposeMode         bool
}

type redisConfig struct {
	address  string
	username string
	password string
	db       int
}

func (o runOptions) validate() error {
	if o.workers <= 0 || o.settlement <= 0 || o.algorithm != recipientcsv.FixtureAlgorithmV1 ||
		(o.backend != campaign.BackendLocal && o.backend != campaign.BackendAsynq) ||
		(o.sink != campaign.SinkDryRun && o.sink != campaign.SinkTestInbox) {
		return errors.New("invalid run options")
	}
	if o.backend == campaign.BackendLocal && o.sink == campaign.SinkDryRun {
		if o.input == "" || o.count != 0 {
			return errors.New("local dry-run requires input")
		}
		return nil
	}
	if o.input != "" || o.count < 1 || o.taskSize < 1 || o.maxRetry < 0 || o.completionDeadline <= 0 {
		return errors.New("optional modes require generated input")
	}
	if o.sink == campaign.SinkTestInbox && (o.count > 10 || o.confirmation != testInboxConfirmation) {
		return errors.New("test inbox guard refused")
	}
	return nil
}

func smtpConfigFromEnvironment() (testinbox.Config, error) {
	keys := []string{
		"EMAIL_PIPELINE_SMTP_HOST", "EMAIL_PIPELINE_SMTP_PORT", "EMAIL_PIPELINE_SMTP_USERNAME",
		"EMAIL_PIPELINE_SMTP_PASSWORD", "EMAIL_PIPELINE_SMTP_FROM", "EMAIL_PIPELINE_TEST_DESTINATION",
		"EMAIL_PIPELINE_TEST_ALLOWLIST", "EMAIL_PIPELINE_SMTP_TIMEOUT",
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key] = os.Getenv(key)
	}
	return testinbox.ParseConfig(values)
}

func smtpSinkFromEnvironment() (*testinbox.Sink, error) {
	config, err := smtpConfigFromEnvironment()
	if err != nil {
		return nil, err
	}
	caFile := strings.TrimSpace(os.Getenv("EMAIL_PIPELINE_SMTP_CA_FILE"))
	if caFile == "" {
		return testinbox.NewSink(config)
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, testinbox.ErrInvalidConfig
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, testinbox.ErrInvalidConfig
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, testinbox.ErrInvalidConfig
	}
	return testinbox.NewSinkWithTLSConfig(config, &tls.Config{ServerName: config.Host, RootCAs: roots, MinVersion: tls.VersionTLS12})
}

func redisConfigFromEnvironment() (redisConfig, error) {
	db := 0
	if raw := strings.TrimSpace(os.Getenv("EMAIL_PIPELINE_REDIS_DB")); raw != "" {
		var err error
		db, err = strconv.Atoi(raw)
		if err != nil || db < 0 {
			return redisConfig{}, errors.New("invalid redis configuration")
		}
	}
	config := redisConfig{
		address: strings.TrimSpace(os.Getenv("EMAIL_PIPELINE_REDIS_ADDR")), username: os.Getenv("EMAIL_PIPELINE_REDIS_USERNAME"),
		password: os.Getenv("EMAIL_PIPELINE_REDIS_PASSWORD"), db: db,
	}
	if config.address == "" {
		return redisConfig{}, errors.New("invalid redis configuration")
	}
	return config, nil
}

func (c redisConfig) redisOptions() *redis.Options {
	return &redis.Options{Addr: c.address, Username: c.username, Password: c.password, DB: c.db}
}

func (c redisConfig) asynqOptions() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: c.address, Username: c.username, Password: c.password, DB: c.db}
}
