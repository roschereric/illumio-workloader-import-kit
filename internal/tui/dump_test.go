package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestDumpScreens writes screenshots (ANSI) of key steps for manual review when UMWL_DUMP is set.
func TestDumpScreens(t *testing.T) {
	dir := os.Getenv("UMWL_DUMP")
	if dir == "" {
		t.Skip()
	}
	m, _ := setup(t)
	m.width, m.height = 120, 36
	m.layout()
	drain(t, m, m.Init())
	save := func(name string) { os.WriteFile(dir+"/"+name+".ans", []byte(m.View()), 0o644) }
	save("0-preflight")
	press(t, m, "enter")
	save("1-csv")
	press(t, m, "enter")
	save("3-labels")
	press(t, m, "enter")
	save("4-reconcile")
	press(t, m, "u")
	press(t, m, "enter")
	save("4b-modal")
	press(t, m, "s")
	save("5-review")
	press(t, m, "e")
	save("5b-edit")
	press(t, m, "esc")
	press(t, m, "enter")
	save("6-dry")
	press(t, m, "enter")
	save("6b-confirm")
	press(t, m, "y")
	save("8-verify")
	press(t, m, "enter")
	save("9-ipl")
	press(t, m, "enter", "y", "enter")
	save("10-report")
	m.help = true
	save("help")
	m.help = false
	m.picker = newPicker("Proposed workloads CSV", "Pick the file with one row per IP. Start: the working folder.", "", []string{".csv"}, func(m *model, p string) tea.Cmd { return nil })
	save("picker")
}
