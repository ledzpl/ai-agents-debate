package openai

import (
	"fmt"
	"strings"

	"debate/internal/orchestrator"
)

func buildTurnSystemPrompt() string {
	return strings.TrimSpace(`You are one persona in a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Contribute one concise, concrete argument.
- Reference at least one previous point when possible.
- Avoid repeating your own previous claims.
- Keep a clearly distinctive voice aligned with the persona profile, especially signature_lens if provided.
- If a real expert is referenced in the persona name, use it only as inspiration; do not claim to be the real person.
- Return plain text only, without speaker labels or markdown.`)
}

func buildTurnUserPrompt(input orchestrator.GenerateTurnInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\n")

	b.WriteString("Current speaker profile:\n")
	b.WriteString(fmt.Sprintf("- id: %s\n- name: %s\n- role: %s\n- stance: %s\n", input.Speaker.ID, input.Speaker.Name, input.Speaker.Role, input.Speaker.Stance))
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
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", p.Name, p.ID, p.Role))
	}

	b.WriteString("\nDebate log so far:\n")
	if len(input.Turns) == 0 {
		b.WriteString("- No previous turns. Start the discussion.\n")
	} else {
		for _, t := range trimTurns(input.Turns, 16) {
			b.WriteString(fmt.Sprintf("[%d][%s] %s\n", t.Index, t.SpeakerName, t.Content))
		}
	}

	b.WriteString("\nNow provide your next utterance.")
	return b.String()
}

func buildJudgeSystemPrompt() string {
	return strings.TrimSpace(`You are a strict consensus judge for a multi-persona debate.
Evaluate whether the participants have reached a workable consensus.
Return exactly one JSON object with keys:
- reached (boolean)
- score (number from 0 to 1)
- summary (string)
- rationale (string)
No markdown, no extra keys.`)
}

func buildModeratorSystemPrompt() string {
	return strings.TrimSpace(`You are the moderator of a multi-persona debate.
Rules:
- Respond in the same language as the problem statement.
- Summarize the most recent discussion point in 1-2 sentences.
- Highlight one unresolved issue or tradeoff.
- Direct the next speaker with one concrete prompt/question tailored to that speaker's signature style.
- Keep it concise and actionable.
- Return plain text only, without markdown.`)
}

func buildModeratorUserPrompt(input orchestrator.GenerateModeratorInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", p.Name, p.ID, p.Role))
	}

	b.WriteString("\nRecent debate log:\n")
	for _, t := range trimTurns(input.Turns, 10) {
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, t.Content))
	}

	b.WriteString("\nLatest persona statement:\n")
	b.WriteString(fmt.Sprintf("[%d][%s] %s\n", input.PreviousTurn.Index, input.PreviousTurn.SpeakerName, input.PreviousTurn.Content))
	b.WriteString("\nNext speaker:\n")
	b.WriteString(fmt.Sprintf("- %s (%s): %s\n", input.NextSpeaker.Name, input.NextSpeaker.ID, input.NextSpeaker.Role))
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
- Provide a final wrap-up and overall assessment in 2-4 concise sentences.
- Include: key agreements, unresolved risks, and a practical next-step recommendation.
- End with one clear concluding sentence.
- Return plain text only, without markdown.`)
}

func buildFinalModeratorUserPrompt(input orchestrator.GenerateFinalModeratorInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", p.Name, p.ID, p.Role))
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
		b.WriteString(fmt.Sprintf("[%d][%s][%s] %s\n", t.Index, t.SpeakerName, t.Type, t.Content))
	}
	b.WriteString("\nNow provide the final moderator wrap-up and overall assessment.")
	return b.String()
}

func buildJudgeUserPrompt(input orchestrator.JudgeConsensusInput) string {
	var b strings.Builder
	b.WriteString("Problem:\n")
	b.WriteString(input.Problem)
	b.WriteString("\n\nParticipants:\n")
	for _, p := range input.Personas {
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", p.Name, p.ID, p.Role))
	}
	b.WriteString("\nDebate log:\n")
	for _, t := range trimTurns(input.Turns, 24) {
		b.WriteString(fmt.Sprintf("[%d][%s] %s\n", t.Index, t.SpeakerName, t.Content))
	}
	return b.String()
}

func trimTurns(turns []orchestrator.Turn, limit int) []orchestrator.Turn {
	if len(turns) <= limit {
		return turns
	}
	return turns[len(turns)-limit:]
}
