package tui

import "charm.land/lipgloss/v2"

var (
	colorDim    = lipgloss.Color("240")
	colorGreen  = lipgloss.Color("2")
	colorRed    = lipgloss.Color("1")
	colorCyan   = lipgloss.Color("6")
	colorYellow = lipgloss.Color("3")

	styleNormal  = lipgloss.NewStyle()
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleGreen   = lipgloss.NewStyle().Foreground(colorGreen)
	styleRed     = lipgloss.NewStyle().Foreground(colorRed)
	styleCyan    = lipgloss.NewStyle().Foreground(colorCyan)
	styleYellow  = lipgloss.NewStyle().Foreground(colorYellow)
	styleBoldDim = lipgloss.NewStyle().Bold(true).Foreground(colorDim)
	styleBoldCol = lipgloss.NewStyle().Bold(true)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(0, 1)

	styleLogDivider = lipgloss.NewStyle().Foreground(colorDim)
	styleTitle      = lipgloss.NewStyle().Bold(true)
)

// icon returns the status icon for a role item.
func icon(status string) string {
	switch status {
	case "done":
		return styleGreen.Render("✓")
	case "error":
		return styleRed.Render("✗")
	case "active":
		return styleCyan.Render("●")
	case "pending":
		return styleDim.Render("○")
	case "skipped":
		return styleDim.Render("–")
	default:
		return " "
	}
}

// suppress unused style warnings
var _ = styleNormal
