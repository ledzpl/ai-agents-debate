package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	viewChromeStyle = lipgloss.NewStyle().Padding(0, 1)

	viewHeroStyle       = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("67")).Background(lipgloss.Color("234")).Padding(0, 1)
	viewBrandPillStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("31")).Padding(0, 1)
	viewSubtitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	viewStatusRunStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("166")).Padding(0, 1)
	viewStatusIdleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("60")).
				Padding(0, 1)

	viewChipStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("237")).Padding(0, 1)
	viewChipHotStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("39")).Padding(0, 1)
	viewMetricStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("188"))
	viewResultTagStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("24")).
				Padding(0, 1)
	viewResultPathStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("151"))

	viewPanelStyle      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("60")).Background(lipgloss.Color("235")).Padding(0, 1)
	viewPanelTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("223"))
	viewPanelMetaStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	viewPanelRuleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	viewCmdRibbonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("188")).Background(lipgloss.Color("236")).Padding(0, 1)
	viewKeycapStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("31")).Padding(0, 1)
	viewCmdTextStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("151"))

	viewHintStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("151"))
	viewHintLabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("223"))
	viewInputBoxStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("67")).Background(lipgloss.Color("236")).Padding(0, 1)
	viewInputLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("31")).
				Padding(0, 1)
	viewInputMetaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("109"))
)

func (m model) View() string {
	contentWidth := maxInt(1, m.width-viewChromeStyle.GetHorizontalFrameSize())
	hero := m.renderHero(contentWidth)
	commands := m.renderCommandRibbon(contentWidth)
	footer := m.renderFooter(contentWidth)

	availableBodyH := m.height - viewChromeStyle.GetVerticalFrameSize() - lipgloss.Height(hero) - lipgloss.Height(commands) - lipgloss.Height(footer)
	minStandardBodyOuterH := maxInt(6, viewPanelStyle.GetVerticalFrameSize()+4)
	if m.width < 76 || availableBodyH < minStandardBodyOuterH {
		return m.clampToWindow(m.renderCompactView())
	}

	leftOuterW := minInt(48, maxInt(32, contentWidth/3))
	rightOuterW := maxInt(36, contentWidth-leftOuterW-1)
	panelOuterH := maxInt(minStandardBodyOuterH, availableBodyH)
	panelBoxLeftW := styleBoxWidth(viewPanelStyle, leftOuterW)
	panelBoxRightW := styleBoxWidth(viewPanelStyle, rightOuterW)
	panelTextLeftW := styleTextWidth(viewPanelStyle, leftOuterW)
	panelTextRightW := styleTextWidth(viewPanelStyle, rightOuterW)
	panelBoxH := styleBoxHeight(viewPanelStyle, panelOuterH)
	panelTextH := styleTextHeight(viewPanelStyle, panelOuterH)
	leftHeader := m.renderPanelHeader("PERSONAS", fmt.Sprintf("loaded=%d", len(m.personas)), maxInt(12, panelTextLeftW))
	leftBodyH := maxInt(1, panelTextH-lipgloss.Height(leftHeader))

	personaBody := m.buildPersonaPanel(maxInt(12, panelTextLeftW), maxInt(1, leftBodyH))
	personaPanel := viewPanelStyle.
		Width(panelBoxLeftW).
		Height(panelBoxH).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			leftHeader,
			personaBody,
		))

	lastSpeaker := "-"
	if strings.TrimSpace(m.lastSpeakerName) != "" {
		lastSpeaker = m.lastSpeakerName
	}
	logMeta := fmt.Sprintf("lines=%d  follow=%s  last=%s", len(m.logs), onOff(m.autoFollow), truncateText(lastSpeaker, 20))
	rightHeader := m.renderPanelHeader("DEBATE LOG", logMeta, maxInt(12, panelTextRightW))
	logPanel := viewPanelStyle.
		Width(panelBoxRightW).
		Height(panelBoxH).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			rightHeader,
			m.logViewport.View(),
		))

	body := lipgloss.JoinHorizontal(lipgloss.Top, personaPanel, " ", logPanel)

	rendered := viewChromeStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		hero,
		commands,
		body,
		footer,
	))
	return m.clampToWindow(rendered)
}

func (m model) renderCompactView() string {
	contentWidth := maxInt(1, m.width-viewChromeStyle.GetHorizontalFrameSize())
	title, meta, commands, hint, prompt := m.compactViewSections(contentWidth)

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

func (m model) clampToWindow(view string) string {
	maxW := maxInt(1, m.width)
	maxH := maxInt(1, m.height)
	return lipgloss.NewStyle().MaxWidth(maxW).MaxHeight(maxH).Render(view)
}

func (m model) compactViewSections(contentWidth int) (string, string, string, string, string) {
	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		viewBrandPillStyle.Render("DEBATE"),
		" ",
		m.statusBadge(),
	)
	meta := viewMetricStyle.Render(fmt.Sprintf("turns=%d personas=%d follow=%s", m.totalTurnCount, len(m.personas), onOff(m.autoFollow)))
	commands := m.renderCommandRibbon(contentWidth)
	hint := lipgloss.JoinHorizontal(lipgloss.Left, viewHintLabelStyle.Render("hint"), " ", viewHintStyle.Render(m.inputHint()))
	promptBoxWidth := styleBoxWidth(viewInputBoxStyle, contentWidth)
	prompt := viewInputBoxStyle.Width(promptBoxWidth).Render(
		lipgloss.JoinHorizontal(lipgloss.Left, viewInputLabelStyle.Render("INPUT"), " ", m.input.View()),
	)

	clamp := lipgloss.NewStyle().MaxWidth(contentWidth)
	return clamp.Render(title), clamp.Render(meta), clamp.Render(commands), clamp.Render(hint), clamp.Render(prompt)
}

func (m model) renderHero(width int) string {
	heroBoxWidth := styleBoxWidth(viewHeroStyle, width)
	heroTextWidth := styleTextWidth(viewHeroStyle, width)
	runtime := "idle"
	if m.running {
		runtime = time.Since(m.runningSince).Round(time.Second).String()
	}

	headerLeft := lipgloss.JoinHorizontal(
		lipgloss.Left,
		viewBrandPillStyle.Render("Debate Studio"),
		" ",
		viewSubtitleStyle.Render("multi-persona orchestration"),
	)
	headerRight := lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.statusBadge(),
		" ",
		m.renderChip("runtime "+runtime, m.running),
	)

	chips := lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.renderChip(fmt.Sprintf("personas %d", len(m.personas)), false),
		m.renderChip(fmt.Sprintf("turns %d", m.totalTurnCount), m.running),
		m.renderChip(fmt.Sprintf("follow %s", onOff(m.autoFollow)), m.autoFollow),
	)

	progress := viewMetricStyle.Render("progress  " + m.progressLine(maxInt(34, heroTextWidth-12)))
	activity := viewPanelMetaStyle.Render("activity  " + m.personaActivityLine(maxInt(16, heroTextWidth-12)))

	result := ""
	if m.lastResultPath != "" {
		result = lipgloss.JoinHorizontal(
			lipgloss.Left,
			viewResultTagStyle.Render("latest result"),
			" ",
			viewResultPathStyle.Render(truncateText(m.lastResultPath, maxInt(20, heroTextWidth-18))),
		)
	}

	inner := lipgloss.JoinVertical(
		lipgloss.Left,
		joinEnds(maxInt(12, heroTextWidth), headerLeft, headerRight),
		chips,
		progress,
		activity,
		result,
	)
	return viewHeroStyle.Width(heroBoxWidth).Render(inner)
}

func (m model) renderCommandRibbon(width int) string {
	ribbonBoxWidth := styleBoxWidth(viewCmdRibbonStyle, width)
	ribbonTextWidth := styleTextWidth(viewCmdRibbonStyle, width)
	items := []string{
		lipgloss.JoinHorizontal(lipgloss.Left, viewKeycapStyle.Render("Enter"), " ", viewCmdTextStyle.Render("run")),
		lipgloss.JoinHorizontal(lipgloss.Left, viewKeycapStyle.Render("Ctrl+P/N"), " ", viewCmdTextStyle.Render("history")),
		lipgloss.JoinHorizontal(lipgloss.Left, viewKeycapStyle.Render("Ctrl+F"), " ", viewCmdTextStyle.Render("follow")),
		lipgloss.JoinHorizontal(lipgloss.Left, viewKeycapStyle.Render("PgUp/PgDn"), " ", viewCmdTextStyle.Render("scroll")),
		lipgloss.JoinHorizontal(lipgloss.Left, viewKeycapStyle.Render("Ctrl+L"), " ", viewCmdTextStyle.Render("clear")),
	}
	line := strings.Join(items, "  ")
	return viewCmdRibbonStyle.Width(ribbonBoxWidth).Render(truncateText(line, ribbonTextWidth))
}

func (m model) renderFooter(width int) string {
	hint := lipgloss.JoinHorizontal(lipgloss.Left, viewHintLabelStyle.Render("hint"), " ", viewHintStyle.Render(m.inputHint()))
	charCount := len([]rune(strings.TrimSpace(m.input.Value())))
	inputBoxWidth := styleBoxWidth(viewInputBoxStyle, width)
	inputTextWidth := styleTextWidth(viewInputBoxStyle, width)
	inputLine := viewInputBoxStyle.Width(inputBoxWidth).Render(
		joinEnds(
			maxInt(8, inputTextWidth),
			lipgloss.JoinHorizontal(lipgloss.Left, viewInputLabelStyle.Render("INPUT"), " ", m.input.View()),
			viewInputMetaStyle.Render(fmt.Sprintf("%d chars", charCount)),
		),
	)
	return lipgloss.JoinVertical(lipgloss.Left, hint, inputLine)
}

func (m model) renderPanelHeader(title string, meta string, width int) string {
	top := joinEnds(
		width,
		viewPanelTitleStyle.Render(title),
		viewPanelMetaStyle.Render(truncateText(meta, maxInt(8, width/2))),
	)
	rule := viewPanelRuleStyle.Render(strings.Repeat("─", maxInt(8, width)))
	return lipgloss.JoinVertical(lipgloss.Left, top, rule)
}

func (m model) statusBadge() string {
	if m.running {
		return viewStatusRunStyle.Render("RUNNING " + m.spin.View())
	}
	return viewStatusIdleStyle.Render("IDLE")
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

func joinEnds(width int, left string, right string) string {
	left = strings.TrimRight(left, " ")
	right = strings.TrimRight(right, " ")
	if width <= 0 {
		return left + " " + right
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	if leftW+1+rightW > width {
		if width <= leftW+1 {
			return truncateText(left, width)
		}
		right = truncateText(right, maxInt(6, width-leftW-1))
		rightW = lipgloss.Width(right)
		if leftW+1+rightW > width {
			left = truncateText(left, maxInt(6, width-rightW-1))
			leftW = lipgloss.Width(left)
		}
	}

	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
