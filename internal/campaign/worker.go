package campaign

import (
	"context"
	"sync"
	"time"
)

type workerConfig struct {
	ready   chan<- workerReady
	results chan<- workerResult
	format  Format
	sink    MessageSinkFunc
}

func runWorker(ctx context.Context, workers *sync.WaitGroup, config workerConfig) {
	defer workers.Done()
	grant := make(chan job)
	for {
		select {
		case config.ready <- workerReady{grant: grant}:
		case <-ctx.Done():
			return
		}
		select {
		case work := <-grant:
			message, category := RenderMessage(work.recipient, config.format)
			reason := Reason("")
			if err := config.sink(ctx, message); err != nil {
				reason = ReasonSink
			}
			event := workerResult{ordinal: work.recipient.Ordinal, category: category, reason: reason, accepted: time.Now()}
			select {
			case config.results <- event:
			case <-ctx.Done():
				select {
				case config.results <- event:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
