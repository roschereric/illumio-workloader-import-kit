package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Modal captures all keys while open. Two kinds: choice (single keypress) and form (text inputs).
type Modal struct {
	Kind    string // "choice" | "form" | "info"
	Title   string
	Body    []string
	Options []Option // choice
	Fields  []Field  // form
	focus   int
	OnPick  func(m *model, key string) tea.Cmd               // choice
	OnForm  func(m *model, values map[string]string) tea.Cmd // form, values by field key
	OnClose func(m *model) tea.Cmd
	Danger  bool
}

type Option struct {
	Key   string
	Label string
	Style lipgloss.Style
}

type Field struct {
	Key     string
	Label   string
	Input   textinput.Model
	Secret  bool
	Default string
}

func choiceModal(title string, body []string, opts []Option, pick func(m *model, key string) tea.Cmd) *Modal {
	return &Modal{Kind: "choice", Title: title, Body: body, Options: opts, OnPick: pick}
}

func infoModal(title string, body []string) *Modal {
	return &Modal{Kind: "info", Title: title, Body: body, Options: []Option{{Key: "enter", Label: "close"}}}
}

func formModal(title string, body []string, fields []Field, submit func(m *model, v map[string]string) tea.Cmd) *Modal {
	for i := range fields {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 512
		ti.Width = 48
		ti.SetValue(fields[i].Default)
		if fields[i].Secret {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		fields[i].Input = ti
	}
	md := &Modal{Kind: "form", Title: title, Body: body, Fields: fields, OnForm: submit}
	if len(md.Fields) > 0 {
		md.Fields[0].Input.Focus()
	}
	return md
}

func (md *Modal) values() map[string]string {
	v := map[string]string{}
	for _, f := range md.Fields {
		v[f.Key] = strings.TrimSpace(f.Input.Value())
	}
	return v
}

func (md *Modal) setFocus(i int) {
	if len(md.Fields) == 0 {
		return
	}
	md.Fields[md.focus].Input.Blur()
	md.focus = (i + len(md.Fields)) % len(md.Fields)
	md.Fields[md.focus].Input.Focus()
}

// update handles a key while the modal is open. Returns (closeModal, cmd).
func (md *Modal) update(m *model, msg tea.KeyMsg) (bool, tea.Cmd) {
	k := msg.String()
	switch md.Kind {
	case "info":
		if k == "enter" || k == "esc" || k == "q" {
			if md.OnClose != nil {
				return true, md.OnClose(m)
			}
			return true, nil
		}
		return false, nil
	case "choice":
		if k == "esc" {
			for _, o := range md.Options {
				if o.Key == "esc" || o.Key == "q" {
					return true, md.OnPick(m, o.Key)
				}
			}
			return false, nil
		}
		for _, o := range md.Options {
			if o.Key == k {
				return true, md.OnPick(m, o.Key)
			}
		}
		return false, nil
	case "form":
		switch k {
		case "esc":
			return true, nil
		case "tab", "down":
			md.setFocus(md.focus + 1)
			return false, nil
		case "shift+tab", "up":
			md.setFocus(md.focus - 1)
			return false, nil
		case "enter":
			if md.focus < len(md.Fields)-1 {
				md.setFocus(md.focus + 1)
				return false, nil
			}
			return true, md.OnForm(m, md.values())
		}
		var cmd tea.Cmd
		md.Fields[md.focus].Input, cmd = md.Fields[md.focus].Input.Update(msg)
		return false, cmd
	}
	return false, nil
}

func (md *Modal) view(width int) string {
	w := width - 14
	if w > 90 {
		w = 90
	}
	if w < 40 {
		w = 40
	}
	var sb strings.Builder
	title := sAccent.Render(md.Title)
	if md.Danger {
		title = sErr.Render(md.Title)
	}
	sb.WriteString(title + "\n\n")
	for _, b := range md.Body {
		sb.WriteString(wrap(b, w-4) + "\n")
	}
	if len(md.Body) > 0 {
		sb.WriteString("\n")
	}
	switch md.Kind {
	case "form":
		for i, f := range md.Fields {
			cur := "  "
			if i == md.focus {
				cur = sAccent.Render("▶ ")
			}
			sb.WriteString(cur + sBold.Render(padRight(f.Label, 22)) + f.Input.View() + "\n")
		}
		sb.WriteString("\n" + key("tab/↑↓", "next field") + "  " + key("enter", "submit") + "  " + key("esc", "cancel"))
	default:
		for _, o := range md.Options {
			label := o.Label
			sb.WriteString("  " + sKey.Render("["+o.Key+"]") + " " + label + "\n")
		}
	}
	return sModal.Width(w).Render(sb.String())
}
