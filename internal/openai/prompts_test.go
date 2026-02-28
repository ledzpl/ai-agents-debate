package openai

import (
	"strings"
	"testing"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

func TestBuildTurnUserPromptIncludesSignatureLensForKnownExpert(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "활성화율을 높이는 방법은?",
		Personas: []persona.Persona{
			{ID: "p1", Name: "그로스 PM (Brian Balfour)", Role: "growth", SignatureLens: []string{"frame ideas as growth loops"}},
			{ID: "p2", Name: "UX 리서처 / 서비스 디자이너 (Nir Eyal)", Role: "ux"},
		},
		Speaker: persona.Persona{
			ID:            "p1",
			Name:          "그로스 PM (Brian Balfour)",
			Role:          "growth",
			Stance:        "experiment-driven",
			SignatureLens: []string{"frame ideas as growth loops"},
		},
	}

	prompt := buildTurnUserPrompt(input)
	if !strings.Contains(prompt, "signature lens") {
		t.Fatalf("expected signature lens guidance, prompt=%q", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "growth loops") {
		t.Fatalf("expected Brian Balfour guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "not identity impersonation") {
		t.Fatalf("expected impersonation guardrail, prompt=%q", prompt)
	}
}

func TestBuildModeratorUserPromptIncludesNextSpeakerLens(t *testing.T) {
	input := orchestrator.GenerateModeratorInput{
		Problem: "리텐션 개선",
		Personas: []persona.Persona{
			{ID: "p1", Name: "데이터 분석가 (Julie Zhuo)", Role: "analytics", SignatureLens: []string{"connect recommendations to product quality"}},
		},
		Turns: []orchestrator.Turn{{Index: 1, SpeakerName: "A", Type: orchestrator.TurnTypePersona, Content: "의견"}},
		PreviousTurn: orchestrator.Turn{
			Index:       1,
			SpeakerName: "A",
			Type:        orchestrator.TurnTypePersona,
			Content:     "의견",
		},
		NextSpeaker: persona.Persona{
			ID:            "p1",
			Name:          "데이터 분석가 (Julie Zhuo)",
			Role:          "analytics",
			SignatureLens: []string{"connect recommendations to product quality"},
		},
	}

	prompt := buildModeratorUserPrompt(input)
	if !strings.Contains(prompt, "next speaker signature lens") {
		t.Fatalf("expected next speaker lens in moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "product quality") {
		t.Fatalf("expected Julie Zhuo lens, prompt=%q", prompt)
	}
}

func TestBuildFinalModeratorUserPromptIncludesFinalStatus(t *testing.T) {
	input := orchestrator.GenerateFinalModeratorInput{
		Problem: "리텐션 개선",
		Personas: []persona.Persona{
			{ID: "p1", Name: "고객 경험/운영 리드 (Ron Kohavi)", Role: "operations"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerName: "A", Type: orchestrator.TurnTypePersona, Content: "의견"},
			{Index: 1, SpeakerName: orchestrator.ModeratorSpeakerName, Type: orchestrator.TurnTypeModerator, Content: "정리"},
		},
		Consensus: orchestrator.Consensus{
			Reached:   true,
			Score:     0.91,
			Summary:   "핵심 가설과 실행안에 합의함",
			Rationale: "실험 우선순위가 정렬됨",
		},
		FinalStatus: orchestrator.StatusConsensusReached,
	}

	prompt := buildFinalModeratorUserPrompt(input)
	if !strings.Contains(prompt, "status: "+orchestrator.StatusConsensusReached) {
		t.Fatalf("expected final status in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "consensus score: 0.91") {
		t.Fatalf("expected consensus score in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "judge rationale") {
		t.Fatalf("expected rationale in prompt, prompt=%q", prompt)
	}
}
