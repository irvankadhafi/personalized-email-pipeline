package campaign

import (
	"context"
	"runtime"
	"time"
)

func addReason(target map[Reason]ReasonSummary, reason Reason, ordinal int64, limit int) {
	summary := target[reason]
	summary.Count++
	if len(summary.Ordinals) < limit {
		summary.Ordinals = append(summary.Ordinals, ordinal)
	} else {
		largest := 0
		for i := range summary.Ordinals {
			if summary.Ordinals[i] > summary.Ordinals[largest] {
				largest = i
			}
		}
		if ordinal < summary.Ordinals[largest] {
			summary.Ordinals[largest] = ordinal
		}
		summary.Omitted++
	}
	target[reason] = summary
}

func startHeapSampler(ctx context.Context) <-chan uint64 {
	result := make(chan uint64, 1)
	go func() {
		ticker := time.NewTicker(HeapSampleInterval)
		defer ticker.Stop()
		var peak uint64
		read := func() {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			if stats.HeapInuse > peak {
				peak = stats.HeapInuse
			}
		}
		read()
		for {
			select {
			case <-ticker.C:
				read()
			case <-ctx.Done():
				read()
				result <- peak
				return
			}
		}
	}()
	return result
}
