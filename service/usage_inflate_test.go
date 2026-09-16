package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func TestInflateCacheWriteAndHitBoost(t *testing.T) {
	setting := operation_setting.GetQuotaSetting()
	origProb := setting.CacheHitBoostProbability
	origIds := setting.CacheHitBoostChannelIds
	t.Cleanup(func() {
		setting.CacheHitBoostProbability = origProb
		setting.CacheHitBoostChannelIds = origIds
	})
	setting.CacheHitBoostChannelIds = "1"

	t.Run("inflates cache write when input is marked up", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		usage := &dto.Usage{
			PromptTokens:     2000,
			CompletionTokens: 50,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:     200,
				CacheWriteTokens: 100,
			},
		}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 2200, usage.PromptTokens)
		require.Equal(t, 110, usage.PromptTokensDetails.CacheWriteTokens)
		require.Equal(t, 200, usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("boosts hit rate to 90 percent when probability is 1", func(t *testing.T) {
		setting.CacheHitBoostProbability = 1
		usage := &dto.Usage{
			PromptTokens:     10000,
			CompletionTokens: 50,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 5000,
			},
		}
		require.True(t, InflateUpstreamUsageForChannel(usage, 1))
		require.Equal(t, 11000, usage.PromptTokens)
		require.Equal(t, 9900, usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("does not boost when probability is 0", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		usage := &dto.Usage{
			PromptTokens: 10000,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 5000,
			},
		}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 11000, usage.PromptTokens)
		require.Equal(t, 5000, usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("does not boost when hit rate already at least 90 percent", func(t *testing.T) {
		setting.CacheHitBoostProbability = 1
		usage := &dto.Usage{
			PromptTokens: 10000,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 10000,
			},
		}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 11000, usage.PromptTokens)
		require.Equal(t, 10000, usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("does not invent cache when there is no cache read", func(t *testing.T) {
		setting.CacheHitBoostProbability = 1
		usage := &dto.Usage{PromptTokens: 10000, CompletionTokens: 50}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 0, usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("anthropic boost keeps total input unchanged when allowlisted", func(t *testing.T) {
		setting.CacheHitBoostProbability = 1
		usage := &dto.Usage{
			PromptTokens:     2000,
			CompletionTokens: 50,
			UsageSemantic:    "anthropic",
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         2000,
				CachedCreationTokens: 100,
			},
		}
		require.True(t, InflateUpstreamUsageForChannel(usage, 1))
		write := usage.PromptTokensDetails.CacheCreationTokensTotal()
		require.Equal(t, 110, write)
		total := usage.PromptTokens + usage.PromptTokensDetails.CachedTokens + write
		require.Equal(t, 4310, total)
		require.Equal(t, 3879, usage.PromptTokensDetails.CachedTokens)
		require.Equal(t, 321, usage.PromptTokens)
	})

	t.Run("does not boost when channel is not allowlisted", func(t *testing.T) {
		setting.CacheHitBoostProbability = 1
		usage := &dto.Usage{
			PromptTokens: 10000,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 5000,
			},
		}
		require.True(t, InflateUpstreamUsageForChannel(usage, 99))
		require.Equal(t, 11000, usage.PromptTokens)
		require.Equal(t, 5000, usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("copy and original agree on boost decision", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0.5
		raw := &dto.Usage{
			PromptTokens:     10000,
			CompletionTokens: 200,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 4000,
			},
		}
		cp := InflatedUsageCopyForChannel(raw, 1)
		InflateUpstreamUsageForChannel(raw, 1)
		require.Equal(t, raw.PromptTokensDetails.CachedTokens, cp.PromptTokensDetails.CachedTokens)
		require.Equal(t, raw.PromptTokens, cp.PromptTokens)
	})

	t.Run("uses configured markup ratio", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		origRatio := setting.TokenMarkupRatio
		setting.TokenMarkupRatio = 0.2
		t.Cleanup(func() { setting.TokenMarkupRatio = origRatio })
		usage := &dto.Usage{PromptTokens: 2000, CompletionTokens: 200}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 2400, usage.PromptTokens)
		require.Equal(t, 240, usage.CompletionTokens)
	})

	t.Run("uses configured input threshold", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		origIn := setting.TokenMarkupInputThreshold
		setting.TokenMarkupInputThreshold = 5000
		t.Cleanup(func() { setting.TokenMarkupInputThreshold = origIn })
		usage := &dto.Usage{PromptTokens: 2000, CompletionTokens: 50}
		require.False(t, InflateUpstreamUsage(usage))
		require.Equal(t, 2000, usage.PromptTokens)
	})
}
