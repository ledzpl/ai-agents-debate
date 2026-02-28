package tui

import (
	"fmt"
	"strings"
	"time"

	"debate/internal/persona"
	"github.com/charmbracelet/lipgloss"
)

var (
	viewTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	viewHelpStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	viewNormalStatus    = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	viewRunningStatus   = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("166")).Bold(true).Padding(0, 1)
	viewIdleStatus      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Bold(true).Padding(0, 1)
	viewPanelStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	viewPanelTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	viewInputStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))
	viewHintStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
)

func (m model) View() string {
	statusBadge := viewIdleStatus.Render("IDLE")
	extraStatus := viewNormalStatus.Render(fmt.Sprintf("personas=%d follow=%t", len(m.personas), m.autoFollow))
	if m.running {
		statusBadge = viewRunningStatus.Render("RUNNING")
		elapsed := time.Since(m.runningSince).Round(time.Second)
		extraStatus = viewNormalStatus.Render(
			fmt.Sprintf(
				"%s elapsed=%s turns=%d personas=%d",
				m.spin.View(),
				elapsed.String(),
				m.totalTurnCount,
				len(m.personas),
			),
		)
	}

	headerLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		viewTitleStyle.Render("Multi-Persona Debate TUI"),
		"  ",
		statusBadge,
		"  ",
		extraStatus,
	)
	help := viewHelpStyle.Render("Enter: 실행  Ctrl+P/N: 히스토리  Ctrl+F: auto-follow 토글  PgUp/PgDn/Home/End: 로그 스크롤  Wheel/트랙패드: 로그 스크롤  Ctrl+L: 로그 초기화")
	commands := viewHelpStyle.Render("Commands: /ask <problem> | /stop | /follow [on|off|toggle] | /show | /load | /help | /exit")
	progress := viewHelpStyle.Render(m.progressLine(maxInt(40, m.width-2)))

	contentWidth := maxInt(40, m.width-2)
	leftW := minInt(44, maxInt(30, contentWidth/3))
	rightW := maxInt(32, contentWidth-leftW-3)
	panelH := maxInt(10, m.height-11)

	personaHeader := viewPanelTitleStyle.Render(fmt.Sprintf("PERSONAS (%d)", len(m.personas)))
	personaBody := m.buildPersonaPanel(maxInt(20, leftW-4))
	personaPanel := viewPanelStyle.
		Width(leftW).
		Height(panelH).
		Render(lipgloss.JoinVertical(lipgloss.Left, personaHeader, personaBody))

	lastSpeaker := "-"
	if strings.TrimSpace(m.lastSpeakerName) != "" {
		lastSpeaker = m.lastSpeakerName
	}
	logHeader := viewPanelTitleStyle.Render(fmt.Sprintf("DEBATE LOG (lines=%d, follow=%s, last=%s)", len(m.logs), onOff(m.autoFollow), truncateText(lastSpeaker, 20)))
	logPanel := viewPanelStyle.
		Width(rightW).
		Height(panelH).
		Render(lipgloss.JoinVertical(lipgloss.Left, logHeader, m.logViewport.View()))

	content := lipgloss.JoinHorizontal(lipgloss.Top, personaPanel, " ", logPanel)

	prompt := viewInputStyle.Render("> ") + m.input.View()
	hint := viewHintStyle.Render("hint: " + m.inputHint())
	lastResult := ""
	if m.lastResultPath != "" {
		lastResult = viewHelpStyle.Render("last result: " + m.lastResultPath)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		headerLine,
		help,
		commands,
		progress,
		content,
		lastResult,
		hint,
		prompt,
	)
}

func (m model) progressLine(width int) string {
	if m.maxTurns <= 0 {
		if m.running {
			return fmt.Sprintf("round progress: %d/INF (unbounded)", m.personaTurnCount)
		}
		return "round progress: INF (unbounded)"
	}

	barWidth := minInt(28, maxInt(10, width-56))
	bar := renderProgressBar(barWidth, m.personaTurnCount, m.maxTurns)
	return fmt.Sprintf("round progress: %s %d/%d", bar, m.personaTurnCount, m.maxTurns)
}

func renderProgressBar(width int, current int, total int) string {
	if width <= 0 {
		return "[]"
	}
	if total <= 0 {
		return "[" + strings.Repeat("=", width) + "]"
	}
	ratio := float64(current) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	if current > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(".", width-filled) + "]"
}

func (m model) inputHint() string {
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return "일반 문장을 입력하면 /ask로 자동 실행됩니다."
	}

	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "/ask"):
		return "토론 시작: /ask <problem>"
	case strings.HasPrefix(lower, "/stop"):
		return "실행 중 토론 중지"
	case strings.HasPrefix(lower, "/follow"):
		return "auto-follow 제어: /follow [on|off|toggle]"
	case strings.HasPrefix(lower, "/show"):
		return "현재 로드된 persona 목록 출력"
	case strings.HasPrefix(lower, "/load"):
		return "personas 파일 다시 로드"
	case strings.HasPrefix(lower, "/help"):
		return "도움말 출력"
	case strings.HasPrefix(lower, "/exit"):
		return "애플리케이션 종료"
	case strings.HasPrefix(lower, "/"):
		return "알 수 없는 명령일 수 있습니다. /help로 확인하세요."
	default:
		return "일반 문장 입력은 /ask로 자동 처리됩니다."
	}
}

func (m *model) buildPersonaPanel(width int) string {
	if len(m.personas) == 0 {
		return "(no personas loaded)"
	}

	lines := make([]string, 0, len(m.personas)*4)
	nameWidth := maxInt(10, width-15)
	metaWidth := maxInt(10, width-6)
	lensWidth := maxInt(10, width-8)
	for i, p := range m.personas {
		displayName := persona.DisplayName(p)
		marker := " "
		if strings.TrimSpace(m.lastSpeakerName) != "" && displayName == m.lastSpeakerName {
			marker = ">"
		}
		turns := m.speakerTurns[p.ID]
		line1 := fmt.Sprintf("%s %2d) %s [%dT]", marker, i+1, truncateText(displayName, nameWidth), turns)
		line2 := fmt.Sprintf("    %s", truncateText("role: "+p.Role+" | stance: "+p.Stance, metaWidth))
		lines = append(lines, line1, line2)
		if len(p.SignatureLens) > 0 {
			lines = append(lines, "    "+truncateText("lens: "+p.SignatureLens[0], lensWidth))
		}
		lines = append(lines, "")
	}

	if strings.TrimSpace(m.lastSpeakerName) != "" {
		lines = append(lines, "last speaker: "+truncateText(m.lastSpeakerName, width))
	}
	return strings.Join(lines, "\n")
}

func (m *model) resizeLayout() {
	m.input.Width = maxInt(20, m.width-4)

	contentWidth := maxInt(40, m.width-2)
	leftW := minInt(44, maxInt(30, contentWidth/3))
	rightW := maxInt(32, contentWidth-leftW-3)
	panelH := maxInt(10, m.height-11)

	m.logViewport.Width = maxInt(20, rightW-2)
	m.logViewport.Height = maxInt(5, panelH-3)
	m.refreshLogViewport()
}

func (m *model) refreshLogViewport() {
	m.wrappedWidth = m.logViewport.Width
	m.wrappedLogs = wrapLogLines(m.logs, m.logViewport.Width)
	m.logViewport.SetContent(strings.Join(m.wrappedLogs, "\n"))
	if m.autoFollow {
		m.logViewport.GotoBottom()
	}
}
