package engine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const Repo = "https://github.com/brian1917/workloader"

// Workloader wraps the binary. Every call is logged to RunDir/<log>.log through --log-file,
// and stdout/stderr lines are streamed to Out (the TUI's log pane) as they appear.
type Workloader struct {
	Bin    string
	PCE    string // --pce name, optional
	RunDir string
	Out    func(line string) // may be nil
	mu     sync.Mutex
	Calls  []Call
}

type Call struct {
	Cmd  []string  `json:"cmd"`
	RC   int       `json:"rc"`
	Log  string    `json:"log"`
	When time.Time `json:"when"`
}

type Result struct {
	RC     int
	Log    string
	Output []string
}

func (w *Workloader) emit(s string) {
	if w.Out != nil {
		w.Out(s)
	}
}

// Run executes workloader with args (+ --log-file, + --pce) and streams output.
func (w *Workloader) Run(logName string, args ...string) Result {
	log := filepath.Join(w.RunDir, logName)
	full := append([]string{}, args...)
	full = append(full, "--log-file", log)
	if w.PCE != "" {
		full = append(full, "--pce", w.PCE)
	}
	w.emit("$ " + filepath.Base(w.Bin) + " " + strings.Join(full, " "))
	cmd := exec.Command(w.Bin, full...)
	cmd.Dir, _ = os.Getwd()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	res := Result{Log: log}
	if err := cmd.Start(); err != nil {
		w.emit("✖ cannot start workloader: " + err.Error())
		res.RC = -1
		return res
	}
	var wg sync.WaitGroup
	var omu sync.Mutex
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			omu.Lock()
			res.Output = append(res.Output, line)
			omu.Unlock()
			w.emit("  " + line)
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.RC = ee.ExitCode()
		} else {
			res.RC = -1
		}
	}
	w.mu.Lock()
	w.Calls = append(w.Calls, Call{Cmd: append([]string{w.Bin}, full...), RC: res.RC, Log: log, When: time.Now()})
	w.mu.Unlock()
	if res.RC != 0 {
		w.emit(fmt.Sprintf("✖ workloader exited with code %d", res.RC))
	}
	return res
}

// LogLines returns the lines of a workloader log matching pattern.
func LogLines(path, pattern string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(pattern)
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimRight(l, "\r")
		if re.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// FindBinary looks for ./workloader, then PATH, then an explicit path. Returns "" if none.
func FindBinary(explicit string) string {
	cands := []string{}
	if explicit != "" {
		cands = append(cands, explicit)
	} else {
		cands = append(cands, "./workloader")
		if p, err := exec.LookPath("workloader"); err == nil {
			cands = append(cands, p)
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

// Version runs `workloader version` and returns its first line.
func Version(bin string) string {
	out, _ := exec.Command(bin, "version").CombinedOutput()
	s := strings.TrimSpace(string(out))
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// LatestTag resolves the latest release tag via the GitHub redirect (no API token needed).
func LatestTag() string {
	out, err := exec.Command("curl", "-sI", "-o", "/dev/null", "-w", "%{redirect_url}", Repo+"/releases/latest").Output()
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`/tag/(v[\d.]+)`).FindStringSubmatch(string(out))
	if m == nil {
		return ""
	}
	return m[1]
}

// DownloadRelease fetches the release zip for this OS, unzips it into cwd as ./workloader, strips
// the macOS quarantine flag. Progress/status lines go to out.
func DownloadRelease(tag string, out func(string)) (string, error) {
	if tag == "" {
		tag = LatestTag()
	}
	if tag == "" {
		return "", fmt.Errorf("could not resolve the latest release tag (no network?)")
	}
	var asset string
	switch runtime.GOOS {
	case "darwin":
		asset = "mac-" + tag + ".zip"
	case "linux":
		if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
			asset = "linux_arm-" + tag + ".zip"
		} else {
			asset = "linux_amd64-" + tag + ".zip"
		}
	case "windows":
		asset = "windows-" + tag + ".zip"
	default:
		return "", fmt.Errorf("unsupported OS %s", runtime.GOOS)
	}
	url := fmt.Sprintf("%s/releases/download/%s/%s", Repo, tag, asset)
	out("$ curl -fsSL -o " + asset + " " + url)
	if b, err := exec.Command("curl", "-fsSL", "-o", asset, url).CombinedOutput(); err != nil {
		return "", fmt.Errorf("download failed: %s %s", err, string(b))
	}
	out("$ unzip -o " + asset)
	if b, err := exec.Command("unzip", "-o", asset).CombinedOutput(); err != nil {
		return "", fmt.Errorf("unzip failed: %s %s", err, string(b))
	}
	os.Remove(asset)
	bin := "./workloader"
	if runtime.GOOS == "windows" {
		bin = "./workloader.exe"
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("zip did not contain %s", bin)
	}
	os.Chmod(bin, 0o755)
	if runtime.GOOS == "darwin" {
		out("$ xattr -d com.apple.quarantine workloader")
		exec.Command("xattr", "-d", "com.apple.quarantine", bin).Run()
	}
	abs, _ := filepath.Abs(bin)
	return abs, nil
}

// PCEStatus describes what pce-list reported.
type PCEStatus struct {
	ConfigPath string
	Configured bool   // at least one PCE in pce.yaml
	Listing    string // raw pce-list output (trimmed)
}

func ConfigPath() string {
	if p := os.Getenv("ILLUMIO_CONFIG"); p != "" {
		return p
	}
	return "./pce.yaml"
}

func (w *Workloader) PCEList() PCEStatus {
	res := w.Run("pce-list.log", "pce-list")
	st := PCEStatus{ConfigPath: ConfigPath(), Listing: strings.Join(res.Output, "\n")}
	joined := strings.ToLower(st.Listing)
	if fi, err := os.Stat(st.ConfigPath); err == nil && fi.Size() > 0 && res.RC == 0 && !strings.Contains(joined, "no pce configured") {
		st.Configured = true
	}
	return st
}

// PCEAdd runs pce-add --api-key non-interactively.
func (w *Workloader) PCEAdd(name, fqdn, port, apiUser, apiSecret, org, disableTLS string) Result {
	// never log the secret: run directly, not through Run()
	args := []string{"pce-add", "--api-key", "--name", name, "--fqdn", fqdn, "--port", port, "--api-user", apiUser, "--api-secret", apiSecret, "--org", org, "--disable-tls-verification", disableTLS}
	w.emit("$ workloader pce-add --api-key --name " + name + " --fqdn " + fqdn + " --port " + port + " --api-user " + apiUser + " --api-secret **** --org " + org)
	cmd := exec.Command(w.Bin, args...)
	b, err := cmd.CombinedOutput()
	res := Result{Output: strings.Split(strings.TrimSpace(string(b)), "\n")}
	for _, l := range res.Output {
		w.emit("  " + l)
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.RC = ee.ExitCode()
		} else {
			res.RC = -1
		}
	}
	return res
}

// ConnTest exports label dimensions as a cheap read-only connectivity check.
func (w *Workloader) ConnTest() bool {
	res := w.Run("conn-test.log", "label-dimension-export", "--output-file", filepath.Join(w.RunDir, "conn-test.csv"))
	return res.RC == 0
}
