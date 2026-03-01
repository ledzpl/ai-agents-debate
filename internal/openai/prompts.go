package openai

import (
	"fmt"
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
- Keep the debate interactive: explicitly connect to one named speaker's prior claim (agree, refine, or challenge).
- If a moderator question/request is provided, answer it in your first sentence before expanding.
- When possible, address one strongest counterpoint from another speaker.
- Add one new delta in each turn (new evidence, boundary condition, metric, dependency, or failure mode), not just restatement.
- If you disagree, state which assumption differs and what evidence would falsify your position.
- If you mostly agree, push toward convergence with a concrete decision criterion or next step.
- Cite 1-2 prior turns by index notation like [3] when relevant.
- Prefer specific, falsifiable statements (assumptions, constraints, metrics, tradeoffs).
- Reference at least one previous point when possible.
- Avoid repeating your own previous claims.
- If a metric/threshold is unchanged from your prior turn, cite it briefly instead of restating the full rationale.
- Keep a clearly distinctive voice aligned with the persona profile, especially signature_lens if provided.
- If a real expert is provided as master_name, use that person's known knowledge from books, papers, and articles as inspiration.
- When master_name exists, include at least one concrete concept/framework from that body of work in your turn.
- Do not claim to be the real person, and do not invent specific titles/dates when you are unsure.
- End with one handoff sentence that helps the next speaker advance the debate.
- End with one final line in this exact format: NEXT: <persona_id> (choose one participant id, not yourself).
- Add one line "CLOSE: yes|no" (yes only if your current view is ready to end the debate now).
- Add one line "NEW_POINT: yes|no" (yes only if this turn adds a materially new point).
- Keep the response compact (roughly 3-6 short sentences, around 110 Korean words).
- Return plain text only, without speaker labels or markdown.`)
}

func buildOpeningSpeakerSelectorSystemPrompt() string {
	return strings.TrimSpace(`You choose which persona should speak first in a multi-persona debate.
Goal:
- Pick the single most relevant persona to start the discussion for the given problem.
- Prioritize domain fit, decision leverage at turn 1, and ability to frame useful criteria for others.
Rules:
- Choose exactly one persona from the provided candidates.
- Return exactly one JSON object with keys:
  - persona_id (string, must match one candidate id exactly)
  - reason (string, one short sentence)
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

	b.WriteString("\nInteraction memory snapshot:\n")
	b.WriteString(buildTurnInteractionSnapshot(input.Turns, input.Speaker, budget))

	b.WriteString("\nTurn objective:\n")
	if len(input.Turns) == 0 {
		b.WriteString("- this is the first turn: set decision criteria and one key risk to test.\n")
	} else {
		b.WriteString("- answer the latest moderator request first when available.\n")
		b.WriteString("- respond to one concrete prior claim by speaker name and include at least one [turn-index] citation.\n")
		b.WriteString("- resolve or sharpen one active tension with a condition/metric.\n")
		b.WriteString("- contribute one new insight, not a restatement of your last claim.\n")
	}
	b.WriteString("- end with a targeted handoff question/request to a specific participant.\n")
	b.WriteString("- final line must be: NEXT: <persona_id> using an exact id from Participants.\n")
	b.WriteString("- include: CLOSE: yes|no and NEW_POINT: yes|no on separate lines at the end.\n")
	b.WriteString("- keep output concise: no long recap of the whole debate.\n")

	b.WriteString("\nNow provide your next utterance.")
	return b.String()
}

func buildJudgeSystemPrompt() string {
	return strings.TrimSpace(`You are a strict consensus judge for a multi-persona debate.
Evaluate whether the participants have reached a workable consensus.
Judging rules:
- Be conservative: set reached=true only if there is clear alignment on goal, approach, and immediate next step.
- Penalize unresolved critical contradictions, blocked dependencies, or unowned risks.
- Score rubric:
  - 0.00-0.39: fragmented, incompatible positions.
  - 0.40-0.69: partial convergence, major unresolved tradeoffs remain.
  - 0.70-0.89: near-consensus with actionable direction but notable risk gaps.
  - 0.90-1.00: workable consensus with explicit decision and next-step clarity.
- In rationale, cite decisive evidence from at least two different speakers/turns when possible.
- Keep output compact: summary in 1 sentence, rationale in 1-2 short sentences.
Return exactly one JSON object with keys:
- reached (boolean)
- score (number from 0 to 1)
- summary (string)
- rationale (string)
No markdown, no extra keys, no trailing text.`)
}

func buildModeratorSystemPrompt() string {
	return strings.TrimSpace(`You are the moderator of a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Start by synthesizing the trajectory across multiple recent turns (not just the latest turn) in 1-2 sentences.
- Avoid recency bias: do not treat the latest statement as the dominant view unless it is corroborated by earlier turns.
- Explicitly account for at least one supporting point and one tension/tradeoff from different speakers when possible.
- Use the provided "Debate memory snapshot" as primary grounding context; treat the latest statement as secondary evidence.
- Keep your intervention structured as: synthesis -> unresolved tradeoff -> targeted next-speaker question.
- Cite speaker names when possible so the handoff is traceable.
- Cite at most two turn indexes (e.g., [5], [7]) for grounding instead of long restatement.
- In the handoff, name at least one specific prior claim (speaker + idea) that the next speaker must respond to.
- Close the loop on your previous intervention: briefly state whether it was answered, partially answered, or still open.
- Do not introduce external facts not grounded in the provided debate context.
- Highlight one unresolved issue or tradeoff.
- Direct the next speaker with one concrete prompt/question tailored to that speaker's signature style.
- Make the question decision-forcing (ask for metric, trigger, owner, or explicit option choice).
- If the next speaker has master_name, explicitly ask them to apply that master's known books, papers, or articles.
- Keep it concise and actionable: about 4 short lines / up to 6 short sentences.
- Minimize repetition: summarize only what changed since the previous moderator turn.
- Return plain text only, without markdown.`)
}

func buildModeratorUserPrompt(input orchestrator.GenerateModeratorInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))

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
	b.WriteString("- ground summary/tradeoff in multiple recent turns and speakers.\n")
	b.WriteString("- when possible, connect one earlier claim and one counterpoint before directing next speaker.\n")
	b.WriteString("- explicitly point the next speaker to at least one prior claim they must answer.\n")
	b.WriteString("- ask for decision-ready output (metric/trigger/owner/option) rather than generic opinion.\n")
	b.WriteString("- avoid long boilerplate recap; emphasize what changed since the previous moderator turn.\n")

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

	b.WriteString("\nDebate log tail:\n")
	for _, t := range trimTurns(input.Turns, 20) {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, moderatorPromptLogSummaryRune)))
	}
	b.WriteString("\nNow provide the final moderator wrap-up and overall assessment.")
	return b.String()
}

func buildJudgeUserPrompt(input orchestrator.JudgeConsensusInput) string {
	budget := derivePromptBudget(len(input.Personas), len(input.Turns))

	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(participantPromptLine(p) + "\n")
	}
	b.WriteString("\nDebate log:\n")
	for _, t := range trimTurns(input.Turns, budget.judgeRecentLogLimit) {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, summarizeTurnContent(t.Content, budget.judgeLogSummaryRunes)))
	}
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
