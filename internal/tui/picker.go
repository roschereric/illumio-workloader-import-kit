package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Picker is a two-pane file chooser (path field on top, directory listing below), Midnight Commander style.
// Enter on a directory descends, on a file selects it; ".." goes up; tab moves to the path field where an
// exact path can be typed (a directory jumps there, a file selects it).
type Picker struct {
	Title    string
	Hint     string
	Dir      string
	Exts     []string // preferred extensions shown first and highlighted (e.g. ".csv"); other files are still listed
	entries  []pEntry
	cur, top int
	input    textinput.Model
	inPath   bool
	err      string
	OnPick   func(m *model, path string) tea.Cmd
	OnCancel func(m *model) tea.Cmd
	Optional bool // esc = "none" instead of cancel
}

type pEntry struct {
	Name  string
	IsDir bool
	Size  int64
	Pref  bool
}

func newPicker(title, hint, dir string, exts []string, pick func(m *model, path string) tea.Cmd) *Picker {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	abs, _ := filepath.Abs(dir)
	p := &Picker{Title: title, Hint: hint, Exts: exts, OnPick: pick}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 1024
	ti.Width = 70
	p.input = ti
	p.load(abs)
	return p
}

func (p *Picker) load(dir string) {
	p.err = ""
	des, err := os.ReadDir(dir)
	if err != nil {
		p.err = err.Error()
		return
	}
	p.Dir = dir
	p.entries = nil
	var dirs, pref, others []pEntry
	for _, d := range des {
		name := d.Name()
		if strings.HasPrefix(name, ".") && name != ".." {
			continue
		}
		if d.IsDir() {
			dirs = append(dirs, pEntry{Name: name, IsDir: true})
			continue
		}
		info, _ := d.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}
		e := pEntry{Name: name, Size: size}
		for _, x := range p.Exts {
			if strings.EqualFold(filepath.Ext(name), x) {
				e.Pref = true
			}
		}
		if e.Pref {
			pref = append(pref, e)
		} else {
			others = append(others, e)
		}
	}
	byName := func(s []pEntry) {
		sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i].Name) < strings.ToLower(s[j].Name) })
	}
	byName(dirs)
	byName(pref)
	byName(others)
	if filepath.Dir(dir) != dir {
		p.entries = append(p.entries, pEntry{Name: "..", IsDir: true})
	}
	p.entries = append(p.entries, dirs...)
	p.entries = append(p.entries, pref...)
	p.entries = append(p.entries, others...)
	p.cur, p.top = 0, 0
	// preselect the first preferred file when there is one
	for i, e := range p.entries {
		if e.Pref {
			p.cur = i
			break
		}
	}
	p.input.SetValue(dir + string(os.PathSeparator))
	p.input.CursorEnd()
}

func (p *Picker) selected() (string, bool) {
	if len(p.entries) == 0 {
		return "", false
	}
	e := p.entries[p.cur]
	if e.Name == ".." {
		return filepath.Dir(p.Dir), true
	}
	return filepath.Join(p.Dir, e.Name), e.IsDir
}

// relPath returns the path relative to the working folder when it is inside it, else absolute.
func relPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	if rel, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// update returns (close, cmd).
func (p *Picker) update(m *model, msg tea.KeyMsg) (bool, tea.Cmd) {
	k := msg.String()
	if p.inPath {
		switch k {
		case "esc", "tab":
			p.inPath = false
			p.input.Blur()
			return false, nil
		case "enter":
			raw := strings.TrimSpace(p.input.Value())
			if strings.HasPrefix(raw, "~") {
				if h, err := os.UserHomeDir(); err == nil {
					raw = h + raw[1:]
				}
			}
			if !filepath.IsAbs(raw) {
				raw = filepath.Join(p.Dir, raw)
			}
			raw = filepath.Clean(raw)
			st, err := os.Stat(raw)
			if err != nil {
				p.err = err.Error()
				return false, nil
			}
			if st.IsDir() {
				p.load(raw)
				p.inPath = false
				p.input.Blur()
				return false, nil
			}
			return true, p.OnPick(m, relPath(raw))
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return false, cmd
	}
	switch k {
	case "esc":
		if p.Optional {
			return true, p.OnPick(m, "")
		}
		if p.OnCancel != nil {
			return true, p.OnCancel(m)
		}
		return true, nil
	case "tab", "/":
		p.inPath = true
		p.input.Focus()
		p.input.CursorEnd()
		return false, nil
	case "up", "k":
		p.cur--
	case "down", "j":
		p.cur++
	case "pgup":
		p.cur -= 10
	case "pgdown":
		p.cur += 10
	case "home", "g":
		p.cur = 0
	case "end", "G":
		p.cur = len(p.entries) - 1
	case "left", "backspace", "h":
		p.load(filepath.Dir(p.Dir))
	case "enter", "right", "l":
		path, isDir := p.selected()
		if path == "" {
			return false, nil
		}
		if isDir {
			p.load(path)
			return false, nil
		}
		return true, p.OnPick(m, relPath(path))
	}
	if p.cur < 0 {
		p.cur = 0
	}
	if p.cur >= len(p.entries) {
		p.cur = len(p.entries) - 1
	}
	return false, nil
}

func (p *Picker) view(width, height int) string {
	w := width - 10
	if w < 50 {
		w = 50
	}
	rows := height - 12
	if rows < 5 {
		rows = 5
	}
	if p.cur < p.top {
		p.top = p.cur
	}
	if p.cur >= p.top+rows {
		p.top = p.cur - rows + 1
	}
	var sb strings.Builder
	sb.WriteString(sAccent.Render(p.Title) + "\n")
	if p.Hint != "" {
		sb.WriteString(sDim.Render(p.Hint) + "\n")
	}
	pathStyle := sDim
	if p.inPath {
		pathStyle = sAccent
	}
	sb.WriteString("\n" + pathStyle.Render("path ") + p.input.View() + "\n")
	if p.err != "" {
		sb.WriteString(sErr.Render("  "+p.err) + "\n")
	}
	sb.WriteString(sDim.Render(strings.Repeat("─", w-4)) + "\n")
	end := p.top + rows
	if end > len(p.entries) {
		end = len(p.entries)
	}
	for i := p.top; i < end; i++ {
		e := p.entries[i]
		name := e.Name
		var line string
		switch {
		case e.IsDir:
			line = sInfo.Render(padRight(truncate(name+"/", w-24), w-24)) + sDim.Render("<dir>")
		case e.Pref:
			line = sBold.Render(padRight(truncate(name, w-24), w-24)) + sDim.Render(humanSize(e.Size))
		default:
			line = sDim.Render(padRight(truncate(name, w-24), w-24) + humanSize(e.Size))
		}
		if i == p.cur && !p.inPath {
			sb.WriteString(sAccent.Render("▶ ") + line + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	if len(p.entries) == 0 {
		sb.WriteString(sDim.Render("  (empty folder)") + "\n")
	}
	if len(p.entries) > rows {
		sb.WriteString(sDim.Render(fmt.Sprintf("  %d–%d of %d", p.top+1, end, len(p.entries))) + "\n")
	}
	esc := key("esc", "cancel")
	if p.Optional {
		esc = key("esc", "none")
	}
	sb.WriteString("\n" + key("↑↓", "move") + " " + key("enter", "open/select") + " " + key("←", "parent") + " " + key("tab", "type path") + " " + esc)
	return sModal.Width(w).Render(sb.String())
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
