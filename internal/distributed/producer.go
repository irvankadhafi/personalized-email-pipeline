package distributed

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

var (
	ErrUnknownState       = errors.New("distributed state unknown")
	ErrCompletionDeadline = errors.New("distributed completion deadline reached")
)

type Enqueuer interface {
	EnqueueContext(context.Context, *asynq.Task, string, string, int, time.Duration) (*asynq.TaskInfo, error)
}

type TaskInspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
}

type ProducerConfig struct {
	Ledger         *Ledger
	Enqueuer       Enqueuer
	Inspector      TaskInspector
	Campaign       Campaign
	Sink           TaskSink
	TaskSize       int64
	MaxRetry       int
	Retention      time.Duration
	EnqueueTimeout time.Duration
	PollInterval   time.Duration
	Now            func() time.Time
}

type ProducerResult struct {
	Snapshot      Snapshot
	KnownEnqueued int64
	Unknown       int64
}

type Producer struct {
	config ProducerConfig
}

func NewProducer(config ProducerConfig) (*Producer, error) {
	if config.Ledger == nil || config.Enqueuer == nil || config.Campaign.Validate() != nil ||
		config.TaskSize < 1 || config.TaskSize > MaxTaskRange || config.MaxRetry < 0 ||
		config.Retention <= 0 || config.EnqueueTimeout <= 0 || !producerSinkMatches(config.Sink, config.Campaign) {
		return nil, ErrInvalidRequest
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 10 * time.Millisecond
	}
	return &Producer{config: config}, nil
}

func (p *Producer) Wait(ctx context.Context, result ProducerResult) (ProducerResult, error) {
	for {
		snapshot, err := p.snapshot()
		if err != nil {
			result.Unknown = p.config.Campaign.Total - result.Snapshot.KnownTerminal()
			return result, ErrUnknownState
		}
		result.Snapshot = snapshot
		if snapshot.IsTerminal() {
			result.KnownEnqueued = snapshot.KnownEnqueued()
			result.Unknown = 0
			return result, nil
		}
		if err := ctx.Err(); err != nil {
			closed, closeErr := p.close(result, FailureInterrupted, err)
			if closeErr != nil && !errors.Is(closeErr, err) {
				return closed, closeErr
			}
			return p.waitClosed(closed, err)
		}
		if !p.config.Now().Before(p.config.Campaign.Deadline) {
			closed, closeErr := p.close(result, FailureDeadline, nil)
			if closeErr != nil {
				return closed, closeErr
			}
			return p.settle(closed, ErrCompletionDeadline)
		}
		timer := time.NewTimer(p.config.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (p *Producer) waitClosed(result ProducerResult, resultErr error) (ProducerResult, error) {
	for {
		if result.Snapshot.IsTerminal() {
			result.KnownEnqueued = result.Snapshot.KnownEnqueued()
			result.Unknown = 0
			return result, resultErr
		}
		if !p.config.Now().Before(p.config.Campaign.Deadline) {
			return p.settle(result, resultErr)
		}
		time.Sleep(p.config.PollInterval)
		snapshot, err := p.snapshot()
		if err != nil {
			result.Unknown = p.config.Campaign.Total - result.Snapshot.KnownTerminal()
			return result, ErrUnknownState
		}
		result.Snapshot = snapshot
	}
}

func (p *Producer) settle(result ProducerResult, resultErr error) (ProducerResult, error) {
	operationCtx, cancel := context.WithTimeout(context.Background(), p.config.EnqueueTimeout)
	defer cancel()
	err := p.config.Ledger.Settle(operationCtx, p.config.Campaign.ID, SettlementRequest{
		Now: p.config.Now(), Reason: FailureDeadline,
	})
	if err != nil {
		result.Unknown = p.config.Campaign.Total - result.Snapshot.KnownTerminal()
		return result, ErrUnknownState
	}
	snapshot, err := p.config.Ledger.Snapshot(operationCtx, p.config.Campaign.ID)
	if err != nil || !snapshot.IsTerminal() {
		result.Unknown = p.config.Campaign.Total - result.Snapshot.KnownTerminal()
		return result, ErrUnknownState
	}
	result.Snapshot = snapshot
	result.KnownEnqueued = snapshot.KnownEnqueued()
	result.Unknown = 0
	return result, resultErr
}

func (p *Producer) snapshot() (Snapshot, error) {
	operationCtx, cancel := context.WithTimeout(context.Background(), p.config.EnqueueTimeout)
	defer cancel()
	return p.config.Ledger.Snapshot(operationCtx, p.config.Campaign.ID)
}

func (p *Producer) Enqueue(ctx context.Context) (ProducerResult, error) {
	if err := p.config.Ledger.Create(ctx, p.config.Campaign); err != nil {
		return ProducerResult{Unknown: p.config.Campaign.Total}, err
	}
	result := ProducerResult{}
	for first := int64(1); first <= p.config.Campaign.Total; first += p.config.TaskSize {
		if err := ctx.Err(); err != nil {
			return p.close(result, FailureInterrupted, err)
		}
		last := min(first+p.config.TaskSize-1, p.config.Campaign.Total)
		confirmed, err := p.enqueueRange(first, last)
		if err != nil || !confirmed {
			return p.closeUnknown(result)
		}
		if err := p.config.Ledger.Acknowledge(context.Background(), p.config.Campaign.ID, AcknowledgeRequest{
			Start: first, End: last, Now: p.config.Now(),
		}); err != nil {
			return p.closeUnknown(result)
		}
		result.KnownEnqueued += last - first + 1
	}
	snapshot, err := p.config.Ledger.Snapshot(ctx, p.config.Campaign.ID)
	if err != nil {
		result.Unknown = p.config.Campaign.Total - result.KnownEnqueued
		return result, err
	}
	result.Snapshot = snapshot
	return result, nil
}

func (p *Producer) enqueueRange(first, last int64) (bool, error) {
	payload := TaskPayload{
		Version: PayloadVersion, CampaignID: p.config.Campaign.ID, Sink: p.config.Sink,
		Algorithm: p.config.Campaign.Algorithm, Seed: p.config.Campaign.Seed, Count: p.config.Campaign.Total,
		First: first, Last: last,
	}
	task, err := NewTask(payload)
	if err != nil {
		return false, err
	}
	id := TaskID(p.config.Campaign.ID, first, last)
	queue := task.Type()
	enqueueCtx, cancel := context.WithTimeout(context.Background(), p.config.EnqueueTimeout)
	info, enqueueErr := p.config.Enqueuer.EnqueueContext(enqueueCtx, task, id, queue, p.config.MaxRetry, p.config.Retention)
	cancel()
	if enqueueErr == nil {
		return info != nil && info.ID == id && info.Queue == queue, nil
	}
	if p.config.Inspector == nil {
		return false, enqueueErr
	}
	info, inspectErr := p.config.Inspector.GetTaskInfo(queue, id)
	if inspectErr != nil {
		return false, enqueueErr
	}
	return info != nil && info.ID == id && info.Queue == queue, nil
}

func (p *Producer) closeUnknown(result ProducerResult) (ProducerResult, error) {
	result.Unknown = p.config.Campaign.Total - result.KnownEnqueued
	return p.close(result, FailureInterrupted, ErrUnknownState)
}

func (p *Producer) close(result ProducerResult, reason FailureReason, resultErr error) (ProducerResult, error) {
	closeCtx, cancel := context.WithTimeout(context.Background(), p.config.EnqueueTimeout)
	defer cancel()
	if err := p.config.Ledger.Close(closeCtx, p.config.Campaign.ID, CloseRequest{Now: p.config.Now(), Reason: reason}); err != nil {
		return result, ErrUnknownState
	}
	snapshot, err := p.config.Ledger.Snapshot(closeCtx, p.config.Campaign.ID)
	if err != nil {
		return result, ErrUnknownState
	}
	result.Snapshot = snapshot
	return result, resultErr
}

func producerSinkMatches(sink TaskSink, campaign Campaign) bool {
	return sink == TaskSinkDryRun && campaign.Policy == PolicyRetryable ||
		sink == TaskSinkTestInbox && campaign.Policy == PolicyOneShotDelivery
}

type AsynqEnqueuer struct {
	Client *asynq.Client
}

func (e AsynqEnqueuer) EnqueueContext(ctx context.Context, task *asynq.Task, id, queue string, maxRetry int, retention time.Duration) (*asynq.TaskInfo, error) {
	return e.Client.EnqueueContext(ctx, task, asynq.TaskID(id), asynq.Queue(queue), asynq.MaxRetry(maxRetry), asynq.Retention(retention))
}
