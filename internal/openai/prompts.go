package openai

import (
	"fmt"
	"sort"
	"strings"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

const (
	turnPromptRecentLogLimit      = 10
	turnPromptSpeakerClaims       = 5
	turnPromptLogSummaryRunes     = 180
	moderatorPromptLogSummaryRune = 200
	judgePromptLogSummaryRunes    = 220
	issuePlaceholderGuardrail     = "no TBD/unknown/later/soon"
	nextActionPlaceholderRule     = "no TBD/unknown/later/soon/next cycle"
)

type promptBudget struct {
	turnRecentLogLimit        int
	turnSpeakerClaims         int
	turnLogSummaryRunes       int
	interactionSummaryRunes   int
	moderatorRecentLogLimit   int
	moderatorLogSummaryRunes  int
	moderatorMemory           moderatorMemoryBudget
	judgeRecentLogLimit       int
	judgeLogSummaryRunes      int
	moderatorLoopSummaryRunes int
}

func derivePromptBudget(personaCount int, turnCount int) promptBudget {
	level := derivePromptCompressionLevel(personaCount, turnCount)
	return promptBudget{
		turnRecentLogLimit:        shrinkInt(turnPromptRecentLogLimit, 2*level, 4),
		turnSpeakerClaims:         shrinkInt(turnPromptSpeakerClaims, level, 3),
		turnLogSummaryRunes:       shrinkInt(turnPromptLogSummaryRunes, 20*level, 100),
		interactionSummaryRunes:   shrinkInt(moderatorClaimSummaryRunes, 12*level, 72),
		moderatorRecentLogLimit:   shrinkInt(moderatorRecentLogLimit, 2*level, 4),
		moderatorLogSummaryRunes:  shrinkInt(moderatorPromptLogSummaryRune, 24*level, 120),
		moderatorMemory:           deriveModeratorMemoryBudget(level),
		judgeRecentLogLimit:       shrinkInt(24, 4*level, 10),
		judgeLogSummaryRunes:      shrinkInt(judgePromptLogSummaryRunes, 24*level, 120),
		moderatorLoopSummaryRunes: shrinkInt(moderatorClaimSummaryRunes, 12*level, 72),
	}
}

func derivePromptCompressionLevel(personaCount int, turnCount int) int {
	level := 0
	if turnCount >= 12 {
		level++
	}
	if turnCount >= 24 {
		level++
	}
	if turnCount >= 40 {
		level++
	}
	if personaCount >= 8 {
		level++
	}
	return level
}

func shrinkInt(base int, reduce int, min int) int {
	if min < 1 {
		min = 1
	}
	v := base - reduce
	if v < min {
		return min
	}
	return v
}

func buildTurnSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are a specialized persona in a high-stakes multi-persona debate. Your goal is to drive the discussion toward a rigorous, evidence-based decision through constructive friction.

### CORE STRUCTURE (Follow this sequence)
1.  **Targeted Response**: Address the moderator's request or a peer's specific claim in your first sentence.
2.  **Opponent's Best Argument**: Before your rebuttal, fairly summarize the strongest version of an opposing view. Perform this task implicitly without naming the technique.
3.  **New Insight**: Add ONE materially new insight (evidence, constraint, or failure mode). Do not restate prior turns.
4.  **Verification Condition**: State one concrete condition that would prove your current position WRONG.
5.  **User Impact**: One plain-language sentence on why this choice matters to the end user.

### INTERACTION RULES
- **Language**: Respond EXCLUSIVELY in the same language as the problem statement.
- **No Jargon Bleed**: DO NOT use English meta-terms like "steel-man", "falsifiability", or "delta" in your prose. Respond naturally in the target language.
- **Audience Mode**: 
  - general: Everyday analogies; technical terms <=3 with short explanations.
  - expert: High density; use precise domain terminology and architectural patterns.
- **Identity Guardrail**: Use the master_name as style/knowledge inspiration; do not claim to be that person.
- **Citations**: Cite prior turns using [Index] when referencing specific data or claims.

### RESPONSE FORMAT (STRICT)
- **Narrative Only**: The main body must contain ONLY natural language. DO NOT embed metadata keys or control labels inside your sentences.
- **No Preamble**: Start directly with your response.
- **Technical Control Block**: Place all required control lines at the absolute end of your response, each on a new line.

### MACHINE-READABLE CONTROLS (At the end)
- **ISSUE_UPDATE**: <issue> | owner=<id> | decide_by=<trigger> | blocker=<item> (Include ONLY when state changes).
- **SELF_CHECK**: <bias detected> -> <mitigation> (Required only every 4th turn).
- **META_DELTA**: changed=<what>; unchanged=<what>; next_question=<question> (Required only every 4th turn).

### TERMINAL COMMANDS (Required for engine)
HANDOFF_ASK: <one concrete question for the NEXT speaker>
NEXT: <persona_id>
CLOSE: yes|no
NEW_POINT: yes|no

### SELF-VERIFICATION STEP
Before outputting, verify:
1. Did I answer the moderator?
2. Did I cite at least one turn index [Index]?
3. Is my prose free of ALL metadata labels and English jargon?
4. Are my terminal commands at the absolute bottom?`)
}

func buildOpeningSpeakerSelectorSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are an expert facilitator responsible for selecting the ideal opening speaker for a multi-persona debate.

### OBJECTIVE
Pick the single most relevant persona to frame the discussion. Prioritize Domain Fit and Strategic Framing.

### OUTPUT FORMAT (STRICT)
- Return exactly one MINIFIED JSON object: {"persona_id": "matched_id"}
- No markdown, no prose, no code blocks.`)
}

func buildOpeningSpeakerSelectorUserPrompt(input orchestrator.SelectOpeningSpeakerInput) string {
	var b strings.Builder
	b.WriteString("<problem>\n")
	b.WriteString(input.Problem)
	b.WriteString("\n</problem>\n\n<candidates>\n")
	for _, p := range input.Personas {
		id := strings.TrimSpace(p.ID)
		b.WriteString(fmt.Sprintf("- ID: %s\n", id))
		b.WriteString(fmt.Sprintf("  Role: %s\n", p.Role))
		if stance := strings.TrimSpace(p.Stance); stance != "" {
			b.WriteString("  Stance: " + stance + "\n")
		}
		if len(p.Expertise) > 0 {
			b.WriteString("  Expertise: " + strings.Join(p.Expertise, ", ") + "\n")
		}
		if master := strings.TrimSpace(p.MasterName); master != "" {
			b.WriteString("  Master: " + master + "\n")
		}
	}
	b.WriteString("</candidates>\n\nSelect the best opening speaker persona_id now.")
	return b.String()
}

func buildTurnUserPrompt(input orchestrator.GenerateTurnInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	phase := debatePhase(len(input.Turns), len(input.Personas))
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

	var b strings.Builder
	b.WriteString("<context>\n")
	b.WriteString("Problem: " + input.Problem + "\n")
	b.WriteString("Audience: " + audienceMode + "\n")
	b.WriteString("Phase: " + phase + "\n")
	b.WriteString("</context>\n\n")

	b.WriteString("<current_persona>\n")
	b.WriteString(fmt.Sprintf("- ID: %s\n- Name: %s\n- Role: %s\n", input.Speaker.ID, input.Speaker.Name, input.Speaker.Role))
	if stance := strings.TrimSpace(input.Speaker.Stance); stance != "" {
		b.WriteString("- Stance: " + stance + "\n")
	}
	if master := strings.TrimSpace(input.Speaker.MasterName); master != "" {
		b.WriteString("- Master Inspiration: " + master + " (Apply their specific frameworks)\n")
	}
	if style := strings.TrimSpace(input.Speaker.Style); style != "" {
		b.WriteString("- Style: " + style + "\n")
	}
	if len(input.Speaker.Expertise) > 0 {
		b.WriteString("- Expertise: " + strings.Join(input.Speaker.Expertise, ", ") + "\n")
	}
	if sigLens := normalizePromptList(input.Speaker.SignatureLens); len(sigLens) > 0 {
		b.WriteString("- Signature Lens: " + strings.Join(sigLens, ", ") + "\n")
	}
	if len(input.Speaker.Constraints) > 0 {
		b.WriteString("- Constraints: " + strings.Join(input.Speaker.Constraints, "; ") + "\n")
	}
	b.WriteString("- Failure-Mode: " + derivePersonaFailureMode(input.Speaker) + "\n")
	b.WriteString("</current_persona>\n\n")

	b.WriteString("<debate_log>\n")
	if len(input.Turns) == 0 {
		b.WriteString("- Initial Turn.\n")
	} else {
		for _, t := range trimTurns(input.Turns, budget.turnRecentLogLimit) {
			b.WriteString(fmt.Sprintf("[%d][%s] %s\n", t.Index, t.SpeakerName, summarizeTurnContent(t.Content, budget.turnLogSummaryRunes)))
		}
	}
	b.WriteString("</debate_log>\n\n")

	b.WriteString("<interaction_memory>\n")
	b.WriteString(buildTurnInteractionSnapshot(input.Turns, input.Speaker, budget))
	b.WriteString("</interaction_memory>\n\n")

	personaTurns := countPersonaTurns(input.Turns)
	noNewPointStreak := trailingNoNewPointStreak(input.Turns)
	closeReadiness := summarizeCloseReadiness(input.Turns)

	b.WriteString("<signals>\n")
	b.WriteString(fmt.Sprintf("- Turn No: %d\n", personaTurns+1))
	b.WriteString(fmt.Sprintf("- Stagnation Streak: %d\n", noNewPointStreak))
	b.WriteString(fmt.Sprintf("- Readiness: blockers=%d, unowned=%d\n", closeReadiness.unresolvedBlockers, closeReadiness.unownedIssues))
	if noNewPointStreak >= 2 {
		b.WriteString("- ACTION: Forced Deadlock Breaker required.\n")
	}
	b.WriteString("</signals>\n\n")

	b.WriteString("Provide your utterance following system rules.")
	return b.String()
}

func buildJudgeSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are a strict consensus judge. Your goal is to determine if the debate has produced a workable decision or a clear, well-defined disagreement.

### JUDGING CRITERIA
1. **Consensus Quality**: Alignment on goal, approach, and next step.
2. **Disagreement Clarity**: If reached=false, identify the exact blocking gap (data or value conflict).
3. **Evidence Grounding**: Cite at least two [Index] markers.
4. **Actionability**: Next actions must be concrete, assigned, and time-bound.

### OUTPUT STRUCTURE (STRICT JSON)
- Return exactly one MINIFIED JSON object on a single line.
- No markdown code fences.
- No placeholders like "TBD/soon".
- Keys: reached, score, summary, rationale, open_risks, next_action_owner, next_action_trigger_or_deadline, next_action_success_metric.`)
}

func buildJudgeUserPrompt(input orchestrator.JudgeConsensusInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	judgeTurns := trimTurns(input.Turns, budget.judgeRecentLogLimit)
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

	var b strings.Builder
	b.WriteString("<problem>\n" + input.Problem + "\n</problem>\n\n")
	b.WriteString("<debate_log>\n")
	for _, t := range judgeTurns {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, budget.judgeLogSummaryRunes)))
	}
	b.WriteString("</debate_log>\n\n")
	b.WriteString("<decision_state>\n")
	b.WriteString(buildJudgeDecisionStateSnapshot(judgeTurns))
	b.WriteString("</decision_state>\n\n")
	b.WriteString("<task>\n- Audience: " + audienceMode + "\n- Return minified JSON now.\n</task>")
	return b.String()
}

func buildModeratorSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are the moderator. Your goal is to sharpen the debate by exposing hidden tensions and forcing choice.

### MANDATORY STRUCTURE (Exactly 4 lines)
1. **SYNTHESIS**: 1 short sentence on current trajectory.
2. **CONFLICT SHARPENING**: Identify the EXACT point of disagreement (cite [Index]).
3. **ASK**: A decision-forcing question for the next speaker.
4. **DECISION_CHECK**: choose Option A or B; metric_threshold=<concrete>; decide_by=<trigger>.

### RULES
- **Anti-Recency**: Weight foundation turns equally with the latest statement.
- **Response Style**: No preamble, no postamble, no markdown.
- **Metric Quality**: threshold must be numeric or a binary testable condition.

### CONSTRAINTS
- Labels must be exact and uppercase.
- No TBD/unknown in DECISION_CHECK.`)
}

func buildModeratorUserPrompt(input orchestrator.GenerateModeratorInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	personaTurnCount := countPersonaTurns(input.Turns)
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

	var b strings.Builder
	b.WriteString("<problem>\n" + input.Problem + "\n</problem>\n\n")
	b.WriteString("<recent_log>\n")
	recentTurns := trimTurns(input.Turns, budget.moderatorRecentLogLimit)
	for _, t := range recentTurns {
		b.WriteString(fmt.Sprintf("[%d][%s] %s\n", t.Index, t.SpeakerName, summarizeTurnContent(t.Content, budget.moderatorLogSummaryRunes)))
	}
	b.WriteString("</recent_log>\n\n")
	b.WriteString("<memory_snapshot>\n")
	b.WriteString(buildModeratorMemorySnapshot(recentTurns, input.PreviousTurn, budget.moderatorMemory))
	b.WriteString("</memory_snapshot>\n\n")
	b.WriteString("<task>\n- Audience: " + audienceMode + "\n- Persona Turns: " + fmt.Sprintf("%d", personaTurnCount) + "\n- Provide moderator intervention now.\n</task>")
	return b.String()
}

func buildFinalModeratorSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are the closing moderator. Your goal is to provide a definitive wrap-up of the entire debate.

### CORE STRUCTURE (Follow this sequence)
1.  **The Verdict**: A single plain-language sentence explaining the final outcome for a general audience.
2.  **Synthesis**: 2-3 sentences covering major agreements, unresolved risks, and the logic behind the final path.
3.  **Action Plan**: One concrete sentence in "Who/What/When" format based on the judge's next action.
4.  **Final Statement**: A decision-oriented concluding sentence.

### RESPONSE FORMAT (STRICT)
- **3-5 Sentences Total**: Be extremely concise.
- **No Markdown Headers**: Do not use ### or headers.
- **Incorporate Data**: Use the provided Consensus Score and Rationale to calibrate your tone.

### INTERACTION RULES
- **Language**: Respond EXCLUSIVELY in the same language as the problem statement.
- **Grounding**: Do not invent new facts beyond the provided verdict and log.`)
}

func buildFinalModeratorUserPrompt(input orchestrator.GenerateFinalModeratorInput) string {
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)
	var b strings.Builder
	b.WriteString("<problem>\n" + input.Problem + "\n</problem>\n\n")
	b.WriteString("<verdict_data>\n")
	b.WriteString(fmt.Sprintf("- Consensus: %t (Score: %.2f)\n", input.Consensus.Reached, input.Consensus.Score))
	b.WriteString("- Summary: " + input.Consensus.Summary + "\n")
	b.WriteString("- Rationale: " + input.Consensus.Rationale + "\n")
	if len(input.Consensus.OpenRisks) > 0 {
		b.WriteString("- Remaining Risks: " + strings.Join(input.Consensus.OpenRisks, "; ") + "\n")
	}
	if input.Consensus.NextActionOwner != "" {
		b.WriteString(fmt.Sprintf("- Judge Next Action: [%s] by [%s]\n", input.Consensus.NextActionOwner, input.Consensus.NextActionTrigger))
	}
	b.WriteString("</verdict_data>\n\n")
	b.WriteString("<task>\n- Audience: " + audienceMode + "\n- Provide the final wrap-up assessment now.\n</task>")
	return b.String()
}

func normalizePromptList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePromptAudienceMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case orchestrator.AudienceModeExpert:
		return orchestrator.AudienceModeExpert
	default:
		return orchestrator.AudienceModeGeneral
	}
}

func buildTurnInteractionSnapshot(turns []orchestrator.Turn, speaker persona.Persona, budget promptBudget) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- turns considered: %d\n", len(turns)))
	if len(turns) == 0 {
		b.WriteString("- latest claim per speaker: unavailable\n")
		return b.String()
	}

	claims := collectLatestSpeakerClaims(turns, budget.turnSpeakerClaims, budget.interactionSummaryRunes)
	if len(claims) == 0 {
		b.WriteString("- latest claim per speaker: unavailable\n")
	} else {
		b.WriteString("- latest claim per speaker:\n")
		for _, claim := range claims {
			b.WriteString(fmt.Sprintf("  - %s: %s\n", claim.speaker, claim.claim))
		}
	}

	if ownClaim := findLatestPersonaClaim(turns, speaker, true, budget.interactionSummaryRunes); ownClaim != "" {
		b.WriteString("- your latest claim: " + ownClaim + "\n")
		b.WriteString("- repeat guardrail: do not restate this verbatim; add a new condition, metric, or dependency.\n")
	} else {
		b.WriteString("- your latest claim: none yet\n")
	}

	if peerClaim := findLatestPersonaClaim(turns, speaker, false, budget.interactionSummaryRunes); peerClaim != "" {
		b.WriteString("- most recent peer claim: " + peerClaim + "\n")
	}

	if modAsk := findLatestModeratorAsk(turns, budget.interactionSummaryRunes); modAsk != "" {
		b.WriteString("- latest moderator ask: " + modAsk + "\n")
	}

	latestTurn := turns[len(turns)-1]
	if tension := buildTensionCandidate(claims, latestTurn, budget.moderatorMemory.tensionSummaryRunes); tension != "" {
		b.WriteString("- active tension candidate: " + tension + "\n")
	}
	return b.String()
}

func findLatestModeratorAsk(turns []orchestrator.Turn, summaryRunes int) string {
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if t.Type != orchestrator.TurnTypeModerator {
			continue
		}
		content := summarizeTurnContent(t.Content, summaryRunes)
		if content == "" {
			continue
		}
		return content
	}
	return ""
}

func buildModeratorLoopStatus(turns []orchestrator.Turn, summaryRunes int) string {
	if len(turns) == 0 {
		return "- previous moderator ask: none\n- first response after that ask: n/a\n"
	}

	lastModeratorIdx := -1
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Type == orchestrator.TurnTypeModerator {
			lastModeratorIdx = i
			break
		}
	}
	if lastModeratorIdx < 0 {
		return "- previous moderator ask: none\n- first response after that ask: n/a\n"
	}

	ask := summarizeTurnContent(turns[lastModeratorIdx].Content, summaryRunes)
	if ask == "" {
		ask = "(empty)"
	}

	var response string
	for i := lastModeratorIdx + 1; i < len(turns); i++ {
		if turns[i].Type != orchestrator.TurnTypePersona {
			continue
		}
		speaker := strings.TrimSpace(turns[i].SpeakerName)
		if speaker == "" {
			speaker = strings.TrimSpace(turns[i].SpeakerID)
		}
		response = fmt.Sprintf("%s: %s", speaker, summarizeTurnContent(turns[i].Content, summaryRunes))
		break
	}
	if response == "" {
		response = "not answered yet"
	}

	var b strings.Builder
	b.WriteString("- previous moderator ask: " + ask + "\n")
	b.WriteString("- first response after that ask: " + response + "\n")
	return b.String()
}

type closeReadinessSummary struct {
	unresolvedBlockers int
	unownedIssues      int
	decideBySignals    int
}

type issueState struct {
	issue    string
	owner    string
	decideBy string
	blocker  string
}

type decisionStateSnapshot struct {
	issues                map[string]issueState
	hasStandaloneDecideBy bool
}

func summarizeCloseReadiness(turns []orchestrator.Turn) closeReadinessSummary {
	snapshot := extractDecisionStateSnapshot(turns)

	summary := closeReadinessSummary{}
	for _, state := range snapshot.issues {
		if isOwnerUnassigned(state.owner) {
			summary.unownedIssues++
		}
		if !isNoBlocker(state.blocker) {
			summary.unresolvedBlockers++
		}
		if !isPlaceholderValue(state.decideBy) {
			summary.decideBySignals++
		}
	}
	if snapshot.hasStandaloneDecideBy {
		summary.decideBySignals++
	}
	// CLOSE gating only needs presence of a concrete decide_by signal.
	if summary.decideBySignals > 1 {
		summary.decideBySignals = 1
	}
	return summary
}

func buildJudgeDecisionStateSnapshot(turns []orchestrator.Turn) string {
	snapshot := extractDecisionStateSnapshot(turns)
	var b strings.Builder

	if len(snapshot.issues) == 0 {
		b.WriteString("- issue registry: none\n")
	} else {
		keys := make([]string, 0, len(snapshot.issues))
		for key := range snapshot.issues {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString("- issue registry:\n")
		for _, key := range keys {
			state := snapshot.issues[key]
			owner := strings.TrimSpace(state.owner)
			decideBy := strings.TrimSpace(state.decideBy)
			blocker := strings.TrimSpace(state.blocker)
			if owner == "" {
				owner = "unassigned"
			}
			if decideBy == "" {
				decideBy = "none"
			}
			if blocker == "" {
				blocker = "none"
			}
			b.WriteString(fmt.Sprintf("  - %s: owner=%s; decide_by=%s; blocker=%s\n", state.issue, owner, decideBy, blocker))
		}
	}

	if snapshot.hasStandaloneDecideBy {
		b.WriteString("- decide_by signal outside issue registry: present\n")
	} else {
		b.WriteString("- decide_by signal outside issue registry: none\n")
	}
	return b.String()
}

func extractDecisionStateSnapshot(turns []orchestrator.Turn) decisionStateSnapshot {
	states := make(map[string]issueState)
	anonymousIssueID := 0
	hasStandaloneDecideBy := false

	for _, t := range turns {
		lines := strings.Split(strings.ReplaceAll(t.Content, "\r\n", "\n"), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "ISSUE_UPDATE:") {
				payload := strings.TrimSpace(trimmed[len("ISSUE_UPDATE:"):])
				applyIssueUpdate(payload, states, &anonymousIssueID)
				continue
			}

			if val := extractDirectiveValue(trimmed, "decide_by="); !isPlaceholderValue(val) {
				hasStandaloneDecideBy = true
				continue
			}
			if val := extractDirectiveValue(trimmed, "deadline="); !isPlaceholderValue(val) {
				hasStandaloneDecideBy = true
			}
		}
	}

	return decisionStateSnapshot{
		issues:                states,
		hasStandaloneDecideBy: hasStandaloneDecideBy,
	}
}

func applyIssueUpdate(payload string, states map[string]issueState, anonymousIssueID *int) {
	parts := strings.Split(payload, "|")
	issue := ""
	if len(parts) > 0 {
		issue = strings.TrimSpace(parts[0])
	}
	if issue == "" {
		*anonymousIssueID = *anonymousIssueID + 1
		issue = fmt.Sprintf("anonymous_issue_%d", *anonymousIssueID)
	}

	key := strings.ToLower(issue)
	state, exists := states[key]
	if !exists {
		state = issueState{issue: issue}
	} else if strings.TrimSpace(state.issue) == "" {
		state.issue = issue
	}

	for _, part := range parts[1:] {
		segment := strings.TrimSpace(part)
		lower := strings.ToLower(segment)
		switch {
		case strings.HasPrefix(lower, "owner="):
			state.owner = strings.TrimSpace(segment[len("owner="):])
		case strings.HasPrefix(lower, "deadline="):
			state.decideBy = strings.TrimSpace(segment[len("deadline="):])
		case strings.HasPrefix(lower, "decide_by="):
			state.decideBy = strings.TrimSpace(segment[len("decide_by="):])
		case strings.HasPrefix(lower, "blocker="):
			state.blocker = strings.TrimSpace(segment[len("blocker="):])
		}
	}

	states[key] = state
}

func extractDirectiveValue(line string, key string) string {
	if line == "" || key == "" {
		return ""
	}
	lowerLine := strings.ToLower(line)
	lowerKey := strings.ToLower(key)
	searchFrom := 0
	for {
		relative := strings.Index(lowerLine[searchFrom:], lowerKey)
		if relative < 0 {
			return ""
		}
		idx := searchFrom + relative
		if idx > 0 {
			prev := lowerLine[idx-1]
			if isDirectiveTokenChar(prev) {
				searchFrom = idx + 1
				continue
			}
		}
		value := strings.TrimSpace(line[idx+len(key):])
		if value == "" {
			return ""
		}
		if cut := strings.IndexAny(value, "|;,"); cut >= 0 {
			value = strings.TrimSpace(value[:cut])
		}
		return value
	}
}

func isOwnerUnassigned(owner string) bool {
	value := strings.ToLower(strings.TrimSpace(owner))
	return value == "" || value == "unassigned" || value == "unknown" || value == "tbd" || value == "none" || value == "-"
}

func isNoBlocker(blocker string) bool {
	value := strings.ToLower(strings.TrimSpace(blocker))
	return value == "" || value == "none" || value == "no" || value == "n/a" || value == "na" || value == "-"
}

func isPlaceholderValue(v string) bool {
	value := strings.ToLower(strings.TrimSpace(v))
	return value == "" ||
		value == "none" ||
		value == "n/a" ||
		value == "na" ||
		value == "tbd" ||
		value == "tba" ||
		value == "unknown" ||
		value == "pending" ||
		value == "later" ||
		value == "soon" ||
		value == "next cycle" ||
		value == "미정" ||
		value == "추후" ||
		value == "나중" ||
		value == "곧" ||
		value == "-"
}

func isDirectiveTokenChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_' ||
		ch == '-'
}

func derivePersonaFailureMode(p persona.Persona) string {
	joined := strings.ToLower(strings.Join([]string{
		p.Role,
		p.Stance,
		p.Style,
		strings.Join(p.Expertise, " "),
		strings.Join(p.SignatureLens, " "),
	}, " "))
	switch {
	case strings.Contains(joined, "risk"), strings.Contains(joined, "security"), strings.Contains(joined, "compliance"):
		return "over-caution bias: blocks progress without calibrated tradeoff"
	case strings.Contains(joined, "growth"), strings.Contains(joined, "marketing"), strings.Contains(joined, "pm"), strings.Contains(joined, "product"):
		return "speed bias: over-prioritizes velocity while underweighting downside risk"
	case strings.Contains(joined, "finance"), strings.Contains(joined, "cost"):
		return "cost bias: optimizes short-term efficiency over strategic upside"
	case strings.Contains(joined, "data"), strings.Contains(joined, "analytics"):
		return "measurement bias: over-trusts proxy metrics without causal validation"
	default:
		return "lens lock-in: overfits one framework and misses cross-functional constraints"
	}
}

func countPersonaTurns(turns []orchestrator.Turn) int {
	count := 0
	for _, t := range turns {
		if t.Type == orchestrator.TurnTypePersona {
			count++
		}
	}
	return count
}

func trailingNoNewPointStreak(turns []orchestrator.Turn) int {
	streak := 0
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if t.Type != orchestrator.TurnTypePersona {
			continue
		}
		value, ok := parseNewPointDirective(t.Content)
		if !ok {
			break
		}
		if value {
			break
		}
		streak++
	}
	return streak
}

func parseNewPointDirective(content string) (bool, bool) {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(strings.ToUpper(line), "NEW_POINT:"):
			raw := strings.TrimSpace(line[len("NEW_POINT:"):])
			return parseDirectiveBool(raw)
		case strings.HasPrefix(strings.ToUpper(line), "NEW_POINT="):
			raw := strings.TrimSpace(line[len("NEW_POINT="):])
			return parseDirectiveBool(raw)
		}
	}
	return false, false
}

func parseDirectiveBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "yes", "true", "y", "1":
		return true, true
	case "no", "false", "n", "0":
		return false, true
	default:
		return false, false
	}
}

func findLatestPersonaClaim(turns []orchestrator.Turn, speaker persona.Persona, self bool, summaryRunes int) string {
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if t.Type != orchestrator.TurnTypePersona {
			continue
		}
		match := samePersonaSpeaker(t, speaker)
		if self && !match {
			continue
		}
		if !self && match {
			continue
		}
		speakerName := strings.TrimSpace(t.SpeakerName)
		if speakerName == "" {
			speakerName = strings.TrimSpace(t.SpeakerID)
		}
		claim := summarizeTurnContent(t.Content, summaryRunes)
		if !self && speakerName != "" {
			return fmt.Sprintf("[%d] %s: %s", t.Index, speakerName, claim)
		}
		return fmt.Sprintf("[%d] %s", t.Index, claim)
	}
	return ""
}

func samePersonaSpeaker(turn orchestrator.Turn, p persona.Persona) bool {
	turnID := strings.TrimSpace(turn.SpeakerID)
	personaID := strings.TrimSpace(p.ID)
	if turnID != "" && personaID != "" {
		return strings.EqualFold(turnID, personaID)
	}

	turnName := strings.TrimSpace(turn.SpeakerName)
	if turnName == "" {
		return false
	}
	if strings.EqualFold(turnName, strings.TrimSpace(persona.DisplayName(p))) {
		return true
	}
	return strings.EqualFold(turnName, strings.TrimSpace(p.Name))
}

func participantPromptLine(p persona.Persona) string {
	displayName := strings.TrimSpace(persona.DisplayName(p))
	id := strings.TrimSpace(p.ID)
	role := strings.TrimSpace(p.Role)
	if displayName == "" {
		displayName = id
	}
	line := "- " + displayName
	if id != "" {
		line += fmt.Sprintf(" (%s)", id)
	}
	if role != "" {
		line += ": " + role
	}
	if strings.TrimSpace(p.MasterName) != "" {
		line += " | master_name=" + strings.TrimSpace(p.MasterName)
	}
	return line
}

func trimTurns(turns []orchestrator.Turn, limit int) []orchestrator.Turn {
	if len(turns) <= limit {
		return turns
	}
	return turns[len(turns)-limit:]
}

func debatePhase(turnCount int, personaCount int) string {
	if personaCount <= 0 {
		return "convergence"
	}
	if turnCount < personaCount*2 {
		return "exploration"
	}
	return "convergence"
}
