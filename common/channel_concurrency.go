package common

import (
	"sync"
	"sync/atomic"
)

var (
	channelConcurrencyMu       sync.Mutex
	channelConcurrencyCounters = make(map[int]*atomic.Int32)
)

func getChannelConcurrencyCounter(channelId int) *atomic.Int32 {
	channelConcurrencyMu.Lock()
	defer channelConcurrencyMu.Unlock()
	if c, ok := channelConcurrencyCounters[channelId]; ok {
		return c
	}
	c := &atomic.Int32{}
	channelConcurrencyCounters[channelId] = c
	return c
}

// TryAcquireChannelSlot attempts to increment the concurrency counter.
// Returns true if acquired (below max). If maxConcurrent <= 0, always returns true.
func TryAcquireChannelSlot(channelId, maxConcurrent int) bool {
	if maxConcurrent <= 0 {
		return true
	}
	counter := getChannelConcurrencyCounter(channelId)
	for {
		current := counter.Load()
		if current >= int32(maxConcurrent) {
			return false
		}
		if counter.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

// ReleaseChannelSlot decrements the concurrency counter atomically.
func ReleaseChannelSlot(channelId int) {
	counter := getChannelConcurrencyCounter(channelId)
	for {
		current := counter.Load()
		if current <= 0 {
			return
		}
		if counter.CompareAndSwap(current, current-1) {
			return
		}
	}
}

// GetChannelConcurrency returns the current concurrent request count.
func GetChannelConcurrency(channelId int) int {
	return int(getChannelConcurrencyCounter(channelId).Load())
}
