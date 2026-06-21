package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const channelHealthRedisPrefix = "channel:health:"

type ChannelHealthStats struct {
	TotalRequests int
	ErrorRequests int
}

func channelHealthRedisKey(channelId int) string {
	return channelHealthRedisPrefix + strconv.Itoa(channelId)
}

func RecordChannelHealthOutcome(ctx context.Context, channelId int, windowSeconds int, isError bool, statusCode int) (ChannelHealthStats, error) {
	rdb := common.RDB
	if !common.RedisEnabled || rdb == nil {
		return ChannelHealthStats{}, nil
	}

	key := channelHealthRedisKey(channelId)
	now := time.Now()
	nowMillis := now.UnixMilli()
	windowStartMillis := now.Add(-time.Duration(windowSeconds) * time.Second).UnixMilli()
	member := fmt.Sprintf("%d:%d:%t", now.UnixNano(), statusCode, isError)

	pipe := rdb.TxPipeline()
	pipe.ZAdd(ctx, key, &redis.Z{Score: float64(nowMillis), Member: member})
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(windowStartMillis-1, 10))
	pipe.Expire(ctx, key, time.Duration(windowSeconds+5)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return ChannelHealthStats{}, err
	}

	members, err := rdb.ZRangeByScore(ctx, key, &redis.ZRangeBy{Min: strconv.FormatInt(windowStartMillis, 10), Max: "+inf"}).Result()
	if err != nil {
		return ChannelHealthStats{}, err
	}

	stats := ChannelHealthStats{TotalRequests: len(members)}
	for _, item := range members {
		parts := strings.Split(item, ":")
		if len(parts) < 3 {
			continue
		}
		if parts[2] == "true" {
			stats.ErrorRequests++
		}
	}
	return stats, nil
}
