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
