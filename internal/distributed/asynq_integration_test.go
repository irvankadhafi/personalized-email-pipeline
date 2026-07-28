package distributed

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/irvankadhafi/personalized-email-pipeline/internal/testinbox"
	"github.com/redis/go-redis/v9"
)

func TestAsynqProducerAndWorkerCompleteDryRun(t *testing.T) {
	address := os.Getenv("REDIS_TEST_ADDR")
	if address == "" {
		t.Skip("REDIS_TEST_ADDR is not set")
	}
	redisClient := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = redisClient.Close() })
	ledger, err := NewLedger(redisClient)
	requireNoError(t, err)
	now := time.Now().UTC()
	campaign := Campaign{
		ID: fmt.Sprintf("asynq-%d", now.UnixNano()), Algorithm: "v1", Seed: 7, Total: 4,
		Created: now, Deadline: now.Add(5 * time.Second), Version: LedgerVersion, Policy: PolicyRetryable,
	}
	keys := campaignKeys(campaign.ID)
	t.Cleanup(func() { _ = redisClient.Del(context.Background(), keys[:]...).Err() })
	queue := TaskTypeDryRun
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: address})
	t.Cleanup(func() {
		_ = inspector.DeleteQueue(queue, true)
		_ = inspector.Close()
	})
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	worker, err := NewWorker(WorkerConfig{Ledger: ledger})
	requireNoError(t, err)
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: address}, asynq.Config{
		Concurrency: 2, Queues: map[string]int{queue: 1}, TaskCheckInterval: 10 * time.Millisecond,
		ShutdownTimeout: time.Second, LogLevel: asynq.ErrorLevel,
	})
	mux := asynq.NewServeMux()
	mux.Handle(queue, worker)
	requireNoError(t, server.Start(mux))
	t.Cleanup(server.Shutdown)
	producer, err := NewProducer(ProducerConfig{
		Ledger: ledger, Enqueuer: AsynqEnqueuer{Client: client}, Inspector: inspector,
		Campaign: campaign, Sink: TaskSinkDryRun, TaskSize: 2, MaxRetry: 3,
		Retention: time.Minute, EnqueueTimeout: time.Second, PollInterval: 5 * time.Millisecond,
	})
	requireNoError(t, err)

	result, err := producer.Enqueue(context.Background())
	requireNoError(t, err)
	result, err = producer.Wait(context.Background(), result)
	requireNoError(t, err)

	if !result.Snapshot.IsTerminal() || result.Snapshot.Completed != campaign.Total || result.Unknown != 0 {
		t.Fatalf("unexpected distributed result: %+v", result)
	}
}

func TestAsynqDuplicateDryRunTasksCommitOneEffect(t *testing.T) {
	address, ledger, redisClient := integrationLedger(t)
	now := time.Now().UTC()
	campaign := Campaign{
		ID: fmt.Sprintf("asynq-duplicate-%d", now.UnixNano()), Algorithm: "v1", Seed: 7, Total: 1,
		Created: now, Deadline: now.Add(5 * time.Second), Version: LedgerVersion, Policy: PolicyRetryable,
	}
	cleanupCampaign(t, redisClient, campaign.ID)
	requireNoError(t, ledger.Create(context.Background(), campaign))
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	worker, err := NewWorker(WorkerConfig{Ledger: ledger})
	requireNoError(t, err)
	server := integrationServer(t, address, TaskTypeDryRun, worker)
	client := integrationClient(t, address, TaskTypeDryRun)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkDryRun,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 1,
	})
	requireNoError(t, err)
	_, err = client.Enqueue(task, asynq.Queue(TaskTypeDryRun), asynq.TaskID("duplicate-a"))
	requireNoError(t, err)
	_, err = client.Enqueue(task, asynq.Queue(TaskTypeDryRun), asynq.TaskID("duplicate-b"))
	requireNoError(t, err)

	waitForTerminalSnapshot(t, ledger, campaign.ID)
	server.Shutdown()
	snapshot, err := ledger.Snapshot(context.Background(), campaign.ID)
	requireNoError(t, err)
	if snapshot.Completed != 1 || snapshot.Failed != 0 || snapshot.Duplicates < 1 {
		t.Fatalf("unexpected duplicate snapshot: %+v", snapshot)
	}
}

func TestAsynqTestInboxRedeliveryAttemptsSMTPOnce(t *testing.T) {
	address, ledger, redisClient := integrationLedger(t)
	now := time.Now().UTC()
	campaign := Campaign{
		ID: fmt.Sprintf("asynq-delivery-%d", now.UnixNano()), Algorithm: "v1", Seed: 7, Total: 1,
		Created: now, Deadline: now.Add(5 * time.Second), Version: LedgerVersion,
		Policy: PolicyOneShotDelivery, DeliveryLimit: MaxDeliveryLimit,
	}
	cleanupCampaign(t, redisClient, campaign.ID)
	requireNoError(t, ledger.Create(context.Background(), campaign))
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 1, Now: now}))
	var deliveries atomic.Int64
	worker, err := NewWorker(WorkerConfig{
		Ledger: ledger,
		Deliver: func(context.Context, []byte) testinbox.DeliveryResult {
			deliveries.Add(1)
			return testinbox.DeliveryResult{Status: testinbox.DeliveryConfirmed}
		},
	})
	requireNoError(t, err)
	server := integrationServer(t, address, TaskTypeTestInbox, worker)
	client := integrationClient(t, address, TaskTypeTestInbox)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkTestInbox,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 1,
	})
	requireNoError(t, err)
	_, err = client.Enqueue(task, asynq.Queue(TaskTypeTestInbox), asynq.TaskID("delivery-a"))
	requireNoError(t, err)
	_, err = client.Enqueue(task, asynq.Queue(TaskTypeTestInbox), asynq.TaskID("delivery-b"))
	requireNoError(t, err)

	waitForTerminalSnapshot(t, ledger, campaign.ID)
	server.Shutdown()
	snapshot, err := ledger.Snapshot(context.Background(), campaign.ID)
	requireNoError(t, err)
	if deliveries.Load() != 1 || snapshot.Completed != 1 || snapshot.DeliveryReserved != 1 || snapshot.Duplicates < 1 {
		t.Fatalf("deliveries=%d snapshot=%+v", deliveries.Load(), snapshot)
	}
}

func TestAsynqRetryExhaustionCommitsDryRunFailure(t *testing.T) {
	address, ledger, redisClient := integrationLedger(t)
	now := time.Now().UTC()
	campaign := Campaign{
		ID: fmt.Sprintf("asynq-exhausted-%d", now.UnixNano()), Algorithm: "v1", Seed: 7, Total: 2,
		Created: now, Deadline: now.Add(5 * time.Second), Version: LedgerVersion, Policy: PolicyRetryable,
	}
	cleanupCampaign(t, redisClient, campaign.ID)
	requireNoError(t, ledger.Create(context.Background(), campaign))
	requireNoError(t, ledger.Acknowledge(context.Background(), campaign.ID, AcknowledgeRequest{Start: 1, End: 2, Now: now}))
	_, err := ledger.Begin(context.Background(), campaign.ID, BeginRequest{Ordinal: 1, Kind: AttemptInitial, Now: now})
	requireNoError(t, err)
	worker, err := NewWorker(WorkerConfig{Ledger: ledger})
	requireNoError(t, err)
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: address}, asynq.Config{
		Concurrency: 1, Queues: map[string]int{TaskTypeDryRun: 1}, TaskCheckInterval: 10 * time.Millisecond,
		ShutdownTimeout: time.Second, LogLevel: asynq.ErrorLevel, ErrorHandler: worker,
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return time.Millisecond },
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeDryRun, func(context.Context, *asynq.Task) error {
		return &recordTaskError{ordinal: 1, err: ErrUnavailable}
	})
	requireNoError(t, server.Start(mux))
	t.Cleanup(server.Shutdown)
	client := integrationClient(t, address, TaskTypeDryRun)
	task, err := NewTask(TaskPayload{
		Version: PayloadVersion, CampaignID: campaign.ID, Sink: TaskSinkDryRun,
		Algorithm: campaign.Algorithm, Seed: campaign.Seed, Count: campaign.Total, First: 1, Last: 2,
	})
	requireNoError(t, err)
	_, err = client.Enqueue(task, asynq.Queue(TaskTypeDryRun), asynq.MaxRetry(0))
	requireNoError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, snapshotErr := ledger.Snapshot(context.Background(), campaign.ID)
		requireNoError(t, snapshotErr)
		if snapshot.Failed == 1 {
			if snapshot.Completed != 0 || snapshot.Started != 0 || snapshot.Acknowledged != 1 {
				t.Fatalf("unexpected exhausted snapshot: %+v", snapshot)
			}
			requireNoError(t, ledger.Close(context.Background(), campaign.ID, CloseRequest{Now: time.Now(), Reason: FailureInterrupted}))
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("retry exhaustion did not reach terminal state")
}

func integrationLedger(t *testing.T) (string, *Ledger, *redis.Client) {
	t.Helper()
	address := os.Getenv("REDIS_TEST_ADDR")
	if address == "" {
		t.Skip("REDIS_TEST_ADDR is not set")
	}
	redisClient := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = redisClient.Close() })
	ledger, err := NewLedger(redisClient)
	requireNoError(t, err)
	return address, ledger, redisClient
}

func cleanupCampaign(t *testing.T, client *redis.Client, id string) {
	t.Helper()
	keys := campaignKeys(id)
	t.Cleanup(func() { _ = client.Del(context.Background(), keys[:]...).Err() })
}

func integrationServer(t *testing.T, address, queue string, worker *Worker) *asynq.Server {
	t.Helper()
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: address}, asynq.Config{
		Concurrency: 2, Queues: map[string]int{queue: 1}, TaskCheckInterval: 10 * time.Millisecond,
		ShutdownTimeout: time.Second, LogLevel: asynq.ErrorLevel,
	})
	mux := asynq.NewServeMux()
	mux.Handle(queue, worker)
	requireNoError(t, server.Start(mux))
	t.Cleanup(server.Shutdown)
	return server
}

func integrationClient(t *testing.T, address, queue string) *asynq.Client {
	t.Helper()
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: address})
	t.Cleanup(func() {
		_ = inspector.DeleteQueue(queue, true)
		_ = inspector.Close()
	})
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func waitForTerminalSnapshot(t *testing.T, ledger *Ledger, id string) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := ledger.Snapshot(context.Background(), id)
		requireNoError(t, err)
		if snapshot.IsTerminal() && snapshot.Duplicates > 0 {
			return snapshot
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("campaign did not reach duplicate terminal state")
	return Snapshot{}
}
