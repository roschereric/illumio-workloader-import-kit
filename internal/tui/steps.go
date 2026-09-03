package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/roschereric/illumio-workloader-import-kit/internal/engine"
)

// ------------------------------------------------------------------ step 0: preflight

func (m *model) runChecks() tea.Cmd {
	m.setBusy("checking workloader and PCE")
	cfg := m.cfg
	w := m.w
	return func() tea.Msg {
		var cs []check
		bin := engine.FindBinary(cfg.WorkloaderBin)
		if bin == "" {
			cs = append(cs, check{"workloader binary", "fail", "not found in ./workloader, PATH or --workloader — press d to download the release"})
			cs = append(cs, check{"pce.yaml", "pending", "checked once workloader is present"})
			cs = append(cs, check{"PCE connection", "pending", ""})
			return checksDoneMsg{cs}
		}
		w.Bin = bin
		ver := engine.Version(bin)
		cs = append(cs, check{"workloader binary", "ok", bin + "  " + ver})
		st := w.PCEList()
		if st.Configured {
			cs = append(cs, check{"pce.yaml", "ok", st.ConfigPath + " — " + pceSummary(st.Listing)})
		} else {
			cs = append(cs, check{"pce.yaml", "fail", st.ConfigPath + " has no PCE — press p to run pce-add --api-key"})
			cs = append(cs, check{"PCE connection", "pending", "after pce-add"})
			return checksDoneMsg{cs}
		}
		if w.ConnTest() {
			cs = append(cs, check{"PCE connection", "ok", "label-dimension-export succeeded (read-only)"})
		} else {
			cs = append(cs, check{"PCE connection", "fail", "label-dimension-export failed — check FQDN / API key / org"})
		}
		return checksDoneMsg{cs}
	}
}

func (m *model) preflightOK() bool {
	for _, c := range m.checks {
		if c.Status != "ok" {
			return false
		}
	}
	return len(m.checks) == 3
}

func (m *model) preflightView() string {
	var sb strings.Builder
	sb.WriteString(section("Environment") + "\n")
	sb.WriteString(fmt.Sprintf("  cwd        %s\n  runs       %s\n  os/arch    %s/%s   umwl-tui %s\n\n", cwd(), m.runDir, runtime.GOOS, runtime.GOARCH, m.cfg.Version))
	sb.WriteString(section("Checks") + "\n")
	if len(m.checks) == 0 {
		sb.WriteString("  " + m.spin.View() + " running…\n")
	}
	for _, c := range m.checks {
		g := map[string]string{"ok": sOK.Render("✔"), "fail": sErr.Render("✖"), "warn": sWarn.Render("▲"), "pending": sDim.Render("○")}[c.Status]
		sb.WriteString(fmt.Sprintf("  %s %-18s %s\n", g, c.Label, wrapIndent(c.Detail, m.mainWidth()-26, 23)))
	}
	sb.WriteString("\n" + section("Actions") + "\n")
	sb.WriteString("  " + key("d", "download workloader release") + "   " + key("p", "pce-add (API key)") + "   " + key("t", "test connection") + "   " + key("r", "re-run checks") + "\n")
	if m.preflightOK() {
		if m.cfg.SetupOnly {
			sb.WriteString("\n" + sOK.Render("Setup complete.") + " Run again with the CSV when it is ready. " + key("q", "quit") + "\n")
		} else if m.cfg.CSV == "" {
			sb.WriteString("\n" + sWarn.Render("No CSV given.") + " Press " + sKey.Render("enter") + " to choose the file, or quit and run: umwl-tui <csv> --ipl <ipl csv>\n")
		} else {
			sb.WriteString("\n" + sOK.Render("Ready.") + " Press " + sKey.Render("enter") + " to load " + m.cfg.CSV + "\n")
		}
	}
	sb.WriteString("\n" + sDim.Render("One working folder per Illumio account + PCE: workloader reads ./pce.yaml from the current directory.") + "\n")
	return sb.String()
}

func (m *model) preflightKeys() []string {
	return []string{key("enter", "next"), key("d", "download"), key("p", "pce-add"), key("t", "test"), key("r", "re-check")}
}

func (m *model) preflightKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "r":
		return m, m.runChecks()
	case "d":
		tag := engine.LatestTag()
		body := []string{"Downloads the workloader release for this OS into the current folder as ./workloader and removes the macOS quarantine flag.",
			"Latest tag: " + orDash(tag) + "   OS: " + runtime.GOOS + "/" + runtime.GOARCH}
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			body = append(body, "The macOS release is an Intel binary; it runs under Rosetta 2. For a native build: brew install go && go build from a clone of "+engine.Repo)
		}
		m.modal = choiceModal("Download workloader "+orDash(tag)+"?", body, []Option{{Key: "y", Label: "download"}, {Key: "esc", Label: "cancel"}}, func(m *model, key string) tea.Cmd {
			if key != "y" {
				return nil
			}
			m.setBusy("downloading workloader")
			out := m.w.Out
			return func() tea.Msg {
				p, err := engine.DownloadRelease(tag, out)
				return downloadDoneMsg{p, err}
			}
		})
		return m, nil
	case "p":
		if m.w.Bin == "" {
			m.modal = infoModal("workloader missing", []string{"Download or install workloader first (d)."})
			return m, nil
		}
		m.modal = formModal("Add PCE (pce-add --api-key)", []string{
			"Create the key in the PCE console: user menu → My API Keys → Add. Copy 'Authentication Username' (api_…) and 'Secret'.",
			"For SaaS, org id is the number in the console URL (…/orgs/<id>/…) and the FQDN is the console host; port 443. Email/password login is never used."},
			[]Field{
				{Key: "name", Label: "PCE short name", Default: "pce"},
				{Key: "fqdn", Label: "FQDN", Default: "xxx.illum.io"},
				{Key: "port", Label: "Port", Default: "443"},
				{Key: "user", Label: "API user (api_…)"},
				{Key: "secret", Label: "API secret", Secret: true},
				{Key: "org", Label: "Org id", Default: "1"},
				{Key: "tls", Label: "Disable TLS verify", Default: "false"},
			}, func(m *model, v map[string]string) tea.Cmd {
				if v["user"] == "" || v["secret"] == "" || v["fqdn"] == "" {
					m.modal = infoModal("Missing values", []string{"FQDN, API user and API secret are required."})
					return nil
				}
				m.setBusy("pce-add")
				w := m.w
				return func() tea.Msg {
					return pceAddDoneMsg{w.PCEAdd(v["name"], v["fqdn"], v["port"], v["user"], v["secret"], v["org"], v["tls"])}
				}
			})
		return m, nil
	case "t":
		if m.w.Bin == "" {
			return m, nil
		}
		m.setBusy("testing PCE connection")
		w := m.w
		return m, func() tea.Msg { return connDoneMsg{w.ConnTest()} }
	case "enter":
		if !m.preflightOK() {
			m.modal = infoModal("Not ready", []string{"All three checks must pass before loading the CSV."})
			return m, nil
		}
		if m.cfg.SetupOnly {
			m.finished = true
			m.quitting = true
			return m, tea.Quit
		}
		if m.cfg.CSV == "" {
			return m, m.openCSVPicker()
		}
		return m, m.loadCSV()
	}
	return m, nil
}

// ------------------------------------------------------------------ step 1: CSV

func (m *model) loadCSV() tea.Cmd {
	m.goTo(stCSV)
	m.setBusy("loading CSV")
	path, prio := m.cfg.CSV, m.cfg.Priority
	return func() tea.Msg {
		cf, err := engine.LoadCSV(path, prio)
		return csvDoneMsg{cf, err}
	}
}

func (m *model) csvView() string {
	if m.csv == nil {
		return m.spin.View() + " loading " + m.cfg.CSV
	}
	var sb strings.Builder
	sb.WriteString(section(fmt.Sprintf("%s — %d rows · label columns: %s", filepath.Base(m.csv.Path), len(m.csv.Rows), strings.Join(m.csv.LabelCols, ", "))) + "\n")
	for _, w := range m.csv.Warnings {
		sb.WriteString("  " + sWarn.Render("▲ ") + truncate(w, m.mainWidth()-6) + "\n")
	}
	for _, d := range m.csv.Dropped {
		sb.WriteString("  " + sErr.Render("✖ ") + truncate(d, m.mainWidth()-6) + "\n")
	}
	reserved := 9 + len(m.csv.Warnings) + len(m.csv.Dropped)
	rows := m.tableRows(reserved)
	m.clampCursor(len(m.csv.Rows))
	m.scrollTo(rows)
	cols := []col{{"IP", 16}, {"name", 30}}
	for _, k := range m.csv.LabelCols {
		cols = append(cols, col{k, 16})
	}
	cols = append(cols, col{"P", 3})
	sb.WriteString("\n" + m.renderTable(cols, len(m.csv.Rows), rows, func(i int) []string {
		r := m.csv.Rows[i]
		cells := []string{r.IP(), r.Get("name")}
		for _, k := range m.csv.LabelCols {
			cells = append(cells, r.Get(k))
		}
		p := ""
		if mm := prioOf(r.Get("description")); mm != "" {
			p = mm
		}
		return append(cells, p)
	}))
	if len(m.csv.Rows) > 0 {
		r := m.csv.Rows[m.cur]
		sb.WriteString("\n" + section("Selected row") + "\n" + wrapIndent(r.Get("description"), m.mainWidth()-6, 2) + "\n")
	}
	return sb.String()
}

func (m *model) csvKeys() []string {
	return []string{key("enter", "inventory + reconcile"), key("↑↓", "browse"), key("o", "other CSV")}
}

func (m *model) csvKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		m.cur--
	case "down", "j":
		m.cur++
	case "pgup":
		m.cur -= 10
	case "pgdown":
		m.cur += 10
	case "home", "g":
		m.cur = 0
	case "end", "G":
		m.cur = 1 << 30
	case "o":
		m.backToPreflight()
		return m, m.openCSVPicker()
	case "enter":
		if m.csv == nil || len(m.csv.Rows) == 0 {
			return m, nil
		}
		return m, m.loadInventory()
	}
	return m, nil
}

// ------------------------------------------------------------------ step 2/3: inventory + labels

func (m *model) loadInventory() tea.Cmd {
	m.goTo(stInventory)
	m.setBusy("exporting PCE inventory (read-only)")
	w, cols := m.w, m.csv.LabelCols
	return func() tea.Msg {
		inv, err := w.LoadInventory(cols)
		return invDoneMsg{inv, err}
	}
}

func (m *model) inventoryView() string {
	if m.inv == nil {
		return m.spin.View() + " wkld-export · label-export · label-dimension-export"
	}
	var sb strings.Builder
	sb.WriteString(section("PCE inventory (read-only snapshot)") + "\n")
	sb.WriteString(fmt.Sprintf("  workloads      %d  (%d managed / VEN)\n  IPs indexed    %d\n  labels         %d\n  label types    %s\n",
		len(m.inv.Workloads), m.inv.Managed, len(m.inv.ByIP), len(m.inv.Labels), strings.Join(m.inv.LabelKeys, ", ")))
	sb.WriteString("\n  files: " + sDim.Render(m.inv.Files["workloads"]) + "\n")
	return sb.String()
}

func (m *model) labelsView() string {
	var sb strings.Builder
	sb.WriteString(section("Label plan") + "\n")
	if len(m.plan.UnknownKeys) > 0 {
		sb.WriteString("  " + sErr.Render("✖ ") + fmt.Sprintf("CSV columns that are NOT label types in this PCE: %s — workloader would ignore them silently.", strings.Join(m.plan.UnknownKeys, ", ")) + "\n")
		sb.WriteString("    " + key("d", "drop those columns and continue") + "   or quit and create the type first: workloader label-dimension-import\n\n")
	}
	if len(m.plan.NewValues) == 0 {
		sb.WriteString("  " + sOK.Render("✔ every label value in the CSV already exists in the PCE") + "\n")
	} else {
		sb.WriteString(fmt.Sprintf("  %d label values will be CREATED by workloader during the import:\n\n", len(m.plan.NewValues)))
		rows := m.tableRows(8)
		m.clampCursor(len(m.plan.NewValues))
		m.scrollTo(rows)
		sb.WriteString(m.renderTable([]col{{"type", 12}, {"value", 40}}, len(m.plan.NewValues), rows, func(i int) []string {
			return []string{m.plan.NewValues[i].Key, m.plan.NewValues[i].Value}
		}))
	}
	sb.WriteString("\n" + key("enter", "accept and reconcile by IP") + "\n")
	return sb.String()
}

func (m *model) labelsKeys() []string {
	ks := []string{key("enter", "accept")}
	if len(m.plan.UnknownKeys) > 0 {
		ks = append(ks, key("d", "drop unknown columns"))
	}
	return ks
}

func (m *model) labelsKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		m.cur--
	case "down", "j":
		m.cur++
	case "d":
		if len(m.plan.UnknownKeys) > 0 {
			drop := map[string]bool{}
			for _, k := range m.plan.UnknownKeys {
				drop[k] = true
			}
			var keep []string
			for _, c := range m.csv.LabelCols {
				if !drop[c] {
					keep = append(keep, c)
				}
			}
			m.csv.LabelCols = keep
			m.plan = engine.PlanLabels(m.csv, m.inv)
			m.event(fmt.Sprintf("dropped non-label columns %v", m.plan.UnknownKeys))
		}
	case "enter":
		if len(m.plan.UnknownKeys) > 0 {
			m.modal = infoModal("Unknown label columns", []string{"Drop them (d) or quit and create the label type first."})
			return m, nil
		}
		for _, kv := range m.plan.NewValues {
			m.report.LabelsPlanned = append(m.report.LabelsPlanned, kv.Key+"="+kv.Value)
		}
		engine.Classify(m.csv, m.inv)
		m.decided = map[*engine.Row]bool{}
		n := map[string]int{}
		for _, r := range m.csv.Rows {
			n[r.Review]++
			if r.Review == "NEW" {
				m.decided[r] = true
			}
		}
		m.event(fmt.Sprintf("reconcile: %d NEW · %d EXISTS · %d CONFLICT", n["NEW"], n["EXISTS-UNMANAGED"], n["CONFLICT-MANAGED"]+n["CONFLICT-MULTIPLE"]))
		m.goTo(stReconcile)
		// jump to the first undecided row
		for i, r := range m.csv.Rows {
			if !m.decided[r] {
				m.cur = i
				break
			}
		}
	}
	return m, nil
}

// ------------------------------------------------------------------ step 4: reconcile

func (m *model) reconcileView() string {
	var sb strings.Builder
	undecided := 0
	for _, r := range m.csv.Rows {
		if !m.decided[r] {
			undecided++
		}
	}
	title := fmt.Sprintf("Reconcile by IP — %d rows · %d awaiting a decision", len(m.csv.Rows), undecided)
	sb.WriteString(section(title) + "\n")
	rows := m.tableRows(9 + len(m.csv.LabelCols))
	m.clampCursor(len(m.csv.Rows))
	m.scrollTo(rows)
	sb.WriteString(m.renderTable([]col{{"IP", 16}, {"proposed name", 28}, {"state", 18}, {"in the PCE", 26}}, len(m.csv.Rows), rows, func(i int) []string {
		r := m.csv.Rows[i]
		pce := ""
		if r.Match != nil {
			pce = firstNonEmpty(r.Match.Hostname, r.Match.Name)
		}
		return []string{r.IP(), r.Get("name"), badge(r.Review), pce}
	}))
	if len(m.csv.Rows) > 0 {
		r := m.csv.Rows[m.cur]
		sb.WriteString("\n" + section("Selected: "+r.IP()) + "\n")
		w := (m.mainWidth() - 10) / 2
		left := sBold.Render("proposed") + "\n" + kv("name", r.Get("name"), w) + kv("hostname", r.Get("hostname"), w)
		for _, k := range m.csv.LabelCols {
			left += kv(k, r.Get(k), w)
		}
		right := ""
		if r.Match != nil {
			mm := r.Match
			tag := "unmanaged"
			if mm.Managed {
				tag = sErr.Render("MANAGED (VEN)")
			}
			right = sBold.Render("in the PCE · "+tag) + "\n" + kv("name", mm.Name, w) + kv("hostname", mm.Hostname, w)
			for _, k := range m.csv.LabelCols {
				right += kv(k, mm.Labels[k], w)
			}
			if len(r.Matches) > 1 {
				right += sWarn.Render(fmt.Sprintf("  %d workloads share this IP", len(r.Matches))) + "\n"
			}
		} else {
			right = sBold.Render("in the PCE") + "\n" + sDim.Render("  no workload has this IP → will be created") + "\n"
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(w+2).Render(left), lipgloss.NewStyle().Width(w+2).Render(right)))
	}
	return sb.String()
}

func (m *model) reconcileKeys() []string {
	return []string{key("u", "update"), key("s", "skip"), key("c", "create anyway"), key("r", "rename+update"), key("U/S", "all of this kind"), key("enter", "next")}
}

func (m *model) reconcileKey(k string) (tea.Model, tea.Cmd) {
	if len(m.csv.Rows) == 0 {
		return m, nil
	}
	r := m.csv.Rows[m.cur]
	kind := r.Review
	apply := func(row *engine.Row, d engine.Decision, name string) {
		if row.Review == "NEW" || strings.HasPrefix(row.Review, "SKIPPED") || strings.HasPrefix(row.Review, "UPDATE") || row.Review == "NEW-DUP" {
			return
		}
		engine.Apply(row, d, name)
		m.decided[row] = true
	}
	next := func() {
		for i := m.cur + 1; i < len(m.csv.Rows); i++ {
			if !m.decided[m.csv.Rows[i]] {
				m.cur = i
				return
			}
		}
		for i := 0; i < len(m.csv.Rows); i++ {
			if !m.decided[m.csv.Rows[i]] {
				m.cur = i
				return
			}
		}
	}
	switch k {
	case "up", "k":
		m.cur--
	case "down", "j":
		m.cur++
	case "pgup":
		m.cur -= 10
	case "pgdown":
		m.cur += 10
	case "n":
		next()
	case "u":
		if r.Review == "CONFLICT-MANAGED" {
			m.modal = choiceModal("Update a MANAGED workload?", []string{"This IP belongs to a workload with a VEN. Updating changes the labels of a managed workload, which can change its policy immediately."},
				[]Option{{Key: "y", Label: "yes, update labels/description"}, {Key: "esc", Label: "cancel"}}, func(m *model, key string) tea.Cmd {
					if key == "y" {
						apply(r, engine.DecUpdate, "")
						next()
					}
					return nil
				})
			m.modal.Danger = true
			return m, nil
		}
		apply(r, engine.DecUpdate, "")
		next()
	case "s":
		apply(r, engine.DecSkip, "")
		next()
	case "c":
		apply(r, engine.DecCreate, "")
		next()
	case "r":
		if m.decided[r] {
			return m, nil
		}
		m.modal = formModal("Rename and update", []string{"The existing workload keeps its href and interfaces; name, labels and description are updated."},
			[]Field{{Key: "name", Label: "New visible name", Default: r.Get("name")}}, func(m *model, v map[string]string) tea.Cmd {
				apply(r, engine.DecUpdate, v["name"])
				next()
				return nil
			})
	case "U", "S":
		d := engine.DecUpdate
		if k == "S" {
			d = engine.DecSkip
		}
		if kind == "NEW" || m.decided[r] {
			return m, nil
		}
		n := 0
		for _, row := range m.csv.Rows {
			if !m.decided[row] && row.Review == kind {
				if kind == "CONFLICT-MANAGED" && d == engine.DecUpdate {
					continue
				}
				apply(row, d, "")
				n++
			}
		}
		m.event(fmt.Sprintf("%s applied to %d %s rows", d, n, kind))
		next()
	case "enter":
		undecided := 0
		for _, row := range m.csv.Rows {
			if !m.decided[row] {
				undecided++
			}
		}
		if undecided > 0 {
			m.modal = choiceModal(fmt.Sprintf("%d rows still undecided", undecided), []string{"Skip them all and continue, or go back and decide one by one (n jumps to the next undecided row)."},
				[]Option{{Key: "s", Label: "skip the undecided rows"}, {Key: "esc", Label: "back"}}, func(m *model, key string) tea.Cmd {
					if key == "s" {
						for _, row := range m.csv.Rows {
							if !m.decided[row] {
								apply(row, engine.DecSkip, "")
							}
						}
						return m.toReview()
					}
					return nil
				})
			return m, nil
		}
		return m, m.toReview()
	}
	return m, nil
}

func (m *model) toReview() tea.Cmd {
	m.buckets = engine.Split(m.csv)
	m.event(fmt.Sprintf("decisions: create %d · update %d · skip %d", len(m.buckets.Create), len(m.buckets.Update), len(m.buckets.Skipped)))
	m.goTo(stReview)
	return nil
}

// ------------------------------------------------------------------ step 5: review new

func (m *model) reviewView() string {
	var sb strings.Builder
	sb.WriteString(section(fmt.Sprintf("Workloads to CREATE — %d  (update %d · skip %d)", len(m.buckets.Create), len(m.buckets.Update), len(m.buckets.Skipped))) + "\n")
	if len(m.buckets.Create) == 0 {
		sb.WriteString("  nothing to create; press enter to dry-run the updates\n")
		return sb.String()
	}
	rows := m.tableRows(9)
	m.clampCursor(len(m.buckets.Create))
	m.scrollTo(rows)
	cols := []col{{"IP", 16}, {"name", 30}, {"hostname", 18}}
	for _, k := range m.csv.LabelCols {
		cols = append(cols, col{k, 14})
	}
	sb.WriteString(m.renderTable(cols, len(m.buckets.Create), rows, func(i int) []string {
		r := m.buckets.Create[i]
		cells := []string{r.IP(), r.Get("name"), r.Get("hostname")}
		for _, k := range m.csv.LabelCols {
			cells = append(cells, r.Get(k))
		}
		return cells
	}))
	r := m.buckets.Create[m.cur]
	sb.WriteString("\n" + section("description") + "\n" + wrapIndent(r.Get("description"), m.mainWidth()-6, 2) + "\n")
	return sb.String()
}

func (m *model) reviewKeys() []string {
	return []string{key("enter", "accept all → dry run"), key("e", "edit"), key("s", "skip"), key("↑↓", "browse")}
}

func (m *model) reviewKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		m.cur--
	case "down", "j":
		m.cur++
	case "pgup":
		m.cur -= 10
	case "pgdown":
		m.cur += 10
	case "s":
		if len(m.buckets.Create) == 0 {
			return m, nil
		}
		r := m.buckets.Create[m.cur]
		r.Review = "SKIPPED-USER"
		m.buckets = engine.Split(m.csv)
	case "e":
		if len(m.buckets.Create) == 0 {
			return m, nil
		}
		r := m.buckets.Create[m.cur]
		fields := []Field{{Key: "name", Label: "name", Default: r.Get("name")}, {Key: "hostname", Label: "hostname", Default: r.Get("hostname")}, {Key: "description", Label: "description", Default: r.Get("description")}}
		for _, k := range m.csv.LabelCols {
			fields = append(fields, Field{Key: "label:" + k, Label: k, Default: r.Get(k)})
		}
		m.modal = formModal("Edit "+r.IP(), nil, fields, func(m *model, v map[string]string) tea.Cmd {
			r.Set("name", v["name"])
			r.Set("hostname", v["hostname"])
			r.Set("description", v["description"])
			for _, k := range m.csv.LabelCols {
				r.Set(k, v["label:"+k])
			}
			return nil
		})
	case "enter":
		return m, m.startDry()
	}
	return m, nil
}

// ------------------------------------------------------------------ step 6: dry run

func (m *model) startDry() tea.Cmd {
	out := append([]string{"hostname", "name", "interfaces", "description"}, m.csv.LabelCols...)
	m.createCSV = filepath.Join(m.runDir, "to-create.csv")
	m.updateCSV = filepath.Join(m.runDir, "to-update.csv")
	engine.WriteCSV(m.createCSV, out, m.buckets.Create)
	engine.WriteCSV(m.updateCSV, append([]string{"href"}, out...), m.buckets.Update)
	engine.WriteCSV(filepath.Join(m.runDir, "skipped.csv"), append(out, "review"), m.buckets.Skipped)
	m.goTo(stDry)
	m.setBusy("dry run (wkld-import without --update-pce)")
	w, c, u, nc, nu := m.w, m.createCSV, m.updateCSV, len(m.buckets.Create), len(m.buckets.Update)
	return func() tea.Msg {
		lines, ok := w.DryRun(c, u, nc, nu)
		return dryDoneMsg{lines, ok}
	}
}

func (m *model) dryView() string {
	var sb strings.Builder
	sb.WriteString(section("Dry run — what workloader intends to do (nothing written yet)") + "\n")
	if m.busy {
		sb.WriteString("  " + m.spin.View() + " running…\n")
		return sb.String()
	}
	rows := m.tableRows(7)
	if len(m.dryLines) > rows {
		m.clampCursor(len(m.dryLines))
		m.scrollTo(rows)
	}
	end := m.top + rows
	if end > len(m.dryLines) {
		end = len(m.dryLines)
	}
	for _, l := range m.dryLines[m.top:end] {
		st := sDim
		if strings.Contains(l, "[ERROR") {
			st = sErr
		} else if strings.Contains(l, "[WARN") {
			st = sWarn
		} else if strings.HasPrefix(l, "──") {
			st = sBold
		}
		sb.WriteString("  " + st.Render(truncate(l, m.mainWidth()-6)) + "\n")
	}
	if !m.dryOK {
		sb.WriteString("\n" + sErr.Render("The dry run reported errors.") + " Read the log above; you can still execute (not recommended).\n")
	}
	sb.WriteString("\n" + fmt.Sprintf("  %s create %d · update %d · skip %d", sBold.Render("Plan:"), len(m.buckets.Create), len(m.buckets.Update), len(m.buckets.Skipped)) + "\n")
	sb.WriteString("  " + key("enter", "EXECUTE against the PCE") + "   " + key("x", "stop here and write the report") + "\n")
	return sb.String()
}

func (m *model) dryKeys() []string {
	return []string{key("enter", "execute"), key("x", "stop + report"), key("↑↓", "scroll")}
}

func (m *model) dryKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		m.cur--
	case "down", "j":
		m.cur++
	case "x":
		m.report.Aborted = true
		m.status[stExec], m.status[stVerify], m.status[stIPL] = "skipped", "skipped", "skipped"
		return m, m.writeReport()
	case "enter":
		body := []string{fmt.Sprintf("Create %d unmanaged workloads and update %d existing ones in the PCE, in batches of %d (wkld-import --update-pce --no-prompt).", len(m.buckets.Create), len(m.buckets.Update), m.cfg.Chunk)}
		if len(m.report.LabelsPlanned) > 0 {
			body = append(body, fmt.Sprintf("%d label values will be created.", len(m.report.LabelsPlanned)))
		}
		if !m.dryOK {
			body = append(body, sErr.Render("The dry run had errors."))
		}
		m.modal = choiceModal("Write to the PCE?", body, []Option{{Key: "y", Label: "yes, execute"}, {Key: "esc", Label: "not yet"}}, func(m *model, key string) tea.Cmd {
			if key == "y" {
				return m.startExec()
			}
			return nil
		})
		m.modal.Danger = true
	}
	return m, nil
}

// ------------------------------------------------------------------ step 7: execute

func (m *model) startExec() tea.Cmd {
	m.goTo(stExec)
	out := append([]string{"hostname", "name", "interfaces", "description"}, m.csv.LabelCols...)
	cc, err := engine.MakeChunks(m.runDir, "create", out, m.buckets.Create, m.cfg.Chunk)
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	uc, err := engine.MakeChunks(m.runDir, "update", append([]string{"href"}, out...), m.buckets.Update, m.cfg.Chunk)
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	m.chunks = append(cc, uc...)
	m.chunkIdx = 0
	m.event(fmt.Sprintf("executing %d batches", len(m.chunks)))
	return m.runNextChunk()
}

func (m *model) runNextChunk() tea.Cmd {
	for m.chunkIdx < len(m.chunks) && m.chunks[m.chunkIdx].Status != "pending" && m.chunks[m.chunkIdx].Status != "failed" {
		m.chunkIdx++
	}
	if m.chunkIdx >= len(m.chunks) {
		return m.startVerify()
	}
	c := m.chunks[m.chunkIdx]
	m.setBusy(fmt.Sprintf("batch %d/%d (%s, %d rows)", c.N, len(m.chunks), c.Name, len(c.Rows)))
	w := m.w
	return tea.Batch(m.prog.SetPercent(float64(m.chunkIdx)/float64(len(m.chunks))), func() tea.Msg {
		labels := w.RunChunk(c)
		return chunkDoneMsg{c, labels}
	})
}

func (m *model) execView() string {
	var sb strings.Builder
	done := 0
	for _, c := range m.chunks {
		if c.Status == "ok" {
			done++
		}
	}
	sb.WriteString(section(fmt.Sprintf("Executing — %d/%d batches ok", done, len(m.chunks))) + "\n")
	sb.WriteString("  " + m.prog.View() + "\n\n")
	rows := m.tableRows(8)
	start := 0
	if len(m.chunks) > rows {
		start = m.chunkIdx - rows/2
		if start < 0 {
			start = 0
		}
		if start+rows > len(m.chunks) {
			start = len(m.chunks) - rows
		}
	}
	end := start + rows
	if end > len(m.chunks) {
		end = len(m.chunks)
	}
	for _, c := range m.chunks[start:end] {
		g := map[string]string{"pending": sDim.Render("○"), "running": sAccent.Render(m.spin.View()), "ok": sOK.Render("✔"), "failed": sErr.Render("✖"), "skipped": sDim.Render("–")}[c.Status]
		first, last := "", ""
		if len(c.Rows) > 0 {
			first, last = c.Rows[0].IP(), c.Rows[len(c.Rows)-1].IP()
		}
		sb.WriteString(fmt.Sprintf("  %s batch %03d  %-6s  %3d rows   %s … %s\n", g, c.N, c.Name, len(c.Rows), first, last))
	}
	return sb.String()
}

func (m *model) execKeys() []string {
	return []string{sDim.Render("running — wait for the batches to finish")}
}

// ------------------------------------------------------------------ step 8/9/10

func (m *model) startVerify() tea.Cmd {
	m.goTo(stVerify)
	m.setBusy("verifying (wkld-export)")
	w := m.w
	created := m.buckets.Create
	return tea.Batch(m.prog.SetPercent(1), func() tea.Msg {
		f, miss, ok := w.Verify(created)
		return verifyDoneMsg{f, miss, ok}
	})
}

func (m *model) verifyView() string {
	var sb strings.Builder
	sb.WriteString(section("Verification") + "\n")
	if m.busy {
		sb.WriteString("  " + m.spin.View() + " re-exporting the inventory…\n")
	} else {
		sb.WriteString("  " + m.verifyMsg + "\n")
		sb.WriteString(fmt.Sprintf("\n  created %d · updated %d · labels created %d · failed batches %d\n", len(m.report.Created), len(m.report.Updated), len(m.report.LabelsCreated), len(m.report.FailedChunks)))
		if m.cfg.IPL != "" {
			sb.WriteString("\n  " + key("enter", "continue to IP lists") + "\n")
		} else {
			sb.WriteString("\n  " + key("enter", "write the report") + "\n")
		}
	}
	return sb.String()
}

func (m *model) verifyKeys() []string { return []string{key("enter", "next")} }

func (m *model) verifyKey(k string) (tea.Model, tea.Cmd) {
	if k != "enter" {
		return m, nil
	}
	if m.cfg.IPL == "" {
		m.status[stIPL] = "skipped"
		return m, m.writeReport()
	}
	m.goTo(stIPL)
	m.setBusy("ipl-import dry run")
	w, p := m.w, m.cfg.IPL
	return m, func() tea.Msg { return iplDryDoneMsg{w.IPLDry(p)} }
}

func (m *model) iplView() string {
	var sb strings.Builder
	sb.WriteString(section("IP lists — "+m.cfg.IPL) + "\n")
	if m.busy {
		sb.WriteString("  " + m.spin.View() + " running…\n")
		return sb.String()
	}
	rows := m.tableRows(6)
	end := len(m.iplLines)
	if end > rows {
		end = rows
	}
	for _, l := range m.iplLines[:end] {
		sb.WriteString("  " + sDim.Render(truncate(l, m.mainWidth()-6)) + "\n")
	}
	if len(m.iplLines) > rows {
		sb.WriteString(sDim.Render(fmt.Sprintf("  … %d more lines in the log pane", len(m.iplLines)-rows)) + "\n")
	}
	if _, done := m.report.IPLists["rc"]; done {
		sb.WriteString("\n  " + key("enter", "write the report") + "\n")
	} else {
		sb.WriteString("\n  " + key("enter", "import the IP lists (--update-pce)") + "   " + key("n", "skip") + "\n")
	}
	return sb.String()
}

func (m *model) iplKeys() []string { return []string{key("enter", "import"), key("n", "skip")} }

func (m *model) iplKey(k string) (tea.Model, tea.Cmd) {
	if _, done := m.report.IPLists["rc"]; done {
		if k == "enter" {
			return m, m.writeReport()
		}
		return m, nil
	}
	switch k {
	case "n":
		m.report.IPLists["skipped"] = true
		return m, m.writeReport()
	case "enter":
		m.modal = choiceModal("Import IP lists?", []string{fmt.Sprintf("%d IP lists from %s will be created/updated (ipl-import --update-pce --no-prompt).", engine.CountRows(m.cfg.IPL), m.cfg.IPL)},
			[]Option{{Key: "y", Label: "yes"}, {Key: "esc", Label: "cancel"}}, func(m *model, key string) tea.Cmd {
				if key != "y" {
					return nil
				}
				m.setBusy("ipl-import")
				w, p := m.w, m.cfg.IPL
				return func() tea.Msg { return iplDoneMsg{w.IPLImport(p)} }
			})
		m.modal.Danger = true
	}
	return m, nil
}

func (m *model) writeReport() tea.Cmd {
	m.goTo(stReport)
	m.setBusy("writing report")
	rp, w, b := m.report, m.w, m.buckets
	return func() tea.Msg {
		p, err := rp.Write(w, b)
		return reportDoneMsg{p, err}
	}
}

func (m *model) reportView() string {
	var sb strings.Builder
	sb.WriteString(section("Run report") + "\n")
	if m.busy {
		sb.WriteString("  " + m.spin.View() + " writing…\n")
		return sb.String()
	}
	status := sOK.Render("executed")
	if m.report.Aborted {
		status = sWarn.Render("stopped before writing to the PCE")
	}
	sb.WriteString("  status         " + status + "\n")
	sb.WriteString(fmt.Sprintf("  created        %d\n  updated        %d\n  skipped        %d\n  labels created %d\n  failed batches %d\n",
		len(m.report.Created), len(m.report.Updated), len(m.buckets.Skipped), len(m.report.LabelsCreated), len(m.report.FailedChunks)))
	if m.verifyMsg != "" {
		sb.WriteString("  verification   " + m.verifyMsg + "\n")
	}
	sb.WriteString("\n  " + sBold.Render("files") + "\n  " + m.reportMD + "\n  " + filepath.Join(m.runDir, "report.json") + "\n  " + m.runDir + "/ (inventories, chunk CSVs, workloader logs)\n")
	sb.WriteString("\n  " + key("q", "quit") + "\n")
	return sb.String()
}

func (m *model) reportKeys() []string { return []string{key("enter", "finish")} }

// ------------------------------------------------------------------ dispatch

func (m *model) mainView() string {
	switch m.step {
	case stPreflight:
		return m.preflightView()
	case stCSV:
		return m.csvView()
	case stInventory:
		return m.inventoryView()
	case stLabels:
		return m.labelsView()
	case stReconcile:
		return m.reconcileView()
	case stReview:
		return m.reviewView()
	case stDry:
		return m.dryView()
	case stExec:
		return m.execView()
	case stVerify:
		return m.verifyView()
	case stIPL:
		return m.iplView()
	case stReport:
		return m.reportView()
	}
	return ""
}

func (m *model) stepKeys() []string {
	switch m.step {
	case stPreflight:
		return m.preflightKeys()
	case stCSV:
		return m.csvKeys()
	case stLabels:
		return m.labelsKeys()
	case stReconcile:
		return m.reconcileKeys()
	case stReview:
		return m.reviewKeys()
	case stDry:
		return m.dryKeys()
	case stExec:
		return m.execKeys()
	case stVerify:
		return m.verifyKeys()
	case stIPL:
		return m.iplKeys()
	case stReport:
		return m.reportKeys()
	}
	return nil
}

func (m *model) stepKey(k string) (tea.Model, tea.Cmd) {
	switch m.step {
	case stPreflight:
		return m.preflightKey(k)
	case stCSV:
		return m.csvKey(k)
	case stLabels:
		return m.labelsKey(k)
	case stReconcile:
		return m.reconcileKey(k)
	case stReview:
		return m.reviewKey(k)
	case stDry:
		return m.dryKey(k)
	case stVerify:
		return m.verifyKey(k)
	case stIPL:
		return m.iplKey(k)
	case stReport:
		if k == "enter" {
			return m, m.askQuit()
		}
	}
	return m, nil
}

// onWork handles the async results.
func (m *model) onWork(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case checksDoneMsg:
		m.checks = msg.checks
		m.idle()
		if m.preflightOK() {
			m.event("preflight ok — " + strings.TrimPrefix(m.checks[1].Detail, engine.ConfigPath()+" — "))
		}
	case downloadDoneMsg:
		m.idle()
		if msg.err != nil {
			m.fail("download failed: " + msg.err.Error())
			return m, nil
		}
		m.cfg.WorkloaderBin = msg.path
		m.event("workloader downloaded: " + msg.path)
		return m, m.runChecks()
	case pceAddDoneMsg:
		m.idle()
		if msg.res.RC != 0 {
			m.fail("pce-add failed; see the log pane")
			return m, nil
		}
		m.event("PCE added to pce.yaml")
		return m, m.runChecks()
	case connDoneMsg:
		m.idle()
		if len(m.checks) == 3 {
			if msg.ok {
				m.checks[2] = check{"PCE connection", "ok", "label-dimension-export succeeded (read-only)"}
			} else {
				m.checks[2] = check{"PCE connection", "fail", "label-dimension-export failed — check FQDN / API key / org"}
			}
		}
	case csvDoneMsg:
		m.idle()
		if msg.err != nil {
			m.status[stCSV] = "failed"
			m.appendLog(sErr.Render("✖ ") + "CSV: " + msg.err.Error())
			m.modal = infoModal("Could not load the CSV", []string{m.cfg.CSV, msg.err.Error(), "", "Close this dialog to choose another file."})
			m.modal.OnClose = func(m *model) tea.Cmd {
				m.backToPreflight()
				return m.openCSVPicker()
			}
			return m, nil
		}
		m.csv = msg.cf
		m.event(fmt.Sprintf("CSV loaded: %d rows, %d dropped", len(msg.cf.Rows), len(msg.cf.Dropped)))
		for _, wmsg := range msg.cf.Warnings {
			m.appendLog(sWarn.Render("▲ ") + wmsg)
		}
	case invDoneMsg:
		m.idle()
		if msg.err != nil {
			m.fail(msg.err.Error())
			return m, nil
		}
		m.inv = msg.inv
		m.event(fmt.Sprintf("inventory: %d workloads, %d IPs", len(msg.inv.Workloads), len(msg.inv.ByIP)))
		m.plan = engine.PlanLabels(m.csv, m.inv)
		m.goTo(stLabels)
	case dryDoneMsg:
		m.idle()
		m.dryLines, m.dryOK = msg.lines, msg.ok
		m.event(fmt.Sprintf("dry run done (ok=%v, %d lines)", msg.ok, len(msg.lines)))
	case chunkDoneMsg:
		c := msg.c
		m.report.LabelsCreated = append(m.report.LabelsCreated, msg.labels...)
		if c.Status == "ok" {
			for _, r := range c.Rows {
				if c.Name == "create" {
					m.report.Created = append(m.report.Created, r.IP())
				} else {
					m.report.Updated = append(m.report.Updated, r.IP())
				}
			}
			m.chunkIdx++
			return m, m.runNextChunk()
		}
		m.idle()
		body := []string{fmt.Sprintf("Batch %d (%s) exited with problems:", c.N, c.Name)}
		n := len(c.Errors)
		if n > 5 {
			c.Errors = c.Errors[n-5:]
		}
		body = append(body, c.Errors...)
		m.modal = choiceModal("Batch failed", body, []Option{{Key: "r", Label: "retry this batch"}, {Key: "s", Label: "skip it"}, {Key: "q", Label: "abort the run (report + quit)"}}, func(m *model, key string) tea.Cmd {
			switch key {
			case "r":
				c.Status = "pending"
				return m.runNextChunk()
			case "s":
				c.Status = "skipped"
				m.report.FailedChunks = append(m.report.FailedChunks, map[string]string{"file": c.File, "log": c.Log})
				m.chunkIdx++
				return m.runNextChunk()
			default:
				m.report.FailedChunks = append(m.report.FailedChunks, map[string]string{"file": c.File, "log": c.Log})
				m.status[stExec] = "failed"
				m.status[stVerify], m.status[stIPL] = "skipped", "skipped"
				return m.writeReport()
			}
		})
		m.modal.Danger = true
	case verifyDoneMsg:
		m.idle()
		if !msg.ok {
			m.verifyMsg = sErr.Render("could not re-export the inventory")
		} else if len(msg.missing) == 0 {
			m.verifyMsg = sOK.Render(fmt.Sprintf("%d/%d created IPs confirmed in the PCE", msg.found, msg.found))
		} else {
			m.verifyMsg = sErr.Render(fmt.Sprintf("%d/%d created IPs confirmed; missing: %s", msg.found, msg.found+len(msg.missing), strings.Join(msg.missing, ", ")))
		}
		m.report.Verify = map[string]any{"created_expected": msg.found + len(msg.missing), "found": msg.found, "missing": msg.missing}
		m.event("verify: " + stripANSI(m.verifyMsg))
	case iplDryDoneMsg:
		m.idle()
		m.iplLines = msg.lines
	case iplDoneMsg:
		m.idle()
		m.report.IPLists = map[string]any{"rc": msg.res.RC, "log": msg.res.Log, "rows": engine.CountRows(m.cfg.IPL)}
		if msg.res.RC == 0 {
			m.event("IP lists imported")
		} else {
			m.event(fmt.Sprintf("ipl-import failed rc=%d", msg.res.RC))
		}
	case reportDoneMsg:
		m.idle()
		m.finished = true
		if msg.err != nil {
			m.fail("report: " + msg.err.Error())
			return m, nil
		}
		m.reportMD = msg.path
		m.status[stReport] = "done"
		m.event("report written: " + msg.path)
	}
	return m, nil
}

func cwd() string {
	d, _ := os.Getwd()
	return d
}

// pceSummary picks the informative line of pce-list (the row with the PCE, not the table frame).
func pceSummary(listing string) string {
	var best string
	for _, l := range strings.Split(listing, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "+") {
			continue
		}
		if strings.Contains(l, "|") {
			l = strings.Join(strings.Fields(strings.ReplaceAll(l, "|", " ")), " ")
		}
		if strings.Contains(strings.ToLower(l), "[info]") {
			continue
		}
		best = l
	}
	if best == "" {
		return firstLine(listing)
	}
	return best
}

// backToPreflight rewinds from a failed/abandoned CSV load so another file can be chosen.
func (m *model) backToPreflight() {
	m.cfg.CSV = ""
	m.csv = nil
	m.step = stPreflight
	m.status[stCSV] = "pending"
	m.status[stPreflight] = "active"
	m.cur, m.top = 0, 0
}

// openCSVPicker chains: workloads CSV → IP lists CSV (optional) → priority → load.
func (m *model) openCSVPicker() tea.Cmd {
	m.picker = newPicker("Proposed workloads CSV", "Pick the file with one row per IP (hostname,name,interfaces,description,<labels>...). Start: the working folder.", "", []string{".csv"},
		func(m *model, path string) tea.Cmd {
			m.cfg.CSV = path
			m.report.CSV = path
			ipl := newPicker("IP lists CSV (optional)", "workloader ipl-import format (name,description,include,exclude,fqdns). esc = no IP lists.", filepath.Dir(path), []string{".csv"},
				func(m *model, p2 string) tea.Cmd {
					m.cfg.IPL = p2
					m.report.IPL = p2
					m.modal = formModal("Priority filter", []string{"Only rows whose description carries [.. P<n> ..] for these priorities (e.g. 1 or 1,2). Leave empty to load every row."},
						[]Field{{Key: "prio", Label: "Priority filter", Default: m.cfg.Priority}}, func(m *model, v map[string]string) tea.Cmd {
							m.cfg.Priority = v["prio"]
							return m.loadCSV()
						})
					return nil
				})
			ipl.Optional = true
			m.picker = ipl
			return nil
		})
	return nil
}
