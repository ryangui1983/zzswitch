package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type opsWebhookEvent struct {
	Event        string `json:"event"`          // "dispatch" | "complete"
	ChannelID    int    `json:"channel_id"`
	RequestID    string `json:"request_id"`
	FirstTokenMs int64  `json:"first_token_ms,omitempty"` // streaming only, >0 when present
	Success      bool   `json:"success"`
	Ts           int64  `json:"ts"` // unix milliseconds
}

const opsWebhookQueueSize = 8192

var (
	opsWebhookQueue      = make(chan opsWebhookEvent, opsWebhookQueueSize)
	opsWebhookDropLastAt atomic.Int64
	opsWebhookClient     = &http.Client{Timeout: 200 * time.Millisecond}
)

func init() {
	workers := max(2, runtime.GOMAXPROCS(0))
	for range workers {
		go func() {
			for event := range opsWebhookQueue {
				sendOpsWebhook(event)
			}
		}()
	}
}

func sendOpsWebhook(event opsWebhookEvent) {
	url := operation_setting.GetOpsAssistantURL()
	if url == "" {
		common.SysLog("sendOpsWebhook: URL is empty")
		return
	}
	common.SysLog(fmt.Sprintf("sendOpsWebhook: sending to %s, event=%+v", url, event))
	body, err := common.Marshal(event)
	if err != nil {
		common.SysError(fmt.Sprintf("ops webhook marshal failed: %v", err))
		return
	}
	resp, err := opsWebhookClient.Post(url+"/inflight/event", "application/json", bytes.NewReader(body))
	if err != nil {
		common.SysError(fmt.Sprintf("ops webhook post failed: %v", err))
		return
	}
	common.SysLog(fmt.Sprintf("sendOpsWebhook: success, status=%d", resp.StatusCode))
	resp.Body.Close()
}

func enqueueOpsWebhook(event opsWebhookEvent) {
	url := operation_setting.GetOpsAssistantURL()
	if url == "" {
		common.SysLog("enqueueOpsWebhook: URL is empty, skipping")
		return
	}
	select {
	case opsWebhookQueue <- event:
	default:
		now := time.Now().Unix()
		last := opsWebhookDropLastAt.Load()
		if now-last >= 60 && opsWebhookDropLastAt.CompareAndSwap(last, now) {
			common.SysError(fmt.Sprintf("ops webhook queue full, dropping (cap=%d)", opsWebhookQueueSize))
		}
	}
}

func opsFirstTokenMs(info *relaycommon.RelayInfo, start time.Time) int64 {
	if info.IsStream && info.HasSendResponse() {
		ms := info.FirstResponseTime.Sub(start).Milliseconds()
		if ms > 0 {
			return ms
		}
	}
	return 0
}
