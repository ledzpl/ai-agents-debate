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
	judgeSnapshotIssueLimit       = 12
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

### CORE STRUCTURE
1. Address the moderator request and answer it in your first sentence.
2. Add a strongest-form summary of an opposing view, then respond to it.
3. Build your argument as core claim -> reason/mechanism -> practical implication.
4. Use falsifiable statements and include one concrete condition that proves your current position wrong.
5. Include one plain-language user-impact sentence.

### INTERACTION RULES
- Respond exclusively in the same language as the problem statement.
- Adapt explanation depth to audience_mode from the user prompt.
- If audience_mode=general, keep technical terms to <=3 and explain them in a language-neutral way.
- If audience_mode=expert, higher density is allowed with precise terminology.
- Use master_name only as inspiration from books, papers, and articles; do not invent specific titles/dates.
- Do not impersonate master identity.
- Cite prior turns using [Index] only when claim reference is clear and verifiable, and avoid non-index bracket labels such as [evidence] or [assumption].
- Avoid repeating the last two turns verbatim.

### OBJECTIVE RULES
- Optimize for the best user outcome under explicit constraints (risk, feasibility, cost, and delivery speed).
- Keep one stable optimization frame unless new evidence justifies changing it.
- If your optimization frame changes, state what changed and why.

### EVIDENCE / QUALITY GATES
- When recommendation or confidence changes materially, include evidence_type=data|experience|assumption and confidence=low|medium|high.
- If a peer argument changed your position, explicitly acknowledge the adopted point and why it is valid.
- ISSUE_UPDATE quality rule: no TBD/unknown/later/soon for decide_by or blocker.
- Do not emit ISSUE_UPDATE or SELF_CHECK when nothing changed.

### DEADLOCK / CLOSE RULES
- Deadlock breaker: if progress stalls, force a comparison table.
- OPTION_A: keep current direction with explicit owner + decide_by + metric_threshold.
- OPTION_B: switch direction with explicit owner + decide_by + metric_threshold.
- If uncertainty blocks convergence, propose the smallest discriminating experiment with owner + decide_by + success_metric + stop_condition.
- CLOSE should be yes only when unresolved blockers <=1, unowned issues = 0, decide_by signals >=1.

### MACHINE-READABLE CONTROLS
- ISSUE_UPDATE: <issue> | owner=<id> | decide_by=<trigger> | blocker=<item>
- PERSUASION_UPDATE: changed=yes|no; adopted=<peer point or none>; rationale=<why>; remaining_gap=<open disagreement or none>
- SELF_CHECK: <likely bias/failure mode> -> <mitigation in this turn>
- META_DELTA: changed=<what>; unchanged=<what>; next_question=<question>

### TERMINAL COMMANDS (Required)
HANDOFF_ASK: <one concrete question for the NEXT speaker>
NEXT: <persona_id>
CLOSE: yes|no
NEW_POINT: yes|no

### BOUNDARY RULES
- Narrative body must be natural language only; put machine controls at the absolute end.
- Do not translate or rename machine control labels.
- Self-repair before final output: if any required control line is malformed/missing, fix it before sending.`)
}

func buildOpeningSpeakerSelectorSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are an expert facilitator responsible for selecting the ideal opening speaker for a multi-persona debate.

### OBJECTIVE
Pick the single most relevant persona to frame the discussion. Prioritize Domain Fit and Strategic Framing.

### OUTPUT FORMAT (STRICT)
- Return exactly one JSON object: {"persona_id":"matched_id"}
- persona_id must be one of the provided candidate ids.
- Ignore candidates whose id is empty.
- No markdown, no prose, no code blocks.`)
}

func buildOpeningSpeakerSelectorUserPrompt(input orchestrator.SelectOpeningSpeakerInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nCandidates:\n")
	allowedIDs := make([]string, 0, len(input.Personas))
	skippedEmptyID := 0
	for _, p := range input.Personas {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			skippedEmptyID++
			continue
		}
		allowedIDs = append(allowedIDs, id)
		b.WriteString(fmt.Sprintf("- id: %s\n", id))
		if name := strings.TrimSpace(p.Name); name != "" {
			b.WriteString("  name: " + name + "\n")
		}
		b.WriteString(fmt.Sprintf("  role: %s\n", strings.TrimSpace(p.Role)))
		if stance := strings.TrimSpace(p.Stance); stance != "" {
			b.WriteString("  stance: " + stance + "\n")
		}
		if expertise := normalizePromptList(p.Expertise); len(expertise) > 0 {
			b.WriteString("  expertise: " + strings.Join(expertise, ", ") + "\n")
		}
		if master := strings.TrimSpace(p.MasterName); master != "" {
			b.WriteString("  master_name: " + master + "\n")
		}
	}
	if skippedEmptyID > 0 {
		b.WriteString(fmt.Sprintf("\nIgnored candidates with empty id: %d\n", skippedEmptyID))
	}
	if len(allowedIDs) > 0 {
		b.WriteString("\nAllowed persona_id values: " + strings.Join(allowedIDs, ", ") + "\n")
	} else {
		b.WriteString("\nAllowed persona_id values: none (all candidate ids were empty)\n")
		b.WriteString("If no selectable id exists, return {\"persona_id\":\"\"}.\n")
	}
	b.WriteString("\nSelect the best opening speaker persona_id now.")
	return b.String()
}

func buildTurnUserPrompt(input orchestrator.GenerateTurnInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	personaTurns := countPersonaTurns(input.Turns)
	effectiveTurns := deriveEffectiveDebateTurns(len(input.Turns), personaTurns)
	phase := debatePhase(effectiveTurns, len(input.Personas))
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)
	turnNo := personaTurns + 1
	noNewPointStreak := trailingNoNewPointStreak(input.Turns)
	closeReadiness := summarizeCloseReadiness(input.Turns)
	requireQualityCheckpoint := (turnNo%4 == 0) || noNewPointStreak >= 2
	requireIssueCheckpoint := (turnNo%4 == 0) || noNewPointStreak >= 2
	requirePersuasionCheckpoint := (turnNo%3 == 0) || noNewPointStreak >= 1
	requireExperimentCheckpoint := noNewPointStreak >= 2
	periodicMetaTurn := turnNo%4 == 0

	var b strings.Builder
	b.WriteString("<context>\n")
	b.WriteString("Problem: " + input.Problem + "\n")
	b.WriteString("Debate phase:\n")
	b.WriteString("- current phase: " + phase + "\n")
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- audience mode: explain for expert readers with precise terminology and compact logic.\n")
	} else {
		b.WriteString("- audience mode: explain so a non-expert can follow quickly.\n")
		b.WriteString("- avoid unexplained jargon or acronyms.\n")
	}
	b.WriteString("- cite [Index] when claim reference is clear and verifiable.\n")
	b.WriteString("</context>\n\n")

	b.WriteString("Optimization frame:\n")
	b.WriteString("- optimize for best user outcome under explicit constraints: risk, feasibility, cost, and delivery speed.\n")
	b.WriteString("- keep one stable objective frame unless evidence forces change.\n")
	if turnNo == 1 {
		b.WriteString("- first-turn requirement: explicitly declare your objective frame in one sentence.\n")
	} else {
		b.WriteString("- if objective frame changes, explain what changed and why.\n")
	}
	b.WriteString("\n")

	b.WriteString("Participants:\n")
	if len(input.Personas) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, p := range input.Personas {
			b.WriteString(participantPromptLine(p) + "\n")
		}
	}
	b.WriteString("\n")

	b.WriteString("<current_persona>\n")
	b.WriteString(fmt.Sprintf("- id: %s\n- name: %s\n- role: %s\n", input.Speaker.ID, input.Speaker.Name, input.Speaker.Role))
	if stance := strings.TrimSpace(input.Speaker.Stance); stance != "" {
		b.WriteString("- stance: " + stance + "\n")
	}
	if master := strings.TrimSpace(input.Speaker.MasterName); master != "" {
		b.WriteString("- master_name: " + master + " (not identity impersonation)\n")
		b.WriteString("- master usage requirement: use ideas from this master's books, papers, or articles; do not invent specific titles/dates.\n")
	}
	if style := strings.TrimSpace(input.Speaker.Style); style != "" {
		b.WriteString("- style: " + style + "\n")
	}
	if expertise := normalizePromptList(input.Speaker.Expertise); len(expertise) > 0 {
		b.WriteString("- expertise: " + strings.Join(expertise, ", ") + "\n")
	}
	if sigLens := normalizePromptList(input.Speaker.SignatureLens); len(sigLens) > 0 {
		b.WriteString("- signature lens: " + strings.Join(sigLens, ", ") + "\n")
	}
	if constraints := normalizePromptList(input.Speaker.Constraints); len(constraints) > 0 {
		b.WriteString("- constraints:\n")
		for _, item := range constraints {
			b.WriteString("  - " + item + "\n")
		}
	}
	b.WriteString("- persona failure-mode watch: " + derivePersonaFailureMode(input.Speaker) + "\n")
	b.WriteString("</current_persona>\n\n")

	b.WriteString("Recent debate log:\n")
	if len(input.Turns) == 0 {
		b.WriteString("- Initial Turn.\n")
	} else {
		written := 0
		for _, t := range trimTurns(input.Turns, budget.turnRecentLogLimit) {
			summary := summarizeTurnWithType(t, budget.turnLogSummaryRunes)
			if summary == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("[%d][%s] %s\n", t.Index, t.SpeakerName, summary))
			written++
		}
		if written == 0 {
			b.WriteString("- none after control-line filtering.\n")
		}
	}
	b.WriteString("\n")

	b.WriteString("Interaction memory snapshot:\n")
	b.WriteString(buildTurnInteractionSnapshot(input.Turns, input.Speaker, budget))
	b.WriteString("\n")

	b.WriteString("Progress signals:\n")
	b.WriteString(fmt.Sprintf("- turn_no: %d\n", turnNo))
	b.WriteString(fmt.Sprintf("- trailing persona NEW_POINT=no streak: %d\n", noNewPointStreak))
	b.WriteString(fmt.Sprintf("- close readiness snapshot: unresolved_blockers=%d, unowned_issues=%d, decide_by_signals=%d\n", closeReadiness.unresolvedBlockers, closeReadiness.unownedIssues, closeReadiness.decideBySignals))
	b.WriteString("- CLOSE decision must use the snapshot above as source of truth.\n")
	b.WriteString("- CLOSE gate: unresolved_blockers<=1, unowned_issues=0, decide_by_signals>=1.\n")
	b.WriteString("- persuasion gate: before CLOSE=yes, show at least one adopted peer point or an explicit uncertainty-reduction experiment.\n")
	if noNewPointStreak >= 2 {
		b.WriteString("- deadlock signal: repeated no-new-point streak detected.\n")
		b.WriteString("- deadlock mode required now: apply the system deadlock breaker (OPTION_A/OPTION_B table).\n")
	}
	b.WriteString("\n")

	b.WriteString("Turn objective:\n")
	b.WriteString("- answer the latest moderator or peer request directly and finish with a decision-forcing handoff question.\n")
	b.WriteString("- include one sentence on what changes for users if this is chosen.\n")
	b.WriteString("- avoid repeating the last two turns; add a new condition, metric, or dependency.\n")
	if phase == "convergence" {
		b.WriteString("- choose one provisional option and include owner + decide_by.\n")
	}
	if requireQualityCheckpoint {
		b.WriteString("- quality checkpoint required now: include evidence_type=data|experience|assumption and confidence=low|medium|high.\n")
	} else {
		b.WriteString("- include evidence-quality clause when confidence/recommendation changes materially.\n")
	}
	if requireIssueCheckpoint {
		b.WriteString("- issue-state checkpoint required now: include ISSUE_UPDATE only when opening a new issue or when owner/decide_by/blocker changes.\n")
	} else {
		b.WriteString("- include ISSUE_UPDATE only when opening a new issue or when owner/decide_by/blocker changes.\n")
	}
	if requirePersuasionCheckpoint {
		b.WriteString("- persuasion checkpoint required now: include PERSUASION_UPDATE and state what you adopted from a peer plus remaining gap.\n")
	} else {
		b.WriteString("- include PERSUASION_UPDATE when your stance changed or when you adopted a peer point.\n")
	}
	if requireExperimentCheckpoint {
		b.WriteString("- deadlock experiment required now: propose the smallest discriminating experiment with owner + decide_by + success_metric + stop_condition.\n")
	}
	b.WriteString("- keep decide_by/blocker concrete (" + issuePlaceholderGuardrail + ").\n")
	if periodicMetaTurn {
		b.WriteString("- periodic meta-summary turn: emit META_DELTA with changed/unchanged/next_question.\n")
	} else {
		b.WriteString("- periodic meta-summary turn: emit META_DELTA with changed/unchanged/next_question every 4th turn.\n")
	}
	b.WriteString("- control-line block must end with:\n")
	b.WriteString("PERSUASION_UPDATE: changed=yes|no; adopted=<peer point or none>; rationale=<why>; remaining_gap=<open disagreement or none>\n")
	b.WriteString("HANDOFF_ASK: <one concrete question for the NEXT speaker>\n")
	b.WriteString("NEXT: <persona_id>\n")
	b.WriteString("CLOSE: yes|no\n")
	b.WriteString("NEW_POINT: yes|no\n")
	b.WriteString("- do not translate or rename any control-line label.\n")
	return b.String()
}

func buildJudgeSystemPrompt() string {
	return fmt.Sprintf(strings.TrimSpace(`### ROLE
You are a strict consensus judge. Your goal is to determine if the debate has produced a workable decision or a clear, well-defined disagreement.

### LANGUAGE RULE
- Respond exclusively in the same language as the problem statement.

### JUDGING CRITERIA
1. Be conservative: set reached=true only if there is clear alignment on goal, approach, and next action.
2. If evidence is mixed or insufficient, prefer reached=false.
3. Rationale must reference at least two different speakers/turns.
4. Score rubric:
   - 0.90-1.00: workable consensus
   - 0.70-0.89: partial alignment with material risk
   - 0.00-0.69: unresolved disagreement
5. Persuasion quality: reached=true requires at least one explicit cross-persona adoption/concession with [Index] support.
6. Optimality check: chosen direction must dominate alternatives on objective/constraints OR include a concrete uncertainty-reduction experiment.

### OUTPUT FORMAT (STRICT JSON)
- Return a single-line minified JSON object.
- Use this exact order: reached, score, summary, rationale, open_risks, next_action_owner, next_action_trigger_or_deadline, next_action_success_metric.
- summary: exactly 1 sentence; keep it short in a language-neutral way.
- open_risks: 0-3 items.
- Never omit required keys.
- next_action_owner: "moderator" if ownership is unclear.
- Avoid placeholder values in next_action fields (%s).
- rationale must state who was persuaded (or not), what changed, and why.
- reached=true only when key objections are resolved by evidence or by a concrete, bounded experiment plan.
- JSON type constraints: reached must be unquoted true/false; score must be numeric 0..1; open_risks must be an array.
- final character must be }.
- JSON template: {"reached":false,"score":0.0,"summary":"","rationale":"","open_risks":[],"next_action_owner":"moderator","next_action_trigger_or_deadline":"48h","next_action_success_metric":"clear measurable criterion"}
- Self-repair before final output: validate shape/types/order and repair malformed JSON.`), nextActionPlaceholderRule)
}

func buildJudgeUserPrompt(input orchestrator.JudgeConsensusInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	judgeTurns := trimTurns(input.Turns, budget.judgeRecentLogLimit)
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

	var b strings.Builder
	b.WriteString("Problem:\n" + input.Problem + "\n\n")
	b.WriteString("Debate log tail:\n")
	writtenLog := 0
	for _, t := range judgeTurns {
		summary := summarizeTurnWithType(t, budget.judgeLogSummaryRunes)
		if summary == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summary))
		writtenLog++
	}
	if writtenLog == 0 {
		b.WriteString("- none after control-line filtering.\n")
	}
	b.WriteString("\nDecision-state snapshot:\n")
	b.WriteString(buildJudgeDecisionStateSnapshot(input.Turns))
	b.WriteString("\nOutput format reminder:\n")
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	b.WriteString("- return one minified JSON object on a single line only.\n")
	b.WriteString("- key order: reached, score, summary, rationale, open_risks, next_action_owner, next_action_trigger_or_deadline, next_action_success_metric.\n")
	b.WriteString("- never omit keys; if uncertain, use conservative concrete defaults.\n")
	b.WriteString("- avoid placeholder values in next_action fields.\n")
	b.WriteString("- reached=true requires at least one explicit persuasion/concession event plus resolved key objections.\n")
	b.WriteString("- if uncertainty remains, reached=true is allowed only with a concrete bounded experiment plan (owner/deadline/metric).\n")
	b.WriteString("- rationale must mention who changed what (or why no one changed).\n")
	b.WriteString("- type constraints: reached is boolean, score is numeric 0..1.\n")
	b.WriteString("- final character must be }.\n")
	return b.String()
}

func buildModeratorSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are the moderator. Your goal is to sharpen the debate by exposing hidden tensions and forcing choice.

### LANGUAGE RULE
- Respond exclusively in the same language as the problem statement.

### MODERATOR PRINCIPLES
- Avoid recency bias: treat the latest turn as one data point, not the whole debate.
- Synthesize multiple recent turns and include one supporting point and one tension/tradeoff.
- Debate memory snapshot should ground your intervention, not just the most recent line.
- Close the loop on your previous intervention.
- Use synthesis -> unresolved tradeoff -> targeted next-speaker question.
- Ask must be decision-forcing and point to a specific prior claim (speaker + idea).
- Force persuasion accounting: ask what point was adopted from another speaker and what still blocks convergence.
- Do not introduce external facts.
- Adapt wording depth to audience_mode.

### REQUIRED LINE FORMAT AND ORDER
Required line format and order:
SYNTHESIS: <one-sentence trajectory summary>
TENSION: <exact unresolved tradeoff with [Index] references>
ASK: <decision-forcing question for next speaker>
DECISION_CHECK: choose Option A or B; metric_threshold=<value>; decide_by=<trigger>

### OPTIONAL DISAMBIGUATION LINES
Optional disambiguation lines:
OPTION_A: <short definition>
OPTION_B: <short definition>
PERSUASION_CHECK: adopted_from=<speaker/index or none>; remaining_gap=<gap>
SCORECARD: coherence=<0-100>; executability=<0-100>; risk_coverage=<0-100>
SCORECARD_REASON: <one sentence>
The 4 required lines remain mandatory.

### CONSTRAINTS
- DECISION_CHECK: choose Option A or B.
- metric_threshold must be numeric or explicit condition.
- no TBD/unknown/later/soon for metric_threshold/decide_by.
- If uncertainty is the blocker, DECISION_CHECK must include a smallest discriminating experiment with owner, deadline, success metric, and stop condition.
- Do not translate or rename required labels.
- Do not add prose before SYNTHESIS or after final metadata line.
- Self-repair before final output.`)
}

func buildModeratorUserPrompt(input orchestrator.GenerateModeratorInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	personaTurnCount := countPersonaTurns(input.Turns)
	noNewPointStreak := trailingNoNewPointStreak(input.Turns)
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

	var b strings.Builder
	b.WriteString("Problem:\n" + input.Problem + "\n\n")
	b.WriteString("Recent debate log:\n")
	recentTurns := trimTurns(input.Turns, budget.moderatorRecentLogLimit)
	writtenRecent := 0
	for _, t := range recentTurns {
		summary := summarizeTurnWithType(t, budget.moderatorLogSummaryRunes)
		if summary == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("[%d][%s] %s\n", t.Index, t.SpeakerName, summary))
		writtenRecent++
	}
	if writtenRecent == 0 {
		b.WriteString("- none after control-line filtering.\n")
	}
	b.WriteString("\nDebate memory snapshot (anti-recency):\n")
	b.WriteString(buildModeratorMemorySnapshot(input.Turns, input.PreviousTurn, budget.moderatorMemory))
	b.WriteString("\nModerator loop status:\n")
	b.WriteString(buildModeratorLoopStatus(input.Turns, budget.moderatorLoopSummaryRunes))
	b.WriteString("\nNext speaker context:\n")
	b.WriteString("- next speaker id: " + strings.TrimSpace(input.NextSpeaker.ID) + "\n")
	b.WriteString("- next speaker role: " + strings.TrimSpace(input.NextSpeaker.Role) + "\n")
	if master := strings.TrimSpace(input.NextSpeaker.MasterName); master != "" {
		b.WriteString("- next speaker master_name: " + master + "\n")
		b.WriteString("- ask the next speaker to use ideas from this master's books, papers, or articles when relevant.\n")
	} else {
		b.WriteString("- next speaker master_name: none\n")
		b.WriteString("- no master-specific source requirement; focus on role and signature lens.\n")
	}
	if sigLens := normalizePromptList(input.NextSpeaker.SignatureLens); len(sigLens) > 0 {
		b.WriteString("- next speaker signature lens: " + strings.Join(sigLens, ", ") + "\n")
	} else {
		b.WriteString("- next speaker signature lens: none\n")
	}
	b.WriteString("\nModerator balancing guidance:\n")
	b.WriteString("- Avoid recency: treat latest turn as one data point, not the whole debate.\n")
	b.WriteString("- Ask for persuasion accounting: what the next speaker adopted from peers and what remains unresolved.\n")
	b.WriteString("- DECISION_CHECK using this exact structure: DECISION_CHECK: choose Option A or B; metric_threshold=<concrete>; decide_by=<trigger>.\n")
	b.WriteString("- metric_threshold and decide_by must both be concrete values.\n")
	b.WriteString("- When uncertainty is the blocker, require a smallest discriminating experiment with owner + decide_by + success_metric + stop_condition.\n")
	b.WriteString("\nModerator cadence signals:\n")
	b.WriteString(fmt.Sprintf("- persona turns observed so far: %d\n", personaTurnCount))
	b.WriteString(fmt.Sprintf("- trailing persona NEW_POINT=no streak: %d\n", noNewPointStreak))
	if personaTurnCount > 0 && personaTurnCount%4 == 0 {
		b.WriteString("- include SCORECARD + SCORECARD_REASON in this intervention.\n")
	}
	if noNewPointStreak >= 2 {
		b.WriteString("- stagnation detected: force OPTION_A/OPTION_B plus experiment-focused DECISION_CHECK.\n")
	}
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	return b.String()
}

func buildFinalModeratorSystemPrompt() string {
	return strings.TrimSpace(`### ROLE
You are the closing moderator. Your goal is to provide a definitive wrap-up of the entire debate.

### LANGUAGE RULE
- Respond exclusively in the same language as the problem statement.

### RESPONSE REQUIREMENTS
- 3-5 concise sentences.
- First sentence must be a plain-language verdict.
- Middle sentences should synthesize major agreement + open risk with clear logic.
- Include one concrete next action in what/who/when format.
- End with a decision-oriented concluding sentence.

### STYLE CALIBRATION
- audience_mode=general, avoid unexplained acronyms/jargon.
- audience_mode=expert, precise terminology is allowed.
- Use consensus score/rationale as confidence calibration.
- Do not introduce new facts beyond the provided debate and judge context.`)
}

func buildFinalModeratorUserPrompt(input orchestrator.GenerateFinalModeratorInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)
	logTail := trimTurns(input.Turns, budget.judgeRecentLogLimit)

	var b strings.Builder
	b.WriteString("Problem:\n" + input.Problem + "\n\n")
	b.WriteString("Final status and judge output:\n")
	b.WriteString("- status: " + strings.TrimSpace(input.FinalStatus) + "\n")
	b.WriteString(fmt.Sprintf("- consensus reached: %t\n", input.Consensus.Reached))
	b.WriteString(fmt.Sprintf("- consensus score: %.2f\n", input.Consensus.Score))
	b.WriteString("- judge summary: " + strings.TrimSpace(input.Consensus.Summary) + "\n")
	b.WriteString("- judge rationale: " + strings.TrimSpace(input.Consensus.Rationale) + "\n")
	if len(input.Consensus.OpenRisks) > 0 {
		b.WriteString("- open risks: " + strings.Join(input.Consensus.OpenRisks, "; ") + "\n")
	} else {
		b.WriteString("- open risks: none\n")
	}
	b.WriteString("- required next action: " + strings.TrimSpace(input.Consensus.RequiredNextAction) + "\n")
	b.WriteString("- next action owner: " + strings.TrimSpace(input.Consensus.NextActionOwner) + "\n")
	b.WriteString("- next action decide_by: " + strings.TrimSpace(input.Consensus.NextActionTrigger) + "\n")
	b.WriteString("- next action success metric: " + strings.TrimSpace(input.Consensus.NextActionSuccessMetric) + "\n")
	b.WriteString("\nFinal debate log tail:\n")
	if len(logTail) == 0 {
		b.WriteString("- none\n")
	} else {
		writtenTail := 0
		for _, t := range logTail {
			summary := summarizeTurnWithType(t, budget.judgeLogSummaryRunes)
			if summary == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summary))
			writtenTail++
		}
		if writtenTail == 0 {
			b.WriteString("- none after control-line filtering.\n")
		}
	}
	b.WriteString("\nAudience guidance:\n")
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- expert mode: concise and precise closing summary.\n")
	} else {
		b.WriteString("- general mode: plain-language closing summary.\n")
	}
	b.WriteString("- Provide the final wrap-up assessment now.\n")
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
		content := summarizeTurnWithType(t, summaryRunes)
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

	ask := summarizeTurnWithType(turns[lastModeratorIdx], summaryRunes)
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
		summary := summarizeTurnWithType(turns[i], summaryRunes)
		if summary == "" {
			continue
		}
		response = fmt.Sprintf("%s: %s", speaker, summary)
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
	issueOrder            map[string]int
	persuasionAdoptions   int
	persuasionGapSignals  int
	hasBoundedExperiment  bool
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
		sort.SliceStable(keys, func(i, j int) bool {
			left := snapshot.issues[keys[i]]
			right := snapshot.issues[keys[j]]
			leftPriority := issueSortPriority(left)
			rightPriority := issueSortPriority(right)
			if leftPriority != rightPriority {
				return leftPriority > rightPriority
			}
			leftOrder := snapshot.issueOrder[keys[i]]
			rightOrder := snapshot.issueOrder[keys[j]]
			if leftOrder != rightOrder {
				return leftOrder > rightOrder
			}
			return strings.Compare(keys[i], keys[j]) < 0
		})

		truncated := 0
		if len(keys) > judgeSnapshotIssueLimit {
			truncated = len(keys) - judgeSnapshotIssueLimit
			keys = keys[:judgeSnapshotIssueLimit]
		}

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
		if truncated > 0 {
			b.WriteString(fmt.Sprintf("  - ... +%d more issues omitted for prompt budget\n", truncated))
		}
	}

	if snapshot.hasStandaloneDecideBy {
		b.WriteString("- decide_by signal outside issue registry: present\n")
	} else {
		b.WriteString("- decide_by signal outside issue registry: none\n")
	}
	b.WriteString(fmt.Sprintf("- persuasion adoption signals: %d\n", snapshot.persuasionAdoptions))
	b.WriteString(fmt.Sprintf("- persuasion remaining gaps signaled: %d\n", snapshot.persuasionGapSignals))
	if snapshot.hasBoundedExperiment {
		b.WriteString("- bounded experiment signal: present\n")
	} else {
		b.WriteString("- bounded experiment signal: none\n")
	}
	return b.String()
}

func extractDecisionStateSnapshot(turns []orchestrator.Turn) decisionStateSnapshot {
	states := make(map[string]issueState)
	issueOrder := make(map[string]int)
	anonymousIssueID := 0
	hasStandaloneDecideBy := false
	persuasionAdoptions := 0
	persuasionGapSignals := 0
	hasBoundedExperiment := false
	updateSeq := 0

	for _, t := range turns {
		lines := strings.Split(strings.ReplaceAll(t.Content, "\r\n", "\n"), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			normalized := normalizeDirectiveLineCandidate(trimmed)
			if normalized == "" {
				continue
			}

			upper := strings.ToUpper(normalized)
			if strings.HasPrefix(upper, "ISSUE_UPDATE:") {
				payload := strings.TrimSpace(normalized[len("ISSUE_UPDATE:"):])
				key := applyIssueUpdate(payload, states, &anonymousIssueID)
				issueOrder[key] = updateSeq
				updateSeq++
				continue
			}
			if strings.HasPrefix(upper, "PERSUASION_UPDATE:") {
				payload := strings.TrimSpace(normalized[len("PERSUASION_UPDATE:"):])
				if hasPersuasionAdoptionSignal(payload) {
					persuasionAdoptions++
				}
				if gap := extractDirectiveValue(payload, "remaining_gap="); !isPlaceholderValue(gap) {
					persuasionGapSignals++
				}
				continue
			}

			if val := extractDirectiveValue(normalized, "decide_by="); !isPlaceholderValue(val) {
				hasStandaloneDecideBy = true
			}
			if val := extractDirectiveValue(normalized, "deadline="); !isPlaceholderValue(val) {
				hasStandaloneDecideBy = true
			}
			if !hasBoundedExperiment && hasBoundedExperimentSignal(normalized) {
				hasBoundedExperiment = true
			}
		}
	}

	return decisionStateSnapshot{
		issues:                states,
		issueOrder:            issueOrder,
		persuasionAdoptions:   persuasionAdoptions,
		persuasionGapSignals:  persuasionGapSignals,
		hasBoundedExperiment:  hasBoundedExperiment,
		hasStandaloneDecideBy: hasStandaloneDecideBy,
	}
}

func hasPersuasionAdoptionSignal(payload string) bool {
	changed, ok := parsePromptBoolToken(extractDirectiveValue(payload, "changed="))
	if !ok || !changed {
		return false
	}
	adopted := extractDirectiveValue(payload, "adopted=")
	return !isPlaceholderValue(adopted)
}

func hasBoundedExperimentSignal(line string) bool {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "success_metric=") || !strings.Contains(lower, "stop_condition=") {
		return false
	}
	if !strings.Contains(lower, "owner=") {
		return false
	}
	return strings.Contains(lower, "decide_by=") || strings.Contains(lower, "deadline=")
}

func parsePromptBoolToken(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "yes", "y", "true", "1":
		return true, true
	case "no", "n", "false", "0":
		return false, true
	default:
		return false, false
	}
}

func applyIssueUpdate(payload string, states map[string]issueState, anonymousIssueID *int) string {
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
	return key
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
		if cut := findFollowingDirectiveTokenCut(value); cut >= 0 {
			value = strings.TrimSpace(value[:cut])
		}
		if cut := findDirectiveDelimiterCut(value); cut >= 0 {
			value = strings.TrimSpace(value[:cut])
		}
		return value
	}
}

func findDirectiveDelimiterCut(value string) int {
	cut := -1
	for _, delim := range []byte{'|', ';'} {
		if idx := strings.IndexByte(value, delim); idx >= 0 {
			if cut < 0 || idx < cut {
				cut = idx
			}
		}
	}

	searchFrom := 0
	for {
		idx := strings.IndexByte(value[searchFrom:], ',')
		if idx < 0 {
			break
		}
		idx += searchFrom
		trailing := strings.TrimSpace(value[idx+1:])
		if trailing == "" {
			if cut < 0 || idx < cut {
				cut = idx
			}
			break
		}
		token := trailing
		if spaceIdx := strings.IndexByte(token, ' '); spaceIdx >= 0 {
			token = token[:spaceIdx]
		}
		if isDirectiveAssignmentToken(token) {
			if cut < 0 || idx < cut {
				cut = idx
			}
			break
		}
		searchFrom = idx + 1
	}
	return cut
}

func findFollowingDirectiveTokenCut(value string) int {
	if strings.TrimSpace(value) == "" {
		return -1
	}
	tokenCount := 0
	for i := 0; i < len(value); {
		for i < len(value) && value[i] == ' ' {
			i++
		}
		if i >= len(value) {
			break
		}
		start := i
		for i < len(value) && value[i] != ' ' {
			i++
		}
		token := value[start:i]
		if tokenCount > 0 && isDirectiveAssignmentToken(token) {
			return start
		}
		tokenCount++
	}
	return -1
}

func isDirectiveAssignmentToken(token string) bool {
	candidate := strings.TrimSpace(token)
	candidate = strings.TrimLeft(candidate, "-*>")
	if candidate == "" {
		return false
	}
	sepIdx := strings.Index(candidate, "=")
	if sepIdx <= 0 {
		return false
	}
	key := candidate[:sepIdx]
	if key == "" {
		return false
	}
	first := key[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}
	for i := 0; i < len(key); i++ {
		ch := key[i]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func isOwnerUnassigned(owner string) bool {
	value := strings.ToLower(strings.TrimSpace(owner))
	return value == "" || value == "unassigned" || value == "unknown" || value == "tbd" || value == "none" || value == "-"
}

func issueSortPriority(state issueState) int {
	priority := 0
	if !isNoBlocker(state.blocker) {
		priority += 4
	}
	if isOwnerUnassigned(state.owner) {
		priority += 2
	}
	if isPlaceholderValue(state.decideBy) {
		priority += 1
	}
	return priority
}

func isNoBlocker(blocker string) bool {
	value := strings.ToLower(strings.TrimSpace(blocker))
	return value == "" || value == "none" || value == "no" || value == "n/a" || value == "na" || value == "-"
}

func isPlaceholderValue(v string) bool {
	if isTemplatePlaceholderValue(v) {
		return true
	}
	value := normalizePlaceholderToken(v)
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

func normalizePlaceholderToken(v string) string {
	value := strings.ToLower(strings.TrimSpace(v))
	for value != "" {
		prev := value
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'`")
		value = strings.TrimSpace(strings.TrimRight(value, ".!?,;:"))
		if len(value) >= 2 {
			switch {
			case strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")"):
				value = strings.TrimSpace(value[1 : len(value)-1])
			case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
				value = strings.TrimSpace(value[1 : len(value)-1])
			}
		}
		// Handle malformed trailing closers, e.g. "soon)" or "<trigger>)".
		value = strings.TrimSpace(strings.TrimRight(value, ")）"))
		if value == prev {
			break
		}
	}
	return value
}

func isTemplatePlaceholderValue(v string) bool {
	value := strings.TrimSpace(v)
	value = strings.Trim(value, "\"'`")
	value = strings.TrimSpace(strings.TrimRight(value, ".!?,;:)）"))
	if value == "" {
		return false
	}

	matchesWrappedTemplate := func(open, close string) bool {
		if !strings.HasPrefix(value, open) {
			return false
		}
		end := strings.Index(value, close)
		if end <= len(open)-1 {
			return false
		}
		core := strings.TrimSpace(value[len(open):end])
		if core == "" {
			return false
		}
		rest := strings.TrimSpace(value[end+len(close):])
		rest = strings.Trim(rest, ")]}）.!?,;:")
		return rest == ""
	}

	return matchesWrappedTemplate("<", ">") || matchesWrappedTemplate("{", "}")
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
		line := normalizeDirectiveLineCandidate(strings.TrimSpace(lines[i]))
		if line == "" {
			continue
		}
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
		if claim == "" {
			continue
		}
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

func deriveEffectiveDebateTurns(totalTurns int, personaTurns int) int {
	if totalTurns <= 0 {
		return 0
	}
	if personaTurns < 0 {
		personaTurns = 0
	}
	if personaTurns > totalTurns {
		personaTurns = totalTurns
	}
	nonPersonaTurns := totalTurns - personaTurns
	// Moderator/system turns still carry debate progress, but with lower phase weight.
	return personaTurns + (nonPersonaTurns+1)/2
}
