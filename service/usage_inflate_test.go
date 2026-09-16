package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
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

	details := OverlayChatUsageJSON([]byte(`{"usage":{"prompt_tokens":2000,"completion_tokens":2795,"total_tokens":4795,"completion_tokens_details":{"reasoning_tokens":1210,"text_tokens":1585}}}`), &dto.Usage{
		PromptTokens:     2200,
		CompletionTokens: 3074,
		TotalTokens:      5274,
		CompletionTokenDetails: dto.OutputTokenDetails{
			TextTokens:      1743,
			ReasoningTokens: 1331,
		},
	})
	require.Contains(t, string(details), `"completion_tokens":3074`)
	require.Contains(t, string(details), `"reasoning_tokens":1331`)
	require.Contains(t, string(details), `"text_tokens":1743`)
	claudeCache := OverlayClaudeUsageJSON([]byte(`{"usage":{"input_tokens":2000,"output_tokens":200,"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":80,"ephemeral_1h_input_tokens":20}}}`), &dto.Usage{PromptTokens: 2200, CompletionTokens: 220, PromptTokensDetails: dto.InputTokenDetails{CachedCreationTokens: 110}, ClaudeCacheCreation5mTokens: 88, ClaudeCacheCreation1hTokens: 22}, "usage")
	require.Contains(t, string(claudeCache), `"input_tokens":2200`)
	require.Contains(t, string(claudeCache), `"cache_creation_input_tokens":110`)
	require.Contains(t, string(claudeCache), `"ephemeral_5m_input_tokens":88`)
	require.Contains(t, string(claudeCache), `"ephemeral_1h_input_tokens":22`)
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
		require.Equal(t, 220, usage.PromptTokensDetails.CachedTokens)
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
		denom := usage.PromptTokens
		require.Greater(t, denom, 0)
		requireHitAroundTarget(t, usage)
		require.GreaterOrEqual(t, usageBillingWeight(usage), usageBillingWeight(&dto.Usage{
			PromptTokens:        10000,
			CompletionTokens:    50,
			PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 5000},
		}))
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
		require.Equal(t, 5500, usage.PromptTokensDetails.CachedTokens)
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
		require.Equal(t, 11000, usage.PromptTokensDetails.CachedTokens)
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
		total := usage.PromptTokens + usage.PromptTokensDetails.CachedTokens + write
		require.Greater(t, total, 0)
		requireHitAroundTarget(t, usage)
		require.GreaterOrEqual(t, usageBillingWeight(usage), usageBillingWeight(&dto.Usage{
			PromptTokens:     2000,
			CompletionTokens: 50,
			UsageSemantic:    "anthropic",
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         2000,
				CachedCreationTokens: 100,
			},
		}))
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
		require.Equal(t, 5500, usage.PromptTokensDetails.CachedTokens)
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

	t.Run("inflates completion details so they sum to completion tokens", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		usage := &dto.Usage{
			PromptTokens:     2000,
			CompletionTokens: 2795,
			CompletionTokenDetails: dto.OutputTokenDetails{
				TextTokens:      1585,
				ReasoningTokens: 1210,
			},
		}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 3074, usage.CompletionTokens)
		require.Equal(t, 1331, usage.CompletionTokenDetails.ReasoningTokens)
		require.Equal(t, 1743, usage.CompletionTokenDetails.TextTokens)
		require.Equal(t, usage.CompletionTokens, usage.CompletionTokenDetails.TextTokens+usage.CompletionTokenDetails.ReasoningTokens)
	})

	t.Run("does not invent completion details when they are empty", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		usage := &dto.Usage{PromptTokens: 2000, CompletionTokens: 200}
		require.True(t, InflateUpstreamUsage(usage))
		require.Equal(t, 0, usage.CompletionTokenDetails.TextTokens)
		require.Equal(t, 0, usage.CompletionTokenDetails.ReasoningTokens)
	})

	t.Run("copy does not mutate original completion details", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		raw := &dto.Usage{
			PromptTokens:     2000,
			CompletionTokens: 2795,
			CompletionTokenDetails: dto.OutputTokenDetails{
				TextTokens:      1585,
				ReasoningTokens: 1210,
			},
		}
		cp := InflatedUsageCopy(raw)
		require.Equal(t, 1585, raw.CompletionTokenDetails.TextTokens)
		require.Equal(t, 1210, raw.CompletionTokenDetails.ReasoningTokens)
		require.Equal(t, 1743, cp.CompletionTokenDetails.TextTokens)
		require.Equal(t, 1331, cp.CompletionTokenDetails.ReasoningTokens)
	})
}

func TestInflateRealtimeUsageDetails(t *testing.T) {
	usage := &dto.RealtimeUsage{
		InputTokens:  2000,
		OutputTokens: 2795,
		TotalTokens:  4795,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: 2000,
		},
		OutputTokenDetails: dto.OutputTokenDetails{
			TextTokens:      1585,
			ReasoningTokens: 1210,
		},
	}
	require.True(t, InflateRealtimeUsage(usage))
	require.Equal(t, 2200, usage.InputTokens)
	require.Equal(t, 3074, usage.OutputTokens)
	require.Equal(t, 5274, usage.TotalTokens)
	require.Equal(t, 2200, usage.InputTokenDetails.TextTokens)
	require.Equal(t, 1331, usage.OutputTokenDetails.ReasoningTokens)
	require.Equal(t, 1743, usage.OutputTokenDetails.TextTokens)
}

func TestCacheHitBoostDoesNotReduceRevenue(t *testing.T) {
	setting := operation_setting.GetQuotaSetting()
	origProb := setting.CacheHitBoostProbability
	origIds := setting.CacheHitBoostChannelIds
	t.Cleanup(func() {
		setting.CacheHitBoostProbability = origProb
		setting.CacheHitBoostChannelIds = origIds
	})
	setting.CacheHitBoostChannelIds = "1"
	setting.CacheHitBoostProbability = 1

	t.Run("compensates with output so weighted charge does not drop", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     10000,
			CompletionTokens: 50,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 5000,
			},
			MarkupHasWeights:      true,
			MarkupCacheRatio:      0.1,
			MarkupCompletionRatio: 4,
		}
		before := &dto.Usage{
			PromptTokens:          10000,
			CompletionTokens:      50,
			PromptTokensDetails:   dto.InputTokenDetails{CachedTokens: 5000},
			MarkupHasWeights:      true,
			MarkupCacheRatio:      0.1,
			MarkupCompletionRatio: 4,
		}
		require.True(t, InflateUpstreamUsageForChannel(usage, 1))
		requireHitAroundTarget(t, usage)
		require.GreaterOrEqual(t, usageBillingWeight(usage), usageBillingWeight(before))
	})

	t.Run("still marks up cache read when boost is skipped", func(t *testing.T) {
		setting.CacheHitBoostProbability = 0
		t.Cleanup(func() { setting.CacheHitBoostProbability = 1 })
		usage := &dto.Usage{
			PromptTokens:     10000,
			CompletionTokens: 50,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 5000,
			},
		}
		require.True(t, InflateUpstreamUsageForChannel(usage, 1))
		require.Equal(t, 11000, usage.PromptTokens)
		require.Equal(t, 5500, usage.PromptTokensDetails.CachedTokens)
		require.Equal(t, 50, usage.CompletionTokens)
	})

	t.Run("anthropic reallocation still does not reduce weighted charge", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     2000,
			CompletionTokens: 50,
			UsageSemantic:    "anthropic",
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         2000,
				CachedCreationTokens: 100,
			},
			MarkupHasWeights:      true,
			MarkupCacheRatio:      0.1,
			MarkupCompletionRatio: 1,
		}
		original := usageBillingWeight(usage)
		require.True(t, InflateUpstreamUsageForChannel(usage, 1))
		require.GreaterOrEqual(t, usageBillingWeight(usage), original)
		requireHitAroundTarget(t, usage)
	})
	t.Run("high original hit rate keeps markup without extra scale", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     10000,
			CompletionTokens: 3000,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 8000,
			},
			MarkupHasWeights:      true,
			MarkupCacheRatio:      0.1,
			MarkupCompletionRatio: 4,
		}
		original := usageBillingWeight(usage)
		require.True(t, InflateUpstreamUsageForChannel(usage, 1))
		require.Equal(t, 11000, usage.PromptTokens)
		requireHitAroundTarget(t, usage)
		require.GreaterOrEqual(t, usageBillingWeight(usage), original)
	})

}

func TestCacheHitBoostUsesTieredExprPrices(t *testing.T) {
	setting := operation_setting.GetQuotaSetting()
	origProb := setting.CacheHitBoostProbability
	origIds := setting.CacheHitBoostChannelIds
	t.Cleanup(func() {
		setting.CacheHitBoostProbability = origProb
		setting.CacheHitBoostChannelIds = origIds
	})
	setting.CacheHitBoostChannelIds = "1"
	setting.CacheHitBoostProbability = 1

	expr := "p*2 + c*6 + cr*0.5"
	usage := &dto.Usage{
		PromptTokens:     5751,
		CompletionTokens: 3109,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 2048,
		},
		MarkupExprString: expr,
		MarkupExprHash:   billingexpr.ExprHashString(expr),
	}
	original := usageBillingWeight(usage)
	require.True(t, InflateUpstreamUsageForChannel(usage, 1))
	requireHitAroundTarget(t, usage)
	require.GreaterOrEqual(t, usageBillingWeight(usage), original)
	require.Less(t, usage.PromptTokens, 8000)
}

func requireHitAroundTarget(t *testing.T, usage *dto.Usage) {
	t.Helper()
	denom := cacheHitDenominator(usage)
	require.Greater(t, denom, 0)
	hit := float64(usage.PromptTokensDetails.CachedTokens) / float64(denom)
	require.GreaterOrEqual(t, hit, 0.84)
	require.LessOrEqual(t, hit, 0.95)
}

func TestJitteredHitRateStaysInBand(t *testing.T) {
	seen := map[string]bool{}
	for i := 1; i <= 40; i++ {
		u := &dto.Usage{
			PromptTokens:        10000 + i*17,
			CompletionTokens:    200 + i,
			PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 3000 + i},
		}
		rate := jitteredHitRate(u, 0.9)
		require.GreaterOrEqual(t, rate, 0.85)
		require.LessOrEqual(t, rate, 0.95)
		seen[fmt.Sprintf("%.6f", rate)] = true
		require.Equal(t, rate, jitteredHitRate(u, 0.9))
	}
	require.Greater(t, len(seen), 5)
}
