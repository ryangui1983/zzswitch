package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestInflateUpstreamUsage(t *testing.T) {
	t.Run("nil is a no-op", func(t *testing.T) {
		require.False(t, InflateUpstreamUsage(nil))
	})

	t.Run("thresholds are exclusive", func(t *testing.T) {
		usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100}
		require.False(t, InflateUpstreamUsage(usage))
		require.Equal(t, 1000, usage.PromptTokens)
		require.Equal(t, 100, usage.CompletionTokens)
		require.Equal(t, 1100, usage.TotalTokens)
		require.True(t, usage.TokensInflated)
	})

	t.Run("inflates input and output independently", func(t *testing.T) {
		usage := &dto.Usage{PromptTokens: 1001, CompletionTokens: 101, TotalTokens: 1102}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 1101, usage.PromptTokens)
		require.Equal(t, 111, usage.CompletionTokens)
		require.Equal(t, 1212, usage.TotalTokens)
		require.Equal(t, 1101, usage.InputTokens)
		require.Equal(t, 111, usage.OutputTokens)
	})

	t.Run("uses InputTokens when PromptTokens is zero", func(t *testing.T) {
		usage := &dto.Usage{InputTokens: 2000, OutputTokens: 50}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 2200, usage.PromptTokens)
		require.Equal(t, 50, usage.CompletionTokens)
		require.Equal(t, 2250, usage.TotalTokens)
	})

	t.Run("second call does not inflate again", func(t *testing.T) {
		usage := &dto.Usage{PromptTokens: 2000, CompletionTokens: 200}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 2200, usage.PromptTokens)
		require.Equal(t, 220, usage.CompletionTokens)
		require.False(t, InflateUpstreamUsage(usage))
		require.Equal(t, 2200, usage.PromptTokens)
		require.Equal(t, 220, usage.CompletionTokens)
	})
}

func TestInflatedUsageCopyLeavesOriginal(t *testing.T) {
	raw := &dto.Usage{PromptTokens: 2000, CompletionTokens: 200, TotalTokens: 2200}
	cp := InflatedUsageCopy(raw)
	require.Equal(t, 2000, raw.PromptTokens)
	require.False(t, raw.TokensInflated)
	require.Equal(t, 2200, cp.PromptTokens)
	require.Equal(t, 220, cp.CompletionTokens)
	require.True(t, cp.TokensInflated)
}

func TestOverlayUsageJSON(t *testing.T) {
	usage := &dto.Usage{PromptTokens: 2200, CompletionTokens: 220, TotalTokens: 2420}
	chat := OverlayChatUsageJSON([]byte(`{"id":"x","usage":{"prompt_tokens":2000,"completion_tokens":200,"total_tokens":2200}}`), usage)
	require.Contains(t, string(chat), `"prompt_tokens":2200`)
	require.Contains(t, string(chat), `"completion_tokens":220`)
	require.Contains(t, string(chat), `"total_tokens":2420`)

	resp := OverlayResponsesUsageJSON([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":2000,"output_tokens":200,"total_tokens":2200}}}`), usage, "response.usage")
	require.Contains(t, string(resp), `"input_tokens":2200`)
	require.Contains(t, string(resp), `"output_tokens":220`)

	claude := OverlayClaudeUsageJSON([]byte(`{"type":"message_delta","usage":{"input_tokens":2000,"output_tokens":200}}`), usage, "usage")
	require.Contains(t, string(claude), `"input_tokens":2200`)
	require.Contains(t, string(claude), `"output_tokens":220`)

	image := OverlayChatUsageJSON([]byte(`{"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`), usage)
	require.NotContains(t, string(image), `"prompt_tokens"`)
	require.Contains(t, string(image), `"input_tokens":3`)
}

func TestInflateRealtimeUsage(t *testing.T) {
	u := &dto.RealtimeUsage{InputTokens: 2000, OutputTokens: 200, TotalTokens: 2200}
	require.True(t, InflateRealtimeUsage(u))
	require.Equal(t, 2200, u.InputTokens)
	require.Equal(t, 220, u.OutputTokens)
	require.Equal(t, 2420, u.TotalTokens)
}
