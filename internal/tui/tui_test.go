package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/roschereric/illumio-workloader-import-kit/internal/engine"
)

// drain executes commands synchronously and feeds their messages back into the model,
// skipping ticks and the log waiter, until nothing is pending.
func drain(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		switch v := msg.(type) {
		case nil:
			continue
		case tea.BatchMsg:
			queue = append(queue, v...)
			continue
		case spinner.TickMsg, progress.FrameMsg:
			continue
		case logMsg:
			// consume without re-arming the waiter
			m.appendLog(string(v))
			continue
		}
		_, next := m.Update(msg)
		queue = append(queue, next)
	}
}

func press(t *testing.T, m *model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := m.Update(msg)
		drain(t, m, cmd)
	}
}

func setup(t *testing.T) (*model, string) {
	t.Helper()
	dir := t.TempDir()
	td, _ := filepath.Abs("../../testdata")
	mock := filepath.Join(dir, "workloader")
	b, err := os.ReadFile(filepath.Join(td, "mock-workloader.py"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(mock, b, 0o755)
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	for _, f := range []string{"cliente3-umwl-import-v2.csv", "cliente3-ipl-import-v2.csv"} {
		b, _ := os.ReadFile(filepath.Join(td, f))
		os.WriteFile(filepath.Join(dir, f), b, 0o644)
	}
	os.WriteFile(filepath.Join(dir, "pce.yaml"), []byte("default_pce_name: poc\n"), 0o644)
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })
	m, err := New(Config{CSV: "cliente3-umwl-import-v2.csv", IPL: "cliente3-ipl-import-v2.csv", Priority: "1", RunsDir: "./runs", Chunk: 20, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	m.noLogWait = true
	m.width, m.height = 140, 45
	m.layout()
	return m, dir
}

func TestFullRun(t *testing.T) {
	m, _ := setup(t)
	drain(t, m, m.Init())
	if !m.preflightOK() {
		t.Fatalf("preflight not ok: %+v", m.checks)
	}
	if v := m.View(); !strings.Contains(v, "Ready.") {
		t.Fatalf("expected Ready in view:\n%s", v)
	}
	press(t, m, "enter") // load CSV
	if m.csv == nil || len(m.csv.Rows) != 42 {
		t.Fatalf("csv rows: %v", m.csv)
	}
	press(t, m, "enter") // inventory → labels
	if m.step != stLabels || m.inv == nil {
		t.Fatalf("expected labels step, got %d", m.step)
	}
	if len(m.plan.NewValues) == 0 {
		t.Fatalf("expected new label values")
	}
	press(t, m, "enter") // → reconcile
	if m.step != stReconcile {
		t.Fatalf("expected reconcile, got %d", m.step)
	}
	// decide: first undecided is the Zabbix server (exists) → update; next conflict-multiple → skip
	press(t, m, "u")
	press(t, m, "S") // skip all remaining of that kind
	press(t, m, "enter")
	if m.step != stReview {
		// undecided rows → modal offered; accept skipping
		press(t, m, "s")
	}
	if m.step != stReview {
		t.Fatalf("expected review, got %d; view:\n%s", m.step, m.View())
	}
	if len(m.buckets.Create) == 0 || len(m.buckets.Update) != 1 {
		t.Fatalf("buckets create=%d update=%d skip=%d", len(m.buckets.Create), len(m.buckets.Update), len(m.buckets.Skipped))
	}
	press(t, m, "enter") // dry run
	if m.step != stDry || !m.dryOK {
		t.Fatalf("dry run: step=%d ok=%v lines=%v", m.step, m.dryOK, m.dryLines)
	}
	press(t, m, "enter") // confirm modal
	press(t, m, "y")     // execute → verify
	if m.step != stVerify {
		t.Fatalf("expected verify, got %d", m.step)
	}
	if len(m.report.Created) != len(m.buckets.Create) {
		t.Fatalf("created %d != %d", len(m.report.Created), len(m.buckets.Create))
	}
	if !strings.Contains(stripANSI(m.verifyMsg), "confirmed") || strings.Contains(m.verifyMsg, "missing") {
		t.Fatalf("verify: %s", m.verifyMsg)
	}
	press(t, m, "enter") // → IPL dry
	if m.step != stIPL {
		t.Fatalf("expected ipl, got %d", m.step)
	}
	press(t, m, "enter", "y") // import
	press(t, m, "enter")      // report
	if m.step != stReport || m.reportMD == "" {
		t.Fatalf("expected report, got %d %q", m.step, m.reportMD)
	}
	if _, err := os.Stat(m.reportMD); err != nil {
		t.Fatal(err)
	}
	v := m.View()
	for _, want := range []string{"executed", "report.md", "Report"} {
		if !strings.Contains(v, want) {
			t.Fatalf("report view missing %q:\n%s", want, v)
		}
	}
}

func TestQuitBeforeExecuteWritesReport(t *testing.T) {
	m, _ := setup(t)
	drain(t, m, m.Init())
	press(t, m, "enter", "enter", "enter") // csv, inventory, labels → reconcile
	press(t, m, "enter", "s")              // skip undecided → review
	press(t, m, "enter")                   // dry
	press(t, m, "x")                       // stop here
	if m.step != stReport || !m.report.Aborted {
		t.Fatalf("expected aborted report, step=%d", m.step)
	}
	if len(m.report.Created) != 0 {
		t.Fatal("nothing should be created")
	}
}

func TestNoPCEConfigured(t *testing.T) {
	m, dir := setup(t)
	// make pce-list say no pce configured
	mock := filepath.Join(dir, "workloader")
	b, _ := os.ReadFile(mock)
	s := strings.Replace(string(b), `elif cmd=="pce-list": print("+------+------------------+\n| poc  | poc.illum.io     | default |")`,
		`elif cmd=="pce-list": print("2026-09-03 [INFO] - no pce configured. run pce-add to add a pce to pce.yaml file.")`, 1)
	os.WriteFile(mock, []byte(s), 0o755)
	drain(t, m, m.Init())
	if m.preflightOK() {
		t.Fatal("preflight should fail without a PCE")
	}
	if !strings.Contains(m.View(), "pce-add") {
		t.Fatalf("view should point to pce-add:\n%s", m.View())
	}
	press(t, m, "p")
	if m.modal == nil || m.modal.Kind != "form" {
		t.Fatal("expected the pce-add form")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	m, _ := setup(t)
	drain(t, m, m.Init())
	for _, w := range []int{80, 100, 160} {
		for _, h := range []int{24, 40} {
			m.width, m.height = w, h
			m.layout()
			v := m.View()
			lines := strings.Split(v, "\n")
			if len(lines) > h {
				t.Errorf("%dx%d: %d lines rendered", w, h, len(lines))
			}
		}
	}
}

func TestPickerFlowAndBadCSVRecovery(t *testing.T) {
	m, dir := setup(t)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "broken.csv"), []byte("name,foo\nx,y\n"), 0o644)
	m.cfg.CSV = ""
	drain(t, m, m.Init())
	press(t, m, "enter")
	if m.picker == nil {
		t.Fatal("expected the file picker")
	}
	v := m.View()
	for _, want := range []string{"Proposed workloads CSV", "cliente3-umwl-import-v2.csv", "sub/", "path "} {
		if !strings.Contains(v, want) {
			t.Fatalf("picker view missing %q:\n%s", want, v)
		}
	}
	// navigate: go to top (..), down into sub/, pick broken.csv → error → recovery reopens the picker
	press(t, m, "g", "down", "down") // "..", "runs/", "sub/"
	if p, isDir := m.picker.selected(); !isDir || filepath.Base(p) != "sub" {
		t.Fatalf("expected sub/ selected, got %s", p)
	}
	press(t, m, "enter") // descend
	if filepath.Base(m.picker.Dir) != "sub" {
		t.Fatalf("expected to be in sub, in %s", m.picker.Dir)
	}
	press(t, m, "enter") // select broken.csv (preselected as first .csv)
	if m.picker == nil || !m.picker.Optional {
		t.Fatal("expected the optional IPL picker")
	}
	press(t, m, "esc") // no IPL
	if m.modal == nil || m.modal.Kind != "form" {
		t.Fatal("expected the priority form")
	}
	press(t, m, "enter") // load → fails (missing columns)
	if m.modal == nil || m.status[stCSV] != "failed" {
		t.Fatalf("expected failure modal, status=%s", m.status[stCSV])
	}
	press(t, m, "enter") // close → back to preflight with the picker open
	if m.picker == nil || m.step != stPreflight {
		t.Fatalf("expected picker reopened at preflight, step=%d picker=%v", m.step, m.picker != nil)
	}
	// type an exact path in the path field
	press(t, m, "tab")
	m.picker.input.SetValue(filepath.Join(dir, "cliente3-umwl-import-v2.csv"))
	press(t, m, "enter")
	press(t, m, "esc")   // no IPL
	press(t, m, "enter") // priority default "1"
	if m.csv == nil || len(m.csv.Rows) != 42 || m.step != stCSV {
		t.Fatalf("csv not loaded via typed path: step=%d", m.step)
	}
	if m.cfg.CSV != "cliente3-umwl-import-v2.csv" {
		t.Fatalf("expected relative path, got %q", m.cfg.CSV)
	}
}

// The workloader binary can live anywhere: w → picker → validate → save in the local or user settings file.
func TestWorkloaderPathSetting(t *testing.T) {
	m, dir := setup(t)
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "home", ".config"))
	// move the mock out of the working folder so the default lookup fails
	shared := filepath.Join(dir, "home", "bin")
	os.MkdirAll(shared, 0o755)
	os.Rename(filepath.Join(dir, "workloader"), filepath.Join(shared, "workloader"))
	drain(t, m, m.Init())
	if m.checks[0].Status != "fail" || !strings.Contains(m.checks[0].Detail, "press b") {
		t.Fatalf("expected missing-binary check offering build: %+v", m.checks[0])
	}
	press(t, m, "w")
	if m.picker == nil || m.picker.Title != "workloader binary" {
		t.Fatal("expected the binary picker")
	}
	// a non-executable file is refused
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	press(t, m, "tab")
	m.picker.input.SetValue(filepath.Join(dir, "notes.txt"))
	press(t, m, "enter")
	if m.modal == nil || m.modal.Title != "Not executable" {
		t.Fatalf("expected refusal, modal=%v", m.modal)
	}
	press(t, m, "enter")
	// pick the real one, typed with ~
	press(t, m, "w", "tab")
	m.picker.input.SetValue("~/bin/workloader")
	press(t, m, "enter")
	if m.modal == nil || m.modal.Title != "Save workloader path" {
		t.Fatalf("expected the save-scope modal, got %v", m.modal)
	}
	press(t, m, "u") // user scope → saved, checks re-run
	st, src := engine.LoadSettings()
	if st.Workloader != "~/bin/workloader" || src != engine.UserSettingsPath() {
		t.Fatalf("settings not saved: %+v from %s", st, src)
	}
	if m.checks[0].Status != "ok" || !strings.Contains(m.checks[0].Detail, "path from ~/.config/umwl-tui/config.json") {
		t.Fatalf("expected the saved path to be used: %+v", m.checks[0])
	}
	// the local file overrides the user file
	engine.SaveSettings(engine.Settings{Workloader: "~/bin/workloader"}, false)
	if _, src := engine.LoadSettings(); !strings.HasSuffix(src, engine.LocalSettingsFile) {
		t.Fatalf("local settings should win, got %s", src)
	}
	if engine.FindBinary("") != filepath.Join(shared, "workloader") {
		t.Fatalf("FindBinary ignored the ~ path: %s", engine.FindBinary(""))
	}
}

// b = build from source: with no Go toolchain on PATH the TUI explains the manual steps instead of failing later.
func TestBuildOfferWithoutGo(t *testing.T) {
	m, dir := setup(t)
	drain(t, m, m.Init())
	empty := filepath.Join(dir, "emptypath")
	os.MkdirAll(empty, 0o755)
	t.Setenv("PATH", empty)
	press(t, m, "b")
	if m.modal == nil || m.modal.Title != "Go toolchain missing" {
		t.Fatalf("expected the toolchain modal, got %v", m.modal)
	}
	v := m.View()
	for _, want := range []string{"git clone https://github.com/brian1917/workloader", "go build -o ../workloader", "brew install go"} {
		if !strings.Contains(v, want) {
			t.Fatalf("manual instructions missing %q:\n%s", want, v)
		}
	}
	press(t, m, "enter")
	if m.modal != nil {
		t.Fatal("modal should close")
	}
}

// With Go and git present the build is offered first (recommended) and esc cancels without touching the disk.
func TestBuildOfferWithGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	m, dir := setup(t)
	drain(t, m, m.Init())
	if !strings.Contains(m.View(), "build workloader from source (native, recommended)") {
		t.Fatalf("build action not offered:\n%s", m.View())
	}
	press(t, m, "b")
	if m.modal == nil || !strings.HasPrefix(m.modal.Title, "Build workloader") {
		t.Fatalf("expected the build modal, got %v", m.modal)
	}
	press(t, m, "esc")
	if _, err := os.Stat(filepath.Join(dir, engine.SrcDir)); err == nil {
		t.Fatal("cancel must not clone")
	}
}
