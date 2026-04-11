// Package tui implements the Bubble Tea TUI for agru.
package tui

import (
	"fmt"
	"os"
	"path"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
)

// Config holds the configuration for the TUI, derived from CLI flags.
type Config struct {
	RequirementsPath string
	RolesPath        string
	DeleteName       string
	Limit            int
	ListInstalled    bool
	InstallMissing   bool
	UpdateFile       bool
	Cleanup          bool
	Verbose          bool
	Keep             bool // keep the TUI open after completion until 'q'
}

type appState int

const (
	stateInit       appState = iota // parsing requirements file
	stateList                       // -l: showing installed roles
	stateChecking                   // -u phase 1: checking for newer versions
	stateInstalling                 // -i: installing / updating roles
	stateDeleting                   // -d: deleting a role
	stateError                      // fatal error, waiting for 'q'
)

// roleItem tracks the install status of a single role.
type roleItem struct {
	name       string
	version    string
	oldVersion string
	status     string // "pending" | "active" | "done" | "skipped" | "error"
	err        error
}

// checkRow holds a version-check result.
type checkRow struct {
	name   string
	oldVer string
	newVer string // empty = up to date
	err    error
}

// listRow holds an installed role entry.
type listRow struct {
	name    string
	version string
}

// --- internal messages ---

type parsedMsg struct {
	entries     models.File
	installOnly models.File
	err         error
}

type (
	deletedMsg     struct{ err error }
	checkDoneMsg   struct{}
	installDoneMsg struct{}
)

// --- channel-wait commands ---

func waitForCheck(ch <-chan parser.CheckProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return checkDoneMsg{}
		}
		return p
	}
}

func waitForInstall(ch <-chan installer.Progress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return installDoneMsg{}
		}
		return p
	}
}

// Model is the Bubble Tea model for agru's TUI.
type Model struct {
	cfg     Config
	parser  *parser.Parser
	inst    *installer.Installer
	state   appState
	spinner spinner.Model
	vp      viewport.Model

	// shared after parse
	entries     models.File // updated in place by checkVersions
	installOnly models.File

	// check phase (-u)
	checkRows  []checkRow
	checkTotal int
	checkCh    <-chan parser.CheckProgress

	// install phase (-i)
	roleItems  []roleItem
	instActive int // roles that started (went "active")
	instDone   int // roles that finished (done or error)
	instErrs   []string
	installCh  <-chan installer.Progress

	// list mode (-l)
	listRows []listRow

	// verbose log panel
	logLines []string

	// completion state
	done bool // true after successful finish; combined with cfg.Keep to stay open

	// error state
	err error

	// terminal dimensions
	width, height int
}

// New creates a new TUI model.
func New(cfg Config, p *parser.Parser, inst *installer.Installer) *Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = styleCyan

	return &Model{
		cfg:     cfg,
		parser:  p,
		inst:    inst,
		state:   stateInit,
		spinner: sp,
		width:   80,
		height:  24,
	}
}

// Init is called by Bubble Tea on program start.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			entries, installOnly, err := m.parser.ParseFile(m.cfg.RequirementsPath)
			return parsedMsg{entries: entries, installOnly: installOnly, err: err}
		},
	)
}

// Update handles messages and updates model state.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.SetWidth(m.vpWidth())
		m.vp.SetHeight(m.vpHeight())
		return m, nil

	case tea.KeyPressMsg:
		if msg.Code == tea.KeyEscape || msg.Text == "q" || msg.Text == "Q" {
			return m, tea.Quit
		}
		if m.cfg.Verbose || m.state == stateList {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case parsedMsg:
		return m.handleParsed(msg)

	case deletedMsg:
		if msg.err != nil {
			m.state = stateError
			m.err = msg.err
			return m, nil
		}
		return m, tea.Quit

	case parser.CheckProgress:
		m.checkRows = append(m.checkRows, checkRow{
			name:   msg.Name,
			oldVer: msg.OldVer,
			newVer: msg.NewVer,
			err:    msg.Err,
		})
		return m, waitForCheck(m.checkCh)

	case checkDoneMsg:
		if !m.cfg.InstallMissing {
			return m.quitOrKeep()
		}
		merged := m.parser.MergeFiles(m.entries, m.installOnly)
		return m.startInstall(merged)

	case installer.Progress:
		return m.handleInstallProgress(&msg)

	case installDoneMsg:
		if len(m.instErrs) > 0 {
			m.state = stateError
			m.err = fmt.Errorf("%s", strings.Join(m.instErrs, "\n"))
			return m, nil
		}
		return m.quitOrKeep()
	}

	return m, nil
}

// handleParsed transitions to the appropriate state after parsing.
func (m *Model) handleParsed(msg parsedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.state = stateError
		m.err = msg.err
		return m, nil
	}

	m.entries = msg.entries
	m.installOnly = msg.installOnly
	merged := m.parser.MergeFiles(msg.entries, msg.installOnly)

	if m.cfg.ListInstalled {
		return m.handleListMode(merged)
	}

	if m.cfg.DeleteName != "" {
		m.state = stateDeleting
		cmd := m.deleteRoleCmd(merged)
		return m, cmd
	}

	if m.cfg.UpdateFile {
		ch := make(chan parser.CheckProgress, 64)
		m.checkCh = ch
		m.checkTotal = msg.entries.RolesLen()
		m.state = stateChecking
		go m.parser.UpdateFile(msg.entries, m.cfg.RequirementsPath, ch) //nolint:errcheck // errors delivered via channel
		return m, waitForCheck(ch)
	}

	if m.cfg.InstallMissing {
		return m.startInstall(merged)
	}

	return m, tea.Quit
}

// handleListMode populates the list view with installed roles.
func (m *Model) handleListMode(merged models.File) (tea.Model, tea.Cmd) {
	installed := m.inst.GetInstalled(merged)
	for _, e := range installed {
		info, err := e.GetInstallInfo(m.inst.FS())
		version := info.Version
		if err != nil {
			version = styleRed.Render("(parse error: " + err.Error() + ")")
		}
		m.listRows = append(m.listRows, listRow{
			name:    e.GetName(),
			version: version,
		})
	}
	m.state = stateList
	m.vp = viewport.New(viewport.WithWidth(m.vpWidth()), viewport.WithHeight(m.vpHeight()))
	m.vp.SetContent(m.renderListContent())
	return m, nil
}

// startInstall transitions to the install phase.
func (m *Model) startInstall(merged models.File) (tea.Model, tea.Cmd) {
	m.state = stateInstalling
	m.roleItems = nil
	m.instActive = 0
	m.instDone = 0

	for _, e := range merged {
		if e.Include != "" {
			continue
		}
		m.roleItems = append(m.roleItems, roleItem{
			name:    e.GetName(),
			version: e.Version,
			status:  "pending",
		})
	}

	if len(m.roleItems) == 0 {
		return m, tea.Quit // nothing to install
	}

	if m.cfg.Verbose {
		m.vp = viewport.New(viewport.WithWidth(m.vpWidth()), viewport.WithHeight(m.vpHeight()))
	}

	ch := make(chan installer.Progress, 64)
	m.installCh = ch
	go m.inst.InstallMissing(merged, ch) //nolint:errcheck // errors delivered via channel
	return m, waitForInstall(ch)
}

// handleInstallProgress updates a role item from an install progress message.
func (m *Model) handleInstallProgress(msg *installer.Progress) (tea.Model, tea.Cmd) {
	switch msg.Status {
	case "active":
		m.instActive++
	case "done", "skipped":
		m.instDone++
	case "error":
		m.instDone++
		m.instErrs = append(m.instErrs, fmt.Sprintf("%s: %v", msg.Name, msg.Err))
	}

	for i, item := range m.roleItems {
		if item.name != msg.Name {
			continue
		}
		m.roleItems[i].status = msg.Status
		if msg.Version != "" {
			m.roleItems[i].version = msg.Version
		}
		m.roleItems[i].oldVersion = msg.OldVersion
		m.roleItems[i].err = msg.Err
		break
	}

	if m.cfg.Verbose && msg.Log != "" {
		m.logLines = append(m.logLines, msg.Log)
		m.vp.SetContent(strings.Join(m.logLines, "\n"))
		m.vp.GotoBottom()
	}

	return m, waitForInstall(m.installCh)
}

// View renders the current state.
func (m *Model) View() tea.View {
	return tea.View{
		Content:   m.buildContent(),
		AltScreen: true,
	}
}

// buildContent assembles the full screen content.
func (m *Model) buildContent() string {
	innerW := m.innerWidth()

	var body string
	switch m.state {
	case stateInit:
		body = m.spinner.View() + " Loading " + m.cfg.RequirementsPath + "…"
	case stateList:
		title := styleTitle.Render("agru") + styleDim.Render(" — installed roles")
		body = title + "\n\n" + m.vp.View() + "\n\n" + styleDim.Render("q  quit")
	case stateChecking, stateInstalling:
		body = m.renderProgress(innerW)
		if m.done {
			body += "\n\n" + styleDim.Render("q  quit")
		}
	case stateDeleting:
		body = m.spinner.View() + " Deleting " + styleBold.Render(m.cfg.DeleteName) + "…"
	case stateError:
		body = styleRed.Render("Error:") + "\n" + m.err.Error() + "\n\n" + styleDim.Render("q  quit")
	}

	if m.cfg.Verbose && len(m.logLines) > 0 && m.state != stateList {
		divider := styleLogDivider.Render("── Log " + strings.Repeat("─", max(0, innerW-7)))
		body = body + "\n" + divider + "\n" + m.vp.View()
	}

	return styleBorder.Width(innerW).Render(body)
}

// renderProgress renders the check + install phases (one or both).
func (m *Model) renderProgress(innerW int) string {
	var sb strings.Builder

	showCheck := len(m.checkRows) > 0 || m.state == stateChecking
	showInstall := m.state == stateInstalling

	available := m.innerHeight()
	// 1 line per section header; spacer between phases when both visible
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
			sb.WriteString("\n") // spacer between phases
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
		rows = rows[len(rows)-maxRows:] // tail: always show newest entries
	}
	for _, row := range rows {
		sb.WriteString(m.renderCheckRow(row) + "\n")
	}
	return sb.String()
}

// renderInstallSection renders Phase 2 (role installs), showing only the last maxRows visible entries.
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
	label := "Installing roles  " + counter
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
		sb.WriteString("  " + styleDim.Render("– all roles are up to date") + "\n")
		return sb.String()
	}
	if maxRows > 0 && len(visible) > maxRows {
		visible = visible[len(visible)-maxRows:] // tail: always show newest entries
	}
	for _, item := range visible {
		sb.WriteString(m.renderRoleItem(item) + "\n")
	}
	return sb.String()
}

// renderCheckRow renders a single version-check row.
func (m *Model) renderCheckRow(row checkRow) string {
	if row.err != nil {
		return "  " + styleRed.Render("✗") + "  " + styleRed.Render(row.name) + "  " + styleDim.Render(row.err.Error())
	}
	if row.newVer != "" {
		return "  " + styleGreen.Render("✓") + "  " + row.name + "  " +
			styleDim.Render(row.oldVer) + styleYellow.Render(" → ") + styleGreen.Render(row.newVer)
	}
	return "  " + styleDim.Render("–") + "  " + styleDim.Render(row.name+"  "+row.oldVer+"  (up to date)")
}

// renderRoleItem renders a single install-progress row.
func (m *Model) renderRoleItem(item *roleItem) string {
	ico := icon(item.status)
	switch item.status {
	case "active":
		return "  " + ico + "  " + item.name + "  " + styleDim.Render(item.version) + styleCyan.Render("  …")
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
	case "pending":
		return "  " + ico + "  " + styleDim.Render(item.name+"  "+item.version+"  pending")
	}
	return "  " + ico + "  " + item.name
}

// renderListContent builds the viewport content for list mode.
func (m *Model) renderListContent() string {
	if len(m.listRows) == 0 {
		return styleDim.Render("(no roles installed)")
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

// deleteRoleCmd returns a command that removes the specified role directory.
func (m *Model) deleteRoleCmd(merged models.File) tea.Cmd {
	return func() tea.Msg {
		for _, entry := range merged {
			if entry.GetName() == m.cfg.DeleteName {
				err := os.RemoveAll(path.Join(m.cfg.RolesPath, entry.GetName()))
				return deletedMsg{err: err}
			}
		}
		return deletedMsg{err: fmt.Errorf("role %q not found", m.cfg.DeleteName)}
	}
}

// quitOrKeep exits the program unless -k is set, in which case it marks done and waits for 'q'.
func (m *Model) quitOrKeep() (tea.Model, tea.Cmd) {
	if m.cfg.Keep {
		m.done = true
		return m, nil
	}
	return m, tea.Quit
}

// --- layout helpers ---

func (m *Model) innerWidth() int {
	w := m.width - 4 // border (2) + padding (2)
	if w < 40 {
		w = 40
	}
	return w
}

// innerHeight returns available content lines inside the border,
// minus the verbose log panel height when it is active.
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
