package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	normalStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	runStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("166")).Bold(true).Padding(0, 1)
	idleStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Bold(true).Padding(0, 1)
	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	inputStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))

	statusBadge := idleStatusStyle.Render("IDLE")
	extraStatus := normalStatusStyle.Render(fmt.Sprintf("personas=%d follow=%t", len(m.personas), m.autoFollow))
	if m.running {
		statusBadge = runStatusStyle.Render("RUNNING")
		elapsed := time.Since(m.runningSince).Round(time.Second)
		progressDenominator := fmt.Sprintf("%d", m.maxTurns)
		if m.maxTurns <= 0 {
			progressDenominator = "∞"
		}
		extraStatus = normalStatusStyle.Render(
			fmt.Sprintf(
				"%s elapsed=%s progress=%d/%s totalTurns=%d personas=%d",
				m.spin.View(),
				elapsed.String(),
				m.personaTurnCount,
				progressDenominator,
				m.totalTurnCount,
				len(m.personas),
			),
		)
	}

	headerLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		titleStyle.Render("Multi-Persona Debate TUI"),
		"  ",
		statusBadge,
		"  ",
		extraStatus,
	)
	help := helpStyle.Render("Enter: 실행  Ctrl+P/N: 히스토리  Ctrl+F: auto-follow 토글  PgUp/PgDn/Home/End: 로그 스크롤  Wheel/트랙패드: 로그 스크롤  Ctrl+L: 로그 초기화")
	commands := helpStyle.Render("Commands: /ask <problem> | /stop | /follow [on|off|toggle] | /show | /load | /help | /exit")

	contentWidth := maxInt(40, m.width-2)
	leftW := minInt(42, maxInt(28, contentWidth/3))
	rightW := maxInt(30, contentWidth-leftW-3)
	panelH := maxInt(10, m.height-10)

	personaPanel := panelStyle.
		Width(leftW).
		Height(panelH).
		Render(m.buildPersonaPanel())

	logPanel := panelStyle.
		Width(rightW).
		Height(panelH).
		Render(m.logViewport.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, personaPanel, " ", logPanel)

	prompt := inputStyle.Render("> ") + m.input.View()
	lastResult := ""
	if m.lastResultPath != "" {
		lastResult = helpStyle.Render("last result: " + m.lastResultPath)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		headerLine,
		help,
		commands,
		content,
		lastResult,
		prompt,
	)
}

func (m *model) buildPersonaPanel() string {
	if len(m.personas) == 0 {
		return "Personas\n\n(no personas loaded)"
	}

	lines := []string{"Personas"}
	for i, p := range m.personas {
		lines = append(lines, fmt.Sprintf("%d) %s", i+1, p.Name))
		lines = append(lines, fmt.Sprintf("   role: %s", p.Role))
		lines = append(lines, fmt.Sprintf("   stance: %s", p.Stance))
		if len(p.SignatureLens) > 0 {
			lines = append(lines, fmt.Sprintf("   lens: %s", p.SignatureLens[0]))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) resizeLayout() {
	m.input.Width = maxInt(20, m.width-4)

	contentWidth := maxInt(40, m.width-2)
	leftW := minInt(42, maxInt(28, contentWidth/3))
	rightW := maxInt(30, contentWidth-leftW-3)
	panelH := maxInt(10, m.height-10)

	m.logViewport.Width = maxInt(20, rightW-2)
	m.logViewport.Height = maxInt(5, panelH-2)
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
