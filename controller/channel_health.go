package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
)

func handleChannelHealthRecordSuccess(channel *model.Channel) {
	if channel == nil {
		return
	}
	settings := channel.GetSetting()
	if !settings.ErrorRatioDisableEnabled {
		return
	}
	_, _ = recordChannelHealthOutcome(channel.Id, settings.GetErrorRatioWindowSeconds(), false, 200)
}

func handleChannelHealthOnError(channel *model.Channel, statusCode int, reason string) {
	if channel == nil || !channel.GetAutoBan() {
		return
	}
	settings := channel.GetSetting()
	if !settings.ShouldTrackErrorRatioStatusCode(statusCode) {
		return
	}
	stats, err := recordChannelHealthOutcome(channel.Id, settings.GetErrorRatioWindowSeconds(), true, statusCode)
	if err != nil {
		common.SysError(fmt.Sprintf("record channel health outcome failed: channel_id=%d, error=%v", channel.Id, err))
		return
	}
	if stats.TotalRequests < settings.GetErrorRatioMinRequests() {
		return
	}
	if stats.TotalRequests == 0 {
		return
	}
	if float64(stats.ErrorRequests)/float64(stats.TotalRequests) >= settings.GetErrorRatioThreshold() {
		service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), fmt.Sprintf("短窗口错误率过高：%d/%d >= %.2f", stats.ErrorRequests, stats.TotalRequests, settings.GetErrorRatioThreshold()))
	}
}

func shouldRecordChannelHealthError(cachedChannel *model.Channel, selectedRetryChannel *model.Channel) bool {
	return cachedChannel == nil || selectedRetryChannel == nil || cachedChannel.Id != selectedRetryChannel.Id
}

func shouldAutoRecoverChannel(channel *model.Channel) bool {
	return channel != nil && channel.Status == common.ChannelStatusAutoDisabled && channel.GetSetting().HealthCheckAutoEnableEnabled
}

func tryAutoRecoverChannel(channel *model.Channel) {
	if !shouldAutoRecoverChannel(channel) {
		return
	}
	result := testChannel(channel, "", string(constant.EndpointTypeOpenAI), false)
	if result.newAPIError != nil || result.localErr != nil {
		return
	}
	latest, err := model.GetChannelById(channel.Id, true)
	if err != nil || !shouldAutoRecoverChannel(latest) {
		return
	}
	service.EnableChannel(latest.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), latest.Name)
}

func recoverAutoDisabledChannels() {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysError(fmt.Sprintf("get channels for auto recovery failed: %v", err))
		return
	}
	for _, channel := range channels {
		tryAutoRecoverChannel(channel)
	}
}

var channelAutoRecoveryOnce sync.Once

func StartChannelAutoRecoveryTask() {
	if !common.IsMasterNode {
		return
	}
	channelAutoRecoveryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				recoverAutoDisabledChannels()
			}
		}()
	})
}

func recordChannelHealthOutcome(channelId int, windowSeconds int, isError bool, statusCode int) (service.ChannelHealthStats, error) {
	return service.RecordChannelHealthOutcome(context.Background(), channelId, windowSeconds, isError, statusCode)
}
