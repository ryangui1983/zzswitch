package dto

// Clone returns an independent copy of the request suitable for per-attempt
// mutation (model mapping, stream options, etc.) without affecting the original.
//
// Maintenance rule: when adding a new slice or map field to GeneralOpenAIRequest
// or OpenAIResponsesRequest, add a corresponding copy step here.
//
// json.RawMessage fields ([]byte) share the backing array — handlers replace
// them rather than mutate bytes in place, so sharing is safe.

func (r *GeneralOpenAIRequest) Clone() *GeneralOpenAIRequest {
	c := *r // value copy: strings, json.RawMessage headers, scalar fields

	// Messages: new slice so append in handler doesn't affect original
	if r.Messages != nil {
		c.Messages = make([]Message, len(r.Messages))
		copy(c.Messages, r.Messages)
	}

	// Tools: new slice
	if r.Tools != nil {
		c.Tools = make([]ToolCallRequest, len(r.Tools))
		copy(c.Tools, r.Tools)
	}

	// Pointer-to-primitive fields
	if r.Stream != nil              { v := *r.Stream; c.Stream = &v }
	if r.MaxTokens != nil           { v := *r.MaxTokens; c.MaxTokens = &v }
	if r.MaxCompletionTokens != nil { v := *r.MaxCompletionTokens; c.MaxCompletionTokens = &v }
	if r.Temperature != nil         { v := *r.Temperature; c.Temperature = &v }
	if r.TopP != nil                { v := *r.TopP; c.TopP = &v }
	if r.TopK != nil                { v := *r.TopK; c.TopK = &v }
	if r.N != nil                   { v := *r.N; c.N = &v }
	if r.FrequencyPenalty != nil    { v := *r.FrequencyPenalty; c.FrequencyPenalty = &v }
	if r.PresencePenalty != nil     { v := *r.PresencePenalty; c.PresencePenalty = &v }
	if r.Seed != nil                { v := *r.Seed; c.Seed = &v }
	if r.ParallelTooCalls != nil    { v := *r.ParallelTooCalls; c.ParallelTooCalls = &v }
	if r.LogProbs != nil            { v := *r.LogProbs; c.LogProbs = &v }
	if r.TopLogProbs != nil         { v := *r.TopLogProbs; c.TopLogProbs = &v }
	if r.Dimensions != nil          { v := *r.Dimensions; c.Dimensions = &v }
	if r.ReturnImages != nil        { v := *r.ReturnImages; c.ReturnImages = &v }
	if r.ReturnRelatedQuestions != nil { v := *r.ReturnRelatedQuestions; c.ReturnRelatedQuestions = &v }
	if r.MaxOutputTokens != nil        { v := *r.MaxOutputTokens; c.MaxOutputTokens = &v }

	// Pointer-to-struct fields (all flat, one-level dereference suffices)
	if r.StreamOptions != nil    { sc := *r.StreamOptions; c.StreamOptions = &sc }
	if r.ResponseFormat != nil   { rf := *r.ResponseFormat; c.ResponseFormat = &rf }
	if r.WebSearchOptions != nil { ws := *r.WebSearchOptions; c.WebSearchOptions = &ws }

	return &c
}

func (r *OpenAIResponsesRequest) Clone() *OpenAIResponsesRequest {
	c := *r // value copy: strings, json.RawMessage headers, scalar fields

	// Pointer-to-primitive fields
	if r.MaxOutputTokens != nil { v := *r.MaxOutputTokens; c.MaxOutputTokens = &v }
	if r.TopLogProbs != nil     { v := *r.TopLogProbs; c.TopLogProbs = &v }
	if r.Stream != nil          { v := *r.Stream; c.Stream = &v }
	if r.Temperature != nil     { v := *r.Temperature; c.Temperature = &v }
	if r.TopP != nil            { v := *r.TopP; c.TopP = &v }
	if r.MaxToolCalls != nil    { v := *r.MaxToolCalls; c.MaxToolCalls = &v }

	// Pointer-to-struct fields
	if r.Reasoning != nil      { rc := *r.Reasoning; c.Reasoning = &rc }
	if r.StreamOptions != nil  { sc := *r.StreamOptions; c.StreamOptions = &sc }

	return &c
}
