package campaign

import (
	"context"
	"sync"
	"time"
)

func runWorker(ctx context.Context, wg *sync.WaitGroup, ready chan<- workerReady, results chan<- workerResult, sink SinkFunc) {
	defer wg.Done()
	grant := make(chan job)
	for {
		select {
		case ready <- workerReady{grant: grant}:
		case <-ctx.Done():
			return
		}
		select {
		case work := <-grant:
			result := Render(ctx, work.recipient, sink)
			event := workerResult{ordinal: work.recipient.Ordinal, category: result.Category, reason: result.Reason, accepted: time.Now()}
			select {
			case results <- event:
			case <-ctx.Done():
				select {
				case results <- event:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
