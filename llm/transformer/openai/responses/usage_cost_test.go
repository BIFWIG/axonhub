package responses

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// Some Responses-compatible providers encode the request cost as a decimal
// string. ZenMux returns, in the final `response.completed` event:
//
//	"usage":{"input_tokens":8,"input_tokens_details":{"cache_write_tokens":0,"cached_tokens":0},
//	         "output_tokens":6,"total_tokens":14,
//	         "cost":"0.000152",
//	         "cost_details":{"prompt":"0.000032","completion":"0.00012","input_cache_read":"0"}}
//
// A strict *float64 decode rejected the whole event, so AxonHub dropped the
// only event carrying usage and recorded no tokens at all:
//
//	failed to unmarshal responses api stream event: json: cannot unmarshal string
//	into Go struct field .usage.cost of type float64
func TestUsage_CostJSON(t *testing.T) {
	t.Run("response.completed with string cost is accepted", func(t *testing.T) {
		var event StreamEvent

		require.NoError(t, json.Unmarshal([]byte(`{
			"type": "response.completed",
			"response": {
				"id": "resp_test",
				"object": "response",
				"created_at": 1786360449,
				"status": "completed",
				"model": "openai/gpt-example",
				"output": [],
				"usage": {
					"input_tokens": 8,
					"input_tokens_details": {"cache_write_tokens": 0, "cached_tokens": 0},
					"output_tokens": 6,
					"output_tokens_details": {"reasoning_tokens": 0},
					"total_tokens": 14,
					"cost": "0.000152",
					"cost_details": {"prompt": "0.000032", "completion": "0.00012", "input_cache_read": "0"}
				}
			}
		}`), &event))

		require.NotNil(t, event.Response)
		require.NotNil(t, event.Response.Usage)
		require.Equal(t, int64(8), event.Response.Usage.InputTokens)
		require.Equal(t, int64(6), event.Response.Usage.OutputTokens)
		require.Equal(t, int64(14), event.Response.Usage.TotalTokens)

		// Upstream cost is not trusted nor propagated.
		require.Nil(t, event.Response.Usage.Cost)
	})

	t.Run("cached tokens survive a string cost", func(t *testing.T) {
		var usage Usage

		require.NoError(t, json.Unmarshal([]byte(`{
			"input_tokens": 32938,
			"input_tokens_details": {"cache_write_tokens": 1512, "cached_tokens": 31000},
			"output_tokens": 4,
			"total_tokens": 32942,
			"cost": "0.123456"
		}`), &usage))

		require.Nil(t, usage.Cost)
		require.Equal(t, int64(32938), usage.InputTokens)
		require.Equal(t, int64(31000), usage.InputTokenDetails.CachedTokens)
		require.Equal(t, int64(1512), usage.InputTokenDetails.CacheWriteTokens)

		converted := usage.ToUsage()
		require.NotNil(t, converted.PromptTokensDetails)
		require.Equal(t, int64(31000), converted.PromptTokensDetails.CachedTokens)
	})

	t.Run("object shaped cost is ignored", func(t *testing.T) {
		var usage Usage

		require.NoError(t, json.Unmarshal([]byte(`{
			"input_tokens": 42,
			"output_tokens": 7,
			"total_tokens": 49,
			"cost": {"usd": 0.000152, "credits": 0.004}
		}`), &usage))

		require.Nil(t, usage.Cost)
		require.Equal(t, int64(42), usage.InputTokens)
		require.Equal(t, int64(7), usage.OutputTokens)
		require.Equal(t, int64(49), usage.TotalTokens)
	})

	t.Run("numeric cost is preserved", func(t *testing.T) {
		var usage Usage

		require.NoError(t, json.Unmarshal([]byte(`{
			"input_tokens": 3,
			"output_tokens": 1,
			"total_tokens": 4,
			"cost": 0.000152
		}`), &usage))

		require.NotNil(t, usage.Cost)
		require.InDelta(t, 0.000152, *usage.Cost, 1e-12)
	})

	t.Run("null cost is cleared", func(t *testing.T) {
		var usage Usage

		require.NoError(t, json.Unmarshal([]byte(`{
			"input_tokens": 3,
			"output_tokens": 1,
			"total_tokens": 4,
			"cost": null
		}`), &usage))

		require.Nil(t, usage.Cost)
		require.Equal(t, int64(3), usage.InputTokens)
	})

	t.Run("omitted when nil", func(t *testing.T) {
		payload, err := json.Marshal(&Usage{InputTokens: 10, OutputTokens: 20})
		require.NoError(t, err)
		require.NotContains(t, string(payload), `"cost"`)
	})

	t.Run("serialized when set", func(t *testing.T) {
		payload, err := json.Marshal(&Usage{
			InputTokens:  10,
			OutputTokens: 20,
			Cost:         lo.ToPtr(0.000005),
		})
		require.NoError(t, err)
		require.Contains(t, string(payload), `"cost":0.000005`)

		var decoded Usage

		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.NotNil(t, decoded.Cost)
		require.InDelta(t, 0.000005, *decoded.Cost, 1e-12)
	})
}
