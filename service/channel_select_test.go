package service

import (
	"errors"
	"net/http"
	"testing"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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

func TestRetryParamSelectRetryChannelSkipsCloudflare524(t *testing.T) {
	param := &RetryParam{}
	channel := &model.Channel{
		Id:      15,
		Setting: common.GetPointer(`{"scheduler_pool_mode_enabled":true,"scheduler_pool_mode_retry_times":2,"scheduler_pool_mode_retry_status_codes":"500-599"}`),
	}
	upstreamErr := types.NewOpenAIError(errors.New("cloudflare timeout"), types.ErrorCodeBadResponseStatusCode, 524)

	param.SelectRetryChannel(upstreamErr, channel)
	require.Nil(t, param.RetryChannel)
	require.Empty(t, param.ExhaustedChannelIds)
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

func TestPinnedTaskPluginChannelTypesUsesPinnedGenerationIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("legacy-select", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "legacy-select")
	assert.Equal(t, []int{constant.ChannelTypeKling}, types)
	assert.Equal(t, []string{"legacy-select"}, keys)
	types, keys = pinnedTaskPluginIdentities(c, "another-plugin")
	assert.Empty(t, types)
	assert.Empty(t, keys)
	types, keys = pinnedTaskPluginIdentities(nil, "legacy-select")
	assert.Empty(t, types)
	assert.Empty(t, keys)
}

func TestPinnedTaskPluginChannelTypesLeavesGenericChannelsKeyed(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("generic-select", constant.ChannelTypeTaskPlugin), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "generic-select")
	assert.Empty(t, types)
	assert.Equal(t, []string{"generic-select"}, keys)
}

func TestPinnedTaskPluginChannelTypesIncludesSharedEndpointProviders(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(channelSelectEndpointPluginSource("gemini-select", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(channelSelectEndpointPluginSource("vertex-select", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
	})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})

	AppendTaskPluginIdentityFilter(c, candidates[0].Plugin.Meta.Key)
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi}, filters[0].TaskPluginChannelTypes)
	assert.Equal(t, []string{"gemini-select", "vertex-select"}, filters[0].TaskPluginKeys)
}

func channelSelectTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectChannelTypesField(channelType int) string {
	if channelType <= 0 || channelType == constant.ChannelTypeTaskPlugin {
		return ""
	}
	return fmt.Sprintf("channelTypes: [%d],", channelType)
}

func TestPinnedTaskPluginChannelTypesIncludesCompatibleTypes(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectCompatiblePluginSource("sora-select", constant.ChannelTypeSora, constant.ChannelTypeOpenAI), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "sora-select")
	assert.Equal(t, []int{constant.ChannelTypeSora, constant.ChannelTypeOpenAI}, types)
	assert.Equal(t, []string{"sora-select"}, keys)
}

func channelSelectCompatiblePluginSource(key string, channelType, compatibleType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d, %d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType, compatibleType)
}

func TestSharedType61IdentityFilterContainsAllCandidateKeys(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		_, err := registry.Register(channelSelectEndpointPluginSource(key, 0), jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Candidates: candidates})
	AppendTaskPluginIdentityFilter(c, "alpha")
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, "alpha", filters[0].TaskPluginKey)
	assert.Equal(t, []string{"alpha", "beta"}, filters[0].TaskPluginKeys)
	assert.Empty(t, filters[0].TaskPluginChannelTypes)
}
