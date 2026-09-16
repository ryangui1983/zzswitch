package service

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// InflateUpstreamUsage overwrites upstream usage: input tokens above 1000
// and output tokens above 100 are increased by 10% (integer division).
// Cache-write tokens are marked up 10% when input is marked up.
// If cache-read hit rate is then below 90%, it may be raised to 90% with
// the configured probability. Safe to call more than once.
func InflateUpstreamUsage(usage *dto.Usage) bool {
	if usage == nil || usage.TokensInflated {
		return false
	}
	usage.TokensInflated = true

	input := usage.PromptTokens
	if input == 0 {
		input = usage.InputTokens
	}
	output := usage.CompletionTokens
	if output == 0 {
		output = usage.OutputTokens
	}

	changed := false
	newInput, newOutput := input, output
	ratio := operation_setting.GetQuotaSetting().EffectiveTokenMarkupRatio()
	inThresh := operation_setting.GetQuotaSetting().TokenMarkupInputThreshold
	outThresh := operation_setting.GetQuotaSetting().TokenMarkupOutputThreshold
	if ratio > 0 {
		if input > inThresh {
			newInput = inflateByRatio(input, ratio)
		}
		if output > outThresh {
			newOutput = inflateByRatio(output, ratio)
		}
	}
	if newInput != input || newOutput != output {
		usage.PromptTokens = newInput
		usage.CompletionTokens = newOutput
		usage.InputTokens = newInput
		usage.OutputTokens = newOutput
		usage.TotalTokens = newInput + newOutput
		changed = true
	}

	if newInput != input {
		if inflateCacheWrite(usage) {
			changed = true
		}
		if maybeBoostCacheHit(usage) {
			changed = true
		}
	}
	return changed
}

func InflateUpstreamUsageForChannel(usage *dto.Usage, channelId int) bool {
	if usage != nil {
		usage.MarkupChannelId = channelId
	}
	return InflateUpstreamUsage(usage)
}

func inflateByRatio(n int, ratio float64) int {
	if n <= 0 || ratio <= 0 {
		return n
	}
	return n + int(float64(n)*ratio)
}

func inflateByTenPercent(n int) int {
	return inflateByRatio(n, operation_setting.GetQuotaSetting().EffectiveTokenMarkupRatio())
}

func inflateCacheWrite(usage *dto.Usage) bool {
	changed := false
	details := usage.PromptTokensDetails
	if details.CacheWriteTokens > 0 {
		details.CacheWriteTokens = inflateByTenPercent(details.CacheWriteTokens)
		changed = true
	}
	if details.CachedCreationTokens > 0 {
		details.CachedCreationTokens = inflateByTenPercent(details.CachedCreationTokens)
		changed = true
	}
	usage.PromptTokensDetails = details
	if usage.InputTokensDetails != nil {
		if usage.InputTokensDetails.CacheWriteTokens > 0 {
			usage.InputTokensDetails.CacheWriteTokens = inflateByTenPercent(usage.InputTokensDetails.CacheWriteTokens)
			changed = true
		}
		if usage.InputTokensDetails.CachedCreationTokens > 0 {
			usage.InputTokensDetails.CachedCreationTokens = inflateByTenPercent(usage.InputTokensDetails.CachedCreationTokens)
			changed = true
		}
	}
	if usage.ClaudeCacheCreation5mTokens > 0 {
		usage.ClaudeCacheCreation5mTokens = inflateByTenPercent(usage.ClaudeCacheCreation5mTokens)
		changed = true
	}
	if usage.ClaudeCacheCreation1hTokens > 0 {
		usage.ClaudeCacheCreation1hTokens = inflateByTenPercent(usage.ClaudeCacheCreation1hTokens)
		changed = true
	}
	return changed
}

func cacheHitDenominator(usage *dto.Usage) int {
	prompt := usage.PromptTokens
	if prompt == 0 {
		prompt = usage.InputTokens
	}
	if usage.UsageSemantic == "anthropic" {
		return prompt + usage.PromptTokensDetails.CachedTokens + usage.PromptTokensDetails.CacheCreationTokensTotal()
	}
	return prompt
}

func maybeBoostCacheHit(usage *dto.Usage) bool {
	setting := operation_setting.GetQuotaSetting()
	if !setting.AllowsCacheHitBoost(usage.MarkupChannelId) {
		return false
	}
	prob := setting.CacheHitBoostProbability
	if prob <= 0 {
		return false
	}
	targetRate := setting.EffectiveCacheHitBoostTarget()
	cacheRead := usage.PromptTokensDetails.CachedTokens
	if cacheRead <= 0 {
		return false
	}
	denom := cacheHitDenominator(usage)
	if denom <= 0 {
		return false
	}
	if float64(cacheRead)/float64(denom) >= targetRate {
		return false
	}
	if !shouldBoostCacheHit(usage, prob) {
		return false
	}

	write := usage.PromptTokensDetails.CacheCreationTokensTotal()
	if usage.UsageSemantic == "anthropic" {
		total := denom
		newRead := int(math.Floor(float64(total) * targetRate))
		newPrompt := total - newRead - write
		if newPrompt < 0 {
			newPrompt = 0
			newRead = total - write
			if newRead < 0 {
				newRead = 0
			}
		}
		usage.PromptTokens = newPrompt
		usage.InputTokens = newPrompt
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		setCacheRead(usage, newRead)
		return true
	}

	prompt := usage.PromptTokens
	if prompt == 0 {
		prompt = usage.InputTokens
	}
	target := int(math.Floor(float64(prompt) * targetRate))
	maxRead := prompt - write
	if maxRead < 0 {
		maxRead = 0
	}
	if target > maxRead {
		target = maxRead
	}
	if target <= cacheRead {
		return false
	}
	setCacheRead(usage, target)
	return true
}
func setCacheRead(usage *dto.Usage, n int) {
	usage.PromptTokensDetails.CachedTokens = n
	if usage.PromptCacheHitTokens > 0 {
		usage.PromptCacheHitTokens = n
	}
	if usage.InputTokensDetails != nil {
		usage.InputTokensDetails.CachedTokens = n
	}
}

// shouldBoostCacheHit is deterministic for the same usage numbers so a
// client-side InflatedUsageCopy and later settlement agree.
func shouldBoostCacheHit(usage *dto.Usage, prob float64) bool {
	if prob <= 0 {
		return false
	}
	if prob >= 1 {
		return true
	}
	var buf [32]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(usage.PromptTokens))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(usage.CompletionTokens))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(usage.PromptTokensDetails.CachedTokens))
	binary.LittleEndian.PutUint64(buf[24:32], uint64(usage.PromptTokensDetails.CacheCreationTokensTotal()))
	sum := sha256.Sum256(buf[:])
	unit := float64(binary.BigEndian.Uint32(sum[:4])) / float64(math.MaxUint32)
	return unit < prob
}

// InflatedUsageCopy returns a copy with 10% markup applied. The original is
// left unchanged so stream accumulators can stay raw while the client frame
// and later settlement both apply the same formula.
func InflatedUsageCopy(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	cp := *usage
	if usage.TokensInflated {
		return &cp
	}
	cp.TokensInflated = false
	InflateUpstreamUsage(&cp)
	return &cp
}

func InflatedUsageCopyForChannel(usage *dto.Usage, channelId int) *dto.Usage {
	if usage != nil {
		usage.MarkupChannelId = channelId
	}
	return InflatedUsageCopy(usage)
}

func overlayJSONInts(payload []byte, fields map[string]int) []byte {
	if len(payload) == 0 || len(fields) == 0 {
		return payload
	}
	for path, value := range fields {
		if !gjson.GetBytes(payload, path).Exists() {
			continue
		}
		patched, err := sjson.SetBytes(payload, path, value)
		if err != nil {
			continue
		}
		payload = patched
	}
	return payload
}

func chatUsageFields(usage *dto.Usage) map[string]int {
	if usage == nil {
		return nil
	}
	return map[string]int{
		"usage.prompt_tokens":                                usage.PromptTokens,
		"usage.completion_tokens":                            usage.CompletionTokens,
		"usage.total_tokens":                                 usage.TotalTokens,
		"usage.prompt_cache_hit_tokens":                      usage.PromptCacheHitTokens,
		"usage.prompt_tokens_details.cached_tokens":          usage.PromptTokensDetails.CachedTokens,
		"usage.prompt_tokens_details.cached_creation_tokens": usage.PromptTokensDetails.CachedCreationTokens,
		"usage.prompt_tokens_details.cache_write_tokens":     usage.PromptTokensDetails.CacheWriteTokens,
		"usage.input_tokens_details.cached_tokens":           cachedTokensOrZero(usage.InputTokensDetails),
		"usage.input_tokens_details.cache_write_tokens":      cacheWriteOrZero(usage.InputTokensDetails),
		"usage.input_tokens_details.cached_creation_tokens":  cacheCreationOrZero(usage.InputTokensDetails),
	}
}

func responsesUsageFields(usage *dto.Usage, prefix string) map[string]int {
	if usage == nil {
		return nil
	}
	if prefix == "" {
		prefix = "usage"
	}
	return map[string]int{
		prefix + ".input_tokens":                                usage.PromptTokens,
		prefix + ".output_tokens":                               usage.CompletionTokens,
		prefix + ".total_tokens":                                usage.TotalTokens,
		prefix + ".input_tokens_details.cached_tokens":          usage.PromptTokensDetails.CachedTokens,
		prefix + ".input_tokens_details.cache_write_tokens":     usage.PromptTokensDetails.CacheWriteTokens,
		prefix + ".input_tokens_details.cached_creation_tokens": usage.PromptTokensDetails.CachedCreationTokens,
		prefix + ".prompt_tokens_details.cached_tokens":         usage.PromptTokensDetails.CachedTokens,
		prefix + ".prompt_tokens_details.cache_write_tokens":    usage.PromptTokensDetails.CacheWriteTokens,
	}
}

func claudeUsageFields(usage *dto.Usage, prefix string) map[string]int {
	if usage == nil {
		return nil
	}
	if prefix == "" {
		prefix = "usage"
	}
	return map[string]int{
		prefix + ".input_tokens":                     usage.PromptTokens,
		prefix + ".output_tokens":                    usage.CompletionTokens,
		prefix + ".cache_read_input_tokens":          usage.PromptTokensDetails.CachedTokens,
		prefix + ".cache_creation_input_tokens":      usage.PromptTokensDetails.CacheCreationTokensTotal(),
		prefix + ".claude_cache_creation_5_m_tokens": usage.ClaudeCacheCreation5mTokens,
		prefix + ".claude_cache_creation_1_h_tokens": usage.ClaudeCacheCreation1hTokens,
	}
}

func cachedTokensOrZero(d *dto.InputTokenDetails) int {
	if d == nil {
		return 0
	}
	return d.CachedTokens
}

func cacheWriteOrZero(d *dto.InputTokenDetails) int {
	if d == nil {
		return 0
	}
	return d.CacheWriteTokens
}

func cacheCreationOrZero(d *dto.InputTokenDetails) int {
	if d == nil {
		return 0
	}
	return d.CachedCreationTokens
}

// OverlayChatUsageJSON writes inflated OpenAI chat usage into a JSON payload.
func OverlayChatUsageJSON(payload []byte, usage *dto.Usage) []byte {
	return overlayJSONInts(payload, chatUsageFields(usage))
}

// OverlayResponsesUsageJSON writes inflated Responses API usage.
// prefix is "usage" for a full response or "response.usage" for a stream event.
func OverlayResponsesUsageJSON(payload []byte, usage *dto.Usage, prefix string) []byte {
	return overlayJSONInts(payload, responsesUsageFields(usage, prefix))
}

// OverlayClaudeUsageJSON writes inflated Anthropic usage.
// prefix is "usage" for message_delta / non-stream, or "message.usage" for message_start.
func OverlayClaudeUsageJSON(payload []byte, usage *dto.Usage, prefix string) []byte {
	return overlayJSONInts(payload, claudeUsageFields(usage, prefix))
}

// ApplyInflatedCountsToClaudeUsage copies inflated totals onto a Claude usage object.
func ApplyInflatedCountsToClaudeUsage(dst *dto.ClaudeUsage, inflated *dto.Usage) {
	if dst == nil || inflated == nil {
		return
	}
	input := inflated.PromptTokens
	if input == 0 {
		input = inflated.InputTokens
	}
	output := inflated.CompletionTokens
	if output == 0 {
		output = inflated.OutputTokens
	}
	dst.InputTokens = input
	dst.OutputTokens = output
	dst.CacheReadInputTokens = inflated.PromptTokensDetails.CachedTokens
	dst.CacheCreationInputTokens = inflated.PromptTokensDetails.CacheCreationTokensTotal()
}

// InflateRealtimeUsage applies the same 10% markup to a realtime usage object.
func InflateRealtimeUsage(usage *dto.RealtimeUsage) bool {
	if usage == nil {
		return false
	}
	tmp := &dto.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if !InflateUpstreamUsage(tmp) {
		return false
	}
	usage.InputTokens = tmp.PromptTokens
	usage.OutputTokens = tmp.CompletionTokens
	usage.TotalTokens = tmp.TotalTokens
	return true
}
