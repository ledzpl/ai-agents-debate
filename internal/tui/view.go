package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	viewChromeStyle     = lipgloss.NewStyle().Padding(0, 1)
	viewHeroStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("74")).Background(lipgloss.Color("236")).Padding(0, 1)
	viewTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("30")).Padding(0, 1)
	viewSubtitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("151")).Italic(true)
	viewMetaStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Bold(true)
	viewChipStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("254")).Background(lipgloss.Color("238")).Padding(0, 1)
	viewChipHotStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("31")).Padding(0, 1).Bold(true)
	viewRunningBadge    = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("166")).Bold(true).Padding(0, 1)
	viewIdleBadge       = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("60")).Bold(true).Padding(0, 1)
	viewPanelStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("67")).Background(lipgloss.Color("235")).Padding(0, 1)
	viewPanelTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("222"))
	viewPanelMetaStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	viewCmdRibbonStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Background(lipgloss.Color("236")).Padding(0, 1)
	viewHintStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	viewInputLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("31")).Bold(true).Padding(0, 1)
	viewInputBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("74")).Background(lipgloss.Color("236")).Padding(0, 1)
)

func (m model) View() string {
	if m.width < 76 || m.height < 18 {
		return m.renderCompactView()
	}

	contentWidth := maxInt(54, m.width-2)
	leftW := minInt(48, maxInt(32, contentWidth/3))
	rightW := maxInt(36, contentWidth-leftW-1)
	panelH := maxInt(10, m.height-14)

	hero := m.renderHero(contentWidth)
	commands := m.renderCommandRibbon(contentWidth)

	personaHeader := viewPanelTitleStyle.Render(fmt.Sprintf("PERSONAS (%d)", len(m.personas)))
	personaBody := m.buildPersonaPanel(maxInt(20, leftW-4), maxInt(4, panelH-3))
	personaPanel := viewPanelStyle.
		Width(leftW).
		Height(panelH).
		Render(lipgloss.JoinVertical(lipgloss.Left, personaHeader, personaBody))

	lastSpeaker := "-"
	if strings.TrimSpace(m.lastSpeakerName) != "" {
		lastSpeaker = m.lastSpeakerName
	}
	logMeta := viewPanelMetaStyle.Render(fmt.Sprintf("lines=%d follow=%s last=%s", len(m.logs), onOff(m.autoFollow), truncateText(lastSpeaker, 22)))
	logHeader := viewPanelTitleStyle.Render("DEBATE LOG")
	logPanel := viewPanelStyle.
		Width(rightW).
		Height(panelH).
		Render(lipgloss.JoinVertical(lipgloss.Left, logHeader, logMeta, m.logViewport.View()))

	body := lipgloss.JoinHorizontal(lipgloss.Top, personaPanel, " ", logPanel)
	footer := m.renderFooter(contentWidth)

	return viewChromeStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		hero,
		commands,
		body,
		footer,
	))
}

func (m model) renderCompactView() string {
	status := m.statusBadge()
	title := lipgloss.JoinHorizontal(lipgloss.Left, viewTitleStyle.Render("Debate Studio"), " ", status)
	meta := viewMetaStyle.Render(fmt.Sprintf("turns=%d personas=%d follow=%s", m.totalTurnCount, len(m.personas), onOff(m.autoFollow)))
	commands := viewCmdRibbonStyle.Render("/ask | /stop | /follow | /show | /load | /help | /exit")
	hint := viewHintStyle.Render("hint: " + m.inputHint())
	prompt := viewInputBoxStyle.Render(viewInputLabelStyle.Render("INPUT") + " " + m.input.View())

	return viewChromeStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		meta,
		commands,
		m.logViewport.View(),
		hint,
		prompt,
	))
}

func (m model) renderHero(width int) string {
	titleLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		viewTitleStyle.Render("Debate Studio"),
		" ",
		viewSubtitleStyle.Render("multi-persona orchestration"),
	)

	runtime := "idle"
	if m.running {
		runtime = time.Since(m.runningSince).Round(time.Second).String()
	}

	chips := []string{
		m.renderChip(fmt.Sprintf("personas %d", len(m.personas)), false),
		m.renderChip(fmt.Sprintf("turns %d", m.totalTurnCount), m.running),
		m.renderChip(fmt.Sprintf("follow %s", onOff(m.autoFollow)), m.autoFollow),
		m.renderChip(fmt.Sprintf("runtime %s", runtime), m.running),
	}

	progress := viewMetaStyle.Render(m.progressLine(maxInt(38, width-8)))
	activity := viewPanelMetaStyle.Render("speaker activity  " + m.personaActivityLine(maxInt(18, width-26)))

	resultLine := ""
	if m.lastResultPath != "" {
		resultLine = viewPanelMetaStyle.Render("latest result  " + truncateText(m.lastResultPath, maxInt(20, width-18)))
	}

	return viewHeroStyle.Width(width).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Left, titleLine, "  ", m.statusBadge()),
		lipgloss.JoinHorizontal(lipgloss.Left, chips...),
		progress,
		activity,
		resultLine,
	))
}

func (m model) renderCommandRibbon(width int) string {
	line := "Enter run · Ctrl+P/N history · Ctrl+F follow · PgUp/PgDn/Home/End scroll · Ctrl+L clear"
	return viewCmdRibbonStyle.Width(width).Render(truncateText(line, width))
}

func (m model) renderFooter(width int) string {
	hint := viewHintStyle.Render("hint: " + m.inputHint())
	inputBox := viewInputBoxStyle.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Left, viewInputLabelStyle.Render("INPUT"), " ", m.input.View()),
	)
	return lipgloss.JoinVertical(lipgloss.Left, hint, inputBox)
}

func (m model) statusBadge() string {
	if m.running {
		return viewRunningBadge.Render("RUNNING " + m.spin.View())
	}
	return viewIdleBadge.Render("IDLE")
}

func (m model) renderChip(text string, hot bool) string {
	if hot {
		return viewChipHotStyle.Render(text + " ")
	}
	return viewChipStyle.Render(text + " ")
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
