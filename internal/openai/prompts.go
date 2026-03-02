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
	return strings.TrimSpace(`You are one persona in a multi-persona debate.
Rules:
- Priority if instructions conflict: control-line/output format rules > direct moderator request > decision progress > persona style.
- Respond in the same language as the problem statement.
- Adapt explanation depth to audience_mode from the user prompt (general|expert).
- If audience_mode=general, prefer everyday words and keep technical terms to <=3 with one short parenthetical explanation each.
- If audience_mode=expert, use precise technical language when useful, but keep it concise and readable.
- Keep each narrative sentence short in a language-neutral way (roughly one to two clauses; for long scripts, target <=80 characters).
- Structure your turn as: core claim -> reason/mechanism -> practical implication.
- Each turn body must include: one claim, one reason/mechanism, and one verification condition (metric, trigger, or falsifier).
- Include one plain-language user-impact sentence (why this matters to a general user).
- Ground claims in observed evidence, inference, or assumption naturally, without bracket labels.
- For your main claim, include one concise evidence-quality clause (evidence_type=data|experience|assumption, confidence=low|medium|high) when confidence or recommendation changed.
- Keep narrative text and machine-readable metadata separate.
- Use metadata labels only as standalone lines; do not embed labels (ISSUE_UPDATE/META_DELTA/SELF_CHECK/OPTION_A/OPTION_B) inside prose sentences.
- Do not translate or rename machine control labels; keep exact uppercase ASCII labels (ISSUE_UPDATE, META_DELTA, SELF_CHECK, OPTION_A, OPTION_B, HANDOFF_ASK, NEXT, CLOSE, NEW_POINT).
- Include one unresolved-issue registry line when opening a new issue or when owner/decide_by/blocker changed:
  ISSUE_UPDATE: <issue> | owner=<id/name or unassigned> | decide_by=<date/trigger> | blocker=<none or blocker>
- ISSUE_UPDATE quality rule: owner may be unassigned, but decide_by and blocker must be concrete non-placeholder values (no TBD/unknown/later/soon).
- Do not emit ISSUE_UPDATE or SELF_CHECK when nothing changed and no checkpoint is requested.
- Keep the debate interactive: explicitly connect to one named speaker's prior claim (agree, refine, or challenge).
- If a moderator question/request is provided, answer it in your first sentence before expanding.
- Before your main rebuttal, include one fair strongest-form summary of an opposing view.
- Do this implicitly; do not mention prompt techniques or bracket-label taxonomies in output.
- Add one new delta in each turn (new evidence, boundary condition, metric, dependency, or failure mode), not just restatement.
- Deadlock breaker: if the same tension keeps repeating or NEW_POINT=no repeats, include a 2-option micro decision table:
  OPTION_A: <upside> | <risk> | <falsifier experiment>
  OPTION_B: <upside> | <risk> | <falsifier experiment>
- If you disagree, state which assumption differs and what evidence would falsify your position.
- If you mostly agree, push toward convergence with a concrete decision criterion or next step.
- Cite 1-2 prior turns by index notation like [3] when relevant.
- Do not invent turn indexes; cite only indexes verifiable from the provided debate log.
- Prefer specific, falsifiable statements (assumptions, constraints, metrics, tradeoffs).
- Do not repeat claims from your last two turns unless assumptions changed or new evidence is added.
- If a metric/threshold is unchanged from your prior turn, cite it briefly instead of restating the full rationale.
- Keep a clearly distinctive voice aligned with the persona profile, especially signature_lens if provided.
- If a real expert is provided as master_name, use that person's known knowledge from books, papers, and articles as inspiration.
- When master_name exists, include at least one concrete concept/framework from that body of work in your turn.
- Do not claim to be the real person, and do not invent specific titles/dates when you are unsure.
- When your likely persona failure mode could distort this turn, include one short self-correction line:
  SELF_CHECK: <likely bias/failure mode> -> <mitigation in this turn>
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
- Self-repair before final output: if any required control line is missing/misordered, or NEXT points to self/non-participant, rewrite once and return corrected output.
- Every 4th persona turn, include one short line before control lines:
  META_DELTA: changed=<what changed>; unchanged=<what is still unresolved>; next_question=<one must-answer question>
- Keep the response compact: body in 2-5 short sentences before control lines.
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
		id := strings.TrimSpace(p.ID)
		name := strings.TrimSpace(p.Name)
		role := strings.TrimSpace(p.Role)
		b.WriteString(fmt.Sprintf("- id: %s\n", id))
		if name != "" {
			b.WriteString(fmt.Sprintf("  name: %s\n", name))
		}
		if role != "" {
			b.WriteString(fmt.Sprintf("  role: %s\n", role))
		}
		if strings.TrimSpace(p.Stance) != "" {
			b.WriteString("  stance: " + strings.TrimSpace(p.Stance) + "\n")
		}
		if strings.TrimSpace(p.Style) != "" {
			b.WriteString("  style: " + strings.TrimSpace(p.Style) + "\n")
		}
		expertise := normalizePromptList(p.Expertise)
		if len(expertise) > 0 {
			b.WriteString("  expertise: " + strings.Join(expertise, ", ") + "\n")
		}
		signatureLens := normalizePromptList(p.SignatureLens)
		if len(signatureLens) > 0 {
			b.WriteString("  signature_lens: " + strings.Join(signatureLens, ", ") + "\n")
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
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\n")

	b.WriteString("Current speaker profile:\n")
	b.WriteString(fmt.Sprintf("- id: %s\n- name: %s\n- role: %s\n",
		strings.TrimSpace(input.Speaker.ID),
		strings.TrimSpace(input.Speaker.Name),
		strings.TrimSpace(input.Speaker.Role),
	))
	if stance := strings.TrimSpace(input.Speaker.Stance); stance != "" {
		b.WriteString("- stance: " + stance + "\n")
	}
	if strings.TrimSpace(input.Speaker.MasterName) != "" {
		b.WriteString("- master_name: " + strings.TrimSpace(input.Speaker.MasterName) + "\n")
		b.WriteString("- master usage requirement: ground this turn in the master's known books, papers, articles, or established frameworks.\n")
	}
	if style := strings.TrimSpace(input.Speaker.Style); style != "" {
		b.WriteString("- style: " + style + "\n")
	}
	expertise := normalizePromptList(input.Speaker.Expertise)
	if len(expertise) > 0 {
		b.WriteString("- expertise: " + strings.Join(expertise, ", ") + "\n")
	}
	constraints := normalizePromptList(input.Speaker.Constraints)
	if len(constraints) > 0 {
		b.WriteString("- constraints:\n")
		for _, constraint := range constraints {
			b.WriteString("  - " + constraint + "\n")
		}
	}
	b.WriteString("- persona voice guardrail: use the expert name as style inspiration, not identity impersonation.\n")

	signatureLens := normalizePromptList(input.Speaker.SignatureLens)
	if len(signatureLens) > 0 {
		b.WriteString("- signature lens (must be reflected in this turn):\n")
		for _, lens := range signatureLens {
			b.WriteString("  - " + lens + "\n")
		}
	}
	b.WriteString("- persona failure-mode watch: " + derivePersonaFailureMode(input.Speaker) + "\n")

	b.WriteString("\nAudience mode:\n")
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- style target: expert readers; prioritize precision and compact technical reasoning.\n")
	} else {
		b.WriteString("- style target: general readers; prioritize plain language and quick comprehension.\n")
	}

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
		b.WriteString("- output expectation: choose one provisional option and include owner + decide_by for immediate next action.\n")
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
	b.WriteString("- CLOSE decision must use the snapshot above as source of truth; if any CLOSE gate is unmet, set CLOSE=no.\n")
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
		b.WriteString("- respond to one concrete prior claim by speaker name; when claim reference is clear and verifiable, include at least one [turn-index] citation.\n")
		b.WriteString("- resolve or sharpen one active tension with a condition/metric.\n")
		b.WriteString("- contribute one new insight, not a restatement of your last claim.\n")
		b.WriteString("- before your rebuttal, briefly summarize the strongest opposing view fairly without meta labels.\n")
	}
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- audience mode: explain for expert readers with precise terminology and compact logic.\n")
		b.WriteString("- jargon is allowed when useful; define only high-impact domain terms once if ambiguity risk is high.\n")
		b.WriteString("- still include one plain sentence on user impact for non-specialists.\n")
	} else {
		b.WriteString("- audience mode: explain so a non-expert can follow quickly.\n")
		b.WriteString("- avoid unexplained jargon or acronyms; when unavoidable, define once in parentheses.\n")
		b.WriteString("- include one plain sentence on user impact: what changes for users if this is chosen.\n")
	}
	if qualityCheckpointRequired {
		b.WriteString("- quality checkpoint required now: include one evidence-quality clause (evidence_type=data|experience|assumption, confidence=low|medium|high) or one SELF_CHECK line.\n")
	} else {
		b.WriteString("- include evidence-quality clause when confidence/recommendation changes materially.\n")
	}
	if issueCheckpointRequired {
		b.WriteString("- issue-state checkpoint required now: include ISSUE_UPDATE: <issue> | owner=<...> | decide_by=<...> | blocker=<...>.\n")
		b.WriteString("- in ISSUE_UPDATE now, avoid placeholders for decide_by/blocker (no TBD/unknown/later/soon).\n")
	} else {
		b.WriteString("- include ISSUE_UPDATE only when opening a new issue or when owner/decide_by/blocker changes (lightweight cadence, not every turn).\n")
		b.WriteString("- when ISSUE_UPDATE is emitted, keep decide_by/blocker concrete (no TBD/unknown/later/soon).\n")
	}
	if noNewPointStreak >= 2 {
		b.WriteString("- deadlock mode required now: apply the system deadlock breaker (OPTION_A/OPTION_B table).\n")
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
	b.WriteString("- do not translate or rename any control-line label; keep exact ASCII labels.\n")

	b.WriteString("\nNow provide your next utterance.")
	return b.String()
}

func buildJudgeSystemPrompt() string {
	return strings.TrimSpace(`You are a strict consensus judge for a multi-persona debate.
Evaluate whether the participants have reached a workable consensus.
Judging rules:
- Be conservative: set reached=true only if there is clear alignment on goal, approach, and immediate next step.
- Adapt summary/rationale wording depth to audience_mode from the user prompt (general|expert).
- If audience_mode=general, prefer plain-language phrasing in summary/rationale.
- If audience_mode=expert, prefer precise technical phrasing while staying concise.
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
- Never omit required keys. If uncertain, still fill conservative concrete defaults:
  - next_action_owner: "moderator"
  - next_action_trigger_or_deadline: "within 7 days"
  - next_action_success_metric: "owner and deadline documented"
- Avoid placeholder values in next_action fields (no TBD/unknown/later/soon/next cycle).
- Keep output compact:
  - summary: exactly 1 sentence; keep it short in a language-neutral way (roughly <=24 words or <=120 chars).
  - rationale: 1-2 short sentences; keep total length compact in a language-neutral way (roughly <=45 words or <=220 chars).
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
- Output format must be a single-line minified JSON object (no newline characters).
- JSON type constraints: reached must be unquoted true/false; score must be numeric 0..1 (not string, not percent); open_risks must be a JSON array.
- Keys and string values must use standard JSON double quotes.
- JSON template: {"reached":false,"score":0.0,"summary":"...","rationale":"...","open_risks":[],"next_action_owner":"moderator","next_action_trigger_or_deadline":"within 7 days","next_action_success_metric":"owner and deadline documented"}
- Self-repair before final output: if your draft is malformed/truncated or has missing keys, rewrite once and return only valid JSON.
- No markdown/code fence, no commentary, no trailing comma, no extra keys, and the final character must be }.`)
}

func buildModeratorSystemPrompt() string {
	return strings.TrimSpace(`You are the moderator of a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Adapt wording depth to audience_mode from the user prompt (general|expert).
- Keep intervention compact: exactly 4 required lines (+ optional scorecard lines).
- Use the provided "Debate memory snapshot" as primary grounding context; treat the latest statement as secondary evidence.
- Avoid recency bias: do not treat the latest statement as the dominant view unless it is corroborated by earlier turns.
- Keep your intervention structured as: synthesis -> unresolved tradeoff -> targeted next-speaker question.
- Required line format and order:
  SYNTHESIS: <1 short sentence on trajectory across multiple recent turns>
  TENSION: <highest-impact unresolved tradeoff + missing evidence>
  ASK: <decision-forcing prompt tailored to the next speaker style>
  DECISION_CHECK: choose Option A or B; metric_threshold=<number/condition>; decide_by=<time or trigger>.
- Optional disambiguation lines (only when options are fuzzy, before DECISION_CHECK):
  OPTION_A: <option in <=8 words>
  OPTION_B: <option in <=8 words>
- The 4 required lines remain mandatory even when optional lines are added.
- Explicitly account for at least one supporting point and one tension/tradeoff from different speakers when possible.
- In the handoff, name at least one specific prior claim (speaker + idea) that the next speaker must respond to.
- Close the loop on your previous intervention: briefly state whether it was answered, partially answered, or still open.
- Cite speaker names when possible so the handoff is traceable.
- Cite at most two turn indexes (e.g., [5], [7]) for grounding instead of long restatement.
- Do not introduce external facts not grounded in the provided debate context.
- DECISION_CHECK must include both metric_threshold and decide_by with concrete non-placeholder values (no TBD/unknown/later/soon).
- metric_threshold must be numeric or explicit condition (for example >=2.5%, p95<300ms, conversion>=15%).
- decide_by must be an explicit deadline or trigger condition.
- If options are still fuzzy, define provisional Option A and Option B in <=8 words each before DECISION_CHECK.
- Do not translate or rename required labels; keep exact uppercase ASCII labels (SYNTHESIS, TENSION, ASK, DECISION_CHECK, OPTION_A, OPTION_B, SCORECARD, SCORECARD_REASON).
- Every 4th persona turn, append a quantitative rubric line:
  SCORECARD: coherence=<0-100>; executability=<0-100>; risk_coverage=<0-100>
  SCORECARD_REASON: <one short reason tying score deltas to recent turns>
- SCORECARD and SCORECARD_REASON are machine-readable metadata lines; keep them standalone and out of narrative prose.
- If the next speaker has master_name, explicitly ask them to apply that master's known books, papers, or articles.
- Keep it concise and actionable: about 4 core lines / up to 6 short sentences.
- Self-repair before final output: if required line prefixes/order are broken or DECISION_CHECK misses metric_threshold/decide_by, rewrite once and return corrected output.
- Do not add prose before SYNTHESIS or after final metadata line.
- Return plain text only, without markdown.`)
}

func buildModeratorUserPrompt(input orchestrator.GenerateModeratorInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	personaTurnCount := countPersonaTurns(input.Turns)
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

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
	b.WriteString("- use compact 4-line output: SYNTHESIS -> TENSION -> ASK -> DECISION_CHECK.\n")
	b.WriteString("- use exact standalone prefixes in order: SYNTHESIS:, TENSION:, ASK:, DECISION_CHECK:.\n")
	b.WriteString("- if options are fuzzy, add optional OPTION_A:/OPTION_B: lines before DECISION_CHECK (these do not replace required 4 lines).\n")
	b.WriteString("- never translate or rename label prefixes; use exact uppercase ASCII labels.\n")
	b.WriteString("- audience mode is " + audienceMode + "; tune wording depth accordingly.\n")
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- expert mode: keep precision high and avoid over-explaining basics.\n")
	} else {
		b.WriteString("- general mode: favor plain language, avoid unexplained acronyms/jargon.\n")
	}
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
	nextSpeakerLens := normalizePromptList(input.NextSpeaker.SignatureLens)
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
- Adapt explanation depth to audience_mode from the user prompt (general|expert).
- Provide a final wrap-up and overall assessment in 3-5 concise sentences.
- First sentence must be a plain-language verdict that non-experts can understand immediately.
- Include: key agreements, unresolved risks, and a practical next-step recommendation.
- Include one concrete action sentence in what/who/when format.
- If audience_mode=general, avoid unexplained acronyms/jargon; if unavoidable, add short parenthetical definitions.
- If audience_mode=expert, precise terminology is allowed; define only high-impact terms when ambiguity risk is high.
- Do not introduce new facts beyond the provided debate and judge context; if uncertain, state uncertainty briefly.
- Incorporate the consensus score/rationale as confidence calibration (without repeating raw JSON).
- End with one clear decision-oriented concluding sentence.
- Return plain text only, without markdown.`)
}

func buildFinalModeratorUserPrompt(input orchestrator.GenerateFinalModeratorInput) string {
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))

	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(participantPromptLine(p) + "\n")
	}
	b.WriteString("\nAudience mode:\n")
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- expert mode: concise and precise closing summary.\n")
	} else {
		b.WriteString("- general mode: plain-language closing summary for non-specialists.\n")
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
		b.WriteString("- next action decide_by: " + strings.TrimSpace(input.Consensus.NextActionTrigger) + "\n")
	}
	if strings.TrimSpace(input.Consensus.NextActionSuccessMetric) != "" {
		b.WriteString("- next action success metric: " + strings.TrimSpace(input.Consensus.NextActionSuccessMetric) + "\n")
	}
	if strings.TrimSpace(input.Consensus.RequiredNextAction) != "" {
		b.WriteString("- required next action: " + strings.TrimSpace(input.Consensus.RequiredNextAction) + "\n")
	}

	b.WriteString("\nDebate log tail:\n")
	for _, t := range trimTurns(input.Turns, budget.judgeRecentLogLimit) {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, budget.moderatorLogSummaryRunes)))
	}
	b.WriteString("\nNow provide the final moderator wrap-up and overall assessment.")
	return b.String()
}

func buildJudgeUserPrompt(input orchestrator.JudgeConsensusInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))
	judgeTurns := trimTurns(input.Turns, budget.judgeRecentLogLimit)
	audienceMode := normalizePromptAudienceMode(input.AudienceMode)

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
	b.WriteString("\nAudience mode:\n")
	b.WriteString("- requested audience_mode: " + audienceMode + "\n")
	if audienceMode == orchestrator.AudienceModeExpert {
		b.WriteString("- style target: expert readers; keep summary/rationale precise and compact.\n")
	} else {
		b.WriteString("- style target: general readers; keep summary/rationale plain-language.\n")
	}
	b.WriteString("\nOutput format reminder:\n")
	b.WriteString("- return one minified JSON object on a single line only.\n")
	b.WriteString("- key order: reached, score, summary, rationale, open_risks, next_action_owner, next_action_trigger_or_deadline, next_action_success_metric.\n")
	b.WriteString("- never omit keys; if uncertain, use conservative concrete defaults: next_action_owner=\"moderator\", next_action_trigger_or_deadline=\"within 7 days\", next_action_success_metric=\"owner and deadline documented\".\n")
	b.WriteString("- avoid placeholder values in next_action fields (no TBD/unknown/later/soon/next cycle).\n")
	b.WriteString("- type constraints: reached is boolean, score is numeric 0..1 (not percent/string), open_risks is an array (use []).\n")
	b.WriteString("- no markdown/code fence, and the final character must be }.\n")
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
