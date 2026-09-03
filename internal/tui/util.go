package tui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type col struct {
	Name  string
	Width int
}

// renderTable draws a header + rows [top, top+visible) with the cursor highlighted.
// Column widths are cell counts; the last column absorbs leftover width.
func (m *model) renderTable(cols []col, n, visible int, cell func(i int) []string) string {
	avail := m.mainWidth() - 6
	total := 0
	for _, c := range cols {
		total += c.Width + 2
	}
	if total < avail && len(cols) > 0 {
		cols[len(cols)-1].Width += avail - total
	} else if total > avail {
		// shrink the widest columns first
		for total > avail {
			wi := 0
			for i, c := range cols {
				if c.Width > cols[wi].Width {
					wi = i
				}
			}
			if cols[wi].Width <= 6 {
				break
			}
			cols[wi].Width--
			total--
		}
	}
	var sb strings.Builder
	hdr := ""
	for _, c := range cols {
		hdr += padRight(strings.ToUpper(c.Name), c.Width) + "  "
	}
	sb.WriteString("  " + sDim.Render(hdr) + "\n")
	end := m.top + visible
	if end > n {
		end = n
	}
	for i := m.top; i < end; i++ {
		cells := cell(i)
		line := ""
		for j, c := range cols {
			v := ""
			if j < len(cells) {
				v = cells[j]
			}
			line += padRight(truncate(v, c.Width), c.Width) + "  "
		}
		if i == m.cur {
			sb.WriteString(sAccent.Render("▶ ") + lipgloss.NewStyle().Bold(true).Render(line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	if n > visible {
		sb.WriteString(sDim.Render("  "+scrollHint(m.top, end, n)) + "\n")
	}
	return sb.String()
}

func scrollHint(top, end, n int) string {
	return "rows " + itoa(top+1) + "–" + itoa(end) + " of " + itoa(n)
}

func itoa(i int) string { return strconv.Itoa(i) }

// truncate cuts a string to w cells (ANSI-aware), adding … when cut.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return ansi.Truncate(s, w-1, "") + "…"
}

func padRight(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}

func wrap(s string, w int) string {
	if w < 10 {
		w = 10
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		cur := ""
		for _, wd := range words {
			if cur == "" {
				cur = wd
				continue
			}
			if ansi.StringWidth(cur)+1+ansi.StringWidth(wd) > w {
				lines = append(lines, cur)
				cur = wd
			} else {
				cur += " " + wd
			}
		}
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

func wrapIndent(s string, w, indent int) string {
	ind := strings.Repeat(" ", indent)
	parts := strings.Split(wrap(s, w), "\n")
	for i := 1; i < len(parts); i++ {
		parts[i] = ind + parts[i]
	}
	return strings.Join(parts, "\n")
}

func section(title string) string { return sAccent.Render("■ ") + sBold.Render(title) }

func kv(k, v string, w int) string {
	if v == "" {
		v = sDim.Render("—")
	}
	return "  " + sDim.Render(padRight(k, 12)) + truncate(v, w-14) + "\n"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

var prioRe = regexp.MustCompile(`\bP(\d)\b`)

func prioOf(desc string) string {
	if mm := prioRe.FindStringSubmatch(desc); mm != nil {
		return mm[1]
	}
	return ""
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// clip hard-limits a block to h lines of at most w cells, so bordered panels never grow.
func clip(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}
