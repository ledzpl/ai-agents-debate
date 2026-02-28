package tui

import "strings"

func (m *model) resizeLayout() {
	m.input.Width = maxInt(22, m.width-12)

	contentWidth := maxInt(54, m.width-2)
	leftW := minInt(48, maxInt(32, contentWidth/3))
	rightW := maxInt(36, contentWidth-leftW-1)
	panelH := maxInt(8, m.height-16)

	if m.width < 76 || m.height < 18 {
		m.logViewport.Width = maxInt(20, m.width-4)
		m.logViewport.Height = maxInt(5, m.height-8)
		m.refreshLogViewport()
		return
	}

	m.logViewport.Width = maxInt(22, rightW-2)
	m.logViewport.Height = maxInt(5, panelH-4)
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
