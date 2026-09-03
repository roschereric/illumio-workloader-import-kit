// Package tui is the full-screen interface (Bubble Tea) of the unmanaged workload importer:
// a status bar, a step list on the left, the active step's panel on the right, the live
// workloader output at the bottom and a key bar. All PCE work runs in the engine package.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/roschereric/illumio-workloader-import-kit/internal/engine"
)

// Config comes from the command line.
type Config struct {
	CSV, IPL, PCE, Priority, WorkloaderBin, RunsDir string
	Chunk                                           int
	SetupOnly                                       bool
	Version                                         string
}

var stepNames = []string{
	"Preflight", "Load CSV", "PCE inventory", "Labels", "Reconcile by IP",
	"Review new", "Dry run", "Execute", "Verify", "IP lists", "Report",
}

const (
	stPreflight = iota
	stCSV
	stInventory
	stLabels
	stReconcile
	stReview
	stDry
	stExec
	stVerify
	stIPL
	stReport
)

type model struct {
	cfg    Config
	w      *engine.Workloader
	runDir string
	report *engine.Report
	logCh  chan string

	width, height int
	step          int
	status        [11]string // pending | active | running | done | failed | skipped
	busy          bool
	busyWhat      string
	spin          spinner.Model
	modal         *Modal
	picker        *Picker
	focusLog      bool
	logLines      []string
	logView       viewport.Model
	events        []string
	quitting      bool
	help          bool
	noLogWait     bool // tests: do not block on the log channel

	// step 0
	checks    []check
	binPath   string
	binVer    string
	pceStat   engine.PCEStatus
	connOK    bool
	connTried bool

	// step 1
	csv *engine.CSVFile
	cur int // selected row in the current table (steps 1,4,5)
	top int // scroll offset

	// step 2/3
	inv  *engine.Inventory
	plan engine.LabelPlan

	// step 4
	decided map[*engine.Row]bool

	// step 6..10
	buckets   engine.Buckets
	createCSV string
	updateCSV string
	dryLines  []string
	dryOK     bool
	chunks    []*engine.Chunk
	chunkIdx  int
	prog      progress.Model
	verifyMsg string
	iplLines  []string
	reportMD  string
	finished  bool
	toolchain string
}

type check struct {
	Label  string
	Status string // ok | warn | fail | pending
	Detail string
}

// messages
type logMsg string
type checksDoneMsg struct {
	checks    []check
	toolchain string
}
type buildDoneMsg struct {
	path string
	err  error
}
type goInstallDoneMsg struct {
	err error
	tag string
}
type downloadDoneMsg struct {
	path string
	err  error
}
type pceAddDoneMsg struct{ res engine.Result }
type connDoneMsg struct{ ok bool }
type csvDoneMsg struct {
	cf  *engine.CSVFile
	err error
}
type invDoneMsg struct {
	inv *engine.Inventory
	err error
}
type dryDoneMsg struct {
	lines []string
	ok    bool
}
type chunkDoneMsg struct {
	c      *engine.Chunk
	labels []string
}
type verifyDoneMsg struct {
	found   int
	missing []string
	ok      bool
}
type iplDryDoneMsg struct{ lines []string }
type iplDoneMsg struct{ res engine.Result }
type reportDoneMsg struct {
	path string
	err  error
}

// New builds the model and creates the run folder.
func New(cfg Config) (*model, error) {
	ts := time.Now().Format("20060102-150405")
	runDir := filepath.Join(cfg.RunsDir, ts)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	m := &model{cfg: cfg, runDir: runDir, logCh: make(chan string, 4096), decided: map[*engine.Row]bool{}}
	m.w = &engine.Workloader{PCE: cfg.PCE, RunDir: runDir, Out: func(s string) {
		select {
		case m.logCh <- s:
		default:
		}
	}}
	m.report = engine.NewReport(cfg.CSV, cfg.IPL, cfg.PCE, runDir)
	m.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(sAccent))
	m.prog = progress.New(progress.WithGradient("#FF5500", "#2E7D32"), progress.WithoutPercentage())
	m.logView = viewport.New(80, 8)
	for i := range m.status {
		m.status[i] = "pending"
	}
	m.status[0] = "active"
	m.width, m.height = 120, 40
	return m, nil
}

// Run starts the program.
func Run(cfg Config) error {
	m, err := New(cfg)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.waitLog(), m.spin.Tick, m.runChecks())
}

func (m *model) waitLog() tea.Cmd {
	if m.noLogWait {
		return nil
	}
	return func() tea.Msg { return logMsg(<-m.logCh) }
}

func (m *model) logf(format string, a ...any) {
	m.appendLog(fmt.Sprintf(format, a...))
}

func (m *model) appendLog(s string) {
	ts := sDim.Render(time.Now().Format("15:04:05")) + " "
	m.logLines = append(m.logLines, ts+s)
	if len(m.logLines) > 5000 {
		m.logLines = m.logLines[len(m.logLines)-5000:]
	}
	m.logView.SetContent(strings.Join(m.logLines, "\n"))
	if !m.focusLog {
		m.logView.GotoBottom()
	}
}

func (m *model) event(s string) {
	m.events = append(m.events, time.Now().Format("15:04")+" "+s)
	m.appendLog(sInfo.Render("• ") + s)
}

func (m *model) setBusy(what string) {
	m.busy = true
	m.busyWhat = what
	m.status[m.step] = "running"
}

func (m *model) idle() {
	m.busy = false
	m.busyWhat = ""
	if m.status[m.step] == "running" {
		m.status[m.step] = "active"
	}
}

func (m *model) goTo(step int) {
	if m.status[m.step] == "active" || m.status[m.step] == "running" {
		m.status[m.step] = "done"
	}
	m.step = step
	m.status[step] = "active"
	m.cur, m.top = 0, 0
}

func (m *model) fail(msg string) {
	m.status[m.step] = "failed"
	m.busy = false
	m.appendLog(sErr.Render("✖ ") + msg)
	m.modal = infoModal("Error", []string{msg})
}

// ------------------------------------------------------------------ Update

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case logMsg:
		m.appendLog(string(msg))
		return m, m.waitLog()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		pm, cmd := m.prog.Update(msg)
		m.prog = pm.(progress.Model)
		return m, cmd
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m.onWork(msg)
}

func (m *model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	if m.picker != nil {
		cur := m.picker
		closeIt, cmd := cur.update(m, msg)
		if closeIt && m.picker == cur { // the handler may have opened another dialog
			m.picker = nil
		}
		return m, cmd
	}
	if m.modal != nil {
		cur := m.modal
		closeIt, cmd := cur.update(m, msg)
		if closeIt && m.modal == cur {
			m.modal = nil
		}
		return m, cmd
	}
	if m.help {
		m.help = false
		return m, nil
	}
	switch k {
	case "?":
		m.help = true
		return m, nil
	case "tab":
		m.focusLog = !m.focusLog
		return m, nil
	case "q":
		return m, m.askQuit()
	}
	if m.focusLog {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}
	if m.busy {
		return m, nil
	}
	return m.stepKey(k)
}

func (m *model) askQuit() tea.Cmd {
	if m.finished || m.step == stPreflight && !m.busy {
		m.quitting = true
		return tea.Quit
	}
	m.modal = choiceModal("Quit?", []string{"Nothing has been written to the PCE unless step 7 (Execute) ran. The run folder keeps every file so far:", m.runDir},
		[]Option{{Key: "y", Label: "quit"}, {Key: "esc", Label: "stay"}}, func(m *model, k string) tea.Cmd {
			if k == "y" {
				m.quitting = true
				return tea.Quit
			}
			return nil
		})
	return nil
}

func (m *model) layout() {
	m.logView.Width = m.width - 4
	m.logView.Height = m.logHeight() - 2
	m.prog.Width = m.mainWidth() - 12
	m.logView.SetContent(strings.Join(m.logLines, "\n"))
	m.logView.GotoBottom()
}

func (m *model) logHeight() int {
	h := m.height / 4
	if h < 6 {
		h = 6
	}
	if h > 14 {
		h = 14
	}
	return h
}
func (m *model) sidebarWidth() int { return 28 }
func (m *model) mainWidth() int    { return m.width - m.sidebarWidth() }
func (m *model) mainHeight() int   { return m.height - 1 - m.logHeight() - 2 }

// tableRows is how many data rows fit in the main panel's table area.
func (m *model) tableRows(reserved int) int {
	n := m.mainHeight() - 2 - reserved
	if n < 3 {
		n = 3
	}
	return n
}

func (m *model) clampCursor(n int) {
	if n == 0 {
		m.cur, m.top = 0, 0
		return
	}
	if m.cur < 0 {
		m.cur = 0
	}
	if m.cur >= n {
		m.cur = n - 1
	}
}

func (m *model) scrollTo(visible int) {
	if m.cur < m.top {
		m.top = m.cur
	}
	if m.cur >= m.top+visible {
		m.top = m.cur - visible + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	top := m.statusBar()
	side := m.sidebar()
	var main string
	if m.help {
		main = m.helpView()
	} else if m.picker != nil {
		main = lipgloss.Place(m.mainWidth(), m.mainHeight(), lipgloss.Center, lipgloss.Center, m.picker.view(m.mainWidth(), m.mainHeight()))
	} else if m.modal != nil {
		main = lipgloss.Place(m.mainWidth(), m.mainHeight(), lipgloss.Center, lipgloss.Center, m.modal.view(m.mainWidth()))
	} else {
		main = m.mainView()
	}
	mainBox := sPanel
	if !m.focusLog {
		mainBox = sPanelFocus
	}
	main = clip(main, m.mainWidth()-4, m.mainHeight()-2)
	mainR := mainBox.Width(m.mainWidth() - 2).Height(m.mainHeight() - 2).Render(main)
	body := lipgloss.JoinHorizontal(lipgloss.Top, side, mainR)
	logBox := sPanel
	if m.focusLog {
		logBox = sPanelFocus
	}
	logTitle := sDim.Render(" workloader output · tab to scroll ")
	logR := logBox.Width(m.width - 2).Height(m.logHeight() - 2).Render(clip(m.logView.View(), m.width-4, m.logHeight()-2))
	logR = overlayTitle(logR, logTitle)
	return lipgloss.JoinVertical(lipgloss.Left, top, body, logR, m.keyBar())
}

// overlayTitle writes a title into the top border of a bordered box.
func overlayTitle(box, title string) string {
	lines := strings.SplitN(box, "\n", 2)
	if len(lines) < 2 {
		return box
	}
	first := lines[0]
	tw := lipgloss.Width(title)
	if tw+4 >= lipgloss.Width(first) {
		return box
	}
	// keep the first 2 border cells, then the title, then the rest of the border
	r := []rune(first)
	prefix := string(r[:2])
	rest := string(r[2+tw:])
	return prefix + title + rest + "\n" + lines[1]
}

func (m *model) statusBar() string {
	state := sOK.Render("● Ready")
	if m.busy {
		state = sAccent.Render(m.spin.View() + " " + m.busyWhat)
	}
	if m.status[m.step] == "failed" {
		state = sErr.Render("● Failed")
	}
	pce := m.cfg.PCE
	if pce == "" {
		pce = "default (pce.yaml)"
	}
	done := 0
	for _, s := range m.status {
		if s == "done" {
			done++
		}
	}
	left := " " + sTitle.Render("umwl-tui") + " " + state
	right := fmt.Sprintf("PCE %s  │  run %s  │  step %d/%d  ", pce, filepath.Base(m.runDir), m.step, len(stepNames)-1)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return sTitleBar.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (m *model) sidebar() string {
	var sb strings.Builder
	sb.WriteString(sDim.Render("STEPS") + "\n")
	for i, n := range stepNames {
		line := fmt.Sprintf("%s %2d %s", glyph(m.status[i]), i, n)
		if i == m.step {
			line = sStepActive.Render(fmt.Sprintf("▶ %2d %s", i, n))
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n" + sDim.Render("EVENTS") + "\n")
	start := 0
	avail := m.mainHeight() - len(stepNames) - 5
	if avail < 1 {
		avail = 1
	}
	if len(m.events) > avail {
		start = len(m.events) - avail
	}
	for _, e := range m.events[start:] {
		sb.WriteString(sDim.Render(truncate(e, m.sidebarWidth()-4)) + "\n")
	}
	return sSidebar.Width(m.sidebarWidth() - 2).Height(m.mainHeight() - 2).Render(clip(sb.String(), m.sidebarWidth()-4, m.mainHeight()-2))
}

func (m *model) keyBar() string {
	var keys []string
	if m.picker != nil {
		return " " + sDim.Render("file chooser — keys shown inside the dialog")
	} else if m.modal != nil {
		keys = []string{key("esc", "close")}
	} else if m.focusLog {
		keys = []string{key("↑↓ pgup pgdn", "scroll log"), key("tab", "back to panel")}
	} else {
		keys = m.stepKeys()
	}
	keys = append(keys, key("tab", "log"), key("?", "help"), key("q", "quit"))
	return " " + strings.Join(keys, "   ")
}

func (m *model) helpView() string {
	lines := []string{
		sAccent.Render("umwl-tui — unmanaged workload importer on top of workloader"), "",
		"The left list is the sequence; the panel shows the active step; the bottom pane is the live workloader output.",
		"Nothing is written to the PCE before step 7 (Execute), and step 7 only runs after you confirm the dry run.", "",
		sBold.Render("Global keys"),
		"  tab   switch focus between the panel and the log (scroll with ↑↓ / pgup / pgdn)",
		"  ?     this help        q  quit (asks first once work is in progress)", "",
		sBold.Render("Step keys are shown in the bottom bar and change per step."),
		"  Reconcile: u update existing · s skip · c create anyway · r rename+update · U/S apply to all of the same kind",
		"  Review:    k keep · s skip · e edit · a accept all", "",
		sDim.Render("Run folder: " + m.runDir),
		"", sDim.Render("press any key to close"),
	}
	return strings.Join(lines, "\n")
}
