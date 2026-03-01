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
			{ID: "p1", Name: "그로스 PM", MasterName: "Brian Balfour", Role: "growth", SignatureLens: []string{"frame ideas as growth loops"}},
			{ID: "p2", Name: "UX 리서처 / 서비스 디자이너", MasterName: "Nir Eyal", Role: "ux"},
		},
		Speaker: persona.Persona{
			ID:            "p1",
			Name:          "그로스 PM",
			MasterName:    "Brian Balfour",
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
	if !strings.Contains(prompt, "master_name: Brian Balfour") {
		t.Fatalf("expected master_name guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "master usage requirement") {
		t.Fatalf("expected master usage requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "master_name=Brian Balfour") {
		t.Fatalf("expected participant master_name listing, prompt=%q", prompt)
	}
}

func TestBuildModeratorUserPromptIncludesNextSpeakerLens(t *testing.T) {
	input := orchestrator.GenerateModeratorInput{
		Problem: "리텐션 개선",
		Personas: []persona.Persona{
			{ID: "p1", Name: "데이터 분석가", MasterName: "Julie Zhuo", Role: "analytics", SignatureLens: []string{"connect recommendations to product quality"}},
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
			Name:          "데이터 분석가",
			MasterName:    "Julie Zhuo",
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
	if !strings.Contains(prompt, "next speaker master_name: Julie Zhuo") {
		t.Fatalf("expected next speaker master_name in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "ask the next speaker to use ideas from this master's books, papers, or articles") {
		t.Fatalf("expected moderator master instruction, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Moderator balancing guidance") {
		t.Fatalf("expected balancing guidance section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "one data point, not the whole debate") {
		t.Fatalf("expected anti-recency guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Recent debate log:") {
		t.Fatalf("expected recent log section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Debate memory snapshot (anti-recency):") {
		t.Fatalf("expected memory snapshot section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "window turns considered:") {
		t.Fatalf("expected memory window summary, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "latest claim per speaker") {
		t.Fatalf("expected speaker claim memory, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Moderator loop status:") {
		t.Fatalf("expected moderator loop status section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "previous moderator ask:") {
		t.Fatalf("expected previous moderator ask summary, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "first response after that ask:") {
		t.Fatalf("expected moderator ask response summary, prompt=%q", prompt)
	}
}

func TestBuildTurnSystemPromptMentionsMasterKnowledgeSources(t *testing.T) {
	prompt := buildTurnSystemPrompt()
	if !strings.Contains(prompt, "books, papers, and articles") {
		t.Fatalf("expected master knowledge source guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "do not invent specific titles/dates") {
		t.Fatalf("expected anti-hallucination guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "core claim -> reason/mechanism -> practical implication") {
		t.Fatalf("expected argument structure guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "falsifiable statements") {
		t.Fatalf("expected specificity guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "answer it in your first sentence") {
		t.Fatalf("expected moderator-question-first guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "NEXT: <persona_id>") {
		t.Fatalf("expected explicit next speaker line format, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "CLOSE: yes|no") || !strings.Contains(prompt, "NEW_POINT: yes|no") {
		t.Fatalf("expected explicit termination signals, prompt=%q", prompt)
	}
}

func TestBuildOpeningSpeakerSelectorPrompts(t *testing.T) {
	systemPrompt := buildOpeningSpeakerSelectorSystemPrompt()
	if !strings.Contains(systemPrompt, "persona_id") {
		t.Fatalf("expected persona_id requirement, prompt=%q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "exactly one JSON object") {
		t.Fatalf("expected strict json output rule, prompt=%q", systemPrompt)
	}

	userPrompt := buildOpeningSpeakerSelectorUserPrompt(orchestrator.SelectOpeningSpeakerInput{
		Problem: "결제 장애를 줄이려면 무엇부터 해야 하나요?",
		Personas: []persona.Persona{
			{ID: "pm", Name: "PM", Role: "prioritization"},
			{ID: "sre", Name: "SRE", Role: "incident response", Expertise: []string{"on-call", "postmortem"}},
		},
	})
	if !strings.Contains(userPrompt, "Candidates:") {
		t.Fatalf("expected candidates section, prompt=%q", userPrompt)
	}
	if !strings.Contains(userPrompt, "id: sre") {
		t.Fatalf("expected persona id listing, prompt=%q", userPrompt)
	}
	if !strings.Contains(userPrompt, "incident response") {
		t.Fatalf("expected role context, prompt=%q", userPrompt)
	}
}

func TestBuildModeratorSystemPromptReducesRecencyBias(t *testing.T) {
	prompt := buildModeratorSystemPrompt()
	if !strings.Contains(prompt, "Avoid recency bias") {
		t.Fatalf("expected anti-recency instruction, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "multiple recent turns") {
		t.Fatalf("expected multi-turn synthesis guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "supporting point and one tension/tradeoff") {
		t.Fatalf("expected balance requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Debate memory snapshot") {
		t.Fatalf("expected memory snapshot grounding, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "synthesis -> unresolved tradeoff -> targeted next-speaker question") {
		t.Fatalf("expected moderator structure guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Do not introduce external facts") {
		t.Fatalf("expected no-external-facts guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "specific prior claim (speaker + idea)") {
		t.Fatalf("expected explicit handoff claim targeting, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decision-forcing") {
		t.Fatalf("expected decision-forcing moderator question guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Close the loop on your previous intervention") {
		t.Fatalf("expected moderator loop-closing guidance, prompt=%q", prompt)
	}
}

func TestBuildModeratorUserPromptIncludesMemoryAnchorsAndTension(t *testing.T) {
	input := orchestrator.GenerateModeratorInput{
		Problem: "신규 기능 론칭 전략",
		Personas: []persona.Persona{
			{ID: "p1", Name: "Growth Lead", Role: "growth"},
			{ID: "p2", Name: "Risk Analyst", Role: "risk"},
			{ID: "p3", Name: "PM", Role: "product"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerName: "Growth Lead", SpeakerID: "p1", Type: orchestrator.TurnTypePersona, Content: "빠른 실험으로 시장 반응을 확인해야 합니다."},
			{Index: 1, SpeakerName: orchestrator.ModeratorSpeakerName, SpeakerID: orchestrator.ModeratorSpeakerID, Type: orchestrator.TurnTypeModerator, Content: "속도와 리스크 균형이 필요합니다."},
			{Index: 2, SpeakerName: "Risk Analyst", SpeakerID: "p2", Type: orchestrator.TurnTypePersona, Content: "가드레일 없이 론칭하면 리스크가 큽니다."},
			{Index: 2, SpeakerName: orchestrator.ModeratorSpeakerName, SpeakerID: orchestrator.ModeratorSpeakerID, Type: orchestrator.TurnTypeModerator, Content: "측정 지표를 명확히 합시다."},
			{Index: 3, SpeakerName: "Growth Lead", SpeakerID: "p1", Type: orchestrator.TurnTypePersona, Content: "초기에는 완화된 가드레일로 속도를 확보합시다."},
		},
		PreviousTurn: orchestrator.Turn{
			Index:       3,
			SpeakerName: "Growth Lead",
			SpeakerID:   "p1",
			Type:        orchestrator.TurnTypePersona,
			Content:     "초기에는 완화된 가드레일로 속도를 확보합시다.",
		},
		NextSpeaker: persona.Persona{
			ID:   "p2",
			Name: "Risk Analyst",
			Role: "risk",
		},
	}

	prompt := buildModeratorUserPrompt(input)
	if !strings.Contains(prompt, "anchor turns before latest") {
		t.Fatalf("expected anchor turn section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Growth Lead:") || !strings.Contains(prompt, "Risk Analyst:") {
		t.Fatalf("expected multi-speaker claims in memory snapshot, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "tension candidate:") {
		t.Fatalf("expected tension candidate in memory snapshot, prompt=%q", prompt)
	}
}

func TestBuildFinalModeratorUserPromptIncludesFinalStatus(t *testing.T) {
	input := orchestrator.GenerateFinalModeratorInput{
		Problem: "리텐션 개선",
		Personas: []persona.Persona{
			{ID: "p1", Name: "고객 경험/운영 리드", MasterName: "Ron Kohavi", Role: "operations"},
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

func TestBuildTurnUserPromptIncludesInteractionSnapshotAndObjectives(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "신규 요금제 실험 전략",
		Personas: []persona.Persona{
			{ID: "p1", Name: "Growth Lead", Role: "growth"},
			{ID: "p2", Name: "Risk Analyst", Role: "risk"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "Growth Lead", Type: orchestrator.TurnTypePersona, Content: "빠르게 실험해서 전환 개선을 확인합시다."},
			{Index: 2, SpeakerID: orchestrator.ModeratorSpeakerID, SpeakerName: orchestrator.ModeratorSpeakerName, Type: orchestrator.TurnTypeModerator, Content: "리스크 가드레일을 포함한 최소 실험안을 제시해 주세요."},
			{Index: 3, SpeakerID: "p2", SpeakerName: "Risk Analyst", Type: orchestrator.TurnTypePersona, Content: "가드레일 없이 실험하면 리스크가 큽니다."},
		},
		Speaker: persona.Persona{
			ID:   "p1",
			Name: "Growth Lead",
			Role: "growth",
		},
	}

	prompt := buildTurnUserPrompt(input)
	if !strings.Contains(prompt, "Interaction memory snapshot:") {
		t.Fatalf("expected interaction memory section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "your latest claim:") {
		t.Fatalf("expected own latest claim reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "most recent peer claim:") {
		t.Fatalf("expected peer claim reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "active tension candidate:") {
		t.Fatalf("expected tension candidate reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "latest moderator ask:") {
		t.Fatalf("expected latest moderator ask reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Turn objective:") {
		t.Fatalf("expected turn objective section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "targeted handoff question/request") {
		t.Fatalf("expected explicit handoff objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "final line must be: NEXT: <persona_id>") {
		t.Fatalf("expected explicit next-speaker objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "CLOSE: yes|no") || !strings.Contains(prompt, "NEW_POINT: yes|no") {
		t.Fatalf("expected explicit close/new-point objective, prompt=%q", prompt)
	}
}

func TestBuildJudgeSystemPromptHasConservativeRubric(t *testing.T) {
	prompt := buildJudgeSystemPrompt()
	if !strings.Contains(prompt, "Be conservative: set reached=true only if there is clear alignment") {
		t.Fatalf("expected conservative reached=true rule, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "0.90-1.00: workable consensus") {
		t.Fatalf("expected score rubric upper band, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "at least two different speakers/turns") {
		t.Fatalf("expected rationale evidence requirement, prompt=%q", prompt)
	}
}

func TestBuildFinalModeratorSystemPromptIsDecisionOriented(t *testing.T) {
	prompt := buildFinalModeratorSystemPrompt()
	if !strings.Contains(prompt, "3-5 concise sentences") {
		t.Fatalf("expected final response length guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "consensus score/rationale as confidence calibration") {
		t.Fatalf("expected calibration guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decision-oriented concluding sentence") {
		t.Fatalf("expected decision-oriented ending guidance, prompt=%q", prompt)
	}
}

func TestDerivePromptBudgetCompressesOnLongDebates(t *testing.T) {
	base := derivePromptBudget(3, 4)
	compressed := derivePromptBudget(10, 64)

	if compressed.turnRecentLogLimit >= base.turnRecentLogLimit {
		t.Fatalf("expected compressed turn log window, base=%d compressed=%d", base.turnRecentLogLimit, compressed.turnRecentLogLimit)
	}
	if compressed.turnLogSummaryRunes >= base.turnLogSummaryRunes {
		t.Fatalf("expected compressed turn summary runes, base=%d compressed=%d", base.turnLogSummaryRunes, compressed.turnLogSummaryRunes)
	}
	if compressed.moderatorRecentLogLimit >= base.moderatorRecentLogLimit {
		t.Fatalf("expected compressed moderator log window, base=%d compressed=%d", base.moderatorRecentLogLimit, compressed.moderatorRecentLogLimit)
	}
	if compressed.judgeRecentLogLimit >= base.judgeRecentLogLimit {
		t.Fatalf("expected compressed judge log window, base=%d compressed=%d", base.judgeRecentLogLimit, compressed.judgeRecentLogLimit)
	}
}
