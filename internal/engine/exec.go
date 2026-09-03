package engine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CreateArgs returns the wkld-import flags for the create pass. workloader picks its match COLUMN by
// priority (href, then hostname, then name) and skips every row whose match column is blank — it never
// falls back to name. With the PCE convention of blank hostnames the create pass must match on name.
func CreateArgs(rows []*Row) []string {
	args := []string{"--umwl", "--update=false"}
	for _, r := range rows {
		if strings.TrimSpace(r.Get("hostname")) == "" {
			return append(args, "--match", "name")
		}
	}
	return args
}

// UpdateArgs: the update pass carries the href of the existing object, so match on it explicitly.
func UpdateArgs() []string { return []string{"--match", "href"} }

// DryRun runs wkld-import without --update-pce for the create and update CSVs and returns the
// interesting log lines. ok=false when workloader exited non-zero or reported skipped/blank rows.
func (w *Workloader) DryRun(createCSV, updateCSV string, create, update []*Row) (lines []string, ok bool) {
	ok = true
	nCreate, nUpdate := len(create), len(update)
	if nCreate > 0 {
		res := w.Run("dry-create.log", append([]string{"wkld-import", createCSV}, CreateArgs(create)...)...)
		lines = append(lines, "── create (--umwl) ──")
		got := LogLines(res.Log, `to be created|needs to be created|to be changed|is not a workload|cannot be blank|nothing to be done|\[WARN|\[ERROR`)
		lines = append(lines, got...)
		if res.RC != 0 || hasProblem(got) {
			ok = false
		}
	} else {
		lines = append(lines, "create: nothing to do")
	}
	if nUpdate > 0 {
		res := w.Run("dry-update.log", append([]string{"wkld-import", updateCSV}, UpdateArgs()...)...)
		lines = append(lines, "── update (by href) ──")
		got := LogLines(res.Log, `to be created|to be changed|is not a workload|cannot be blank|nothing to be done|\[WARN|\[ERROR`)
		lines = append(lines, got...)
		if res.RC != 0 || hasProblem(got) {
			ok = false
		}
	} else {
		lines = append(lines, "update: nothing to do")
	}
	return lines, ok
}

// Chunk is one batch of rows to import.
type Chunk struct {
	N      int
	Name   string // "create" | "update"
	File   string
	Log    string
	Rows   []*Row
	Status string // pending | running | ok | failed | skipped
	Errors []string
}

// MakeChunks writes chunk CSVs into the run dir.
func MakeChunks(runDir, name string, cols []string, rows []*Row, size int) ([]*Chunk, error) {
	if size <= 0 {
		size = 20
	}
	var out []*Chunk
	for i := 0; i < len(rows); i += size {
		end := i + size
		if end > len(rows) {
			end = len(rows)
		}
		n := len(out) + 1
		c := &Chunk{N: n, Name: name, Rows: rows[i:end], Status: "pending",
			File: filepath.Join(runDir, fmt.Sprintf("%s-chunk%03d.csv", name, n))}
		if err := WriteCSV(c.File, cols, c.Rows); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

var labelCreatedRe = regexp.MustCompile(`created new .* label`)

// RunChunk imports one chunk with --update-pce --no-prompt. Returns labels created (log lines).
func (w *Workloader) RunChunk(c *Chunk) (labels []string) {
	extra := UpdateArgs()
	if c.Name == "create" {
		extra = CreateArgs(c.Rows)
	}
	args := append([]string{"wkld-import", c.File, "--update-pce", "--no-prompt"}, extra...)
	c.Status = "running"
	res := w.Run(fmt.Sprintf("%s-chunk%03d.log", c.Name, c.N), args...)
	c.Log = res.Log
	for _, l := range LogLines(res.Log, labelCreatedRe.String()) {
		if i := strings.Index(l, " - "); i >= 0 {
			labels = append(labels, l[i+3:])
		} else {
			labels = append(labels, l)
		}
	}
	c.Errors = LogLines(res.Log, `\[ERROR|\[WARN|cannot be blank|nothing to be done`)
	if res.RC == 0 && len(c.Errors) == 0 {
		c.Status = "ok"
	} else {
		c.Status = "failed"
	}
	return labels
}

// Verify re-exports the inventory and checks every created IP is now present.
func (w *Workloader) Verify(created []*Row) (found int, missing []string, ok bool) {
	after := filepath.Join(w.RunDir, "pce-workloads-after.csv")
	res := w.Run("wkld-export-after.log", "wkld-export", "--output-file", after)
	if res.RC != 0 {
		return 0, nil, false
	}
	_, recs, err := readCSV(after)
	if err != nil {
		return 0, nil, false
	}
	have := map[string]bool{}
	for _, r := range recs {
		for _, ip := range ParseIPs(r["interfaces"]) {
			have[ip] = true
		}
	}
	for _, r := range created {
		if have[r.IP()] {
			found++
		} else {
			missing = append(missing, r.IP())
		}
	}
	return found, missing, true
}

// IPLDry / IPLImport wrap ipl-import.
func (w *Workloader) IPLDry(path string) []string {
	res := w.Run("dry-ipl.log", "ipl-import", path)
	return LogLines(res.Log, `create|update|\[WARN|\[ERROR`)
}
func (w *Workloader) IPLImport(path string) Result {
	return w.Run("ipl-import.log", "ipl-import", path, "--update-pce", "--no-prompt")
}

// hasProblem flags dry-run lines that mean rows were skipped or nothing would happen.
func hasProblem(lines []string) bool {
	for _, l := range lines {
		if strings.Contains(l, "cannot be blank") || strings.Contains(l, "[WARN") || strings.Contains(l, "[ERROR") || strings.Contains(l, "nothing to be done") {
			return true
		}
	}
	return false
}
