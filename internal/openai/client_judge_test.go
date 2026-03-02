package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

type scriptedHTTPDoer struct {
	t         *testing.T
	responses []responseBody
	requests  []responseRequest
}

func (d *scriptedHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	d.t.Helper()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var payload responseRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		d.t.Fatalf("decode request payload: %v; body=%s", err, string(body))
	}
	d.requests = append(d.requests, payload)

	if len(d.responses) == 0 {
		return nil, errors.New("unexpected request: no scripted response left")
	}
	resp := d.responses[0]
	d.responses = d.responses[1:]

	raw, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}, nil
}

func TestJudgeConsensusRetriesThirdAttemptWhenStrictRetryIsTruncated(t *testing.T) {
	doer := &scriptedHTTPDoer{
		t: t,
		responses: []responseBody{
			{
				OutputText: "not-json",
				Usage:      apiUsage{InputTokens: 12, OutputTokens: 32, TotalTokens: 44},
			},
			{
				OutputText: `{"reached":true,"score":0.91,"summary":"done","rationale":"aligned","open_risks":[],"next_action_owner":"ops","next_action_trigger_or_deadline":"today","next_action_success_metric":"owner`,
				Usage:      apiUsage{InputTokens: 18, OutputTokens: judgeRetryMaxOutputTokens, TotalTokens: 530},
			},
			{
				OutputText: `{"reached":true,"score":0.91,"summary":"done","rationale":"aligned","open_risks":[],"next_action_owner":"ops","next_action_trigger_or_deadline":"by EOD","next_action_success_metric":"trigger documented"}`,
				Usage:      apiUsage{InputTokens: 20, OutputTokens: 92, TotalTokens: 112},
			},
		},
	}
	client := &Client{
		apiKey:     "test-key",
		endpoint:   defaultEndpoint,
		model:      "gpt-test",
		timeout:    time.Second,
		maxRetries: 0,
		httpClient: doer,
	}

	out, err := client.JudgeConsensus(context.Background(), sampleJudgeInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Consensus.Summary != "done" {
		t.Fatalf("unexpected summary: %q", out.Consensus.Summary)
	}
	if len(doer.requests) != 3 {
		t.Fatalf("expected 3 judge attempts, got %d", len(doer.requests))
	}

	wantMaxTokens := []int{judgeMaxOutputTokens, judgeRetryMaxOutputTokens, judgeTruncationRetryMaxToken}
	for i, want := range wantMaxTokens {
		if got := doer.requests[i].MaxOutputTokens; got != want {
			t.Fatalf("attempt %d max_output_tokens=%d, want=%d", i+1, got, want)
		}
	}

	if !strings.Contains(doer.requests[1].Input[1].Content[0].Text, "Return only one minified JSON object") {
		t.Fatalf("strict retry prompt missing on attempt 2: %q", doer.requests[1].Input[1].Content[0].Text)
	}
	if !strings.Contains(doer.requests[2].Input[1].Content[0].Text, "previous response was truncated") {
		t.Fatalf("truncation retry prompt missing on attempt 3: %q", doer.requests[2].Input[1].Content[0].Text)
	}
}

func TestJudgeConsensusStopsAfterSecondAttemptWhenNotTruncation(t *testing.T) {
	doer := &scriptedHTTPDoer{
		t: t,
		responses: []responseBody{
			{
				OutputText: "not-json",
				Usage:      apiUsage{InputTokens: 10, OutputTokens: 30, TotalTokens: 40},
			},
			{
				OutputText: `{"reached":true,"score":0.8,"summary":"almost","rationale":"x","open_risks":[],"next_action_owner":"ops","next_action_trigger_or_deadline":"today"}`,
				Usage:      apiUsage{InputTokens: 16, OutputTokens: 96, TotalTokens: 112},
			},
		},
	}
	client := &Client{
		apiKey:     "test-key",
		endpoint:   defaultEndpoint,
		model:      "gpt-test",
		timeout:    time.Second,
		maxRetries: 0,
		httpClient: doer,
	}

	_, err := client.JudgeConsensus(context.Background(), sampleJudgeInput())
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "missing required consensus key: next_action_success_metric") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doer.requests) != 2 {
		t.Fatalf("expected 2 judge attempts, got %d", len(doer.requests))
	}
}

func sampleJudgeInput() orchestrator.JudgeConsensusInput {
	return orchestrator.JudgeConsensusInput{
		Problem: "sample problem",
		Personas: []persona.Persona{
			{ID: "a", Name: "A", Role: "strategy"},
			{ID: "b", Name: "B", Role: "operations"},
		},
		Turns: []orchestrator.Turn{
			{
				Index:       1,
				SpeakerID:   "a",
				SpeakerName: "A",
				Type:        orchestrator.TurnTypePersona,
				Content:     "기본안은 A로 가자.",
			},
		},
	}
}
