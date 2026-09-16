package service

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	inflateInputTokenThreshold  = 1000
	inflateOutputTokenThreshold = 100
)

// InflateUpstreamUsage overwrites upstream usage: input tokens above 1000
// and output tokens above 100 are increased by 10% (integer division).
// Safe to call more than once; returns whether any counter changed.
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

	newInput, newOutput := input, output
	if input > inflateInputTokenThreshold {
		newInput = input + input/10
	}
	if output > inflateOutputTokenThreshold {
		newOutput = output + output/10
	}
	if newInput == input && newOutput == output {
		return false
	}

	usage.PromptTokens = newInput
	usage.CompletionTokens = newOutput
	usage.InputTokens = newInput
	usage.OutputTokens = newOutput
	usage.TotalTokens = newInput + newOutput
	return true
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
		"usage.prompt_tokens":     usage.PromptTokens,
		"usage.completion_tokens": usage.CompletionTokens,
		"usage.total_tokens":      usage.TotalTokens,
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
		prefix + ".input_tokens":  usage.PromptTokens,
		prefix + ".output_tokens": usage.CompletionTokens,
		prefix + ".total_tokens":  usage.TotalTokens,
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
		prefix + ".input_tokens":  usage.PromptTokens,
		prefix + ".output_tokens": usage.CompletionTokens,
	}
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
