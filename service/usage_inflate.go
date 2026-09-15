package service

import "github.com/QuantumNous/new-api/relaykit/dto"

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
