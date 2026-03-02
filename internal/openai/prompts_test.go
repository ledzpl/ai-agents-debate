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
	if !strings.Contains(prompt, "DECISION_CHECK using this exact structure") {
		t.Fatalf("expected fixed decision-check guidance in moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "metric_threshold and decide_by must both be concrete values") {
		t.Fatalf("expected concrete decision-check values guidance in moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Moderator cadence signals:") {
		t.Fatalf("expected moderator cadence signal section, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "strongest-form summary of an opposing view") {
		t.Fatalf("expected fair opposing-view summary guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "evidence_type=data|experience|assumption") || !strings.Contains(prompt, "confidence=low|medium|high") {
		t.Fatalf("expected evidence-quality gate guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "SELF_CHECK: <likely bias/failure mode> -> <mitigation in this turn>") {
		t.Fatalf("expected persona self-check guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "ISSUE_UPDATE: <issue>") {
		t.Fatalf("expected unresolved issue registry guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Do not emit ISSUE_UPDATE or SELF_CHECK when nothing changed") {
		t.Fatalf("expected selective metadata emission guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Deadlock breaker") || !strings.Contains(prompt, "OPTION_A:") || !strings.Contains(prompt, "OPTION_B:") {
		t.Fatalf("expected deadlock decision-table guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "META_DELTA: changed=") {
		t.Fatalf("expected periodic meta summary guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "unresolved blockers <=1") || !strings.Contains(prompt, "unowned issues = 0") || !strings.Contains(prompt, "decide_by signals >=1") {
		t.Fatalf("expected quantitative close criteria guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "without bracket labels") {
		t.Fatalf("expected non-tagged evidence/inference guidance, prompt=%q", prompt)
	}
	if strings.Contains(prompt, "Tag each core claim as [evidence], [inference], or [assumption].") {
		t.Fatalf("did not expect forced bracket label taxonomy, prompt=%q", prompt)
	}
	if strings.Contains(strings.ToLower(prompt), "steelman") {
		t.Fatalf("did not expect explicit steelman token in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "last two turns") {
		t.Fatalf("expected repeat guardrail for two-turn window, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "NEXT: <persona_id>") {
		t.Fatalf("expected explicit next speaker line format, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "HANDOFF_ASK:") {
		t.Fatalf("expected explicit handoff ask control line, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "DECISION_CHECK: choose Option A or B") {
		t.Fatalf("expected fixed decision-check format, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "no TBD/unknown/later/soon") {
		t.Fatalf("expected non-placeholder decision-check values guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "metric_threshold must be numeric or explicit condition") {
		t.Fatalf("expected metric-threshold specificity guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "SCORECARD: coherence=<0-100>; executability=<0-100>; risk_coverage=<0-100>") {
		t.Fatalf("expected periodic scorecard rubric guidance, prompt=%q", prompt)
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
			Reached:                 true,
			Score:                   0.91,
			Summary:                 "핵심 가설과 실행안에 합의함",
			Rationale:               "실험 우선순위가 정렬됨",
			OpenRisks:               []string{"모니터링 임계치 미확정"},
			NextActionOwner:         "SRE",
			NextActionTrigger:       "오늘 EOD",
			NextActionSuccessMetric: "롤백 트리거 문서 반영",
			RequiredNextAction:      "SRE가 롤백 트리거를 오늘 확정",
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
	if !strings.Contains(prompt, "open risks:") {
		t.Fatalf("expected open risks in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "required next action") {
		t.Fatalf("expected required next action in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "next action owner") || !strings.Contains(prompt, "next action success metric") {
		t.Fatalf("expected structured next action fields in prompt, prompt=%q", prompt)
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
			{Index: 4, SpeakerID: "p1", SpeakerName: "Growth Lead", Type: orchestrator.TurnTypePersona, Content: "속도를 유지하되 블로커 임계치를 명시합시다."},
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
	if !strings.Contains(prompt, "persona failure-mode watch:") {
		t.Fatalf("expected persona failure-mode watch in speaker profile, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "Progress signals:") {
		t.Fatalf("expected progress signals section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "close readiness snapshot: unresolved_blockers=") {
		t.Fatalf("expected close-readiness snapshot in prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Debate phase:") || !strings.Contains(prompt, "current phase:") {
		t.Fatalf("expected debate phase guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decision-forcing handoff question") {
		t.Fatalf("expected decision-forcing handoff objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "choose one provisional option and include owner + trigger/deadline") {
		t.Fatalf("expected convergence phase decision guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "without bracket labels like [evidence] or [inference]") {
		t.Fatalf("expected anti-labeling guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "quality checkpoint required now") || !strings.Contains(prompt, "evidence_type=data|experience|assumption") {
		t.Fatalf("expected quality-checkpoint guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "include ISSUE_UPDATE only when opening a new issue or when owner/deadline/blocker changes") {
		t.Fatalf("expected conditional issue registry guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "include SELF_CHECK when bias/confidence risk is material") {
		t.Fatalf("expected conditional self-check guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "keep narrative human-readable, and keep machine metadata lines standalone") {
		t.Fatalf("expected narrative/metadata separation guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "metadata labels are machine-readable control data") {
		t.Fatalf("expected metadata non-display policy guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "periodic meta-summary turn") {
		t.Fatalf("expected periodic meta-summary cadence trigger, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "META_DELTA with changed/unchanged/next_question") {
		t.Fatalf("expected meta delta requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "HANDOFF_ASK:") || !strings.Contains(prompt, "NEXT: <persona_id>") {
		t.Fatalf("expected explicit control lines for handoff, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "CLOSE: yes|no") || !strings.Contains(prompt, "NEW_POINT: yes|no") {
		t.Fatalf("expected explicit close/new-point objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "unresolved_blockers<=1") || !strings.Contains(prompt, "unowned_issues=0") || !strings.Contains(prompt, "decide_by_signals>=1") {
		t.Fatalf("expected quantitative close readiness rule, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "last two-turn claims") {
		t.Fatalf("expected repeat guardrail in turn objective, prompt=%q", prompt)
	}
}

func TestBuildTurnUserPromptConvergenceIncludesDecisionTemplate(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "가격 정책 확정",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
			{ID: "p2", Name: "Finance", Role: "finance"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "옵션 A를 검토합시다."},
			{Index: 2, SpeakerID: "p2", SpeakerName: "Finance", Type: orchestrator.TurnTypePersona, Content: "옵션 B가 수익성에 유리합니다."},
			{Index: 3, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "리스크는 CAC 상승입니다."},
			{Index: 4, SpeakerID: "p2", SpeakerName: "Finance", Type: orchestrator.TurnTypePersona, Content: "마진 임계치를 정의합시다."},
		},
		Speaker: persona.Persona{
			ID:   "p1",
			Name: "PM",
			Role: "product",
		},
	}

	prompt := buildTurnUserPrompt(input)
	if !strings.Contains(prompt, "current phase: convergence") {
		t.Fatalf("expected convergence phase, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "choose one provisional option and include owner + trigger/deadline") {
		t.Fatalf("expected convergence decision template, prompt=%q", prompt)
	}
}

func TestBuildTurnUserPromptDeadlockModeWhenNoNewPointStreak(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "이탈률 개선",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
			{ID: "p2", Name: "Data", Role: "analytics"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "옵션 A를 유지합시다.\nNEW_POINT: no"},
			{Index: 2, SpeakerID: "p2", SpeakerName: "Data", Type: orchestrator.TurnTypePersona, Content: "옵션 B를 보완합시다.\nNEW_POINT: no"},
			{Index: 3, SpeakerID: orchestrator.ModeratorSpeakerID, SpeakerName: orchestrator.ModeratorSpeakerName, Type: orchestrator.TurnTypeModerator, Content: "교착을 깨기 위한 비교가 필요합니다."},
		},
		Speaker: persona.Persona{
			ID:   "p1",
			Name: "PM",
			Role: "product",
		},
	}

	prompt := buildTurnUserPrompt(input)
	if !strings.Contains(prompt, "trailing persona NEW_POINT=no streak: 2") {
		t.Fatalf("expected no-new-point streak summary, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "deadlock signal: repeated no-new-point streak detected") {
		t.Fatalf("expected deadlock signal guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "deadlock mode required now: include OPTION_A/OPTION_B micro decision table") {
		t.Fatalf("expected deadlock mode requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "quality checkpoint required now") {
		t.Fatalf("expected quality checkpoint guidance in deadlock mode, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "issue-state checkpoint required now") {
		t.Fatalf("expected issue checkpoint guidance in deadlock mode, prompt=%q", prompt)
	}
}

func TestBuildTurnUserPromptDoesNotForceQualityCheckpointEveryTurn(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "온보딩 개선",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
			{ID: "p2", Name: "Data", Role: "analytics"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "초기 가설"},
			{Index: 2, SpeakerID: orchestrator.ModeratorSpeakerID, SpeakerName: orchestrator.ModeratorSpeakerName, Type: orchestrator.TurnTypeModerator, Content: "지표 기준을 제시해 주세요."},
		},
		Speaker: persona.Persona{
			ID:   "p1",
			Name: "PM",
			Role: "product",
		},
	}

	prompt := buildTurnUserPrompt(input)
	if strings.Contains(prompt, "quality checkpoint required now") {
		t.Fatalf("did not expect forced quality checkpoint on non-checkpoint turn, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "include evidence-quality clause when confidence/recommendation changes materially") {
		t.Fatalf("expected conditional quality guidance on non-checkpoint turn, prompt=%q", prompt)
	}
}

func TestBuildModeratorUserPromptIncludesScorecardCadenceTrigger(t *testing.T) {
	input := orchestrator.GenerateModeratorInput{
		Problem: "성장 전략",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
			{ID: "p2", Name: "Risk", Role: "risk"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "안 A 제안"},
			{Index: 2, SpeakerID: "p2", SpeakerName: "Risk", Type: orchestrator.TurnTypePersona, Content: "안 B 보완"},
			{Index: 3, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "실험 지표 제안"},
			{Index: 4, SpeakerID: "p2", SpeakerName: "Risk", Type: orchestrator.TurnTypePersona, Content: "가드레일 제안"},
		},
		PreviousTurn: orchestrator.Turn{
			Index:       4,
			SpeakerID:   "p2",
			SpeakerName: "Risk",
			Type:        orchestrator.TurnTypePersona,
			Content:     "가드레일 제안",
		},
		NextSpeaker: persona.Persona{
			ID:   "p1",
			Name: "PM",
			Role: "product",
		},
	}

	prompt := buildModeratorUserPrompt(input)
	if !strings.Contains(prompt, "persona turns observed so far: 4") {
		t.Fatalf("expected persona turn count in cadence signals, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "include SCORECARD + SCORECARD_REASON in this intervention") {
		t.Fatalf("expected scorecard cadence trigger, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "If evidence is mixed or insufficient, prefer reached=false") {
		t.Fatalf("expected conservative fallback rule, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "at least two different speakers/turns") {
		t.Fatalf("expected rationale evidence requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "open_risks") || !strings.Contains(prompt, "next_action_owner") {
		t.Fatalf("expected expanded judge output schema, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "next_action_trigger_or_deadline") {
		t.Fatalf("expected next action calibration guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "summary: exactly 1 sentence") || !strings.Contains(prompt, "open_risks: 0-3 items") {
		t.Fatalf("expected compact output-length and risk-count constraints, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "exact order") || !strings.Contains(prompt, "single-line minified JSON object") {
		t.Fatalf("expected strict json formatting guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "final character must be }") {
		t.Fatalf("expected json closing brace requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Never omit required keys") || !strings.Contains(prompt, "next_action_owner: \"unassigned\"") {
		t.Fatalf("expected fallback placeholder guidance for missing fields, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Self-repair before final output") {
		t.Fatalf("expected malformed-json self-repair guidance, prompt=%q", prompt)
	}
}

func TestBuildJudgeUserPromptIncludesFormatReminder(t *testing.T) {
	prompt := buildJudgeUserPrompt(orchestrator.JudgeConsensusInput{
		Problem: "릴리즈 Go/No-Go 결정",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
		},
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "PM", Type: orchestrator.TurnTypePersona, Content: "기능 범위를 축소하면 가능"},
		},
	})
	if !strings.Contains(prompt, "Output format reminder:") {
		t.Fatalf("expected output format reminder section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "return one minified JSON object on a single line only") {
		t.Fatalf("expected minified single-line guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "key order: reached, score, summary, rationale, open_risks") {
		t.Fatalf("expected key order guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "never omit keys; if uncertain, use placeholders") {
		t.Fatalf("expected placeholder fallback reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "final character must be }") {
		t.Fatalf("expected explicit final brace guidance, prompt=%q", prompt)
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

func TestDebatePhaseSwitchesByTurnWindow(t *testing.T) {
	if got := debatePhase(1, 3); got != "exploration" {
		t.Fatalf("expected exploration phase, got %s", got)
	}
	if got := debatePhase(6, 3); got != "convergence" {
		t.Fatalf("expected convergence phase, got %s", got)
	}
}

func TestSummarizeCloseReadinessCapsStandaloneDecideBySignals(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "DECISION_CHECK: choose Option A or B; metric_threshold>=2%; decide_by=2026-03-10",
		},
		{
			Index:       2,
			SpeakerID:   "p2",
			SpeakerName: "SRE",
			Type:        orchestrator.TurnTypePersona,
			Content:     "추가 검증 필요. decide_by=2026-03-11",
		},
	}

	summary := summarizeCloseReadiness(turns)
	if summary.decideBySignals != 1 {
		t.Fatalf("expected capped decide_by signal=1, got %d", summary.decideBySignals)
	}
}

func TestSummarizeCloseReadinessMergesIssueUpdatesByIssue(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "ISSUE_UPDATE: payment-rollout | owner=unassigned | deadline=2026-03-15 | blocker=fraud rules",
		},
		{
			Index:       2,
			SpeakerID:   "p2",
			SpeakerName: "Risk",
			Type:        orchestrator.TurnTypePersona,
			Content:     "ISSUE_UPDATE: payment-rollout | owner=risk-lead",
		},
	}

	summary := summarizeCloseReadiness(turns)
	if summary.unownedIssues != 0 {
		t.Fatalf("expected owner update to clear unowned issue, got %d", summary.unownedIssues)
	}
	if summary.unresolvedBlockers != 1 {
		t.Fatalf("expected blocker state to persist across partial issue updates, got %d", summary.unresolvedBlockers)
	}
}

func TestBuildJudgeUserPromptIncludesDecisionStateSnapshot(t *testing.T) {
	prompt := buildJudgeUserPrompt(orchestrator.JudgeConsensusInput{
		Problem: "릴리즈 의사결정",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
			{ID: "p2", Name: "Risk", Role: "risk"},
		},
		Turns: []orchestrator.Turn{
			{
				Index:       1,
				SpeakerID:   "p1",
				SpeakerName: "PM",
				Type:        orchestrator.TurnTypePersona,
				Content:     "ISSUE_UPDATE: release-window | owner=pm | deadline=2026-03-20 | blocker=none",
			},
			{
				Index:       2,
				SpeakerID:   "p2",
				SpeakerName: "Risk",
				Type:        orchestrator.TurnTypePersona,
				Content:     "DECISION_CHECK: choose Option A or B; metric_threshold=p95<300ms; decide_by=2026-03-19",
			},
		},
	})

	if !strings.Contains(prompt, "Decision-state snapshot:") {
		t.Fatalf("expected decision-state snapshot section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "issue registry:") || !strings.Contains(prompt, "release-window: owner=pm; deadline=2026-03-20; blocker=none") {
		t.Fatalf("expected issue registry summary in judge prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decide_by signal outside issue registry: present") {
		t.Fatalf("expected standalone decide_by signal summary in judge prompt, prompt=%q", prompt)
	}
}
