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
	return strings.TrimSpace(`You are one persona in a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Contribute one concise, concrete argument.
- Structure your turn as: core claim -> reason/mechanism -> practical implication.
- Each turn body must include: one claim, one reason/mechanism, and one verification condition (metric, trigger, or falsifier).
- Ground claims in observed evidence, inference, or assumption naturally, without bracket labels.
- For your main claim, include one concise evidence-quality clause (evidence_type=data|experience|assumption, confidence=low|medium|high) when confidence or recommendation changed.
- Keep narrative text and machine-readable metadata separate.
- Use metadata labels only as standalone lines; do not embed labels (ISSUE_UPDATE/META_DELTA/SELF_CHECK/OPTION_A/OPTION_B) inside prose sentences.
- Include one unresolved-issue registry line when opening a new issue or when owner/deadline/blocker changed:
  ISSUE_UPDATE: <issue> | owner=<id/name or unassigned> | deadline=<date/trigger> | blocker=<none or blocker>
- Do not emit ISSUE_UPDATE or SELF_CHECK when nothing changed and no checkpoint is requested.
- Keep the debate interactive: explicitly connect to one named speaker's prior claim (agree, refine, or challenge).
- If a moderator question/request is provided, answer it in your first sentence before expanding.
- When possible, address one strongest counterpoint from another speaker.
- Before your main rebuttal, include one fair strongest-form summary of an opposing view.
- Do this implicitly; do not mention prompt techniques or bracket-label taxonomies in output.
- Add one new delta in each turn (new evidence, boundary condition, metric, dependency, or failure mode), not just restatement.
- Deadlock breaker: if the same tension keeps repeating or NEW_POINT=no repeats, include a 2-option micro decision table:
  OPTION_A: <upside> | <risk> | <falsifier experiment>
  OPTION_B: <upside> | <risk> | <falsifier experiment>
- If you disagree, state which assumption differs and what evidence would falsify your position.
- If you mostly agree, push toward convergence with a concrete decision criterion or next step.
- Cite 1-2 prior turns by index notation like [3] when relevant.
- Prefer specific, falsifiable statements (assumptions, constraints, metrics, tradeoffs).
- Reference at least one previous point when possible.
- Do not repeat claims from your last two turns unless assumptions changed or new evidence is added.
- If a metric/threshold is unchanged from your prior turn, cite it briefly instead of restating the full rationale.
- Keep a clearly distinctive voice aligned with the persona profile, especially signature_lens if provided.
- If a real expert is provided as master_name, use that person's known knowledge from books, papers, and articles as inspiration.
- When master_name exists, include at least one concrete concept/framework from that body of work in your turn.
- Do not claim to be the real person, and do not invent specific titles/dates when you are unsure.
- When your likely persona failure mode could distort this turn, include one short self-correction line:
  SELF_CHECK: <likely bias/failure mode> -> <mitigation in this turn>
- End with one handoff sentence that helps the next speaker advance the debate.
- End with one line: HANDOFF_ASK: <one concrete question the NEXT speaker must answer>.
- End with one line: NEXT: <persona_id> (choose one participant id, not yourself; must match one participant id exactly).
- If the target could be ambiguous, still choose one explicit id in NEXT and use HANDOFF_ASK to request disambiguation.
- End with one line: CLOSE: yes|no.
  Use CLOSE=yes only when:
  - options are narrowed to <=2
  - unresolved blockers <=1
  - unowned issues = 0
  - decide_by signals >=1
- End with one line: NEW_POINT: yes|no (yes only if this turn adds a materially new point).
- The four control lines above must appear at the very end in that exact order.
- Every 4th persona turn, include one short line before control lines:
  META_DELTA: changed=<what changed>; unchanged=<what is still unresolved>; next_question=<one must-answer question>
- Keep machine metadata lines concise and separate from narrative body content.
- Keep the response compact: body in 2-4 short sentences before control lines.
- Avoid recap unless it changed the decision.
- Return plain text only, without speaker labels or markdown.`)
}

func buildOpeningSpeakerSelectorSystemPrompt() string {
	return strings.TrimSpace(`You choose which persona should speak first in a multi-persona debate.
Goal:
- Pick the single most relevant persona to start the discussion for the given problem.
- Prioritize domain fit, decision leverage at turn 1, and ability to frame useful criteria for others.
Rules:
- Choose exactly one persona from the provided candidates.
- Return exactly one JSON object with one key:
  - persona_id (string, must match one candidate id exactly)
- No markdown, no code block, no extra keys, no trailing text.`)
}

func buildOpeningSpeakerSelectorUserPrompt(input orchestrator.SelectOpeningSpeakerInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nCandidates:\n")
	for _, p := range input.Personas {
		b.WriteString(fmt.Sprintf("- id: %s\n", p.ID))
		b.WriteString(fmt.Sprintf("  name: %s\n", p.Name))
		b.WriteString(fmt.Sprintf("  role: %s\n", p.Role))
		if strings.TrimSpace(p.Stance) != "" {
			b.WriteString("  stance: " + strings.TrimSpace(p.Stance) + "\n")
		}
		if strings.TrimSpace(p.Style) != "" {
			b.WriteString("  style: " + strings.TrimSpace(p.Style) + "\n")
		}
		if len(p.Expertise) > 0 {
			b.WriteString("  expertise: " + strings.Join(p.Expertise, ", ") + "\n")
		}
		if len(p.SignatureLens) > 0 {
			b.WriteString("  signature_lens: " + strings.Join(p.SignatureLens, ", ") + "\n")
		}
		if strings.TrimSpace(p.MasterName) != "" {
			b.WriteString("  master_name: " + strings.TrimSpace(p.MasterName) + "\n")
		}
	}
	b.WriteString("\nSelect the best opening speaker now.")
	return b.String()
}

func buildTurnUserPrompt(input orchestrator.GenerateTurnInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	phase := debatePhase(len(input.Turns), len(input.Personas))

	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\n")

	b.WriteString("Current speaker profile:\n")
	b.WriteString(fmt.Sprintf("- id: %s\n- name: %s\n- role: %s\n- stance: %s\n", input.Speaker.ID, input.Speaker.Name, input.Speaker.Role, input.Speaker.Stance))
	if strings.TrimSpace(input.Speaker.MasterName) != "" {
		b.WriteString("- master_name: " + strings.TrimSpace(input.Speaker.MasterName) + "\n")
		b.WriteString("- master usage requirement: ground this turn in the master's known books, papers, articles, or established frameworks.\n")
	}
	if input.Speaker.Style != "" {
		b.WriteString("- style: " + input.Speaker.Style + "\n")
	}
	if len(input.Speaker.Expertise) > 0 {
		b.WriteString("- expertise: " + strings.Join(input.Speaker.Expertise, ", ") + "\n")
	}
	if len(input.Speaker.Constraints) > 0 {
		b.WriteString("- constraints:\n")
		for _, constraint := range input.Speaker.Constraints {
			b.WriteString("  - " + constraint + "\n")
		}
	}
	b.WriteString("- persona voice guardrail: use the expert name as style inspiration, not identity impersonation.\n")

	signatureLens := input.Speaker.SignatureLens
	if len(signatureLens) > 0 {
		b.WriteString("- signature lens (must be reflected in this turn):\n")
		for _, lens := range signatureLens {
			b.WriteString("  - " + lens + "\n")
		}
	}
	b.WriteString("- persona failure-mode watch: " + derivePersonaFailureMode(input.Speaker) + "\n")

	b.WriteString("\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(participantPromptLine(p) + "\n")
	}

	b.WriteString("\nDebate log so far:\n")
	if len(input.Turns) == 0 {
		b.WriteString("- No previous turns. Start the discussion.\n")
	} else {
		for _, t := range trimTurns(input.Turns, budget.turnRecentLogLimit) {
			b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, budget.turnLogSummaryRunes)))
		}
	}

	b.WriteString("\nDebate phase:\n")
	b.WriteString("- current phase: " + phase + "\n")
	if phase == "exploration" {
		b.WriteString("- objective: expand options, expose assumptions, surface failure modes.\n")
		b.WriteString("- avoid premature convergence; prioritize breadth with concrete evidence.\n")
		b.WriteString("- output expectation: compare at least two plausible options (A/B) and include one discriminating metric or falsifier to test next.\n")
	} else {
		b.WriteString("- objective: compress options, force decisions, and close open risks.\n")
		b.WriteString("- prioritize explicit tradeoff choices, owners, and triggers.\n")
		b.WriteString("- output expectation: choose one provisional option and include owner + trigger/deadline for immediate next action.\n")
	}

	b.WriteString("\nInteraction memory snapshot:\n")
	b.WriteString(buildTurnInteractionSnapshot(input.Turns, input.Speaker, budget))

	personaTurnsSoFar := countPersonaTurns(input.Turns)
	upcomingTurnNo := personaTurnsSoFar + 1
	noNewPointStreak := trailingNoNewPointStreak(input.Turns)
	closeReadiness := summarizeCloseReadiness(input.Turns)
	issueCheckpointRequired := closeReadiness.unresolvedBlockers > 0 ||
		closeReadiness.unownedIssues > 0 ||
		noNewPointStreak >= 2
	qualityCheckpointRequired := upcomingTurnNo%4 == 0 || noNewPointStreak >= 2
	b.WriteString("\nProgress signals:\n")
	b.WriteString(fmt.Sprintf("- upcoming persona turn number: %d\n", upcomingTurnNo))
	b.WriteString(fmt.Sprintf("- trailing persona NEW_POINT=no streak: %d\n", noNewPointStreak))
	b.WriteString(fmt.Sprintf("- close readiness snapshot: unresolved_blockers=%d, unowned_issues=%d, decide_by_signals=%d\n",
		closeReadiness.unresolvedBlockers, closeReadiness.unownedIssues, closeReadiness.decideBySignals))
	if upcomingTurnNo%4 == 0 {
		b.WriteString("- cadence trigger: this is a periodic meta-summary turn (every 4 turns).\n")
	}
	if noNewPointStreak >= 2 {
		b.WriteString("- deadlock signal: repeated no-new-point streak detected; force a concrete option comparison and experiment plan now.\n")
	}

	b.WriteString("\nTurn objective:\n")
	if personaTurnsSoFar == 0 {
		b.WriteString("- this is the first turn: set decision criteria and one key risk to test.\n")
		b.WriteString("- propose option A and option B briefly, then state what metric decides between them.\n")
	} else {
		b.WriteString("- answer the latest moderator request first when available.\n")
		b.WriteString("- respond to one concrete prior claim by speaker name and include at least one [turn-index] citation.\n")
		b.WriteString("- resolve or sharpen one active tension with a condition/metric.\n")
		b.WriteString("- contribute one new insight, not a restatement of your last claim.\n")
		b.WriteString("- before your rebuttal, briefly summarize the strongest opposing view fairly without meta labels.\n")
	}
	b.WriteString("- do not repeat your last two-turn claims unless assumptions changed or evidence is new.\n")
	b.WriteString("- each turn body must include one claim, one reason/mechanism, and one verification condition (metric/trigger/falsifier).\n")
	b.WriteString("- keep narrative human-readable, and keep machine metadata lines standalone (ISSUE_UPDATE/META_DELTA/SELF_CHECK).\n")
	if qualityCheckpointRequired {
		b.WriteString("- quality checkpoint required now: include one evidence-quality clause (evidence_type=data|experience|assumption, confidence=low|medium|high) or one SELF_CHECK line.\n")
	} else {
		b.WriteString("- include evidence-quality clause when confidence/recommendation changes materially.\n")
	}
	if issueCheckpointRequired {
		b.WriteString("- issue-state checkpoint required now: include ISSUE_UPDATE: <issue> | owner=<...> | deadline=<...> | blocker=<...>.\n")
	} else {
		b.WriteString("- include ISSUE_UPDATE only when opening a new issue or when owner/deadline/blocker changes (lightweight cadence, not every turn).\n")
	}
	b.WriteString("- include SELF_CHECK when bias/confidence risk is material for this turn.\n")
	b.WriteString("- metadata labels are machine-readable control data; do not mention those label names inside narrative sentences.\n")
	b.WriteString("- express evidence/inference/assumption naturally without bracket labels like [evidence] or [inference].\n")
	b.WriteString("- if NEW_POINT=no in two consecutive persona turns, switch to decision mode: Option A vs B, choose one provisional option, and request one blocker check.\n")
	if noNewPointStreak >= 2 {
		b.WriteString("- deadlock mode required now: include OPTION_A/OPTION_B micro decision table with upside, risk, and falsifier experiment.\n")
	}
	if upcomingTurnNo%4 == 0 {
		b.WriteString("- periodic meta-summary required now: include META_DELTA with changed/unchanged/next_question before control lines.\n")
	}
	b.WriteString("- include a decision-forcing handoff question for the next speaker.\n")
	b.WriteString("- tail control lines (required, exact order):\n")
	b.WriteString("  - HANDOFF_ASK: <one concrete question the NEXT speaker must answer>\n")
	b.WriteString("  - NEXT: <persona_id> (must match one Participants id exactly; if ambiguous, still choose one explicit id)\n")
	b.WriteString("  - CLOSE: yes|no (yes only if <=2 options, unresolved_blockers<=1, unowned_issues=0, decide_by_signals>=1)\n")
	b.WriteString("  - NEW_POINT: yes|no\n")
	b.WriteString("- keep body to 2-4 short sentences before control lines; avoid recap unless the decision changed.\n")
	b.WriteString("- keep output concise: no long recap of the whole debate.\n")

	b.WriteString("\nNow provide your next utterance.")
	return b.String()
}

func buildJudgeSystemPrompt() string {
	return strings.TrimSpace(`You are a strict consensus judge for a multi-persona debate.
Evaluate whether the participants have reached a workable consensus.
Judging rules:
- Be conservative: set reached=true only if there is clear alignment on goal, approach, and immediate next step.
- If evidence is mixed or insufficient, prefer reached=false and explain the blocking gap.
- Penalize unresolved critical contradictions, blocked dependencies, or unowned risks.
- Score rubric:
  - 0.00-0.39: fragmented, incompatible positions.
  - 0.40-0.69: partial convergence, major unresolved tradeoffs remain.
  - 0.70-0.89: near-consensus with actionable direction but notable risk gaps.
  - 0.90-1.00: workable consensus with explicit decision and next-step clarity.
- In rationale, cite decisive evidence from at least two different speakers/turns when possible.
- Provide next action fields as structured outputs:
  - next_action_owner: who executes now
  - next_action_trigger_or_deadline: when it must run (trigger or deadline)
  - next_action_success_metric: how completion/success is verified
- When reached=false, these next action fields must still be concrete and executable within one cycle.
- Never omit required keys. If uncertain, still fill conservative non-empty placeholders:
  - next_action_owner: "unassigned"
  - next_action_trigger_or_deadline: "next cycle"
  - next_action_success_metric: "completion criteria documented"
- Keep output compact:
  - summary: exactly 1 sentence, <= 24 words.
  - rationale: 1-2 short sentences, <= 45 words total.
  - open_risks: 0-3 items, each <= 8 words.
Return exactly one JSON object with keys (in this exact order):
- reached (boolean)
- score (number from 0 to 1)
- summary (string)
- rationale (string)
- open_risks (array of short strings, max 3 items, empty array if none)
- next_action_owner (string)
- next_action_trigger_or_deadline (string)
- next_action_success_metric (string)
- Output format must be a single-line minified JSON object.
- Self-repair before final output: if your draft is malformed/truncated or has missing keys, rewrite once and return only valid JSON.
- No markdown/code fence, no commentary, no trailing comma, no extra keys, and the final character must be }.`)
}

func buildModeratorSystemPrompt() string {
	return strings.TrimSpace(`You are the moderator of a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Keep intervention compact: 3 core lines (+ optional scorecard lines).
- Use the provided "Debate memory snapshot" as primary grounding context; treat the latest statement as secondary evidence.
- Avoid recency bias: do not treat the latest statement as the dominant view unless it is corroborated by earlier turns.
- Keep your intervention structured as: synthesis -> unresolved tradeoff -> targeted next-speaker question.
- Line 1 (SYNTHESIS): summarize the trajectory across multiple recent turns in 1 short sentence.
- Line 2 (TENSION): highlight one highest-impact unresolved tradeoff and what evidence is still missing.
- Line 3 (ASK): direct the next speaker with one decision-forcing prompt tailored to that speaker's signature style.
- Explicitly account for at least one supporting point and one tension/tradeoff from different speakers when possible.
- In the handoff, name at least one specific prior claim (speaker + idea) that the next speaker must respond to.
- Close the loop on your previous intervention: briefly state whether it was answered, partially answered, or still open.
- Cite speaker names when possible so the handoff is traceable.
- Cite at most two turn indexes (e.g., [5], [7]) for grounding instead of long restatement.
- Do not introduce external facts not grounded in the provided debate context.
- Use fixed question format at the end:
  DECISION_CHECK: choose Option A or B; metric_threshold=<number/condition>; decide_by=<time or trigger>.
- DECISION_CHECK must include both metric_threshold and decide_by with concrete non-placeholder values (no TBD/unknown/later/soon).
- metric_threshold must be numeric or explicit condition (for example >=2.5%, p95<300ms, conversion>=15%).
- decide_by must be an explicit deadline or trigger condition.
- If options are still fuzzy, define provisional Option A and Option B in <=8 words each before DECISION_CHECK.
- Every 4th persona turn, append a quantitative rubric line:
  SCORECARD: coherence=<0-100>; executability=<0-100>; risk_coverage=<0-100>
  SCORECARD_REASON: <one short reason tying score deltas to recent turns>
- SCORECARD and SCORECARD_REASON are machine-readable metadata lines; keep them standalone and out of narrative prose.
- If the next speaker has master_name, explicitly ask them to apply that master's known books, papers, or articles.
- Keep it concise and actionable: about 3 core lines / up to 5 short sentences.
- Return plain text only, without markdown.`)
}

func buildModeratorUserPrompt(input orchestrator.GenerateModeratorInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	personaTurnCount := countPersonaTurns(input.Turns)

	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(participantPromptLine(p) + "\n")
	}

	recentTurns := trimTurns(input.Turns, budget.moderatorRecentLogLimit)
	b.WriteString("\nRecent debate log:\n")
	for _, t := range recentTurns {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, budget.moderatorLogSummaryRunes)))
	}

	b.WriteString("\nModerator balancing guidance:\n")
	b.WriteString("- treat the latest persona statement as one data point, not the whole debate.\n")
	b.WriteString("- use compact 3-block output: SYNTHESIS -> TENSION -> ASK.\n")
	b.WriteString("- ground synthesis/tradeoff in multiple recent turns and speakers.\n")
	b.WriteString("- explicitly point the next speaker to at least one prior claim they must answer.\n")
	b.WriteString("- ask for decision-ready output (metric/trigger/owner/option) rather than generic opinion.\n")
	b.WriteString("- end with DECISION_CHECK using this exact structure: choose Option A or B; metric_threshold=...; decide_by=...\n")
	b.WriteString("- in DECISION_CHECK, metric_threshold and decide_by must both be concrete values (no TBD/unknown/later/soon).\n")
	b.WriteString("- for metric_threshold, prefer numeric threshold or explicit condition operator.\n")
	b.WriteString("- keep metadata labels (SCORECARD/SCORECARD_REASON) standalone and not inside narrative sentences.\n")
	b.WriteString("- avoid long boilerplate recap; emphasize what changed since the previous moderator turn.\n")

	b.WriteString("\nModerator cadence signals:\n")
	b.WriteString(fmt.Sprintf("- persona turns observed so far: %d\n", personaTurnCount))
	if personaTurnCount > 0 && personaTurnCount%4 == 0 {
		b.WriteString("- cadence trigger: include SCORECARD + SCORECARD_REASON in this intervention.\n")
	}

	b.WriteString("\nDebate memory snapshot (anti-recency):\n")
	b.WriteString(buildModeratorMemorySnapshot(recentTurns, input.PreviousTurn, budget.moderatorMemory))

	b.WriteString("\nModerator loop status:\n")
	b.WriteString(buildModeratorLoopStatus(recentTurns, budget.moderatorLoopSummaryRunes))

	b.WriteString("\nLatest persona statement:\n")
	b.WriteString(fmt.Sprintf("[%d][%s] %s\n", input.PreviousTurn.Index, input.PreviousTurn.SpeakerName, summarizeTurnContent(input.PreviousTurn.Content, budget.moderatorLogSummaryRunes)))
	b.WriteString("\nNext speaker:\n")
	b.WriteString(participantPromptLine(input.NextSpeaker) + "\n")
	if strings.TrimSpace(input.NextSpeaker.MasterName) != "" {
		b.WriteString("- next speaker master_name: " + strings.TrimSpace(input.NextSpeaker.MasterName) + "\n")
		b.WriteString("- moderator instruction: ask the next speaker to use ideas from this master's books, papers, or articles.\n")
	}
	nextSpeakerLens := input.NextSpeaker.SignatureLens
	if len(nextSpeakerLens) > 0 {
		b.WriteString("- next speaker signature lens:\n")
		for _, lens := range nextSpeakerLens {
			b.WriteString("  - " + lens + "\n")
		}
	}
	b.WriteString("\nNow provide the moderator intervention.")
	return b.String()
}

func buildFinalModeratorSystemPrompt() string {
	return strings.TrimSpace(`You are the moderator closing a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Provide a final wrap-up and overall assessment in 3-5 concise sentences.
- Include: key agreements, unresolved risks, and a practical next-step recommendation.
- Incorporate the consensus score/rationale as confidence calibration (without repeating raw JSON).
- End with one clear decision-oriented concluding sentence.
- Return plain text only, without markdown.`)
}

func buildFinalModeratorUserPrompt(input orchestrator.GenerateFinalModeratorInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(participantPromptLine(p) + "\n")
	}

	b.WriteString("\nFinal status:\n")
	b.WriteString(fmt.Sprintf("- status: %s\n", input.FinalStatus))
	b.WriteString(fmt.Sprintf("- consensus reached: %t\n", input.Consensus.Reached))
	b.WriteString(fmt.Sprintf("- consensus score: %.2f\n", input.Consensus.Score))
	if strings.TrimSpace(input.Consensus.Summary) != "" {
		b.WriteString("- consensus summary: " + strings.TrimSpace(input.Consensus.Summary) + "\n")
	}
	if strings.TrimSpace(input.Consensus.Rationale) != "" {
		b.WriteString("- judge rationale: " + strings.TrimSpace(input.Consensus.Rationale) + "\n")
	}
	if len(input.Consensus.OpenRisks) > 0 {
		b.WriteString("- open risks:\n")
		for _, risk := range input.Consensus.OpenRisks {
			b.WriteString("  - " + strings.TrimSpace(risk) + "\n")
		}
	}
	if strings.TrimSpace(input.Consensus.NextActionOwner) != "" {
		b.WriteString("- next action owner: " + strings.TrimSpace(input.Consensus.NextActionOwner) + "\n")
	}
	if strings.TrimSpace(input.Consensus.NextActionTrigger) != "" {
		b.WriteString("- next action trigger/deadline: " + strings.TrimSpace(input.Consensus.NextActionTrigger) + "\n")
	}
	if strings.TrimSpace(input.Consensus.NextActionSuccessMetric) != "" {
		b.WriteString("- next action success metric: " + strings.TrimSpace(input.Consensus.NextActionSuccessMetric) + "\n")
	}
	if strings.TrimSpace(input.Consensus.RequiredNextAction) != "" {
		b.WriteString("- required next action: " + strings.TrimSpace(input.Consensus.RequiredNextAction) + "\n")
	}

	b.WriteString("\nDebate log tail:\n")
	for _, t := range trimTurns(input.Turns, 20) {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, moderatorPromptLogSummaryRune)))
	}
	b.WriteString("\nNow provide the final moderator wrap-up and overall assessment.")
	return b.String()
}

func buildJudgeUserPrompt(input orchestrator.JudgeConsensusInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	judgeTurns := trimTurns(input.Turns, budget.judgeRecentLogLimit)

	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(participantPromptLine(p) + "\n")
	}
	b.WriteString("\nDebate log:\n")
	for _, t := range judgeTurns {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, budget.judgeLogSummaryRunes)))
	}
	b.WriteString("\nDecision-state snapshot:\n")
	b.WriteString(buildJudgeDecisionStateSnapshot(judgeTurns))
	b.WriteString("\nOutput format reminder:\n")
	b.WriteString("- return one minified JSON object on a single line only.\n")
	b.WriteString("- key order: reached, score, summary, rationale, open_risks, next_action_owner, next_action_trigger_or_deadline, next_action_success_metric.\n")
	b.WriteString("- never omit keys; if uncertain, use placeholders: next_action_owner=\"unassigned\", next_action_trigger_or_deadline=\"next cycle\", next_action_success_metric=\"completion criteria documented\".\n")
	b.WriteString("- no markdown/code fence, and the final character must be }.\n")
	return b.String()
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
	deadline string
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
		if !isPlaceholderValue(state.deadline) {
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
			deadline := strings.TrimSpace(state.deadline)
			blocker := strings.TrimSpace(state.blocker)
			if owner == "" {
				owner = "unassigned"
			}
			if deadline == "" {
				deadline = "none"
			}
			if blocker == "" {
				blocker = "none"
			}
			b.WriteString(fmt.Sprintf("  - %s: owner=%s; deadline=%s; blocker=%s\n", state.issue, owner, deadline, blocker))
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
			state.deadline = strings.TrimSpace(segment[len("deadline="):])
		case strings.HasPrefix(lower, "decide_by="):
			state.deadline = strings.TrimSpace(segment[len("decide_by="):])
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
	lower := strings.ToLower(line)
	idx := strings.Index(lower, key)
	if idx < 0 {
		return ""
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
	return value == "" || value == "none" || value == "n/a" || value == "na" || value == "tbd" || value == "unknown" || value == "-"
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
	line := fmt.Sprintf("- %s (%s): %s", persona.DisplayName(p), p.ID, p.Role)
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
