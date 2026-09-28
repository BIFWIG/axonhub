package responses

import (
	"encoding/json"

	"github.com/looplj/axonhub/llm"
)

type Usage struct {
	InputTokens       int64 `json:"input_tokens"`
	InputTokenDetails struct {
		// CacheWriteTokens is the number of input tokens written to the prompt cache.
		CacheWriteTokens int64 `json:"cache_write_tokens"`
		// CachedTokens is the number of input tokens retrieved from the prompt cache.
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokens       int64 `json:"output_tokens"`
	OutputTokenDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	TotalTokens int64    `json:"total_tokens"`
	Cost        *float64 `json:"cost,omitempty"`
}

// UnmarshalJSON tolerates provider-specific usage.cost values.
//
// Some providers (ZenMux among them) encode cost as a decimal string, e.g.
// "0.000152", and add a non-standard usage.cost_details object. AxonHub does
// not trust or propagate upstream cost values, so a non-numeric cost must not
// break usage parsing: a strict *float64 decode rejected the whole event that
// carried the usage, which either lost the token accounting or aborted the
// stream outright.
//
// Mirrors (*openai.Usage).UnmarshalJSON in the chat_completions transformer:
// keep numeric values for round-trips, ignore any other valid JSON shape
// without losing the token counts.
func (u *Usage) UnmarshalJSON(data []byte) error {
	type usageAlias Usage

	decoded := struct {
		*usageAlias

		Cost json.RawMessage `json:"cost"`
	}{
		usageAlias: (*usageAlias)(u),
	}

	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	// A present null or non-number cost is an upstream extension that should not
	// affect usage parsing. u.Cost stays nil -> AxonHub's own price table wins.
	u.Cost = nil

	if len(decoded.Cost) > 0 {
		// Decoding into *float64 keeps an explicit null nil and rejects every
		// non-number shape; decoding null into a plain float64 silently succeeds
		// and would leave a pointer to zero behind.
		var cost *float64
		if err := json.Unmarshal(decoded.Cost, &cost); err == nil {
			u.Cost = cost
		}
	}

	return nil
}

func (u *Usage) ToUsage() *llm.Usage {
	return &llm.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
		PromptTokensDetails: &llm.PromptTokensDetails{
			CachedTokens:      u.InputTokenDetails.CachedTokens,
			WriteCachedTokens: u.InputTokenDetails.CacheWriteTokens,
		},
		CompletionTokensDetails: &llm.CompletionTokensDetails{
			ReasoningTokens: u.OutputTokenDetails.ReasoningTokens,
		},
	}
}

// ConvertLLMUsageToResponsesUsage converts llm.Usage to Responses API Usage.
func ConvertLLMUsageToResponsesUsage(usage *llm.Usage) *Usage {
	if usage == nil {
		return nil
	}

	result := &Usage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
		Cost:         usage.Cost,
	}

	if usage.PromptTokensDetails != nil {
		result.InputTokenDetails.CachedTokens = usage.PromptTokensDetails.CachedTokens
		result.InputTokenDetails.CacheWriteTokens = usage.PromptTokensDetails.WriteCachedTokens
	}

	if usage.CompletionTokensDetails != nil {
		result.OutputTokenDetails.ReasoningTokens = usage.CompletionTokensDetails.ReasoningTokens
	}

	return result
}
