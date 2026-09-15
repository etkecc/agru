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

	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
)

type appState int

const (
	stateInit       appState = iota // parsing requirements file
	stateList                       // -l: showing installed roles
	stateChecking                   // -u phase 1: checking for newer versions
	stateInstalling                 // -i: installing / updating roles
	stateDeleting                   // -d: deleting a role
	stateError                      // fatal error, waiting for 'q'
)

// --- internal messages ---

type parsedMsg struct {
	reqFiles []parser.RequirementsFile
	err      error
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
	cfg     *config.Config
	parser  *parser.Parser
	inst    *installer.Installer
	state   appState
	spinner spinner.Model
	vp      viewport.Model

	// shared after parse
	reqFiles    []parser.RequirementsFile
	mergedColls models.Collections

	// check phase (-u)
	checkRows  []checkRow
	checkTotal int
	checkCh    <-chan parser.CheckProgress

	// install phase (-i)
	roleItems  []roleItem
	instActive int // items that started (went "active")
	instDone   int // items that finished (done or error)
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
func New(cfg *config.Config, p *parser.Parser, inst *installer.Installer) *Model {
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
			files, err := m.parser.ParsePattern(m.cfg.RequirementsPath)
			return parsedMsg{reqFiles: files, err: err}
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
			notice: msg.Notice,
			err:    msg.Err,
		})
		return m, waitForCheck(m.checkCh)

	case checkDoneMsg:
		if !m.cfg.InstallMissing {
			return m.quitOrKeep()
		}
		merged := m.parser.MergeAll(m.reqFiles)
		mergedColls := m.parser.MergeAllCollections(m.reqFiles)
		return m.startInstall(merged, mergedColls)

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

	m.reqFiles = msg.reqFiles
	m.mergedColls = m.parser.MergeAllCollections(msg.reqFiles)
	merged := m.parser.MergeAll(msg.reqFiles)

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
		var total int
		for _, f := range msg.reqFiles {
			total += f.Entries.RolesLen() + len(f.Collections)
		}
		m.checkTotal = total
		m.state = stateChecking
		go func() { _ = m.parser.UpdateAll(msg.reqFiles, ch) }()
		return m, waitForCheck(ch)
	}

	if m.cfg.InstallMissing {
		return m.startInstall(merged, m.mergedColls)
	}

	return m, tea.Quit
}

// handleListMode populates the list view with installed roles and collections.
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
	// Add installed collections
	installedColls := m.inst.GetInstalledCollections(m.mergedColls)
	for _, c := range installedColls {
		version := c.GetInstalledVersion(m.inst.CollFS())
		m.listRows = append(m.listRows, listRow{
			name:    c.GetFQCN(),
			version: version,
		})
	}

	m.state = stateList
	m.vp = viewport.New(viewport.WithWidth(m.vpWidth()), viewport.WithHeight(m.vpHeight()))
	m.vp.SetContent(m.renderListContent())
	return m, nil
}

// startInstall transitions to the install phase.
func (m *Model) startInstall(merged models.File, mergedColls models.Collections) (tea.Model, tea.Cmd) {
	m.state = stateInstalling
	m.roleItems = nil
	m.instActive = 0
	m.instDone = 0

	// Add roles
	for _, e := range merged {
		if e.Include != "" {
			continue
		}
		item := roleItem{
			name:    e.GetName(),
			version: e.Version,
			status:  "pending",
		}
		if e.Unsupported() != "" {
			item.status = "unsupported"
			item.notice = e.Unsupported()
		}
		m.roleItems = append(m.roleItems, item)
	}
	// Add collections
	for _, c := range mergedColls {
		item := roleItem{
			name:    c.GetFQCN(),
			version: c.Version,
			status:  "pending",
		}
		if c.Unsupported() != "" {
			item.status = "unsupported"
			item.notice = c.Unsupported()
		}
		m.roleItems = append(m.roleItems, item)
	}

	if len(m.roleItems) == 0 {
		return m, tea.Quit // nothing to install
	}

	if m.cfg.Verbose {
		m.vp = viewport.New(viewport.WithWidth(m.vpWidth()), viewport.WithHeight(m.vpHeight()))
	}

	ch := make(chan installer.Progress, 64)
	m.installCh = ch
	go m.inst.InstallMissing(merged, mergedColls, ch) //nolint:errcheck // errors delivered via channel
	return m, waitForInstall(ch)
}

// handleInstallProgress updates a role/collection item from an install progress message.
func (m *Model) handleInstallProgress(msg *installer.Progress) (tea.Model, tea.Cmd) {
	switch msg.Status {
	case "active":
		m.instActive++
	case "unsupported":
		m.instActive++
		m.instDone++
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
		body = m.spinner.View() + " Loading " + m.cfg.RequirementsPath + "..."
	case stateList:
		title := styleTitle.Render("agru") + styleDim.Render(": installed roles and collections")
		body = title + "\n\n" + m.vp.View() + "\n\n" + styleDim.Render("q  quit")
	case stateChecking, stateInstalling:
		body = m.renderProgress(innerW)
		if m.done {
			body += "\n\n" + styleDim.Render("q  quit")
		}
	case stateDeleting:
		body = m.spinner.View() + " Deleting " + styleBold.Render(m.cfg.DeleteName) + "..."
	case stateError:
		body = styleRed.Render("Error:") + "\n" + m.err.Error() + "\n\n" + styleDim.Render("q  quit")
	}

	if m.cfg.Verbose && len(m.logLines) > 0 && m.state != stateList {
		divider := styleLogDivider.Render("── Log " + strings.Repeat("─", max(0, innerW-7)))
		body = body + "\n" + divider + "\n" + m.vp.View()
	}

	return styleBorder.Width(innerW).Render(body)
}

// deleteRoleCmd returns a command that removes the specified role or collection directory.
func (m *Model) deleteRoleCmd(merged models.File) tea.Cmd {
	return func() tea.Msg {
		// Try roles first
		for _, entry := range merged {
			if entry.GetName() == m.cfg.DeleteName {
				err := os.RemoveAll(path.Join(m.cfg.RolesPath, entry.GetName()))
				return deletedMsg{err: err}
			}
		}
		// Then collections
		for _, c := range m.mergedColls {
			if c.GetFQCN() == m.cfg.DeleteName {
				target := c.GetPath(m.cfg.CollectionsPath)
				if target != "" {
					err := os.RemoveAll(target)
					return deletedMsg{err: err}
				}
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
