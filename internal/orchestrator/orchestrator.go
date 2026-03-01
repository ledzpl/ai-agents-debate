package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"debate/internal/persona"
)

const (
	StatusConsensusReached  = "consensus_reached"
	StatusMaxTurnsReached   = "max_turns_reached"
	StatusDurationReached   = "duration_limit_reached"
	StatusTokenLimitReached = "token_limit_reached"
	StatusNoProgressReached = "no_progress_reached"
	StatusError             = "error"

	TurnTypePersona   = "persona"
	TurnTypeModerator = "moderator"

	ModeratorSpeakerID   = "moderator"
	ModeratorSpeakerName = "사회자"
)

const (
	defaultConsensusThreshold     = 0.90
	defaultConsensusConfirmations = 2
	defaultMaxDuration            = 20 * time.Minute
	defaultMaxTotalTokens         = 120000
	defaultMaxNoProgress          = 6
	defaultNoProgressEpsilon      = 0.01
)

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Turn struct {
	Index       int       `json:"index"`
	SpeakerID   string    `json:"speaker_id"`
	SpeakerName string    `json:"speaker_name"`
	Type        string    `json:"type"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
}

type Consensus struct {
	Reached   bool    `json:"reached"`
	Score     float64 `json:"score"`
	Summary   string  `json:"summary"`
	Rationale string  `json:"rationale"`
}

type Metrics struct {
	LatencyMS        int64 `json:"latency_ms"`
	PromptTokens     int   `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	TotalTokens      int   `json:"total_tokens"`
}

type Result struct {
	Problem   string            `json:"problem"`
	Personas  []persona.Persona `json:"personas"`
	Turns     []Turn            `json:"turns"`
	Consensus Consensus         `json:"consensus"`
	Status    string            `json:"status"`
	Metrics   Metrics           `json:"metrics"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
}

type GenerateTurnInput struct {
	Problem  string
	Personas []persona.Persona
	Turns    []Turn
	Speaker  persona.Persona
}

type GenerateTurnOutput struct {
	Content string
	Usage   Usage
}

type GenerateModeratorInput struct {
	Problem       string
	Personas      []persona.Persona
	Turns         []Turn
	PreviousTurn  Turn
	NextSpeaker   persona.Persona
	CurrentTurnNo int
}

type GenerateModeratorOutput struct {
	Content string
	Usage   Usage
}

type GenerateFinalModeratorInput struct {
	Problem     string
	Personas    []persona.Persona
	Turns       []Turn
	Consensus   Consensus
	FinalStatus string
}

type GenerateFinalModeratorOutput struct {
	Content string
	Usage   Usage
}

type JudgeConsensusInput struct {
	Problem  string
	Personas []persona.Persona
	Turns    []Turn
}

type JudgeConsensusOutput struct {
	Consensus Consensus
	Usage     Usage
}

type SelectOpeningSpeakerInput struct {
	Problem  string
	Personas []persona.Persona
}

type SelectOpeningSpeakerOutput struct {
	PersonaID string
	Usage     Usage
}

type LLMClient interface {
	GenerateTurn(ctx context.Context, input GenerateTurnInput) (GenerateTurnOutput, error)
	GenerateModerator(ctx context.Context, input GenerateModeratorInput) (GenerateModeratorOutput, error)
	GenerateFinalModerator(ctx context.Context, input GenerateFinalModeratorInput) (GenerateFinalModeratorOutput, error)
	JudgeConsensus(ctx context.Context, input JudgeConsensusInput) (JudgeConsensusOutput, error)
}

// OpeningSpeakerSelector is optional. When implemented, the orchestrator asks
// the model to choose the best first persona for the given problem.
type OpeningSpeakerSelector interface {
	SelectOpeningSpeaker(ctx context.Context, input SelectOpeningSpeakerInput) (SelectOpeningSpeakerOutput, error)
}

type Config struct {
	MaxTurns            int
	ConsensusThreshold  float64
	MaxDuration         time.Duration
	MaxTotalTokens      int
	MaxNoProgressJudges int
	NoProgressEpsilon   float64
}

type Orchestrator struct {
	llm LLMClient
	cfg Config
}

type judgeProgress struct {
	noProgressJudges int
	hasPrevScore     bool
	prevScore        float64
	// Consecutive confirmations reduce false positives from a single optimistic judge call.
	consecutiveConsensusJudges int
}

func New(llm LLMClient, cfg Config) *Orchestrator {
	// MaxTurns == 0 means unbounded rounds with safety guards.
	if cfg.MaxTurns < 0 {
		cfg.MaxTurns = 0
	}
	if cfg.ConsensusThreshold < 0 || cfg.ConsensusThreshold > 1 {
		cfg.ConsensusThreshold = defaultConsensusThreshold
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = defaultMaxDuration
	}
	if cfg.MaxTotalTokens <= 0 {
		cfg.MaxTotalTokens = defaultMaxTotalTokens
	}
	if cfg.MaxNoProgressJudges <= 0 {
		cfg.MaxNoProgressJudges = defaultMaxNoProgress
	}
	if cfg.NoProgressEpsilon <= 0 {
		cfg.NoProgressEpsilon = defaultNoProgressEpsilon
	}
	return &Orchestrator{llm: llm, cfg: cfg}
}

func (o *Orchestrator) Run(ctx context.Context, problem string, personas []persona.Persona, onTurn func(Turn)) (Result, error) {
	started := time.Now().UTC()
	res := Result{
		Problem:   strings.TrimSpace(problem),
		StartedAt: started,
	}
	if o == nil || o.llm == nil {
		finalizeResult(&res, started, StatusError)
		return res, errors.New("llm client is required")
	}

	if res.Problem == "" {
		finalizeResult(&res, started, StatusError)
		return res, errors.New("problem must not be empty")
	}

	normalized, err := persona.NormalizeAndValidate(personas)
	if err != nil {
		finalizeResult(&res, started, StatusError)
		return res, fmt.Errorf("invalid personas: %w", err)
	}
	res.Personas = normalized

	openingSpeakerIndex, openingStopStatus, openingShouldStop := o.chooseOpeningSpeakerIndex(ctx, started, &res, normalized)
	if openingShouldStop {
		return o.finalizeWithModerator(ctx, &res, started, openingStopStatus, onTurn)
	}

	progress := judgeProgress{}

	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			finalizeResult(&res, started, StatusError)
			return res, fmt.Errorf("debate canceled: %w", err)
		}

		if status, shouldStop := o.preTurnStatus(started, i); shouldStop {
			return o.finalizeWithModerator(ctx, &res, started, status, onTurn)
		}

		turnNo := i + 1
		speaker := normalized[(openingSpeakerIndex+i)%len(normalized)]
		stepCtx, cancel := o.callContext(ctx, started)
		personaTurn, err := o.generatePersonaTurn(stepCtx, &res, normalized, speaker, turnNo)
		cancel()
		if err != nil {
			if status, isDurationStop := o.durationStatusOnLLMError(started, err); isDurationStop {
				return o.finalizeWithModerator(ctx, &res, started, status, onTurn)
			}
			finalizeResult(&res, started, StatusError)
			return res, fmt.Errorf("generate turn %d: %w", turnNo, err)
		}
		res.Turns = append(res.Turns, personaTurn)
		if onTurn != nil {
			onTurn(personaTurn)
		}

		if reachedTokenLimit(res.Metrics.TotalTokens, o.cfg.MaxTotalTokens) {
			return o.finalizeWithModerator(ctx, &res, started, StatusTokenLimitReached, onTurn)
		}

		if o.shouldJudgeAtTurn(i, len(normalized)) {
			stepCtx, cancel := o.callContext(ctx, started)
			status, done, err := o.evaluateConsensus(stepCtx, &res, normalized, turnNo, &progress)
			cancel()
			if err != nil {
				if status, isDurationStop := o.durationStatusOnLLMError(started, err); isDurationStop {
					return o.finalizeWithModerator(ctx, &res, started, status, onTurn)
				}
				finalizeResult(&res, started, StatusError)
				return res, fmt.Errorf("judge consensus at turn %d: %w", turnNo, err)
			}
			if done {
				return o.finalizeWithModerator(ctx, &res, started, status, onTurn)
			}
		}

		if !hasNextPersonaTurn(i, o.cfg.MaxTurns) {
			return o.finalizeWithModerator(ctx, &res, started, StatusMaxTurnsReached, onTurn)
		}

		nextSpeaker := normalized[(openingSpeakerIndex+i+1)%len(normalized)]
		stepCtx, cancel = o.callContext(ctx, started)
		moderatorTurn, err := o.generateModeratorTurn(stepCtx, &res, normalized, personaTurn, nextSpeaker, turnNo)
		cancel()
		if err != nil {
			if status, isDurationStop := o.durationStatusOnLLMError(started, err); isDurationStop {
				return o.finalizeWithModerator(ctx, &res, started, status, onTurn)
			}
			finalizeResult(&res, started, StatusError)
			return res, fmt.Errorf("generate moderator after turn %d: %w", turnNo, err)
		}
		res.Turns = append(res.Turns, moderatorTurn)
		if onTurn != nil {
			onTurn(moderatorTurn)
		}
		if reachedTokenLimit(res.Metrics.TotalTokens, o.cfg.MaxTotalTokens) {
			return o.finalizeWithModerator(ctx, &res, started, StatusTokenLimitReached, onTurn)
		}
	}
}

func (o *Orchestrator) chooseOpeningSpeakerIndex(ctx context.Context, started time.Time, res *Result, personas []persona.Persona) (int, string, bool) {
	index := defaultOpeningSpeakerIndex(res.Problem, personas)
	selector, ok := o.llm.(OpeningSpeakerSelector)
	if !ok {
		return index, "", false
	}

	stepCtx, cancel := o.callContext(ctx, started)
	out, err := selector.SelectOpeningSpeaker(stepCtx, SelectOpeningSpeakerInput{
		Problem:  res.Problem,
		Personas: personas,
	})
	cancel()
	if err != nil {
		if status, isDurationStop := o.durationStatusOnLLMError(started, err); isDurationStop {
			return index, status, true
		}
		return index, "", false
	}

	addUsage(&res.Metrics, out.Usage)
	if idx := findPersonaIndex(personas, out.PersonaID); idx >= 0 {
		index = idx
	}
	if reachedTokenLimit(res.Metrics.TotalTokens, o.cfg.MaxTotalTokens) {
		return index, StatusTokenLimitReached, true
	}
	return index, "", false
}

func (o *Orchestrator) preTurnStatus(started time.Time, turnIndex int) (string, bool) {
	if o.cfg.MaxTurns > 0 && turnIndex >= o.cfg.MaxTurns {
		return StatusMaxTurnsReached, true
	}
	if reachedDurationLimit(started, o.cfg.MaxDuration) {
		return StatusDurationReached, true
	}
	return "", false
}

func (o *Orchestrator) generatePersonaTurn(ctx context.Context, res *Result, personas []persona.Persona, speaker persona.Persona, turnNo int) (Turn, error) {
	out, err := o.llm.GenerateTurn(ctx, GenerateTurnInput{
		Problem:  res.Problem,
		Personas: personas,
		Turns:    res.Turns,
		Speaker:  speaker,
	})
	if err != nil {
		return Turn{}, err
	}
	addUsage(&res.Metrics, out.Usage)

	content := strings.TrimSpace(out.Content)
	if content == "" {
		return Turn{}, fmt.Errorf("turn %d was empty", turnNo)
	}
	return Turn{
		Index:       nextTurnIndex(res.Turns),
		SpeakerID:   speaker.ID,
		SpeakerName: persona.DisplayName(speaker),
		Type:        TurnTypePersona,
		Content:     content,
		Timestamp:   time.Now().UTC(),
	}, nil
}

func (o *Orchestrator) shouldJudgeAtTurn(turnIndex int, personaCount int) bool {
	if o.cfg.MaxTurns > 0 && turnIndex+1 >= o.cfg.MaxTurns {
		return true
	}
	return shouldJudgeConsensus(turnIndex, personaCount)
}

func (o *Orchestrator) evaluateConsensus(ctx context.Context, res *Result, personas []persona.Persona, turnNo int, progress *judgeProgress) (string, bool, error) {
	judgeOut, err := o.llm.JudgeConsensus(ctx, JudgeConsensusInput{
		Problem:  res.Problem,
		Personas: personas,
		Turns:    res.Turns,
	})
	if err != nil {
		return "", false, err
	}
	addUsage(&res.Metrics, judgeOut.Usage)
	res.Consensus = judgeOut.Consensus

	if reachedTokenLimit(res.Metrics.TotalTokens, o.cfg.MaxTotalTokens) {
		return StatusTokenLimitReached, true, nil
	}
	if consensusSatisfied(res.Consensus, o.cfg.ConsensusThreshold) {
		progress.consecutiveConsensusJudges++
	} else {
		progress.consecutiveConsensusJudges = 0
	}
	if progress.consecutiveConsensusJudges >= requiredConsensusConfirmations(len(personas)) {
		return StatusConsensusReached, true, nil
	}

	progress.update(res.Consensus.Score, o.cfg.NoProgressEpsilon)
	if progress.noProgressJudges >= o.cfg.MaxNoProgressJudges {
		return StatusNoProgressReached, true, nil
	}
	return "", false, nil
}

func consensusSatisfied(consensus Consensus, threshold float64) bool {
	return consensus.Reached && consensus.Score >= threshold
}

func requiredConsensusConfirmations(personaCount int) int {
	if personaCount <= 1 {
		return 1
	}
	return defaultConsensusConfirmations
}

func (o *Orchestrator) generateModeratorTurn(ctx context.Context, res *Result, personas []persona.Persona, previousTurn Turn, nextSpeaker persona.Persona, turnNo int) (Turn, error) {
	out, err := o.llm.GenerateModerator(ctx, GenerateModeratorInput{
		Problem:       res.Problem,
		Personas:      personas,
		Turns:         res.Turns,
		PreviousTurn:  previousTurn,
		NextSpeaker:   nextSpeaker,
		CurrentTurnNo: turnNo,
	})
	if err != nil {
		return Turn{}, err
	}
	addUsage(&res.Metrics, out.Usage)

	content := strings.TrimSpace(out.Content)
	if content == "" {
		return Turn{}, fmt.Errorf("moderator turn after %d was empty", turnNo)
	}

	return Turn{
		Index:       nextTurnIndex(res.Turns),
		SpeakerID:   ModeratorSpeakerID,
		SpeakerName: ModeratorSpeakerName,
		Type:        TurnTypeModerator,
		Content:     content,
		Timestamp:   time.Now().UTC(),
	}, nil
}

func (p *judgeProgress) update(score float64, epsilon float64) {
	if p.hasPrevScore {
		if score <= p.prevScore+epsilon {
			p.noProgressJudges++
		} else {
			p.noProgressJudges = 0
		}
	}
	p.prevScore = score
	p.hasPrevScore = true
}

func addUsage(metrics *Metrics, usage Usage) {
	metrics.PromptTokens += usage.PromptTokens
	metrics.CompletionTokens += usage.CompletionTokens
	metrics.TotalTokens += usage.TotalTokens
}
