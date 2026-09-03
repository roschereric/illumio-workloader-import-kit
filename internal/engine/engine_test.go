package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIPs(t *testing.T) {
	got := ParseIPs("eth0:10.1.1.1;umwl:10.1.1.2, 10.1.1.3/24 not-an-ip 2001:db8::1")
	want := []string{"10.1.1.1", "10.1.1.2", "10.1.1.3", "2001:db8::1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

// Security: CSV cells with shell metacharacters must round-trip untouched into the import files —
// they are data, never part of a command line.
func TestCSVRoundTripHostileCells(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.csv")
	hostile := "$(id); `uname -a` | rm -rf / && echo \"quoted\""
	os.WriteFile(in, []byte("hostname,name,interfaces,description,role,app,env,loc,review\n"+
		",\""+strings.ReplaceAll(hostile, "\"", "\"\"")+" 10.0.0.1\",eth0:10.0.0.1,\"[C1 P1 conf:Alta] "+strings.ReplaceAll(hostile, "\"", "\"\"")+"\",R_X,,E_Prod,L_CDLV,PENDING\n"), 0o644)
	cf, err := LoadCSV(in, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cf.Rows) != 1 || cf.Rows[0].IP() != "10.0.0.1" {
		t.Fatalf("rows %+v", cf.Rows)
	}
	out := filepath.Join(dir, "out.csv")
	if err := WriteCSV(out, []string{"hostname", "name", "interfaces", "description", "role", "app", "env", "loc"}, cf.Rows); err != nil {
		t.Fatal(err)
	}
	_, recs, err := readCSV(out)
	if err != nil {
		t.Fatal(err)
	}
	if recs[0]["name"] != hostile+" 10.0.0.1" || recs[0]["description"] != "[C1 P1 conf:Alta] "+hostile {
		t.Fatalf("cells altered: %+v", recs[0])
	}
	if len(cf.LabelCols) != 3 { // app is empty everywhere → ignored
		t.Fatalf("label cols %v", cf.LabelCols)
	}
}

func TestLoadCSVDuplicatesAndPriority(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.csv")
	os.WriteFile(in, []byte("hostname,name,interfaces,description,role\n"+
		",Zabbix 10.0.0.1,eth0:10.0.0.1,[C1 P1 conf:Alta] a,R_M\n"+
		",Zabbix 10.0.0.1,eth0:10.0.0.2,[C1 P2 conf:Alta] b,R_M\n"+
		",Dup IP,eth0:10.0.0.1,[C1 P1 conf:Alta] c,R_M\n"+
		",No IP,eth0:nope,[C1 P1 conf:Alta] d,R_M\n"), 0o644)
	cf, err := LoadCSV(in, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cf.Rows) != 2 || len(cf.Dropped) != 2 {
		t.Fatalf("rows %d dropped %v", len(cf.Rows), cf.Dropped)
	}
	if cf.Rows[1].Get("name") != "Zabbix 10.0.0.1 10.0.0.2" {
		t.Fatalf("duplicate name not suffixed: %q", cf.Rows[1].Get("name"))
	}
	cf, _ = LoadCSV(in, "1")
	if len(cf.Rows) != 1 {
		t.Fatalf("priority filter: %d rows", len(cf.Rows))
	}
}

// Security: the API secret must never appear in what the wrapper emits to the UI/log.
func TestPCEAddMasksSecret(t *testing.T) {
	dir := t.TempDir()
	mock := filepath.Join(dir, "wl.sh")
	os.WriteFile(mock, []byte("#!/bin/sh\necho \"args: $*\"\n"), 0o755)
	var emitted []string
	w := &Workloader{Bin: mock, RunDir: dir, Out: func(s string) { emitted = append(emitted, s) }}
	res := w.PCEAdd("poc", "x.illum.io", "443", "api_user", "S3CR3T-VALUE", "12", "false")
	if res.RC != 0 {
		t.Fatalf("rc %d", res.RC)
	}
	// the mock echoes its argv: that line is workloader's own output, which the wrapper relays.
	// The wrapper's own command echo (first emitted line) must be masked.
	if strings.Contains(emitted[0], "S3CR3T") {
		t.Fatalf("secret leaked in command echo: %s", emitted[0])
	}
	if !strings.Contains(emitted[0], "--api-secret ****") {
		t.Fatalf("expected masked echo, got %s", emitted[0])
	}
	for _, c := range w.Calls {
		for _, a := range c.Cmd {
			if strings.Contains(a, "S3CR3T") {
				t.Fatal("secret recorded in Calls")
			}
		}
	}
}

func TestClassifyAndApply(t *testing.T) {
	inv := &Inventory{ByIP: map[string][]*PCEWorkload{}, Labels: map[string]bool{"role=R_DNS": true}, LabelKeys: []string{"role", "app"}}
	umwl := &PCEWorkload{Href: "/w/1", Hostname: "dns1", Name: "DNS 1", Labels: map[string]string{"role": "R_DNS"}}
	ven := &PCEWorkload{Href: "/w/2", Hostname: "srv", Managed: true, Labels: map[string]string{}}
	inv.ByIP["10.0.0.1"] = []*PCEWorkload{umwl}
	inv.ByIP["10.0.0.2"] = []*PCEWorkload{ven}
	inv.ByIP["10.0.0.3"] = []*PCEWorkload{umwl, ven}
	mk := func(ip, role string) *Row {
		return &Row{Fields: map[string]string{"hostname": "", "name": "X " + ip, "interfaces": "eth0:" + ip, "role": role}, IPs: []string{ip}}
	}
	cf := &CSVFile{LabelCols: []string{"role", "os"}, Rows: []*Row{mk("10.0.0.1", "R_DNS"), mk("10.0.0.2", "R_X"), mk("10.0.0.3", "R_X"), mk("10.0.0.4", "R_NEW")}}
	plan := PlanLabels(cf, inv)
	if len(plan.UnknownKeys) != 1 || plan.UnknownKeys[0] != "os" {
		t.Fatalf("unknown keys %v", plan.UnknownKeys)
	}
	if len(plan.NewValues) != 2 { // R_X, R_NEW
		t.Fatalf("new values %v", plan.NewValues)
	}
	Classify(cf, inv)
	want := []string{"EXISTS-UNMANAGED", "CONFLICT-MANAGED", "CONFLICT-MANAGED", "NEW"}
	for i, r := range cf.Rows {
		if r.Review != want[i] {
			t.Fatalf("row %d: %s want %s", i, r.Review, want[i])
		}
	}
	Apply(cf.Rows[0], DecUpdate, "")
	r := cf.Rows[0]
	if r.Href != "/w/1" || r.Get("interfaces") != "" || r.Get("hostname") != "dns1" || r.Get("name") != "DNS 1" || r.Review != "UPDATE-EXISTS-UNMANAGED" {
		t.Fatalf("apply update: %+v href=%s", r.Fields, r.Href)
	}
	Apply(cf.Rows[1], DecSkip, "")
	b := Split(cf)
	if len(b.Update) != 1 || len(b.Skipped) != 1 || len(b.Create) != 1 {
		t.Fatalf("split %+v", b)
	}
}
