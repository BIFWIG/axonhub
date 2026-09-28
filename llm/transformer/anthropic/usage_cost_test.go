package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// Some Anthropic-compatible providers encode the request cost as a decimal
// string. ZenMux returns, in `message_delta`:
//
//	"usage":{"input_tokens":9,"cache_creation_input_tokens":0,
//	         "cache_read_input_tokens":0,"output_tokens":6,
//	         "cost":"0.000078",
//	         "cost_details":{"prompt":"0.000018","completion":"0.00006","input_cache_read":"0"}}
//
// A strict *float64 decode rejected the whole event, so AxonHub aborted the
// stream with:
//
//	failed to unmarshal anthropic stream event: json: cannot unmarshal string
//	into Go struct field StreamEvent.usage.cost of type float64
func TestUsage_CostJSON(t *testing.T) {
	t.Run("message_delta with string cost is accepted", func(t *testing.T) {
		var event StreamEvent

		require.NoError(t, json.Unmarshal([]byte(`{
			"type": "message_delta",
			"delta": {"stop_reason": "end_turn", "stop_sequence": null},
			"usage": {
				"input_tokens": 9,
				"cache_creation_input_tokens": 21,
				"cache_read_input_tokens": 5688,
				"cache_creation": {"ephemeral_5m_input_tokens": 21, "ephemeral_1h_input_tokens": 0},
				"output_tokens": 6,
				"output_tokens_details": {"thinking_tokens": 0},
				"cost": "0.000078",
				"cost_details": {"prompt": "0.000018", "completion": "0.00006", "input_cache_read": "0"}
			}
		}`), &event))

		require.NotNil(t, event.Usage)
		require.Equal(t, int64(9), event.Usage.InputTokens)
		require.Equal(t, int64(6), event.Usage.OutputTokens)
		require.Equal(t, int64(21), event.Usage.CacheCreationInputTokens)
		require.Equal(t, int64(5688), event.Usage.CacheReadInputTokens)
		require.Equal(t, int64(21), event.Usage.CacheCreation.Ephemeral5mInputTokens)

		// Upstream cost is not trusted nor propagated; it must not survive
		// decoding as a number either.
		require.Nil(t, event.Usage.Cost)
	})

	t.Run("object shaped cost is ignored", func(t *testing.T) {
		var event StreamEvent

		require.NoError(t, json.Unmarshal([]byte(`{
			"type": "message_delta",
			"usage": {
				"input_tokens": 42,
				"output_tokens": 7,
				"cost": {"usd": 0.000078, "credits": 0.002}
			}
		}`), &event))

		require.NotNil(t, event.Usage)
		require.Nil(t, event.Usage.Cost)
		require.Equal(t, int64(42), event.Usage.InputTokens)
		require.Equal(t, int64(7), event.Usage.OutputTokens)
	})

	t.Run("numeric cost is preserved", func(t *testing.T) {
		var event StreamEvent

		require.NoError(t, json.Unmarshal([]byte(`{
			"type": "message_delta",
			"usage": {"input_tokens": 3, "output_tokens": 1, "cost": 0.000078}
		}`), &event))

		require.NotNil(t, event.Usage)
		require.NotNil(t, event.Usage.Cost)
		require.InDelta(t, 0.000078, *event.Usage.Cost, 1e-12)
	})

	t.Run("null cost is cleared", func(t *testing.T) {
		var event StreamEvent

		require.NoError(t, json.Unmarshal([]byte(`{
			"type": "message_delta",
			"usage": {"input_tokens": 3, "output_tokens": 1, "cost": null}
		}`), &event))

		require.NotNil(t, event.Usage)
		require.Nil(t, event.Usage.Cost)
	})

	t.Run("message_start usage without cost still decodes", func(t *testing.T) {
		var event StreamEvent

		require.NoError(t, json.Unmarshal([]byte(`{
			"type": "message_start",
			"message": {
				"id": "msg_test",
				"type": "message",
				"role": "assistant",
				"content": [],
				"usage": {"input_tokens": 1200, "output_tokens": 1}
			}
		}`), &event))

		require.NotNil(t, event.Message)
		require.NotNil(t, event.Message.Usage)
		require.Equal(t, int64(1200), event.Message.Usage.InputTokens)
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
