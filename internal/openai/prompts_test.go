package openai

import (
	"fmt"
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
	if !strings.Contains(prompt, "Ask for persuasion accounting") {
		t.Fatalf("expected persuasion-accounting guidance in moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "smallest discriminating experiment with owner + decide_by + success_metric + stop_condition") {
		t.Fatalf("expected uncertainty-reduction experiment guidance in moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Moderator cadence signals:") {
		t.Fatalf("expected moderator cadence signal section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "trailing persona NEW_POINT=no streak:") {
		t.Fatalf("expected no-new-point streak signal in moderator prompt, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "Adapt explanation depth to audience_mode from the user prompt") {
		t.Fatalf("expected audience-mode adaptive guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "technical terms to <=3") {
		t.Fatalf("expected relaxed technical-term limit guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "language-neutral way") {
		t.Fatalf("expected language-neutral sentence-length guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Include one plain-language user-impact sentence") {
		t.Fatalf("expected user-impact sentence guidance, prompt=%q", prompt)
	}
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
	if !strings.Contains(prompt, "best user outcome under explicit constraints") {
		t.Fatalf("expected explicit optimization-frame guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Keep one stable optimization frame unless new evidence justifies changing it") {
		t.Fatalf("expected optimization-frame stability guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "PERSUASION_UPDATE: changed=yes|no") {
		t.Fatalf("expected persuasion state-update control line guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "SELF_CHECK: <likely bias/failure mode> -> <mitigation in this turn>") {
		t.Fatalf("expected persona self-check guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "ISSUE_UPDATE: <issue>") {
		t.Fatalf("expected unresolved issue registry guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "ISSUE_UPDATE quality rule") || !strings.Contains(prompt, "no TBD/unknown/later/soon") {
		t.Fatalf("expected issue-update placeholder guardrail guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Do not emit ISSUE_UPDATE or SELF_CHECK when nothing changed") {
		t.Fatalf("expected selective metadata emission guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Deadlock breaker") || !strings.Contains(prompt, "OPTION_A:") || !strings.Contains(prompt, "OPTION_B:") {
		t.Fatalf("expected deadlock decision-table guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "smallest discriminating experiment with owner + decide_by + success_metric + stop_condition") {
		t.Fatalf("expected deadlock experiment guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "META_DELTA: changed=") {
		t.Fatalf("expected periodic meta summary guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "unresolved blockers <=1") || !strings.Contains(prompt, "unowned issues = 0") || !strings.Contains(prompt, "decide_by signals >=1") {
		t.Fatalf("expected quantitative close criteria guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "avoid non-index bracket labels") {
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
	if !strings.Contains(prompt, "Do not translate or rename machine control labels") {
		t.Fatalf("expected control-label non-translation guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Self-repair before final output") {
		t.Fatalf("expected self-repair guidance for required control lines, prompt=%q", prompt)
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
	if !strings.Contains(systemPrompt, "Ignore candidates whose id is empty") {
		t.Fatalf("expected empty-id candidate handling guidance, prompt=%q", systemPrompt)
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

func TestBuildOpeningSpeakerSelectorUserPromptSkipsEmptyIDCandidates(t *testing.T) {
	userPrompt := buildOpeningSpeakerSelectorUserPrompt(orchestrator.SelectOpeningSpeakerInput{
		Problem: "오프닝 화자 선택",
		Personas: []persona.Persona{
			{Name: "Unnamed", Role: "generalist"},
			{ID: "risk", Name: "Risk", Role: "risk"},
		},
	})

	if strings.Contains(userPrompt, "- id: \n") {
		t.Fatalf("did not expect empty-id candidate line, prompt=%q", userPrompt)
	}
	if !strings.Contains(userPrompt, "Ignored candidates with empty id: 1") {
		t.Fatalf("expected skipped-empty-id summary, prompt=%q", userPrompt)
	}
	if !strings.Contains(userPrompt, "Allowed persona_id values: risk") {
		t.Fatalf("expected allowed-id list to include only non-empty ids, prompt=%q", userPrompt)
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
	if !strings.Contains(prompt, "Force persuasion accounting") {
		t.Fatalf("expected persuasion-accounting principle in moderator system prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Required line format and order") {
		t.Fatalf("expected fixed moderator line-order guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Optional disambiguation lines") {
		t.Fatalf("expected optional option-line guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "PERSUASION_CHECK: adopted_from=") {
		t.Fatalf("expected optional persuasion-check line guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "The 4 required lines remain mandatory") {
		t.Fatalf("expected required-line preservation guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "SYNTHESIS:") || !strings.Contains(prompt, "TENSION:") || !strings.Contains(prompt, "ASK:") {
		t.Fatalf("expected explicit SYNTHESIS/TENSION/ASK line prefixes, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "smallest discriminating experiment with owner, deadline, success metric, and stop condition") {
		t.Fatalf("expected uncertainty-blocker experiment guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "SCORECARD: coherence=<0-100>; executability=<0-100>; risk_coverage=<0-100>") {
		t.Fatalf("expected periodic scorecard rubric guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Adapt wording depth to audience_mode") {
		t.Fatalf("expected audience-mode adaptation guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Do not translate or rename required labels") {
		t.Fatalf("expected moderator label non-translation guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Self-repair before final output") {
		t.Fatalf("expected moderator self-repair guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Do not add prose before SYNTHESIS or after final metadata line") {
		t.Fatalf("expected strict moderator boundary guidance, prompt=%q", prompt)
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

func TestFindLatestModeratorAskPreservesStructuredModeratorLines(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerName: orchestrator.ModeratorSpeakerName,
			SpeakerID:   orchestrator.ModeratorSpeakerID,
			Type:        orchestrator.TurnTypeModerator,
			Content: strings.Join([]string{
				"SYNTHESIS: 속도와 안정성의 균형이 필요합니다.",
				"TENSION: 성장 속도와 리스크 허용치가 충돌합니다.",
				"ASK: 어떤 가드레일을 우선 적용할까요?",
				"DECISION_CHECK: choose Option A or B; metric_threshold=p95<300ms; decide_by=2026-03-15",
			}, "\n"),
		},
	}

	ask := findLatestModeratorAsk(turns, 220)
	if ask == "" {
		t.Fatalf("expected structured moderator content to remain in summary")
	}
	if !strings.Contains(ask, "속도와 안정성의 균형") {
		t.Fatalf("expected moderator synthesis payload to be preserved, ask=%q", ask)
	}
	if strings.Contains(strings.ToUpper(ask), "SYNTHESIS:") {
		t.Fatalf("did not expect moderator control label to remain in summary, ask=%q", ask)
	}
}

func TestBuildModeratorLoopStatusSkipsEmptyPersonaResponse(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerName: orchestrator.ModeratorSpeakerName,
			SpeakerID:   orchestrator.ModeratorSpeakerID,
			Type:        orchestrator.TurnTypeModerator,
			Content:     "SYNTHESIS: 리스크를 정리합시다.\nASK: 우선순위는?",
		},
		{
			Index:       2,
			SpeakerName: "A",
			SpeakerID:   "a",
			Type:        orchestrator.TurnTypePersona,
			Content:     "HANDOFF_ASK: 다음은?\nNEXT: b\nCLOSE: no\nNEW_POINT: no",
		},
		{
			Index:       3,
			SpeakerName: "B",
			SpeakerID:   "b",
			Type:        orchestrator.TurnTypePersona,
			Content:     "실제 답변입니다. 우선 로그 모니터링부터 강화해야 합니다.",
		},
	}

	status := buildModeratorLoopStatus(turns, 220)
	if strings.Contains(status, "A: ") {
		t.Fatalf("did not expect empty summarized response from speaker A, status=%q", status)
	}
	if !strings.Contains(status, "B: 실제 답변입니다.") {
		t.Fatalf("expected first non-empty persona response to be selected, status=%q", status)
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
	if !strings.Contains(prompt, "next action decide_by") {
		t.Fatalf("expected decide_by label in final moderator user prompt, prompt=%q", prompt)
	}
}

func TestBuildFinalModeratorUserPromptCompressesLogTailByBudget(t *testing.T) {
	personas := []persona.Persona{
		{ID: "p1", Name: "PM", Role: "product"},
		{ID: "p2", Name: "Risk", Role: "risk"},
		{ID: "p3", Name: "Data", Role: "analytics"},
		{ID: "p4", Name: "Ops", Role: "operations"},
		{ID: "p5", Name: "Eng", Role: "engineering"},
		{ID: "p6", Name: "Finance", Role: "finance"},
		{ID: "p7", Name: "Legal", Role: "compliance"},
		{ID: "p8", Name: "Growth", Role: "growth"},
		{ID: "p9", Name: "Design", Role: "ux"},
		{ID: "p10", Name: "SRE", Role: "risk"},
	}
	budget := derivePromptBudget(len(personas), 15)
	totalTurns := budget.judgeRecentLogLimit + 4

	turns := make([]orchestrator.Turn, 0, totalTurns)
	for i := 1; i <= totalTurns; i++ {
		turns = append(turns, orchestrator.Turn{
			Index:       i,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "turn content",
		})
	}

	prompt := buildFinalModeratorUserPrompt(orchestrator.GenerateFinalModeratorInput{
		Problem:   "최종 결정",
		Personas:  personas,
		Turns:     turns,
		Consensus: orchestrator.Consensus{Score: 0.7},
	})

	if strings.Contains(prompt, "[1][PM][persona]") {
		t.Fatalf("expected compressed final log tail to drop earliest turns, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, fmt.Sprintf("[%d][PM][persona]", totalTurns)) {
		t.Fatalf("expected compressed final log tail to keep latest turn, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "Optimization frame:") {
		t.Fatalf("expected optimization frame section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "optimize for best user outcome under explicit constraints") {
		t.Fatalf("expected optimization objective guidance in turn prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decision-forcing handoff question") {
		t.Fatalf("expected decision-forcing handoff objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "choose one provisional option and include owner + decide_by") {
		t.Fatalf("expected convergence phase decision guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "requested audience_mode: general") {
		t.Fatalf("expected default general audience mode indicator, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "audience mode: explain so a non-expert can follow quickly") {
		t.Fatalf("expected non-expert audience-mode guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "avoid unexplained jargon or acronyms") {
		t.Fatalf("expected jargon/acronym guardrail, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "what changes for users if this is chosen") {
		t.Fatalf("expected explicit user-impact guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "when claim reference is clear and verifiable") {
		t.Fatalf("expected softened citation requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "CLOSE decision must use the snapshot above as source of truth") {
		t.Fatalf("expected snapshot-first close gating guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "persuasion gate: before CLOSE=yes") {
		t.Fatalf("expected persuasion gate guidance for close, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "quality checkpoint required now") || !strings.Contains(prompt, "evidence_type=data|experience|assumption") {
		t.Fatalf("expected quality-checkpoint guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "include ISSUE_UPDATE only when opening a new issue or when owner/decide_by/blocker changes") {
		t.Fatalf("expected conditional issue registry guidance in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "keep decide_by/blocker concrete (no TBD/unknown/later/soon)") {
		t.Fatalf("expected decide_by/blocker placeholder guardrail in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "periodic meta-summary turn") {
		t.Fatalf("expected periodic meta-summary cadence trigger, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "META_DELTA with changed/unchanged/next_question") {
		t.Fatalf("expected meta delta requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "PERSUASION_UPDATE: changed=yes|no") {
		t.Fatalf("expected explicit persuasion update control line in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "HANDOFF_ASK:") || !strings.Contains(prompt, "NEXT: <persona_id>") {
		t.Fatalf("expected explicit control lines for handoff, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "CLOSE: yes|no") || !strings.Contains(prompt, "NEW_POINT: yes|no") {
		t.Fatalf("expected explicit close/new-point objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "do not translate or rename any control-line label") {
		t.Fatalf("expected label non-translation reminder in turn objective, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "unresolved_blockers<=1") || !strings.Contains(prompt, "unowned_issues=0") || !strings.Contains(prompt, "decide_by_signals>=1") {
		t.Fatalf("expected quantitative close readiness rule, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "choose one provisional option and include owner + decide_by") {
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
	if !strings.Contains(prompt, "deadlock mode required now: apply the system deadlock breaker (OPTION_A/OPTION_B table)") {
		t.Fatalf("expected deadlock mode requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "quality checkpoint required now") {
		t.Fatalf("expected quality checkpoint guidance in deadlock mode, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "issue-state checkpoint required now") {
		t.Fatalf("expected issue checkpoint guidance in deadlock mode, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "deadlock experiment required now: propose the smallest discriminating experiment") {
		t.Fatalf("expected deadlock experiment requirement in deadlock mode, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "persuasion checkpoint required now") {
		t.Fatalf("expected persuasion checkpoint requirement in deadlock mode, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "trailing persona NEW_POINT=no streak:") {
		t.Fatalf("expected no-new-point streak signal in moderator cadence, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "Persuasion quality: reached=true requires at least one explicit cross-persona adoption/concession") {
		t.Fatalf("expected persuasion quality gate for reached=true, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Optimality check: chosen direction must dominate alternatives on objective/constraints OR include a concrete uncertainty-reduction experiment") {
		t.Fatalf("expected optimality check guidance in judge prompt, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "summary: exactly 1 sentence; keep it short in a language-neutral way") || !strings.Contains(prompt, "open_risks: 0-3 items") {
		t.Fatalf("expected language-neutral compact output-length and risk-count constraints, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "exact order") || !strings.Contains(prompt, "single-line minified JSON object") {
		t.Fatalf("expected strict json formatting guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "final character must be }") {
		t.Fatalf("expected json closing brace requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Never omit required keys") || !strings.Contains(prompt, "next_action_owner: \"moderator\"") {
		t.Fatalf("expected fallback concrete-default guidance for missing fields, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Avoid placeholder values in next_action fields") {
		t.Fatalf("expected anti-placeholder next_action guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "rationale must state who was persuaded (or not), what changed, and why") {
		t.Fatalf("expected persuasion-change rationale requirement, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "reached=true only when key objections are resolved by evidence or by a concrete, bounded experiment plan") {
		t.Fatalf("expected reached=true guardrail with bounded experiment plan, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Self-repair before final output") {
		t.Fatalf("expected malformed-json self-repair guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "JSON type constraints: reached must be unquoted true/false") {
		t.Fatalf("expected json primitive-type constraints, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "JSON template: {\"reached\":false") {
		t.Fatalf("expected json template guidance, prompt=%q", prompt)
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
	if !strings.Contains(prompt, "requested audience_mode: general") {
		t.Fatalf("expected default general audience mode in judge prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "return one minified JSON object on a single line only") {
		t.Fatalf("expected minified single-line guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "key order: reached, score, summary, rationale, open_risks") {
		t.Fatalf("expected key order guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "never omit keys; if uncertain, use conservative concrete defaults") {
		t.Fatalf("expected concrete-default fallback reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "avoid placeholder values in next_action fields") {
		t.Fatalf("expected anti-placeholder reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "reached=true requires at least one explicit persuasion/concession event plus resolved key objections") {
		t.Fatalf("expected persuasion/concession reminder in judge user prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "rationale must mention who changed what") {
		t.Fatalf("expected rationale-change reminder in judge user prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "type constraints: reached is boolean, score is numeric 0..1") {
		t.Fatalf("expected type constraint reminder, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "final character must be }") {
		t.Fatalf("expected explicit final brace guidance, prompt=%q", prompt)
	}
}

func TestBuildTurnUserPromptSkipsEmptySpeakerFields(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "가격 정책 실험",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
			{ID: "p2", Name: "Risk", Role: "risk"},
		},
		Speaker: persona.Persona{
			ID:          "p1",
			Name:        "PM",
			Role:        "product",
			Stance:      "   ",
			Style:       " ",
			Expertise:   []string{"  ", "pricing"},
			Constraints: []string{"", "법적 검토 필요"},
		},
	}

	prompt := buildTurnUserPrompt(input)
	if strings.Contains(prompt, "- stance: \n") {
		t.Fatalf("did not expect blank stance field, prompt=%q", prompt)
	}
	if strings.Contains(prompt, "- style: \n") {
		t.Fatalf("did not expect blank style field, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "- expertise: pricing") {
		t.Fatalf("expected trimmed expertise list, prompt=%q", prompt)
	}
	if strings.Contains(prompt, "- expertise: ,") {
		t.Fatalf("did not expect empty expertise item, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "  - 법적 검토 필요") {
		t.Fatalf("expected trimmed constraints item, prompt=%q", prompt)
	}
	if strings.Contains(prompt, "  - \n") {
		t.Fatalf("did not expect blank constraints item, prompt=%q", prompt)
	}
}

func TestParticipantPromptLineOmitsEmptyRoleNoise(t *testing.T) {
	line := participantPromptLine(persona.Persona{
		ID:   "p1",
		Name: "PM",
		Role: "   ",
	})
	if strings.Contains(line, ": ") {
		t.Fatalf("did not expect empty role suffix, line=%q", line)
	}
	if !strings.Contains(line, "PM (p1)") {
		t.Fatalf("expected name and id, line=%q", line)
	}
}

func TestBuildFinalModeratorSystemPromptIsDecisionOriented(t *testing.T) {
	prompt := buildFinalModeratorSystemPrompt()
	if !strings.Contains(prompt, "3-5 concise sentences") {
		t.Fatalf("expected final response length guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "First sentence must be a plain-language verdict") {
		t.Fatalf("expected plain-language first sentence guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "what/who/when format") {
		t.Fatalf("expected concrete what/who/when action guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "audience_mode=general, avoid unexplained acronyms/jargon") {
		t.Fatalf("expected general-mode anti-jargon guidance in final moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "audience_mode=expert, precise terminology is allowed") {
		t.Fatalf("expected expert-mode terminology allowance in final moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "Do not introduce new facts beyond the provided debate and judge context") {
		t.Fatalf("expected no-new-facts guardrail in final moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "consensus score/rationale as confidence calibration") {
		t.Fatalf("expected calibration guidance, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decision-oriented concluding sentence") {
		t.Fatalf("expected decision-oriented ending guidance, prompt=%q", prompt)
	}
}

func TestBuildTurnUserPromptExpertAudienceMode(t *testing.T) {
	input := orchestrator.GenerateTurnInput{
		Problem: "SLO 개선 전략",
		Personas: []persona.Persona{
			{ID: "p1", Name: "SRE", Role: "risk"},
			{ID: "p2", Name: "PM", Role: "product"},
		},
		Speaker: persona.Persona{
			ID:   "p1",
			Name: "SRE",
			Role: "risk",
		},
		AudienceMode: orchestrator.AudienceModeExpert,
	}

	prompt := buildTurnUserPrompt(input)
	if !strings.Contains(prompt, "requested audience_mode: expert") {
		t.Fatalf("expected expert audience mode indicator, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "audience mode: explain for expert readers with precise terminology and compact logic") {
		t.Fatalf("expected expert audience guidance, prompt=%q", prompt)
	}
	if strings.Contains(prompt, "audience mode: explain so a non-expert can follow quickly") {
		t.Fatalf("did not expect general audience guidance in expert mode, prompt=%q", prompt)
	}
}

func TestBuildFinalModeratorUserPromptIncludesAudienceMode(t *testing.T) {
	input := orchestrator.GenerateFinalModeratorInput{
		Problem: "릴리즈 의사결정",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
		},
		AudienceMode: orchestrator.AudienceModeExpert,
	}

	prompt := buildFinalModeratorUserPrompt(input)
	if !strings.Contains(prompt, "requested audience_mode: expert") {
		t.Fatalf("expected audience mode indicator in final moderator prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "expert mode: concise and precise closing summary") {
		t.Fatalf("expected expert-mode final guidance, prompt=%q", prompt)
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

func TestSummarizeCloseReadinessIgnoresPlaceholderDecideBySignals(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "DECISION_CHECK: choose Option A or B; metric_threshold>=2%; decide_by=later",
		},
		{
			Index:       2,
			SpeakerID:   "p2",
			SpeakerName: "SRE",
			Type:        orchestrator.TurnTypePersona,
			Content:     "추가 검증 필요. decide_by=미정",
		},
		{
			Index:       3,
			SpeakerID:   "p3",
			SpeakerName: "Risk",
			Type:        orchestrator.TurnTypePersona,
			Content:     "리스크 검토 후 공유. deadline=soon",
		},
	}

	summary := summarizeCloseReadiness(turns)
	if summary.decideBySignals != 0 {
		t.Fatalf("expected placeholder decide_by/deadline values to be ignored, got %d", summary.decideBySignals)
	}
}

func TestSummarizeCloseReadinessCountsStandaloneDeadlineDirective(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "DECISION_CHECK: choose Option A or B; metric_threshold>=2%; deadline=2026-03-12",
		},
	}

	summary := summarizeCloseReadiness(turns)
	if summary.decideBySignals != 1 {
		t.Fatalf("expected standalone deadline signal to count as decide_by signal, got %d", summary.decideBySignals)
	}
}

func TestExtractDirectiveValueRequiresDirectiveBoundary(t *testing.T) {
	if got := extractDirectiveValue("context not_decide_by=2026-03-10", "decide_by="); got != "" {
		t.Fatalf("expected no extraction from embedded token, got %q", got)
	}
	if got := extractDirectiveValue("context not_decide_by=2026-03-10; decide_by=2026-03-11", "decide_by="); got != "2026-03-11" {
		t.Fatalf("expected extraction from explicit directive token, got %q", got)
	}
}

func TestSummarizeCloseReadinessParsesMarkdownPrefixedIssueUpdate(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "- ISSUE_UPDATE: launch-window | owner=pm | decide_by=2026-03-10 | blocker=none",
		},
	}

	summary := summarizeCloseReadiness(turns)
	if summary.unownedIssues != 0 {
		t.Fatalf("expected markdown-prefixed issue update to parse owner, got %d", summary.unownedIssues)
	}
	if summary.unresolvedBlockers != 0 {
		t.Fatalf("expected markdown-prefixed issue update to parse blocker, got %d", summary.unresolvedBlockers)
	}
	if summary.decideBySignals != 1 {
		t.Fatalf("expected markdown-prefixed issue update to parse decide_by, got %d", summary.decideBySignals)
	}
}

func TestExtractDirectiveValueStopsBeforeFollowingDirectiveToken(t *testing.T) {
	if got := extractDirectiveValue("DECISION_CHECK: choose Option A; decide_by=soon blocker=none", "decide_by="); got != "soon" {
		t.Fatalf("expected decide_by value to stop before following directive token, got %q", got)
	}
	if got := extractDirectiveValue("DECISION_CHECK: decide_by=2026-03-10 17:00 blocker=none", "decide_by="); got != "2026-03-10 17:00" {
		t.Fatalf("expected datetime with space to be preserved, got %q", got)
	}
}

func TestTrailingNoNewPointStreakParsesMarkdownPrefixedDirective(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "1. NEW_POINT: no",
		},
		{
			Index:       2,
			SpeakerID:   "p2",
			SpeakerName: "Risk",
			Type:        orchestrator.TurnTypePersona,
			Content:     "> NEW_POINT=no",
		},
	}

	if got := trailingNoNewPointStreak(turns); got != 2 {
		t.Fatalf("expected markdown-prefixed NEW_POINT directives to be parsed, got %d", got)
	}
}

func TestExtractDirectiveValuePreservesURLTokenInValue(t *testing.T) {
	line := "DECISION_CHECK: decide_by=2026-03-10 https://runbook.example.com/ops blocker=none"
	if got := extractDirectiveValue(line, "decide_by="); got != "2026-03-10 https://runbook.example.com/ops" {
		t.Fatalf("expected URL token to remain part of decide_by value, got %q", got)
	}
}

func TestExtractDirectiveValuePreservesCommaInNaturalLanguageDate(t *testing.T) {
	line := "DECISION_CHECK: decide_by=March 10, 2026 blocker=none"
	if got := extractDirectiveValue(line, "decide_by="); got != "March 10, 2026" {
		t.Fatalf("expected comma-containing date value to remain intact, got %q", got)
	}
}

func TestExtractDirectiveValueStopsAtCommaDelimitedDirectiveToken(t *testing.T) {
	line := "DECISION_CHECK: decide_by=soon, blocker=none"
	if got := extractDirectiveValue(line, "decide_by="); got != "soon" {
		t.Fatalf("expected decide_by to stop at comma-delimited directive token, got %q", got)
	}
}

func TestExtractDirectiveValueStopsAtCommaDelimitedDirectiveTokenWithoutSpace(t *testing.T) {
	line := "DECISION_CHECK: decide_by=soon,blocker=none"
	if got := extractDirectiveValue(line, "decide_by="); got != "soon" {
		t.Fatalf("expected decide_by to stop at compact comma-delimited directive token, got %q", got)
	}
}

func TestIsPlaceholderValueHandlesWrappedVariants(t *testing.T) {
	cases := []string{
		"(soon)",
		"(unknown)",
		"<trigger>)",
		"{deadline}",
	}
	for _, tc := range cases {
		if !isPlaceholderValue(tc) {
			t.Fatalf("expected wrapped/template placeholder %q to be treated as placeholder", tc)
		}
	}
	if isPlaceholderValue("2026-03-10 17:00") {
		t.Fatalf("did not expect concrete datetime to be placeholder")
	}
}

func TestSummarizeTurnContentStripsMarkdownPrefixedDirectiveLines(t *testing.T) {
	content := strings.Join([]string{
		"> ISSUE_UPDATE: launch-window | owner=pm | decide_by=2026-03-10 | blocker=none",
		"- PERSUASION_UPDATE: changed=yes; adopted=risk guardrail; rationale=incident data; remaining_gap=rollout scope",
		"- DECISION_CHECK: choose Option A or B; metric_threshold>=2%; decide_by=2026-03-12",
		"1. SCORECARD: coherence=80; executability=70; risk_coverage=75",
		"실제 사용자 영향: 초기 전환율은 소폭 개선되지만 리스크 모니터링이 필요합니다.",
	}, "\n")

	summary := summarizeTurnContent(content, 200)
	if strings.Contains(strings.ToUpper(summary), "ISSUE_UPDATE") ||
		strings.Contains(strings.ToUpper(summary), "PERSUASION_UPDATE") ||
		strings.Contains(strings.ToUpper(summary), "DECISION_CHECK") ||
		strings.Contains(strings.ToUpper(summary), "SCORECARD") {
		t.Fatalf("expected directive-prefixed lines to be stripped from summary, got %q", summary)
	}
	if !strings.Contains(summary, "실제 사용자 영향") {
		t.Fatalf("expected non-directive narrative to remain in summary, got %q", summary)
	}
}

func TestSummarizeTurnContentControlOnlyReturnsEmpty(t *testing.T) {
	content := strings.Join([]string{
		"HANDOFF_ASK: 다음 화자가 검증할 핵심 지표는?",
		"NEXT: p2",
		"CLOSE: no",
		"NEW_POINT: no",
	}, "\n")

	if got := summarizeTurnContent(content, 120); got != "" {
		t.Fatalf("expected control-only content to summarize as empty, got %q", got)
	}
}

func TestSummarizeTurnContentDoesNotStripYearLikeSentencePrefix(t *testing.T) {
	content := "2026. 3월까지 실험 완료 후 결과를 공유합니다."
	got := summarizeTurnContent(content, 120)
	if !strings.Contains(got, "2026. 3월까지") {
		t.Fatalf("expected year-like sentence prefix to remain, got %q", got)
	}
}

func TestBuildJudgeDecisionStateSnapshotTruncatesIssueRegistryByBudget(t *testing.T) {
	turns := make([]orchestrator.Turn, 0, judgeSnapshotIssueLimit+3)
	for i := 1; i <= judgeSnapshotIssueLimit+3; i++ {
		turns = append(turns, orchestrator.Turn{
			Index:       i,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     fmt.Sprintf("ISSUE_UPDATE: issue-%02d | owner=pm | decide_by=2026-03-%02d | blocker=none", i, i),
		})
	}

	snapshot := buildJudgeDecisionStateSnapshot(turns)
	if !strings.Contains(snapshot, "more issues omitted for prompt budget") {
		t.Fatalf("expected truncated snapshot notice, snapshot=%q", snapshot)
	}
	if got := strings.Count(snapshot, "owner="); got != judgeSnapshotIssueLimit {
		t.Fatalf("expected exactly %d issue lines after truncation, got %d", judgeSnapshotIssueLimit, got)
	}
}

func TestBuildJudgeDecisionStateSnapshotPrioritizesUnresolvedAndRecentIssues(t *testing.T) {
	turns := make([]orchestrator.Turn, 0, judgeSnapshotIssueLimit+2)
	for i := 1; i <= judgeSnapshotIssueLimit+1; i++ {
		turns = append(turns, orchestrator.Turn{
			Index:       i,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     fmt.Sprintf("ISSUE_UPDATE: issue-%02d | owner=pm | decide_by=2026-03-%02d | blocker=none", i, i),
		})
	}
	turns = append(turns, orchestrator.Turn{
		Index:       judgeSnapshotIssueLimit + 2,
		SpeakerID:   "p2",
		SpeakerName: "Risk",
		Type:        orchestrator.TurnTypePersona,
		Content:     "ISSUE_UPDATE: z-urgent-risk | owner=unassigned | decide_by=tbd | blocker=security review pending",
	})

	snapshot := buildJudgeDecisionStateSnapshot(turns)
	if !strings.Contains(snapshot, "z-urgent-risk: owner=unassigned; decide_by=tbd; blocker=security review pending") {
		t.Fatalf("expected unresolved/recent issue to be prioritized into capped snapshot, snapshot=%q", snapshot)
	}
}

func TestBuildJudgeDecisionStateSnapshotIncludesPersuasionAndExperimentSignals(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "ISSUE_UPDATE: rollout | owner=pm | decide_by=2026-03-20 | blocker=none",
		},
		{
			Index:       2,
			SpeakerID:   "p2",
			SpeakerName: "Risk",
			Type:        orchestrator.TurnTypePersona,
			Content:     "PERSUASION_UPDATE: changed=yes; adopted=가드레일 우선 적용; rationale=장애 비용 근거; remaining_gap=실험 범위 확정",
		},
		{
			Index:       3,
			SpeakerID:   "p3",
			SpeakerName: "Data",
			Type:        orchestrator.TurnTypePersona,
			Content:     "DECISION_CHECK: choose Option A or B; owner=data; decide_by=2026-03-18; success_metric=p95<300ms; stop_condition=error_rate>1%",
		},
	}

	snapshot := buildJudgeDecisionStateSnapshot(turns)
	if !strings.Contains(snapshot, "persuasion adoption signals: 1") {
		t.Fatalf("expected persuasion adoption signal count in snapshot, snapshot=%q", snapshot)
	}
	if !strings.Contains(snapshot, "persuasion remaining gaps signaled: 1") {
		t.Fatalf("expected persuasion remaining-gap signal count in snapshot, snapshot=%q", snapshot)
	}
	if !strings.Contains(snapshot, "bounded experiment signal: present") {
		t.Fatalf("expected bounded experiment signal in snapshot, snapshot=%q", snapshot)
	}
}

func TestBuildModeratorMemorySnapshotShowsAnchorFallbackAfterFiltering(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "HANDOFF_ASK: 다음 액션?\nNEXT: p2\nCLOSE: no\nNEW_POINT: no",
		},
		{
			Index:       2,
			SpeakerID:   "p2",
			SpeakerName: "Risk",
			Type:        orchestrator.TurnTypePersona,
			Content:     "ISSUE_UPDATE: risk | owner=risk | decide_by=2026-03-12 | blocker=none",
		},
	}

	out := buildModeratorMemorySnapshot(turns, turns[len(turns)-1], defaultModeratorMemoryBudget())
	if !strings.Contains(out, "anchor turns before latest:") {
		t.Fatalf("expected anchor section header, output=%q", out)
	}
	if !strings.Contains(out, "none after control-line filtering") {
		t.Fatalf("expected anchor fallback notice after filtering, output=%q", out)
	}
}

func TestBuildJudgeUserPromptShowsFilteredNoneWhenDebateTailHasOnlyControlLines(t *testing.T) {
	prompt := buildJudgeUserPrompt(orchestrator.JudgeConsensusInput{
		Problem: "릴리즈 의사결정",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
		},
		Turns: []orchestrator.Turn{
			{
				Index:       1,
				SpeakerID:   "p1",
				SpeakerName: "PM",
				Type:        orchestrator.TurnTypePersona,
				Content:     "HANDOFF_ASK: 다음 액션?\nNEXT: p1\nCLOSE: no\nNEW_POINT: no",
			},
		},
	})

	if !strings.Contains(prompt, "- none after control-line filtering.") {
		t.Fatalf("expected filtered-none fallback in judge debate tail, prompt=%q", prompt)
	}
}

func TestBuildFinalModeratorUserPromptShowsFilteredNoneWhenLogTailHasOnlyControlLines(t *testing.T) {
	prompt := buildFinalModeratorUserPrompt(orchestrator.GenerateFinalModeratorInput{
		Problem: "최종 정리",
		Personas: []persona.Persona{
			{ID: "p1", Name: "PM", Role: "product"},
		},
		Turns: []orchestrator.Turn{
			{
				Index:       1,
				SpeakerID:   "p1",
				SpeakerName: "PM",
				Type:        orchestrator.TurnTypePersona,
				Content:     "HANDOFF_ASK: 다음 액션?\nNEXT: p1\nCLOSE: no\nNEW_POINT: no",
			},
		},
	})

	if !strings.Contains(prompt, "Final debate log tail:\n- none after control-line filtering.") {
		t.Fatalf("expected filtered-none fallback in final debate log tail, prompt=%q", prompt)
	}
}

func TestSummarizeCloseReadinessMergesIssueUpdatesByIssue(t *testing.T) {
	turns := []orchestrator.Turn{
		{
			Index:       1,
			SpeakerID:   "p1",
			SpeakerName: "PM",
			Type:        orchestrator.TurnTypePersona,
			Content:     "ISSUE_UPDATE: payment-rollout | owner=unassigned | decide_by=2026-03-15 | blocker=fraud rules",
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
				Content:     "ISSUE_UPDATE: release-window | owner=pm | decide_by=2026-03-20 | blocker=none",
			},
			{
				Index:       2,
				SpeakerID:   "p2",
				SpeakerName: "Risk",
				Type:        orchestrator.TurnTypePersona,
				Content:     "PERSUASION_UPDATE: changed=yes; adopted=릴리즈 창 축소; rationale=장애 리스크 근거; remaining_gap=none",
			},
			{
				Index:       3,
				SpeakerID:   "p2",
				SpeakerName: "Risk",
				Type:        orchestrator.TurnTypePersona,
				Content:     "DECISION_CHECK: choose Option A or B; metric_threshold=p95<300ms; owner=risk; decide_by=2026-03-19; success_metric=에러율 1% 미만; stop_condition=에러율 1% 초과",
			},
		},
	})

	if !strings.Contains(prompt, "Decision-state snapshot:") {
		t.Fatalf("expected decision-state snapshot section, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "issue registry:") || !strings.Contains(prompt, "release-window: owner=pm; decide_by=2026-03-20; blocker=none") {
		t.Fatalf("expected issue registry summary in judge prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "decide_by signal outside issue registry: present") {
		t.Fatalf("expected standalone decide_by signal summary in judge prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "persuasion adoption signals: 1") {
		t.Fatalf("expected persuasion adoption signal summary in judge prompt, prompt=%q", prompt)
	}
	if !strings.Contains(prompt, "bounded experiment signal: present") {
		t.Fatalf("expected bounded experiment signal summary in judge prompt, prompt=%q", prompt)
	}
}
