package openai

import (
	"strings"
	"testing"
)

func TestMarshalRequestIncludesMaxOutputTokensWhenSet(t *testing.T) {
	payload, err := marshalRequest(responseRequest{
		Model:           "gpt-5.2",
		Input:           []inputMsg{makeMessage("user", "hello")},
		MaxOutputTokens: 123,
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "\"max_output_tokens\":123") {
		t.Fatalf("expected max_output_tokens in payload, got %s", text)
	}
}

func TestMarshalRequestOmitsMaxOutputTokensWhenZero(t *testing.T) {
	payload, err := marshalRequest(responseRequest{
		Model: "gpt-5.2",
		Input: []inputMsg{makeMessage("user", "hello")},
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	text := string(payload)
	if strings.Contains(text, "\"max_output_tokens\"") {
		t.Fatalf("did not expect max_output_tokens in payload, got %s", text)
	}
}
