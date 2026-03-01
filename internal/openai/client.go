package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

const (
	turnMaxOutputTokens          = 720
	moderatorMaxOutputTokens     = 760
	finalModeratorMaxOutputToken = 360
	judgeMaxOutputTokens         = 320
	judgeRetryMaxOutputTokens    = 512
	openingSpeakerMaxOutputToken = 180
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
		turnMaxOutputTokens,
	)
	if err != nil {
		return orchestrator.GenerateTurnOutput{}, err
	}

	return orchestrator.GenerateTurnOutput{
		Content: text,
		Usage:   usage,
	}, nil
}

func (c *Client) SelectOpeningSpeaker(ctx context.Context, input orchestrator.SelectOpeningSpeakerInput) (orchestrator.SelectOpeningSpeakerOutput, error) {
	text, usage, err := c.generatePlainText(
		ctx,
		buildOpeningSpeakerSelectorSystemPrompt(),
		buildOpeningSpeakerSelectorUserPrompt(input),
		"empty opening speaker output",
		openingSpeakerMaxOutputToken,
	)
	if err != nil {
		return orchestrator.SelectOpeningSpeakerOutput{}, err
	}

	personaID, err := parseOpeningSpeakerID(text)
	if err != nil {
		return orchestrator.SelectOpeningSpeakerOutput{}, fmt.Errorf("parse opening speaker id: %w", err)
	}
	if !containsPersonaID(input.Personas, personaID) {
		return orchestrator.SelectOpeningSpeakerOutput{}, fmt.Errorf("parse opening speaker id: unknown persona_id %q", personaID)
	}

	return orchestrator.SelectOpeningSpeakerOutput{
		PersonaID: personaID,
		Usage:     usage,
	}, nil
}

func containsPersonaID(personas []persona.Persona, id string) bool {
	needle := strings.TrimSpace(id)
	if needle == "" {
		return false
	}
	for _, p := range personas {
		if strings.EqualFold(strings.TrimSpace(p.ID), needle) {
			return true
		}
	}
	return false
}

func (c *Client) GenerateModerator(ctx context.Context, input orchestrator.GenerateModeratorInput) (orchestrator.GenerateModeratorOutput, error) {
	text, usage, err := c.generatePlainText(
		ctx,
		buildModeratorSystemPrompt(),
		buildModeratorUserPrompt(input),
		"empty moderator output",
		moderatorMaxOutputTokens,
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
		finalModeratorMaxOutputToken,
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
		maxOutputTokens := judgeMaxOutputTokens
		if attempt > 0 {
			maxOutputTokens = judgeRetryMaxOutputTokens
		}
		currentUserPrompt := userPrompt
		if attempt > 0 {
			currentUserPrompt += "\n\nReturn only one minified JSON object on a single line. No markdown/code fence."
		}
		resp, err := c.callResponses(ctx, []inputMsg{
			makeMessage("system", systemPrompt),
			makeMessage("user", currentUserPrompt),
		}, maxOutputTokens)
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

func (c *Client) callResponses(ctx context.Context, input []inputMsg, maxOutputTokens int) (responseBody, error) {
	reqBody := responseRequest{
		Model:           c.model,
		Input:           input,
		MaxOutputTokens: maxOutputTokens,
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

func (c *Client) generatePlainText(ctx context.Context, systemPrompt string, userPrompt string, emptyOutputError string, maxOutputTokens int) (string, orchestrator.Usage, error) {
	resp, err := c.callResponses(ctx, []inputMsg{
		makeMessage("system", systemPrompt),
		makeMessage("user", userPrompt),
	}, maxOutputTokens)
	if err != nil {
		return "", orchestrator.Usage{}, err
	}

	text := strings.TrimSpace(extractOutputText(resp))
	if text == "" {
		return "", orchestrator.Usage{}, errors.New(emptyOutputError)
	}

	usage := toUsage(resp.Usage)
	if looksLikeTruncatedText(text, usage.CompletionTokens, maxOutputTokens) {
		retryCap := maxOutputTokens * 2
		if retryCap < maxOutputTokens+120 {
			retryCap = maxOutputTokens + 120
		}
		if retryCap > 1400 {
			retryCap = 1400
		}
		retryPrompt := userPrompt + "\n\nYour previous response was cut off. Rewrite the whole answer from scratch, concise but complete, and end with a complete sentence."

		retryResp, retryErr := c.callResponses(ctx, []inputMsg{
			makeMessage("system", systemPrompt),
			makeMessage("user", retryPrompt),
		}, retryCap)
		if retryErr == nil {
			retryText := strings.TrimSpace(extractOutputText(retryResp))
			if retryText != "" {
				retryUsage := toUsage(retryResp.Usage)
				usage.PromptTokens += retryUsage.PromptTokens
				usage.CompletionTokens += retryUsage.CompletionTokens
				usage.TotalTokens += retryUsage.TotalTokens
				text = retryText
			}
		}
	}

	return text, usage, nil
}

func looksLikeTruncatedText(text string, completionTokens int, maxOutputTokens int) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	if maxOutputTokens <= 0 {
		return false
	}
	if completionTokens < maxOutputTokens-6 {
		return false
	}

	trimmed := strings.TrimSpace(text)
	completeSuffixes := []string{
		".", "!", "?", "…", "\"", "'", "”", "’",
		"다.", "요.", "니다.", "했다.", "됩니다.", "한다.", "합니다.", "해요.", "임.", "됨.",
		"다", "요", "니다", "합니다", "해요", "됨", "임",
		"}", "]", ")",
	}
	for _, suffix := range completeSuffixes {
		if strings.HasSuffix(trimmed, suffix) {
			return false
		}
	}
	return true
}
