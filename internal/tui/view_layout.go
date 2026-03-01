package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) resizeLayout() {
	availableWidth := maxInt(1, m.width-viewChromeStyle.GetHorizontalFrameSize())
	targetInputWidth := maxInt(8, m.width-12)
	maxInputWidth := maxInt(8, styleTextWidth(viewInputBoxStyle, availableWidth)-8)
	if targetInputWidth > maxInputWidth {
		targetInputWidth = maxInputWidth
	}
	m.input.Width = targetInputWidth

	contentWidth := maxInt(1, m.width-viewChromeStyle.GetHorizontalFrameSize())
	hero := m.renderHero(contentWidth)
	commands := m.renderCommandRibbon(contentWidth)
	footer := m.renderFooter(contentWidth)
	availableBodyH := m.height - viewChromeStyle.GetVerticalFrameSize() - lipgloss.Height(hero) - lipgloss.Height(commands) - lipgloss.Height(footer)
	minStandardBodyOuterH := maxInt(6, viewPanelStyle.GetVerticalFrameSize()+4)

	if m.width < 76 || availableBodyH < minStandardBodyOuterH {
		compactContentWidth := maxInt(1, m.width-viewChromeStyle.GetHorizontalFrameSize())
		title, meta, compactCommands, hint, prompt := m.compactViewSections(compactContentWidth)
		staticH := lipgloss.Height(title) + lipgloss.Height(meta) + lipgloss.Height(compactCommands) + lipgloss.Height(hint) + lipgloss.Height(prompt)
		logH := m.height - viewChromeStyle.GetVerticalFrameSize() - staticH
		m.logViewport.Width = maxInt(8, compactContentWidth)
		m.logViewport.Height = maxInt(1, logH)
		m.refreshLogViewport()
		return
	}

	leftOuterW := minInt(48, maxInt(32, contentWidth/3))
	rightOuterW := maxInt(36, contentWidth-leftOuterW-1)
	panelOuterH := maxInt(minStandardBodyOuterH, availableBodyH)
	panelTextRightW := styleTextWidth(viewPanelStyle, rightOuterW)
	panelTextH := styleTextHeight(viewPanelStyle, panelOuterH)
	rightHeader := m.renderPanelHeader("DEBATE LOG", "lines=0", maxInt(12, panelTextRightW))
	rightBodyH := maxInt(1, panelTextH-lipgloss.Height(rightHeader))

	m.logViewport.Width = panelTextRightW
	m.logViewport.Height = rightBodyH
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
