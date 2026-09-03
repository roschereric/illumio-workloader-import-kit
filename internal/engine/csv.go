// Package engine holds everything the TUI needs that is not presentation:
// CSV contract, workloader wrapper, PCE inventory index, reconciliation by IP,
// chunked execution and the run report. No terminal I/O happens here.
package engine

import (
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Row is one proposed unmanaged workload (one IP per row) plus bookkeeping fields.
type Row struct {
	Fields  map[string]string // every CSV column as read/edited
	IPs     []string          // parsed from "interfaces"
	Review  string            // NEW / EXISTS-UNMANAGED / CONFLICT-* / SKIPPED-* / UPDATE-*
	Href    string            // set when updating an existing workload
	Match   *PCEWorkload      // first matching PCE object, if any
	Matches []*PCEWorkload
	Line    int // 1-based CSV line for messages
}

func (r *Row) Get(k string) string { return r.Fields[k] }
func (r *Row) Set(k, v string)     { r.Fields[k] = v }
func (r *Row) IP() string {
	if len(r.IPs) > 0 {
		return r.IPs[0]
	}
	return ""
}

// CSVFile is a loaded proposed-workloads CSV.
type CSVFile struct {
	Path      string
	Header    []string
	Rows      []*Row
	LabelCols []string // header columns that are label keys (everything not reserved)
	Warnings  []string
	Dropped   []string
}

var reserved = map[string]bool{"hostname": true, "name": true, "interfaces": true, "description": true, "review": true, "href": true,
	"public_ip": true, "os_id": true, "os_detail": true, "data_center": true, "external_data_set": true, "external_data_reference": true}

var prioRe = regexp.MustCompile(`\bP(\d)\b`)

// ParseIPs accepts "eth0:10.1.1.1;umwl:10.1.1.2", "10.1.1.1/24", comma/space separated.
func ParseIPs(s string) []string {
	var out []string
	for _, tok := range regexp.MustCompile(`[;,\s]+`).Split(s, -1) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.Count(tok, ":") == 1 {
			tok = tok[strings.Index(tok, ":")+1:]
		}
		if i := strings.Index(tok, "/"); i >= 0 {
			tok = tok[:i]
		}
		if net.ParseIP(tok) != nil {
			out = append(out, tok)
		}
	}
	return out
}

func readCSV(path string) ([]string, []map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(recs) == 0 {
		return nil, nil, fmt.Errorf("empty file")
	}
	hdr := recs[0]
	if len(hdr) > 0 {
		hdr[0] = strings.TrimPrefix(hdr[0], "\ufeff")
	}
	var rows []map[string]string
	for _, rec := range recs[1:] {
		m := map[string]string{}
		for i, h := range hdr {
			if i < len(rec) {
				m[h] = rec[i]
			} else {
				m[h] = ""
			}
		}
		rows = append(rows, m)
	}
	return hdr, rows, nil
}

// LoadCSV validates the proposed CSV: required columns, IPs, duplicate IPs, duplicate hostnames/names,
// empty label columns, optional priority filter ("1" or "1,2").
func LoadCSV(path, priority string) (*CSVFile, error) {
	hdr, recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, h := range hdr {
		have[h] = true
	}
	for _, req := range []string{"hostname", "name", "interfaces"} {
		if !have[req] {
			return nil, fmt.Errorf("missing required column %q", req)
		}
	}
	cf := &CSVFile{Path: path, Header: hdr}
	for _, h := range hdr {
		if !reserved[h] && h != "" {
			cf.LabelCols = append(cf.LabelCols, h)
		}
	}
	keep := map[string]bool{}
	if priority != "" {
		for _, p := range strings.Split(priority, ",") {
			keep[strings.TrimSpace(p)] = true
		}
	}
	seenIP := map[string]int{}
	before := 0
	for i, rec := range recs {
		before++
		if len(keep) > 0 {
			m := prioRe.FindStringSubmatch(rec["description"])
			if m == nil || !keep[m[1]] {
				continue
			}
		}
		row := &Row{Fields: rec, IPs: ParseIPs(rec["interfaces"]), Line: i + 2}
		if len(row.IPs) == 0 {
			cf.Dropped = append(cf.Dropped, fmt.Sprintf("line %d (%s): no valid IP in interfaces=%q", row.Line, rec["name"], rec["interfaces"]))
			continue
		}
		if strings.TrimSpace(rec["hostname"]) == "" && strings.TrimSpace(rec["name"]) == "" {
			cf.Dropped = append(cf.Dropped, fmt.Sprintf("line %d: hostname and name both empty", row.Line))
			continue
		}
		if n, dup := seenIP[row.IP()]; dup {
			cf.Dropped = append(cf.Dropped, fmt.Sprintf("line %d: IP %s already on line %d (first one wins)", row.Line, row.IP(), n))
			continue
		}
		seenIP[row.IP()] = row.Line
		cf.Rows = append(cf.Rows, row)
	}
	if len(keep) > 0 {
		cf.Warnings = append(cf.Warnings, fmt.Sprintf("priority filter %v: %d → %d rows", sortedKeys(keep), before, len(cf.Rows)))
	}
	// empty label columns are ignored
	var cols []string
	for _, k := range cf.LabelCols {
		used := false
		for _, r := range cf.Rows {
			if strings.TrimSpace(r.Get(k)) != "" {
				used = true
				break
			}
		}
		if used {
			cols = append(cols, k)
		} else {
			cf.Warnings = append(cf.Warnings, fmt.Sprintf("label column %q is empty in every row: ignored", k))
		}
	}
	cf.LabelCols = cols
	// blank hostnames → workloader matches on name; names must then be unique
	blank := 0
	nameCount := map[string]int{}
	hostCount := map[string]int{}
	for _, r := range cf.Rows {
		if strings.TrimSpace(r.Get("hostname")) == "" {
			blank++
			nameCount[r.Get("name")]++
		} else {
			hostCount[r.Get("hostname")]++
		}
	}
	if blank > 0 {
		cf.Warnings = append(cf.Warnings, fmt.Sprintf("%d rows without hostname: workloader identifies them by name (convention \"Role IP\")", blank))
	}
	for _, r := range cf.Rows {
		if strings.TrimSpace(r.Get("hostname")) == "" && nameCount[r.Get("name")] > 1 {
			cf.Warnings = append(cf.Warnings, fmt.Sprintf("duplicate name %q: IP appended to keep rows distinct", r.Get("name")))
			r.Set("name", r.Get("name")+" "+r.IP())
		} else if r.Get("hostname") != "" && hostCount[r.Get("hostname")] > 1 {
			cf.Warnings = append(cf.Warnings, fmt.Sprintf("duplicate hostname %q: IP appended as suffix", r.Get("hostname")))
			r.Set("hostname", r.Get("hostname")+"--"+r.IP())
		}
	}
	return cf, nil
}

// LabelCounts returns value→count for one label column.
func (c *CSVFile) LabelCounts(col string) []KV {
	m := map[string]int{}
	for _, r := range c.Rows {
		if v := r.Get(col); v != "" {
			m[v]++
		}
	}
	var out []KV
	for k, v := range m {
		out = append(out, KV{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].K < out[j].K
	})
	return out
}

type KV struct {
	K string
	N int
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WriteCSV writes rows with the given columns (missing values → "").
func WriteCSV(path string, cols []string, rows []*Row) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(cols); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			switch c {
			case "href":
				rec[i] = r.Href
			case "review":
				rec[i] = r.Review
			default:
				rec[i] = r.Fields[c]
			}
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// CountRows counts data rows of any CSV (for the IPL file).
func CountRows(path string) int {
	_, recs, err := readCSV(path)
	if err != nil {
		return 0
	}
	return len(recs)
}
