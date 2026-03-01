package openai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: defaultEndpoint},
		{name: "base", in: "https://api.openai.com", want: "https://api.openai.com/v1/responses"},
		{name: "v1", in: "https://api.openai.com/v1", want: "https://api.openai.com/v1/responses"},
		{name: "responses", in: "https://proxy.example/v1/responses", want: "https://proxy.example/v1/responses"},
		{name: "custom-v1-path", in: "https://proxy.example/v1/custom", want: "https://proxy.example/v1/custom"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeEndpoint(tc.in); got != tc.want {
				t.Fatalf("normalizeEndpoint(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsRetriableError(t *testing.T) {
	if isRetriableError(context.Canceled) {
		t.Fatal("context canceled should not be retriable")
	}
	if isRetriableError(context.DeadlineExceeded) {
		t.Fatal("context deadline should not be retriable")
	}
	if !isRetriableError(&httpStatusError{statusCode: 500}) {
		t.Fatal("5xx should be retriable")
	}
	if !isRetriableError(&httpStatusError{statusCode: 429}) {
		t.Fatal("429 should be retriable")
	}
	if isRetriableError(&httpStatusError{statusCode: 400}) {
		t.Fatal("4xx should not be retriable")
	}
	if !isRetriableError(fmt.Errorf("openai request failed: %w", &net.DNSError{IsTemporary: true})) {
		t.Fatal("network errors should be retriable")
	}
	if !isRetriableError(errors.New("read response body: unexpected EOF")) {
		t.Fatal("body read transient errors should be retriable")
	}
	if isRetriableError(errors.New("build request: invalid URL")) {
		t.Fatal("build request errors should not be retriable")
	}
	if isRetriableError(errors.New("decode response: invalid character")) {
		t.Fatal("decode errors should not be retriable")
	}
	if isRetriableError(errors.New("read response body: exceeds limit (8388608 bytes)")) {
		t.Fatal("response size limit errors should not be retriable")
	}
}
