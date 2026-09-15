package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
)

// --- check / install progress rows ---

// checkRow holds a version-check result.
type checkRow struct {
	name   string
	oldVer string
	newVer string // empty = up to date
	notice string // unsupported-entry reason
	err    error
}

// roleItem tracks the install status of a single role or collection.
type roleItem struct {
	name       string
	version    string
	oldVersion string
	status     string // "pending" | "active" | "done" | "skipped" | "error" | "unsupported"
	notice     string
	err        error
}

// listRow holds an installed role or collection entry.
type listRow struct {
	name    string
	version string
}

// renderProgress renders the check + install phases (one or both).
func (m *Model) renderProgress(innerW int) string {
	var sb strings.Builder

	showCheck := len(m.checkRows) > 0 || m.state == stateChecking
	showInstall := m.state == stateInstalling

	available := m.innerHeight()
	checkMax := available - 1
	installMax := available - 1
	if showCheck && showInstall {
		split := (available - 3) / 2 // 3 = check header + spacer + install header
		checkMax = split
		installMax = split
	}

	if showCheck {
		sb.WriteString(m.renderCheckSection(checkMax))
	}

	if showInstall {
		if len(m.checkRows) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(m.renderInstallSection(innerW, installMax))
	}

	return sb.String()
}

// renderCheckSection renders Phase 1 (version checks), showing only the last maxRows entries.
func (m *Model) renderCheckSection(maxRows int) string {
	var sb strings.Builder
	checking := m.state == stateChecking
	counter := fmt.Sprintf("[%d/%d]", len(m.checkRows), m.checkTotal)
	var hdr string
	if checking {
		hdr = m.spinner.View() + " Phase 1: Checking versions  " + counter
	} else {
		hdr = styleGreen.Render("✓") + " Phase 1: Checking versions  " + counter
	}
	sb.WriteString(styleBoldCol.Render(hdr) + "\n")
	rows := m.checkRows
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[len(rows)-maxRows:]
	}
	for i := range rows {
		sb.WriteString(m.renderCheckRow(&rows[i]) + "\n")
	}
	return sb.String()
}

// renderInstallSection renders Phase 2 (role/collection installs), showing only the last maxRows visible entries.
func (m *Model) renderInstallSection(_, maxRows int) string {
	var sb strings.Builder
	inProgress := m.instDone < m.instActive
	var prefix string
	if inProgress || m.instActive == 0 {
		prefix = m.spinner.View() + " "
	} else {
		prefix = styleGreen.Render("✓") + " "
	}
	counter := fmt.Sprintf("[%d/%d]", m.instDone, m.instActive)
	label := "Installing roles and collections  " + counter
	if m.cfg.UpdateFile {
		label = "Phase 2: " + label
	}
	sb.WriteString(prefix + styleBoldCol.Render(label) + "\n")

	visible := make([]*roleItem, 0, len(m.roleItems))
	for i := range m.roleItems {
		if m.roleItems[i].status != "skipped" {
			visible = append(visible, &m.roleItems[i])
		}
	}
	if len(visible) == 0 {
		sb.WriteString("  " + styleDim.Render(". all items are up to date") + "\n")
		return sb.String()
	}
	if maxRows > 0 && len(visible) > maxRows {
		visible = visible[len(visible)-maxRows:]
	}
	for _, item := range visible {
		sb.WriteString(m.renderRoleItem(item) + "\n")
	}
	return sb.String()
}

// renderCheckRow renders a single version-check row.
func (m *Model) renderCheckRow(row *checkRow) string {
	if row.notice != "" {
		return "  " + styleDim.Render(".") + "  " + styleDim.Render(row.name+"  ("+row.notice+")")
	}
	if row.err != nil {
		return "  " + styleRed.Render("✗") + "  " + styleRed.Render(row.name) + "  " + styleDim.Render(row.err.Error())
	}
	if row.newVer != "" {
		return "  " + styleGreen.Render("✓") + "  " + row.name + "  " +
			styleDim.Render(row.oldVer) + styleYellow.Render(" → ") + styleGreen.Render(row.newVer)
	}
	return "  " + styleDim.Render(".") + "  " + styleDim.Render(row.name+"  "+row.oldVer+"  (up to date)")
}

// renderRoleItem renders a single install-progress row.
func (m *Model) renderRoleItem(item *roleItem) string {
	ico := icon(item.status)
	switch item.status {
	case "active":
		return "  " + ico + "  " + item.name + "  " + styleDim.Render(item.version) + styleCyan.Render("  ...")
	case "done":
		if item.oldVersion != "" && item.oldVersion != item.version {
			return "  " + ico + "  " + item.name + "  " +
				styleDim.Render(item.oldVersion) + styleYellow.Render(" → ") + styleGreen.Render(item.version)
		}
		return "  " + ico + "  " + item.name + "  " + styleDim.Render(item.version)
	case "error":
		errStr := ""
		if item.err != nil {
			errStr = "  " + styleRed.Render(item.err.Error())
		}
		return "  " + ico + "  " + styleRed.Render(item.name) + errStr
	case "unsupported":
		return "  " + ico + "  " + styleDim.Render(item.name+"  ("+item.notice+")")
	case "pending":
		return "  " + ico + "  " + styleDim.Render(item.name+"  "+item.version+"  pending")
	}
	return "  " + ico + "  " + item.name
}

// renderListContent builds the viewport content for list mode.
func (m *Model) renderListContent() string {
	if len(m.listRows) == 0 {
		return styleDim.Render("(nothing installed)")
	}
	maxName := 4
	for _, r := range m.listRows {
		if len(r.name) > maxName {
			maxName = len(r.name)
		}
	}
	var sb strings.Builder
	sb.WriteString(styleBoldDim.Render(padRight("Name", maxName+2)) + styleBoldDim.Render("Version") + "\n")
	sb.WriteString(styleDim.Render(strings.Repeat("─", maxName+14)) + "\n")
	for _, r := range m.listRows {
		sb.WriteString(padRight(r.name, maxName+2) + styleDim.Render(r.version) + "\n")
	}
	return sb.String()
}

// --- layout helpers ---

func (m *Model) innerWidth() int {
	w := m.width - 4 // border (2) + padding (2)
	if w < 40 {
		w = 40
	}
	return w
}

// innerHeight returns available content lines inside the border, minus the verbose log panel when active.
func (m *Model) innerHeight() int {
	h := m.height - 2 // top + bottom border
	if m.cfg.Verbose && len(m.logLines) > 0 && m.state != stateList {
		h -= m.vpHeight() + 2 // divider line + blank
	}
	if h < 4 {
		h = 4
	}
	return h
}

func (m *Model) vpWidth() int {
	return m.innerWidth()
}

func (m *Model) vpHeight() int {
	if m.state == stateList {
		// border(2) + title(1) + blank(1) + blank(1) + "q quit"(1) = 6 overhead
		h := m.height - 6
		if h < 3 {
			h = 3
		}
		return h
	}
	// verbose log panel: bottom 1/3, min 4
	h := m.height / 3
	if h < 4 {
		h = 4
	}
	if h > m.height/2 {
		h = m.height / 2
	}
	return h
}

// padRight right-pads s to width using spaces.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// icon returns the status icon for a role or collection item.
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
		return styleDim.Render("-")
	case "unsupported":
		return styleDim.Render("-")
	default:
		return " "
	}
}

// suppress unused imports
var _ = viewport.New
