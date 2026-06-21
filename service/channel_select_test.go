package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRetryParamSelectRetryChannelPoolMode(t *testing.T) {
	param := &RetryParam{}
	channel := &model.Channel{
		Id:      7,
		Setting: common.GetPointer(`{"scheduler_pool_mode_enabled":true,"scheduler_pool_mode_retry_times":2,"scheduler_pool_mode_retry_status_codes":"429"}`),
	}
	upstreamErr := types.NewOpenAIError(errors.New("too many requests"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)

	param.SelectRetryChannel(upstreamErr, channel)
	require.Equal(t, channel, param.RetryChannel)
	require.Empty(t, param.ExhaustedChannelIds)

	param.RetryChannel = nil
	param.SelectRetryChannel(upstreamErr, channel)
	require.Equal(t, channel, param.RetryChannel)
	require.Empty(t, param.ExhaustedChannelIds)

	param.RetryChannel = nil
	param.SelectRetryChannel(upstreamErr, channel)
	require.Nil(t, param.RetryChannel)
	require.True(t, param.ExhaustedChannelIds[channel.Id])
}

func TestRetryParamSelectRetryChannelSwitchesOnNonPoolStatus(t *testing.T) {
	param := &RetryParam{}
	channel := &model.Channel{
		Id:      9,
		Setting: common.GetPointer(`{"scheduler_pool_mode_enabled":true,"scheduler_pool_mode_retry_times":2,"scheduler_pool_mode_retry_status_codes":"429"}`),
	}
	upstreamErr := types.NewOpenAIError(errors.New("server error"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	param.SelectRetryChannel(upstreamErr, channel)
	require.Nil(t, param.RetryChannel)
	require.True(t, param.ExhaustedChannelIds[channel.Id])
}

func TestRetryParamSelectRetryChannelSkipsImagePermission403(t *testing.T) {
	param := &RetryParam{}
	channel := &model.Channel{
		Id:      11,
		Setting: common.GetPointer(`{"scheduler_pool_mode_enabled":true,"scheduler_pool_mode_retry_times":2,"scheduler_pool_mode_retry_status_codes":"403"}`),
	}
	upstreamErr := types.NewOpenAIError(errors.New("Image generation is not enabled for this group"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)

	param.SelectRetryChannel(upstreamErr, channel)
	require.Nil(t, param.RetryChannel)
	require.True(t, param.ExhaustedChannelIds[channel.Id])
}

func TestRetryParamSelectRetryChannelIgnoresDisabledPoolMode(t *testing.T) {
	param := &RetryParam{}
	channel := &model.Channel{
		Id:      13,
		Setting: common.GetPointer(`{"scheduler_pool_mode_enabled":false,"scheduler_pool_mode_retry_times":2,"scheduler_pool_mode_retry_status_codes":"429"}`),
	}
	upstreamErr := types.NewOpenAIError(errors.New("too many requests"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)

	param.SelectRetryChannel(upstreamErr, channel)
	require.Nil(t, param.RetryChannel)
	require.True(t, param.ExhaustedChannelIds[channel.Id])
}
