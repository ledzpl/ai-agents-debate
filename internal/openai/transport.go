package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const defaultEndpoint = "https://api.openai.com/v1/responses"

const maxResponseBodyBytes = 8 * 1024 * 1024

type responseRequest struct {
	Model           string     `json:"model"`
	Input           []inputMsg `json:"input"`
	MaxOutputTokens int        `json:"max_output_tokens,omitempty"`
}

type inputMsg struct {
	Role    string         `json:"role"`
	Content []inputContent `json:"content"`
}

type inputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseBody struct {
	OutputText string       `json:"output_text"`
	Output     []outputItem `json:"output"`
	Usage      apiUsage     `json:"usage"`
	Error      *apiError    `json:"error"`
}

type outputItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Text    string          `json:"text"`
	Content []contentOutput `json:"content"`
}

type contentOutput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func newDefaultHTTPClient() *http.Client {
	return &http.Client{}
}

func normalizeEndpoint(base string) string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return defaultEndpoint
	}

	trimmed = strings.TrimRight(trimmed, "/")
	switch {
	case strings.HasSuffix(trimmed, "/responses"):
		return trimmed
	case strings.HasSuffix(trimmed, "/v1"):
		return trimmed + "/responses"
	case strings.Contains(trimmed, "/v1/"):
		return trimmed
	default:
		return trimmed + "/v1/responses"
	}
}

func marshalRequest(req responseRequest) ([]byte, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return payload, nil
}

func (c *Client) doRequest(ctx context.Context, payload []byte) (responseBody, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return responseBody{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return responseBody{}, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return responseBody{}, fmt.Errorf("read response body: %w", err)
	}
	if len(body) > maxResponseBodyBytes {
		return responseBody{}, fmt.Errorf("read response body: exceeds limit (%d bytes)", maxResponseBodyBytes)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := decodeAPIError(body)
		return responseBody{}, &httpStatusError{statusCode: resp.StatusCode, message: apiErr}
	}

	var decoded responseBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		return responseBody{}, fmt.Errorf("decode response: %w", err)
	}
	if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
		return responseBody{}, fmt.Errorf("api error: %s", decoded.Error.Message)
	}

	return decoded, nil
}

func decodeAPIError(body []byte) string {
	var wrapped struct {
		Error *apiError `json:"error"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Error != nil && strings.TrimSpace(wrapped.Error.Message) != "" {
		return strings.TrimSpace(wrapped.Error.Message)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "empty error response"
	}
	return text
}

type httpStatusError struct {
	statusCode int
	message    string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("openai status %d: %s", e.statusCode, e.message)
}

func isRetriableError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.statusCode == http.StatusTooManyRequests || statusErr.statusCode >= 500
	}
	return true
}

func backoffDuration(attempt int) time.Duration {
	base := 500.0
	exp := math.Pow(2, float64(attempt))
	ms := int(base * exp)
	if ms > 4000 {
		ms = 4000
	}
	return time.Duration(ms) * time.Millisecond
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func makeMessage(role, text string) inputMsg {
	return inputMsg{
		Role: role,
		Content: []inputContent{
			{Type: "input_text", Text: text},
		},
	}
}
