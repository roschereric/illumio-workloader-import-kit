package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PCEWorkload is one row of wkld-export.
type PCEWorkload struct {
	Href     string
	Hostname string
	Name     string
	Managed  bool
	Labels   map[string]string
	IPs      []string
}

// Inventory is the read-only snapshot of the PCE used for reconciliation.
type Inventory struct {
	Workloads []*PCEWorkload
	ByIP      map[string][]*PCEWorkload
	Managed   int
	Labels    map[string]bool // "key=value"
	LabelKeys []string        // label dimensions (types)
	Files     map[string]string
}

// LoadInventory runs wkld-export, label-export and label-dimension-export into the run dir.
func (w *Workloader) LoadInventory(labelCols []string) (*Inventory, error) {
	inv := &Inventory{ByIP: map[string][]*PCEWorkload{}, Labels: map[string]bool{}, Files: map[string]string{}}
	wk := filepath.Join(w.RunDir, "pce-workloads.csv")
	if res := w.Run("wkld-export.log", "wkld-export", "--output-file", wk); res.RC != 0 {
		return nil, fmt.Errorf("wkld-export failed (rc=%d) — see %s", res.RC, res.Log)
	}
	inv.Files["workloads"] = wk
	_, recs, err := readCSV(wk)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		managed := strings.EqualFold(strings.TrimSpace(r["managed"]), "true") || strings.TrimSpace(r["ven_href"]) != ""
		wl := &PCEWorkload{Href: r["href"], Hostname: r["hostname"], Name: r["name"], Managed: managed, Labels: map[string]string{}}
		for _, k := range labelCols {
			wl.Labels[k] = r[k]
		}
		seen := map[string]bool{}
		for _, ip := range append(ParseIPs(r["interfaces"]), ParseIPs(r["public_ip"])...) {
			if !seen[ip] {
				seen[ip] = true
				wl.IPs = append(wl.IPs, ip)
				inv.ByIP[ip] = append(inv.ByIP[ip], wl)
			}
		}
		if managed {
			inv.Managed++
		}
		inv.Workloads = append(inv.Workloads, wl)
	}
	lab := filepath.Join(w.RunDir, "pce-labels.csv")
	if res := w.Run("label-export.log", "label-export", "--output-file", lab); res.RC == 0 {
		inv.Files["labels"] = lab
		if _, recs, err := readCSV(lab); err == nil {
			for _, r := range recs {
				inv.Labels[r["key"]+"="+r["value"]] = true
			}
		}
	}
	dim := filepath.Join(w.RunDir, "pce-label-dimensions.csv")
	if res := w.Run("label-dimension-export.log", "label-dimension-export", "--output-file", dim); res.RC == 0 {
		inv.Files["dimensions"] = dim
		if _, recs, err := readCSV(dim); err == nil {
			for _, r := range recs {
				if k := strings.TrimSpace(r["key"]); k != "" {
					inv.LabelKeys = append(inv.LabelKeys, k)
				}
			}
		}
	}
	if len(inv.LabelKeys) == 0 {
		inv.LabelKeys = []string{"role", "app", "env", "loc"}
	}
	return inv, nil
}

// LabelPlan: which CSV columns are not PCE label types, and which values would be created.
type LabelPlan struct {
	UnknownKeys []string
	NewValues   []KV2
}
type KV2 struct{ Key, Value string }

func PlanLabels(cf *CSVFile, inv *Inventory) LabelPlan {
	known := map[string]bool{}
	for _, k := range inv.LabelKeys {
		known[k] = true
	}
	var plan LabelPlan
	for _, k := range cf.LabelCols {
		if !known[k] {
			plan.UnknownKeys = append(plan.UnknownKeys, k)
		}
	}
	seen := map[string]bool{}
	for _, r := range cf.Rows {
		for _, k := range cf.LabelCols {
			v := r.Get(k)
			if v == "" || !known[k] || inv.Labels[k+"="+v] || seen[k+"="+v] {
				continue
			}
			seen[k+"="+v] = true
			plan.NewValues = append(plan.NewValues, KV2{k, v})
		}
	}
	sort.Slice(plan.NewValues, func(i, j int) bool {
		if plan.NewValues[i].Key != plan.NewValues[j].Key {
			return plan.NewValues[i].Key < plan.NewValues[j].Key
		}
		return plan.NewValues[i].Value < plan.NewValues[j].Value
	})
	return plan
}

// Classify sets Review/Match on every row by looking its IP(s) up in the inventory.
func Classify(cf *CSVFile, inv *Inventory) {
	for _, r := range cf.Rows {
		seen := map[string]bool{}
		r.Matches = nil
		for _, ip := range r.IPs {
			for _, m := range inv.ByIP[ip] {
				if !seen[m.Href] {
					seen[m.Href] = true
					r.Matches = append(r.Matches, m)
				}
			}
		}
		switch {
		case len(r.Matches) == 0:
			r.Review = "NEW"
		default:
			r.Match = r.Matches[0]
			managed := false
			for _, m := range r.Matches {
				if m.Managed {
					managed = true
				}
			}
			switch {
			case managed:
				r.Review = "CONFLICT-MANAGED"
			case len(r.Matches) > 1:
				r.Review = "CONFLICT-MULTIPLE"
			default:
				r.Review = "EXISTS-UNMANAGED"
			}
		}
	}
}

// Decision applied to a non-NEW row.
type Decision string

const (
	DecUpdate Decision = "update" // labels/description on the existing object (by href)
	DecSkip   Decision = "skip"
	DecCreate Decision = "create" // create anyway (duplicate)
	DecNone   Decision = ""
)

// Apply turns a decision into the row's final state.
func Apply(r *Row, d Decision, newName string) {
	kind := r.Review
	switch d {
	case DecSkip:
		r.Review = "SKIPPED-" + kind
	case DecCreate:
		r.Review = "NEW-DUP"
	case DecUpdate:
		if newName != "" {
			r.Set("name", newName)
		} else if r.Match != nil && r.Match.Name != "" {
			r.Set("name", r.Match.Name)
		}
		if r.Match != nil {
			r.Href = r.Match.Href
			if r.Match.Hostname != "" {
				r.Set("hostname", r.Match.Hostname)
			}
		}
		r.Set("interfaces", "") // blank = leave existing interfaces untouched
		r.Review = "UPDATE-" + kind
	}
}

// Buckets splits rows by final state.
type Buckets struct{ Create, Update, Skipped []*Row }

func Split(cf *CSVFile) Buckets {
	var b Buckets
	for _, r := range cf.Rows {
		switch {
		case r.Review == "NEW" || r.Review == "NEW-DUP":
			b.Create = append(b.Create, r)
		case strings.HasPrefix(r.Review, "UPDATE-"):
			b.Update = append(b.Update, r)
		case strings.HasPrefix(r.Review, "SKIPPED-"):
			b.Skipped = append(b.Skipped, r)
		}
	}
	return b
}

// Report is the JSON/Markdown run summary.
type Report struct {
	Started       string              `json:"started"`
	Finished      string              `json:"finished"`
	CSV           string              `json:"csv"`
	IPL           string              `json:"ipl,omitempty"`
	PCE           string              `json:"pce"`
	Aborted       bool                `json:"aborted"`
	Created       []string            `json:"created"`
	Updated       []string            `json:"updated"`
	Skipped       []map[string]string `json:"skipped"`
	LabelsPlanned []string            `json:"labels_planned"`
	LabelsCreated []string            `json:"labels_created"`
	FailedChunks  []map[string]string `json:"failed_chunks"`
	Verify        map[string]any      `json:"verify"`
	IPLists       map[string]any      `json:"iplists"`
	Commands      []Call              `json:"commands"`
	Dir           string              `json:"run_dir"`
}

func NewReport(csvPath, ipl, pce, runDir string) *Report {
	return &Report{Started: time.Now().Format("20060102-150405"), CSV: csvPath, IPL: ipl, PCE: pce, Dir: runDir,
		Verify: map[string]any{}, IPLists: map[string]any{}}
}

// Write produces report.json and report.md in the run dir and returns the md path.
func (rp *Report) Write(w *Workloader, b Buckets) (string, error) {
	rp.Finished = time.Now().Format("20060102-150405")
	rp.Commands = w.Calls
	rp.Skipped = nil
	for _, r := range b.Skipped {
		rp.Skipped = append(rp.Skipped, map[string]string{"ip": r.IP(), "name": r.Get("name"), "why": r.Review})
	}
	js, _ := json.MarshalIndent(rp, "", "  ")
	os.WriteFile(filepath.Join(rp.Dir, "report.json"), js, 0o644)
	pce := rp.PCE
	if pce == "" {
		pce = "default (pce.yaml)"
	}
	status := "executed"
	if rp.Aborted {
		status = "aborted before writing to the PCE"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Unmanaged workload load — %s\n\n- Source CSV: `%s`\n- PCE: `%s`\n- Status: %s\n- Run folder: `%s`\n\n", rp.Started, rp.CSV, pce, status, rp.Dir)
	fmt.Fprintf(&sb, "## Summary\n\n| Created | Updated | Skipped | Failed chunks | Labels created |\n|---|---|---|---|---|\n| %d | %d | %d | %d | %d |\n\n",
		len(rp.Created), len(rp.Updated), len(rp.Skipped), len(rp.FailedChunks), len(rp.LabelsCreated))
	if len(rp.Verify) > 0 {
		fmt.Fprintf(&sb, "Post-load verification: %v\n\n", rp.Verify)
	}
	if len(rp.IPLists) > 0 {
		fmt.Fprintf(&sb, "IP lists: %v\n\n", rp.IPLists)
	}
	section := func(title string, rows []*Row) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&sb, "## %s\n\n| IP | hostname | name | state |\n|---|---|---|---|\n", title)
		for _, r := range rows {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", r.IP(), r.Get("hostname"), r.Get("name"), r.Review)
		}
		sb.WriteString("\n")
	}
	section("Created", b.Create)
	section("Updated", b.Update)
	section("Skipped", b.Skipped)
	if len(rp.LabelsCreated) > 0 {
		sb.WriteString("## Labels created\n\n")
		for _, l := range rp.LabelsCreated {
			fmt.Fprintf(&sb, "- %s\n", l)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## workloader calls\n\n")
	for _, c := range rp.Commands {
		fmt.Fprintf(&sb, "- rc=%d `%s` → `%s`\n", c.RC, strings.Join(c.Cmd, " "), c.Log)
	}
	p := filepath.Join(rp.Dir, "report.md")
	return p, os.WriteFile(p, []byte(sb.String()), 0o644)
}
