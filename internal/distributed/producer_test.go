package distributed

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/campaign"
)

func TestProducerPropagatesFormatWithoutChangingTaskIdentityOrQueue(t *testing.T) {
	tests := []struct {
		name       string
		format     campaign.Format
		wantMember bool
	}{
		{name: "default text", format: campaign.TextFormat},
		{name: "explicit html", format: campaign.HTMLFormat, wantMember: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			ledger, _, distributedCampaign, now := newTestLedger(t, 3)
			keys := campaignKeys(distributedCampaign.ID)
			requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
			enqueuer := &fakeEnqueuer{}
			producer, err := NewProducer(ProducerConfig{
				Ledger: ledger, Enqueuer: enqueuer, Campaign: distributedCampaign, Sink: TaskSinkDryRun,
				Format: test.format, TaskSize: 2, MaxRetry: 3, Retention: time.Hour,
				EnqueueTimeout: time.Second, Now: func() time.Time { return now },
			})
			requireNoError(t, err)

			// When
			_, err = producer.Enqueue(context.Background())
			requireNoError(t, err)

			// Then
			for index, call := range enqueuer.calls {
				payload, decodeErr := DecodeTask(call.task)
				requireNoError(t, decodeErr)
				if payload.Format != test.format {
					t.Fatalf("task %d format=%q", index, payload.Format.String())
				}
				hasMember := bytes.Contains(call.task.Payload(), []byte(`"format"`))
				if hasMember != test.wantMember || call.taskID != TaskID(distributedCampaign.ID, payload.First, payload.Last) || call.queue != TaskTypeDryRun {
					t.Fatalf("task %d call=%+v payload=%s", index, call, call.task.Payload())
				}
			}
		})
	}
}

func TestProducerRejectsOppositeFormatDuplicateBeforeEnqueue(t *testing.T) {
	tests := []struct {
		name         string
		firstFormat  campaign.Format
		secondFormat campaign.Format
	}{
		{name: "text first html second", firstFormat: campaign.TextFormat, secondFormat: campaign.HTMLFormat},
		{name: "html first text second", firstFormat: campaign.HTMLFormat, secondFormat: campaign.TextFormat},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			ledger, _, distributedCampaign, now := newTestLedger(t, 2)
			keys := campaignKeys(distributedCampaign.ID)
			requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
			firstEnqueuer := &fakeEnqueuer{}
			firstProducer, err := NewProducer(ProducerConfig{
				Ledger: ledger, Enqueuer: firstEnqueuer, Campaign: distributedCampaign, Sink: TaskSinkDryRun,
				Format: test.firstFormat, TaskSize: 2, MaxRetry: 3, Retention: time.Hour,
				EnqueueTimeout: time.Second, Now: func() time.Time { return now },
			})
			requireNoError(t, err)
			_, err = firstProducer.Enqueue(context.Background())
			requireNoError(t, err)
			before, err := ledger.Snapshot(context.Background(), distributedCampaign.ID)
			requireNoError(t, err)

			secondEnqueuer := &fakeEnqueuer{}
			secondProducer, err := NewProducer(ProducerConfig{
				Ledger: ledger, Enqueuer: secondEnqueuer, Campaign: distributedCampaign, Sink: TaskSinkDryRun,
				Format: test.secondFormat, TaskSize: 2, MaxRetry: 3, Retention: time.Hour,
				EnqueueTimeout: time.Second, Now: func() time.Time { return now },
			})
			requireNoError(t, err)

			// When
			_, err = secondProducer.Enqueue(context.Background())

			// Then
			if !errors.Is(err, ErrAlreadyExists) {
				t.Fatalf("expected ErrAlreadyExists, got %v", err)
			}
			if len(secondEnqueuer.calls) != 0 {
				t.Fatalf("expected no enqueue calls, got %d", len(secondEnqueuer.calls))
			}
			after, snapshotErr := ledger.Snapshot(context.Background(), distributedCampaign.ID)
			requireNoError(t, snapshotErr)
			if after != before {
				t.Fatalf("ledger snapshot changed: before=%+v after=%+v", before, after)
			}
		})
	}
}

type enqueueCall struct {
	task      *asynq.Task
	taskID    string
	queue     string
	maxRetry  int
	retention time.Duration
}

type fakeEnqueuer struct {
	mu      sync.Mutex
	calls   []enqueueCall
	results []enqueueResult
}

type enqueueResult struct {
	info *asynq.TaskInfo
	err  error
}

func (f *fakeEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, taskID, queue string, maxRetry int, retention time.Duration) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, enqueueCall{task: task, taskID: taskID, queue: queue, maxRetry: maxRetry, retention: retention})
	index := len(f.calls) - 1
	if index < len(f.results) {
		return f.results[index].info, f.results[index].err
	}
	return &asynq.TaskInfo{ID: taskID, Queue: queue}, nil
}

type fakeInspector struct {
	info *asynq.TaskInfo
	err  error
}

func (f fakeInspector) GetTaskInfo(string, string) (*asynq.TaskInfo, error) {
	return f.info, f.err
}

func TestProducerEnqueuesAndAcknowledgesRangesSequentially(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 5)
	keys := campaignKeys(campaign.ID)
	client := ledger.client
	requireNoError(t, client.Del(context.Background(), keys[:]...).Err())
	enqueuer := &fakeEnqueuer{}
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: enqueuer, Campaign: campaign, Sink: TaskSinkDryRun,
		TaskSize: 2, MaxRetry: 3, Retention: time.Hour, EnqueueTimeout: time.Second, Now: func() time.Time { return now },
	})
	requireNoError(t, err)

	result, err := producer.Enqueue(context.Background())
	requireNoError(t, err)

	if result.KnownEnqueued != 5 || result.Unknown != 0 || len(enqueuer.calls) != 3 {
		t.Fatalf("unexpected enqueue result=%+v calls=%d", result, len(enqueuer.calls))
	}
	wantIDs := []string{TaskID(campaign.ID, 1, 2), TaskID(campaign.ID, 3, 4), TaskID(campaign.ID, 5, 5)}
	for index, call := range enqueuer.calls {
		if call.taskID != wantIDs[index] || call.queue != TaskTypeDryRun || call.maxRetry != 3 || call.retention != time.Hour {
			t.Fatalf("unexpected enqueue call %d: %+v", index, call)
		}
	}
	snapshot, err := ledger.Snapshot(context.Background(), campaign.ID)
	requireNoError(t, err)
	if snapshot.Acknowledged != 5 {
		t.Fatalf("expected five acknowledged ordinals, got %+v", snapshot)
	}
}

func TestProducerConfirmsAmbiguousEnqueueByDeterministicTaskID(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 1)
	keys := campaignKeys(campaign.ID)
	requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
	taskID := TaskID(campaign.ID, 1, 1)
	enqueuer := &fakeEnqueuer{results: []enqueueResult{{err: context.DeadlineExceeded}}}
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: enqueuer, Inspector: fakeInspector{info: &asynq.TaskInfo{ID: taskID, Queue: TaskTypeDryRun}},
		Campaign: campaign, Sink: TaskSinkDryRun, TaskSize: 1, MaxRetry: 1, Retention: time.Hour,
		EnqueueTimeout: time.Second, Now: func() time.Time { return now },
	})
	requireNoError(t, err)

	result, err := producer.Enqueue(context.Background())
	requireNoError(t, err)

	if result.KnownEnqueued != 1 || result.Unknown != 0 {
		t.Fatalf("unexpected confirmed result: %+v", result)
	}
}

func TestProducerClosesUnknownWhenAmbiguousEnqueueCannotBeConfirmed(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 2)
	keys := campaignKeys(campaign.ID)
	requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
	enqueuer := &fakeEnqueuer{results: []enqueueResult{{err: context.DeadlineExceeded}}}
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: enqueuer, Inspector: fakeInspector{err: asynq.ErrTaskNotFound},
		Campaign: campaign, Sink: TaskSinkDryRun, TaskSize: 1, MaxRetry: 1, Retention: time.Hour,
		EnqueueTimeout: time.Second, Now: func() time.Time { return now },
	})
	requireNoError(t, err)

	result, err := producer.Enqueue(context.Background())
	if !errors.Is(err, ErrUnknownState) {
		t.Fatalf("expected unknown state, got %v", err)
	}
	if result.KnownEnqueued != 0 || result.Unknown != 2 || len(enqueuer.calls) != 1 {
		t.Fatalf("unexpected unknown result=%+v calls=%d", result, len(enqueuer.calls))
	}
	snapshot, snapshotErr := ledger.Snapshot(context.Background(), campaign.ID)
	requireNoError(t, snapshotErr)
	if snapshot.State != StateClosed || snapshot.IsTerminal() || snapshot.Unprocessed != 0 {
		t.Fatalf("unexpected closed unknown snapshot: %+v", snapshot)
	}
}

func TestProducerWaitClosesAcknowledgedWorkOnCancellation(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 2)
	keys := campaignKeys(campaign.ID)
	requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: &fakeEnqueuer{}, Campaign: campaign, Sink: TaskSinkDryRun,
		TaskSize: 2, MaxRetry: 1, Retention: time.Hour, EnqueueTimeout: time.Second,
		PollInterval: time.Millisecond, Now: func() time.Time { return now },
	})
	requireNoError(t, err)
	result, err := producer.Enqueue(context.Background())
	requireNoError(t, err)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	result, err = producer.Wait(cancelled, result)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if !result.Snapshot.IsTerminal() || result.Snapshot.Unprocessed != 2 || result.Unknown != 0 {
		t.Fatalf("unexpected cancelled result: %+v", result)
	}
}

func TestProducerWaitSettlesStartedWorkAtDeadline(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 1)
	keys := campaignKeys(campaign.ID)
	requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
	campaign.Deadline = now.Add(time.Second)
	clock := now
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: &fakeEnqueuer{}, Campaign: campaign, Sink: TaskSinkDryRun,
		TaskSize: 1, MaxRetry: 1, Retention: time.Hour, EnqueueTimeout: time.Second,
		PollInterval: time.Millisecond, Now: func() time.Time { return clock },
	})
	requireNoError(t, err)
	result, err := producer.Enqueue(context.Background())
	requireNoError(t, err)
	_, err = ledger.Begin(context.Background(), campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	clock = campaign.Deadline

	result, err = producer.Wait(context.Background(), result)
	if !errors.Is(err, ErrCompletionDeadline) {
		t.Fatalf("expected completion deadline, got %v", err)
	}
	if !result.Snapshot.IsTerminal() || result.Snapshot.Failed != 1 || result.Snapshot.Started != 0 {
		t.Fatalf("unexpected deadline result: %+v", result)
	}
}

func TestProducerWaitAfterCancellationObservesStartedWorkToCompletion(t *testing.T) {
	ledger, _, campaign, now := newTestLedger(t, 1)
	keys := campaignKeys(campaign.ID)
	requireNoError(t, ledger.client.Del(context.Background(), keys[:]...).Err())
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: &fakeEnqueuer{}, Campaign: campaign, Sink: TaskSinkDryRun,
		TaskSize: 1, MaxRetry: 1, Retention: time.Hour, EnqueueTimeout: time.Second,
		PollInterval: time.Millisecond, Now: func() time.Time { return now },
	})
	requireNoError(t, err)
	result, err := producer.Enqueue(context.Background())
	requireNoError(t, err)
	_, err = ledger.Begin(context.Background(), campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	committed := make(chan struct{})
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = ledger.Commit(context.Background(), campaign.ID, CommitRequest{
			Ordinal: 1, State: TerminalCompleted, Digest: validDigest(), Now: now,
		})
		close(committed)
	}()

	result, err = producer.Wait(cancelled, result)
	<-committed
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if !result.Snapshot.IsTerminal() || result.Snapshot.Completed != 1 || result.Unknown != 0 {
		t.Fatalf("unexpected completed cancellation result: %+v", result)
	}
}
