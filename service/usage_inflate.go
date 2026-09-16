package service

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
	originalWeight := usageBillingWeight(usage)

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
		if inflateInputDetails(usage, ratio) {
			changed = true
		}
		if inflateCacheWrite(usage) {
			changed = true
		}
	}
	if newOutput != output {
		if inflateOutputDetails(usage, ratio) {
			changed = true
		}
	}
	if newInput != input {
		if maybeBoostCacheHit(usage, originalWeight) {
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

func AttachMarkupWeightsFromInfo(usage *dto.Usage, info *relaycommon.RelayInfo) {
	if usage == nil || info == nil {
		return
	}
	usage.MarkupCacheRatio = info.PriceData.CacheRatio
	usage.MarkupCompletionRatio = info.PriceData.CompletionRatio
	usage.MarkupCacheCreationRatio = info.PriceData.CacheCreationRatio
	usage.MarkupHasWeights = true
	if snap := info.TieredBillingSnapshot; snap != nil {
		usage.MarkupExprString = snap.ExprString
		usage.MarkupExprHash = snap.ExprHash
	}
}

func InflateUpstreamUsageFromInfo(usage *dto.Usage, info *relaycommon.RelayInfo) bool {
	cid := 0
	if info != nil {
		cid = info.GetChannelID()
		AttachMarkupWeightsFromInfo(usage, info)
	}
	return InflateUpstreamUsageForChannel(usage, cid)
}

func InflatedUsageCopyFromInfo(usage *dto.Usage, info *relaycommon.RelayInfo) *dto.Usage {
	cid := 0
	if info != nil {
		cid = info.GetChannelID()
		AttachMarkupWeightsFromInfo(usage, info)
	}
	return InflatedUsageCopyForChannel(usage, cid)
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

func inflatePositive(n int, ratio float64) int {
	if n <= 0 {
		return n
	}
	return inflateByRatio(n, ratio)
}

func inflateInputDetails(usage *dto.Usage, ratio float64) bool {
	changed := inflateInputTokenDetails(&usage.PromptTokensDetails, ratio)
	if usage.InputTokensDetails != nil {
		if inflateInputTokenDetails(usage.InputTokensDetails, ratio) {
			changed = true
		}
	}
	return changed
}

func inflateInputTokenDetails(d *dto.InputTokenDetails, ratio float64) bool {
	if d == nil || ratio <= 0 {
		return false
	}
	changed := false
	if n := inflatePositive(d.TextTokens, ratio); n != d.TextTokens {
		d.TextTokens = n
		changed = true
	}
	if n := inflatePositive(d.AudioTokens, ratio); n != d.AudioTokens {
		d.AudioTokens = n
		changed = true
	}
	if n := inflatePositive(d.ImageTokens, ratio); n != d.ImageTokens {
		d.ImageTokens = n
		changed = true
	}
	return changed
}

func inflateOutputDetails(usage *dto.Usage, ratio float64) bool {
	changed := inflateOutputTokenDetails(&usage.CompletionTokenDetails, usage.CompletionTokens, ratio)
	if usage.OutputTokensDetails != nil {
		if inflateOutputTokenDetails(usage.OutputTokensDetails, usage.CompletionTokens, ratio) {
			changed = true
		}
	}
	return changed
}

func inflateOutputTokenDetails(d *dto.OutputTokenDetails, total int, ratio float64) bool {
	if d == nil || ratio <= 0 {
		return false
	}
	if d.TextTokens == 0 && d.ReasoningTokens == 0 && d.AudioTokens == 0 && d.ImageTokens == 0 {
		return false
	}
	d.ReasoningTokens = inflatePositive(d.ReasoningTokens, ratio)
	d.AudioTokens = inflatePositive(d.AudioTokens, ratio)
	d.ImageTokens = inflatePositive(d.ImageTokens, ratio)
	text := total - d.ReasoningTokens - d.AudioTokens - d.ImageTokens
	if text < 0 {
		text = 0
	}
	d.TextTokens = text
	return true
}

func inflateCacheWrite(usage *dto.Usage) bool {
	changed := false
	details := usage.PromptTokensDetails
	if details.CachedTokens > 0 {
		details.CachedTokens = inflateByTenPercent(details.CachedTokens)
		changed = true
	}
	if details.CacheWriteTokens > 0 {
		details.CacheWriteTokens = inflateByTenPercent(details.CacheWriteTokens)
		changed = true
	}
	if details.CachedCreationTokens > 0 {
		details.CachedCreationTokens = inflateByTenPercent(details.CachedCreationTokens)
		changed = true
	}
	usage.PromptTokensDetails = details
	if usage.PromptCacheHitTokens > 0 {
		usage.PromptCacheHitTokens = inflateByTenPercent(usage.PromptCacheHitTokens)
		changed = true
	}
	if usage.InputTokensDetails != nil {
		if usage.InputTokensDetails.CachedTokens > 0 {
			usage.InputTokensDetails.CachedTokens = inflateByTenPercent(usage.InputTokensDetails.CachedTokens)
			changed = true
		}
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

func maybeBoostCacheHit(usage *dto.Usage, originalWeight float64) bool {
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

	rate := jitteredHitRate(usage, targetRate)
	r := boostScaleFactor(usage, rate, originalWeight)
	if r > 1 {
		scaleUsageCounts(usage, r)
	}
	applyTargetCacheHit(usage, rate)
	compensateBoostRevenue(usage, originalWeight)
	return true
}

const cacheHitJitter = 0.05

func usageHashUnit(usage *dto.Usage, byteOff int) float64 {
	var buf [32]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(usage.PromptTokens))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(usage.CompletionTokens))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(usage.PromptTokensDetails.CachedTokens))
	binary.LittleEndian.PutUint64(buf[24:32], uint64(usage.PromptTokensDetails.CacheCreationTokensTotal()))
	sum := sha256.Sum256(buf[:])
	if byteOff < 0 || byteOff > 28 {
		byteOff = 0
	}
	return float64(binary.BigEndian.Uint32(sum[byteOff:byteOff+4])) / float64(math.MaxUint32)
}

func jitteredHitRate(usage *dto.Usage, target float64) float64 {
	lo := target - cacheHitJitter
	hi := target + cacheHitJitter
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	rate := lo + usageHashUnit(usage, 4)*(hi-lo)
	denom := cacheHitDenominator(usage)
	if denom > 0 {
		current := float64(usage.PromptTokensDetails.CachedTokens) / float64(denom)
		if rate < current {
			rate = current
		}
	}
	if rate > 1 {
		rate = 1
	}
	return rate
}

func scaleInt(n int, r float64) int {
	if n <= 0 || r <= 1 {
		return n
	}
	return int(math.Ceil(float64(n) * r))
}

func boostScaleFactor(usage *dto.Usage, targetRate, originalWeight float64) float64 {
	b := billingWeightAtTargetHit(usage, targetRate)
	if b <= 0 {
		return 1
	}
	r := originalWeight / b
	if r < 1 {
		return 1
	}
	return r
}

func billingWeightAtTargetHit(usage *dto.Usage, targetRate float64) float64 {
	if usage == nil {
		return 0
	}
	cp := *usage
	cp.PromptTokensDetails = usage.PromptTokensDetails.Clone()
	if usage.InputTokensDetails != nil {
		cloned := usage.InputTokensDetails.Clone()
		cp.InputTokensDetails = &cloned
	}
	applyTargetCacheHit(&cp, targetRate)
	return usageBillingWeight(&cp)
}

func scaleUsageCounts(usage *dto.Usage, r float64) {
	if usage == nil || r <= 1 {
		return
	}
	prompt, completion := usagePromptCompletion(usage)
	usage.PromptTokens = scaleInt(prompt, r)
	usage.InputTokens = usage.PromptTokens
	usage.CompletionTokens = scaleInt(completion, r)
	usage.OutputTokens = usage.CompletionTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	scaleCacheWrite(usage, r)
	scaleInputTokenDetails(&usage.PromptTokensDetails, r)
	if usage.InputTokensDetails != nil {
		scaleInputTokenDetails(usage.InputTokensDetails, r)
	}
	scaleOutputTokenDetails(&usage.CompletionTokenDetails, usage.CompletionTokens, r)
	if usage.OutputTokensDetails != nil {
		scaleOutputTokenDetails(usage.OutputTokensDetails, usage.CompletionTokens, r)
	}
}

func scaleCacheWrite(usage *dto.Usage, r float64) {
	details := usage.PromptTokensDetails
	details.CachedTokens = scaleInt(details.CachedTokens, r)
	details.CacheWriteTokens = scaleInt(details.CacheWriteTokens, r)
	details.CachedCreationTokens = scaleInt(details.CachedCreationTokens, r)
	usage.PromptTokensDetails = details
	if usage.InputTokensDetails != nil {
		usage.InputTokensDetails.CachedTokens = scaleInt(usage.InputTokensDetails.CachedTokens, r)
		usage.InputTokensDetails.CacheWriteTokens = scaleInt(usage.InputTokensDetails.CacheWriteTokens, r)
		usage.InputTokensDetails.CachedCreationTokens = scaleInt(usage.InputTokensDetails.CachedCreationTokens, r)
	}
	usage.ClaudeCacheCreation5mTokens = scaleInt(usage.ClaudeCacheCreation5mTokens, r)
	usage.ClaudeCacheCreation1hTokens = scaleInt(usage.ClaudeCacheCreation1hTokens, r)
}

func scaleInputTokenDetails(d *dto.InputTokenDetails, r float64) {
	if d == nil {
		return
	}
	d.TextTokens = scaleInt(d.TextTokens, r)
	d.AudioTokens = scaleInt(d.AudioTokens, r)
	d.ImageTokens = scaleInt(d.ImageTokens, r)
}

func scaleOutputTokenDetails(d *dto.OutputTokenDetails, total int, r float64) {
	if d == nil {
		return
	}
	if d.TextTokens == 0 && d.ReasoningTokens == 0 && d.AudioTokens == 0 && d.ImageTokens == 0 {
		return
	}
	d.ReasoningTokens = scaleInt(d.ReasoningTokens, r)
	d.AudioTokens = scaleInt(d.AudioTokens, r)
	d.ImageTokens = scaleInt(d.ImageTokens, r)
	text := total - d.ReasoningTokens - d.AudioTokens - d.ImageTokens
	if text < 0 {
		text = 0
	}
	d.TextTokens = text
}

func applyTargetCacheHit(usage *dto.Usage, targetRate float64) {
	write := usage.PromptTokensDetails.CacheCreationTokensTotal()
	if usage.UsageSemantic == "anthropic" {
		total := cacheHitDenominator(usage)
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
		return
	}
	prompt, _ := usagePromptCompletion(usage)
	target := int(math.Floor(float64(prompt) * targetRate))
	maxRead := prompt - write
	if maxRead < 0 {
		maxRead = 0
	}
	if target > maxRead {
		target = maxRead
	}
	if target > usage.PromptTokensDetails.CachedTokens {
		setCacheRead(usage, target)
	}
}

func markupWeights(usage *dto.Usage) (cacheRatio, completionRatio, cacheCreationRatio float64) {
	if usage != nil && usage.MarkupHasWeights {
		kc := usage.MarkupCompletionRatio
		if kc <= 0 {
			kc = 1
		}
		return usage.MarkupCacheRatio, kc, usage.MarkupCacheCreationRatio
	}
	// Conservative: treat cache reads as free and completion as 1x input so
	// missing PriceData cannot under-charge relative to any real cache_ratio in [0,1]
	// when completion_ratio >= 1.
	return 0, 1, 1.25
}

func usagePromptCompletion(usage *dto.Usage) (prompt, completion int) {
	prompt = usage.PromptTokens
	if prompt == 0 {
		prompt = usage.InputTokens
	}
	completion = usage.CompletionTokens
	if completion == 0 {
		completion = usage.OutputTokens
	}
	return prompt, completion
}

func usageBillingWeight(usage *dto.Usage) float64 {
	if usage == nil {
		return 0
	}
	if usage.MarkupExprString != "" {
		used := billingexpr.UsedVarsByHash(usage.MarkupExprString, usage.MarkupExprHash)
		params := BuildTieredTokenParams(usage, usage.UsageSemantic == "anthropic", used)
		if cost, _, err := billingexpr.RunExprByHash(usage.MarkupExprString, usage.MarkupExprHash, params); err == nil {
			return cost
		}
	}
	prompt, completion := usagePromptCompletion(usage)
	cacheRead := usage.PromptTokensDetails.CachedTokens
	cacheWrite := usage.PromptTokensDetails.CacheCreationTokensTotal()
	kr, kc, kw := markupWeights(usage)
	if usage.UsageSemantic == "anthropic" {
		return float64(prompt) + kr*float64(cacheRead) + kw*float64(cacheWrite) + kc*float64(completion)
	}
	base := prompt - cacheRead - cacheWrite
	if base < 0 {
		base = 0
	}
	return float64(base) + kr*float64(cacheRead) + kw*float64(cacheWrite) + kc*float64(completion)
}

func addCompletionTokens(usage *dto.Usage, extra int) {
	if usage == nil || extra <= 0 {
		return
	}
	usage.CompletionTokens += extra
	usage.OutputTokens += extra
	prompt, completion := usagePromptCompletion(usage)
	usage.PromptTokens = prompt
	usage.InputTokens = prompt
	usage.CompletionTokens = completion
	usage.OutputTokens = completion
	usage.TotalTokens = prompt + completion
	usage.CompletionTokenDetails.TextTokens += extra
	if usage.OutputTokensDetails != nil {
		usage.OutputTokensDetails.TextTokens += extra
	}
}

func compensateBoostRevenue(usage *dto.Usage, weightBefore float64) {
	after := usageBillingWeight(usage)
	if after+1e-9 >= weightBefore {
		return
	}
	need := weightBefore - after
	_, kc, _ := markupWeights(usage)
	extra := 0
	if kc > 0 {
		extra = int(math.Ceil(need / kc))
	}
	if extra < 1 {
		extra = 1
	}
	addCompletionTokens(usage, extra)
	if usageBillingWeight(usage)+1e-9 < weightBefore {
		addCompletionTokens(usage, 1)
	}
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
	return usageHashUnit(usage, 0) < prob
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
	cp.PromptTokensDetails = usage.PromptTokensDetails.Clone()
	if usage.InputTokensDetails != nil {
		cloned := usage.InputTokensDetails.Clone()
		cp.InputTokensDetails = &cloned
	}
	if usage.OutputTokensDetails != nil {
		out := *usage.OutputTokensDetails
		cp.OutputTokensDetails = &out
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
		"usage.prompt_tokens_details.text_tokens":            usage.PromptTokensDetails.TextTokens,
		"usage.prompt_tokens_details.audio_tokens":           usage.PromptTokensDetails.AudioTokens,
		"usage.prompt_tokens_details.image_tokens":           usage.PromptTokensDetails.ImageTokens,
		"usage.completion_tokens_details.text_tokens":        usage.CompletionTokenDetails.TextTokens,
		"usage.completion_tokens_details.reasoning_tokens":   usage.CompletionTokenDetails.ReasoningTokens,
		"usage.completion_tokens_details.audio_tokens":       usage.CompletionTokenDetails.AudioTokens,
		"usage.completion_tokens_details.image_tokens":       usage.CompletionTokenDetails.ImageTokens,
		"usage.input_tokens_details.cached_tokens":           cachedTokensOrZero(usage.InputTokensDetails),
		"usage.input_tokens_details.cache_write_tokens":      cacheWriteOrZero(usage.InputTokensDetails),
		"usage.input_tokens_details.cached_creation_tokens":  cacheCreationOrZero(usage.InputTokensDetails),
		"usage.input_tokens_details.text_tokens":             inputDetailOrZero(usage.InputTokensDetails, func(d *dto.InputTokenDetails) int { return d.TextTokens }),
		"usage.input_tokens_details.audio_tokens":            inputDetailOrZero(usage.InputTokensDetails, func(d *dto.InputTokenDetails) int { return d.AudioTokens }),
		"usage.input_tokens_details.image_tokens":            inputDetailOrZero(usage.InputTokensDetails, func(d *dto.InputTokenDetails) int { return d.ImageTokens }),
		"usage.output_tokens_details.text_tokens":            outputDetailOrZero(usage.OutputTokensDetails, func(d *dto.OutputTokenDetails) int { return d.TextTokens }),
		"usage.output_tokens_details.reasoning_tokens":       outputDetailOrZero(usage.OutputTokensDetails, func(d *dto.OutputTokenDetails) int { return d.ReasoningTokens }),
		"usage.output_tokens_details.audio_tokens":           outputDetailOrZero(usage.OutputTokensDetails, func(d *dto.OutputTokenDetails) int { return d.AudioTokens }),
		"usage.output_tokens_details.image_tokens":           outputDetailOrZero(usage.OutputTokensDetails, func(d *dto.OutputTokenDetails) int { return d.ImageTokens }),
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
		prefix + ".input_tokens_details.text_tokens":            usage.PromptTokensDetails.TextTokens,
		prefix + ".input_tokens_details.audio_tokens":           usage.PromptTokensDetails.AudioTokens,
		prefix + ".input_tokens_details.image_tokens":           usage.PromptTokensDetails.ImageTokens,
		prefix + ".output_tokens_details.text_tokens":           usage.CompletionTokenDetails.TextTokens,
		prefix + ".output_tokens_details.reasoning_tokens":      usage.CompletionTokenDetails.ReasoningTokens,
		prefix + ".output_tokens_details.audio_tokens":          usage.CompletionTokenDetails.AudioTokens,
		prefix + ".output_tokens_details.image_tokens":          usage.CompletionTokenDetails.ImageTokens,
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
		prefix + ".input_tokens":                             usage.PromptTokens,
		prefix + ".output_tokens":                            usage.CompletionTokens,
		prefix + ".cache_read_input_tokens":                  usage.PromptTokensDetails.CachedTokens,
		prefix + ".cache_creation_input_tokens":              usage.PromptTokensDetails.CacheCreationTokensTotal(),
		prefix + ".cache_creation.ephemeral_5m_input_tokens": usage.ClaudeCacheCreation5mTokens,
		prefix + ".cache_creation.ephemeral_1h_input_tokens": usage.ClaudeCacheCreation1hTokens,
		prefix + ".claude_cache_creation_5_m_tokens":         usage.ClaudeCacheCreation5mTokens,
		prefix + ".claude_cache_creation_1_h_tokens":         usage.ClaudeCacheCreation1hTokens,
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

func inputDetailOrZero(d *dto.InputTokenDetails, get func(*dto.InputTokenDetails) int) int {
	if d == nil {
		return 0
	}
	return get(d)
}

func outputDetailOrZero(d *dto.OutputTokenDetails, get func(*dto.OutputTokenDetails) int) int {
	if d == nil {
		return 0
	}
	return get(d)
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
	dst.ClaudeCacheCreation5mTokens = inflated.ClaudeCacheCreation5mTokens
	dst.ClaudeCacheCreation1hTokens = inflated.ClaudeCacheCreation1hTokens
	if inflated.ClaudeCacheCreation5mTokens > 0 || inflated.ClaudeCacheCreation1hTokens > 0 {
		if dst.CacheCreation == nil {
			dst.CacheCreation = &dto.ClaudeCacheCreationUsage{}
		}
		dst.CacheCreation.Ephemeral5mInputTokens = inflated.ClaudeCacheCreation5mTokens
		dst.CacheCreation.Ephemeral1hInputTokens = inflated.ClaudeCacheCreation1hTokens
	}
}

// InflateRealtimeUsage applies the same 10% markup to a realtime usage object.
func InflateRealtimeUsage(usage *dto.RealtimeUsage) bool {
	if usage == nil {
		return false
	}
	tmp := &dto.Usage{
		PromptTokens:           usage.InputTokens,
		CompletionTokens:       usage.OutputTokens,
		InputTokens:            usage.InputTokens,
		OutputTokens:           usage.OutputTokens,
		TotalTokens:            usage.TotalTokens,
		PromptTokensDetails:    usage.InputTokenDetails,
		CompletionTokenDetails: usage.OutputTokenDetails,
	}
	if !InflateUpstreamUsage(tmp) {
		return false
	}
	usage.InputTokens = tmp.PromptTokens
	usage.OutputTokens = tmp.CompletionTokens
	usage.TotalTokens = tmp.TotalTokens
	usage.InputTokenDetails = tmp.PromptTokensDetails
	usage.OutputTokenDetails = tmp.CompletionTokenDetails
	return true
}
