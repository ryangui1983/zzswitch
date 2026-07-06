package common

import (
	"sync"
	"sync/atomic"
)

var channelConcurrencyMap sync.Map // key: int → *atomic.Int32

func getChannelConcurrencyCounter(channelId int) *atomic.Int32 {
	if v, ok := channelConcurrencyMap.Load(channelId); ok {
		return v.(*atomic.Int32)
	}
	newCounter := &atomic.Int32{}
	v, _ := channelConcurrencyMap.LoadOrStore(channelId, newCounter)
	return v.(*atomic.Int32)
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
