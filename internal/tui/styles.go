package tui

import "github.com/charmbracelet/lipgloss"

var (
	cOrange = lipgloss.Color("#FF5500")
	cInk    = lipgloss.Color("#313638")
	cGrey   = lipgloss.Color("#8A8D8F")
	cPaper  = lipgloss.Color("#F7F4EE")
	cGreen  = lipgloss.Color("#2E7D32")
	cRed    = lipgloss.Color("#BE122F")
	cYellow = lipgloss.Color("#E0A100")
	cBlue   = lipgloss.Color("#3B7DD8")

	sTitle    = lipgloss.NewStyle().Bold(true).Foreground(cPaper).Background(cOrange).Padding(0, 1)
	sTitleBar = lipgloss.NewStyle().Background(cInk).Foreground(cPaper)
	sDim      = lipgloss.NewStyle().Foreground(cGrey)
	sBold     = lipgloss.NewStyle().Bold(true)
	sOK       = lipgloss.NewStyle().Foreground(cGreen)
	sWarn     = lipgloss.NewStyle().Foreground(cYellow)
	sErr      = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sInfo     = lipgloss.NewStyle().Foreground(cBlue)
	sAccent   = lipgloss.NewStyle().Foreground(cOrange).Bold(true)
	sKey      = lipgloss.NewStyle().Foreground(cOrange).Bold(true)
	sFooter   = lipgloss.NewStyle().Foreground(cGrey)

	sPanel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cGrey).Padding(0, 1)
	sPanelFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cOrange).Padding(0, 1)
	sSidebar    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cGrey).Padding(0, 1)
	sModal      = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(cOrange).Padding(1, 2)

	sStepActive  = lipgloss.NewStyle().Foreground(cOrange).Bold(true)
	sStepDone    = lipgloss.NewStyle().Foreground(cGreen)
	sStepPending = lipgloss.NewStyle().Foreground(cGrey)
	sStepFailed  = lipgloss.NewStyle().Foreground(cRed)
	sStepSkipped = lipgloss.NewStyle().Foreground(cGrey).Strikethrough(true)

	sBadgeNew    = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sBadgeExists = lipgloss.NewStyle().Foreground(cBlue).Bold(true)
	sBadgeConf   = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sBadgeSkip   = lipgloss.NewStyle().Foreground(cGrey)
)

func glyph(status string) string {
	switch status {
	case "done":
		return sStepDone.Render("✔")
	case "active":
		return sStepActive.Render("▶")
	case "failed":
		return sStepFailed.Render("✖")
	case "skipped":
		return sStepSkipped.Render("–")
	case "running":
		return sStepActive.Render("●")
	}
	return sStepPending.Render("○")
}

func badge(review string) string {
	switch {
	case review == "NEW" || review == "NEW-DUP":
		return sBadgeNew.Render(review)
	case review == "EXISTS-UNMANAGED":
		return sBadgeExists.Render(review)
	case len(review) >= 8 && review[:8] == "CONFLICT":
		return sBadgeConf.Render(review)
	case len(review) >= 7 && review[:7] == "SKIPPED":
		return sBadgeSkip.Render(review)
	case len(review) >= 6 && review[:6] == "UPDATE":
		return sBadgeExists.Render(review)
	}
	return review
}

func key(k, label string) string { return sKey.Render(k) + sFooter.Render(" "+label) }
