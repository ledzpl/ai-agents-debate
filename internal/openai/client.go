package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"debate/internal/orchestrator"
)

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

type Client struct {
	apiKey     string
	endpoint   string
	model      string
	timeout    time.Duration
	maxRetries int
	httpClient httpDoer
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("api key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("model is required")
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("timeout must be > 0")
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}

	return &Client{
		apiKey:     strings.TrimSpace(cfg.APIKey),
		endpoint:   normalizeEndpoint(cfg.BaseURL),
		model:      strings.TrimSpace(cfg.Model),
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		httpClient: newDefaultHTTPClient(),
	}, nil
}

func (c *Client) GenerateTurn(ctx context.Context, input orchestrator.GenerateTurnInput) (orchestrator.GenerateTurnOutput, error) {
	text, usage, err := c.generatePlainText(
		ctx,
		buildTurnSystemPrompt(),
		buildTurnUserPrompt(input),
		"empty model output",
	)
	if err != nil {
		return orchestrator.GenerateTurnOutput{}, err
	}

	return orchestrator.GenerateTurnOutput{
		Content: text,
		Usage:   usage,
	}, nil
}

func (c *Client) GenerateModerator(ctx context.Context, input orchestrator.GenerateModeratorInput) (orchestrator.GenerateModeratorOutput, error) {
	text, usage, err := c.generatePlainText(
		ctx,
		buildModeratorSystemPrompt(),
		buildModeratorUserPrompt(input),
		"empty moderator output",
	)
	if err != nil {
		return orchestrator.GenerateModeratorOutput{}, err
	}

	return orchestrator.GenerateModeratorOutput{
		Content: text,
		Usage:   usage,
	}, nil
}

func (c *Client) GenerateFinalModerator(ctx context.Context, input orchestrator.GenerateFinalModeratorInput) (orchestrator.GenerateFinalModeratorOutput, error) {
	text, usage, err := c.generatePlainText(
		ctx,
		buildFinalModeratorSystemPrompt(),
		buildFinalModeratorUserPrompt(input),
		"empty final moderator output",
	)
	if err != nil {
		return orchestrator.GenerateFinalModeratorOutput{}, err
	}

	return orchestrator.GenerateFinalModeratorOutput{
		Content: text,
		Usage:   usage,
	}, nil
}

func (c *Client) JudgeConsensus(ctx context.Context, input orchestrator.JudgeConsensusInput) (orchestrator.JudgeConsensusOutput, error) {
	systemPrompt := buildJudgeSystemPrompt()
	userPrompt := buildJudgeUserPrompt(input)

	var aggregated orchestrator.Usage
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.callResponses(ctx, []inputMsg{
			makeMessage("system", systemPrompt),
			makeMessage("user", userPrompt),
		})
		if err != nil {
			return orchestrator.JudgeConsensusOutput{}, err
		}

		usage := toUsage(resp.Usage)
		aggregated.PromptTokens += usage.PromptTokens
		aggregated.CompletionTokens += usage.CompletionTokens
		aggregated.TotalTokens += usage.TotalTokens

		raw := strings.TrimSpace(extractOutputText(resp))
		parsed, parseErr := parseConsensus(raw)
		if parseErr == nil {
			return orchestrator.JudgeConsensusOutput{Consensus: parsed, Usage: aggregated}, nil
		}
		if attempt == 1 {
			return orchestrator.JudgeConsensusOutput{}, fmt.Errorf("parse consensus json: %w", parseErr)
		}
	}

	return orchestrator.JudgeConsensusOutput{}, errors.New("unreachable consensus parser state")
}

func (c *Client) callResponses(ctx context.Context, input []inputMsg) (responseBody, error) {
	reqBody := responseRequest{
		Model: c.model,
		Input: input,
	}

	payload, err := marshalRequest(reqBody)
	if err != nil {
		return responseBody{}, err
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		apiCtx, cancel := context.WithTimeout(ctx, c.timeout)
		resp, err := c.doRequest(apiCtx, payload)
		cancel()

		if err == nil {
			return resp, nil
		}
		lastErr = err

		if attempt == c.maxRetries {
			break
		}
		if !isRetriableError(err) {
			break
		}
		if err := sleepWithContext(ctx, backoffDuration(attempt)); err != nil {
			return responseBody{}, err
		}
	}

	if lastErr == nil {
		lastErr = errors.New("unknown openai error")
	}
	return responseBody{}, lastErr
}

func (c *Client) generatePlainText(ctx context.Context, systemPrompt string, userPrompt string, emptyOutputError string) (string, orchestrator.Usage, error) {
	resp, err := c.callResponses(ctx, []inputMsg{
		makeMessage("system", systemPrompt),
		makeMessage("user", userPrompt),
	})
	if err != nil {
		return "", orchestrator.Usage{}, err
	}

	text := strings.TrimSpace(extractOutputText(resp))
	if text == "" {
		return "", orchestrator.Usage{}, errors.New(emptyOutputError)
	}
	return text, toUsage(resp.Usage), nil
}
