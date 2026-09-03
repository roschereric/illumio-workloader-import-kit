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

// FindBinary resolves the workloader binary: --workloader flag, then the path saved in the umwl-tui settings
// (./umwl-tui.json, then ~/.config/umwl-tui/config.json), then ./workloader, then PATH. Returns "" if none.
func FindBinary(explicit string) string {
	cands := []string{}
	if explicit != "" {
		cands = append(cands, ExpandHome(explicit))
	} else {
		if st, _ := LoadSettings(); st.Workloader != "" {
			cands = append(cands, ExpandHome(st.Workloader))
		}
		cands = append(cands, "./workloader")
		if p, err := exec.LookPath("workloader"); err == nil {
			cands = append(cands, p)
		}
	}
	for _, c := range cands {
		if IsExecutable(c) {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

// IsExecutable is true for a regular file with an execute bit.
func IsExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
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

// LatestTag resolves the latest release tag: first the GitHub redirect of /releases/latest (no API token
// needed), then `git ls-remote --tags` (works where plain HTTPS to github.com is filtered but git is not).
func LatestTag() string {
	out, err := exec.Command("curl", "-sI", "-o", "/dev/null", "-w", "%{redirect_url}", Repo+"/releases/latest").Output()
	if err == nil {
		u := strings.TrimSpace(string(out))
		if i := strings.LastIndex(u, "/tag/"); i >= 0 && u[i+5:] != "" {
			return u[i+5:]
		}
	}
	if !HaveGit() {
		return ""
	}
	out, err = exec.Command("git", "ls-remote", "--tags", "--refs", Repo).Output()
	if err != nil {
		return ""
	}
	best, bestKey := "", []int{}
	for _, l := range strings.Split(string(out), "\n") {
		i := strings.LastIndex(l, "refs/tags/")
		if i < 0 {
			continue
		}
		tag := strings.TrimSpace(l[i+len("refs/tags/"):])
		key := semverKey(tag)
		if key == nil {
			continue
		}
		if best == "" || semverLess(bestKey, key) {
			best, bestKey = tag, key
		}
	}
	return best
}

// semverKey parses vX.Y.Z (a leading v optional) into comparable ints; nil when it is not a plain release tag.
func semverKey(tag string) []int {
	t := strings.TrimPrefix(tag, "v")
	parts := strings.Split(t, ".")
	if len(parts) != 3 {
		return nil
	}
	key := make([]int, 3)
	for i, p := range parts {
		n := 0
		if p == "" {
			return nil
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil
			}
			n = n*10 + int(c-'0')
		}
		key[i] = n
	}
	return key
}

func semverLess(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
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

// ---------------------------------------------------------------- build from source (preferred on Apple Silicon)

const SrcDir = "workloader-src"

// GoVersion returns the output of `go version` or "" when Go is not installed.
func GoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func HaveGit() bool  { _, err := exec.LookPath("git"); return err == nil }
func HaveBrew() bool { _, err := exec.LookPath("brew"); return err == nil }

// BinaryArch reports the CPU architecture a Mach-O or ELF binary was built for ("arm64", "amd64", "" unknown).
func BinaryArch(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	hdr := make([]byte, 20)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return ""
	}
	// Mach-O 64-bit little endian: magic 0xFEEDFACF, cputype at offset 4
	if hdr[0] == 0xCF && hdr[1] == 0xFA && hdr[2] == 0xED && hdr[3] == 0xFE {
		cpu := uint32(hdr[4]) | uint32(hdr[5])<<8 | uint32(hdr[6])<<16 | uint32(hdr[7])<<24
		switch cpu {
		case 0x0100000C:
			return "arm64"
		case 0x01000007:
			return "amd64"
		}
		return ""
	}
	// Mach-O universal (fat): magic 0xCAFEBABE big endian
	if hdr[0] == 0xCA && hdr[1] == 0xFE && hdr[2] == 0xBA && hdr[3] == 0xBE {
		return "universal"
	}
	// ELF: e_machine at offset 18 (little endian)
	if hdr[0] == 0x7F && hdr[1] == 'E' && hdr[2] == 'L' && hdr[3] == 'F' {
		m := uint16(hdr[18]) | uint16(hdr[19])<<8
		switch m {
		case 0x3E:
			return "amd64"
		case 0xB7:
			return "arm64"
		}
	}
	return ""
}

// NativeMismatch is true when the binary would run under emulation (Rosetta 2) on this machine.
func NativeMismatch(path string) bool {
	a := BinaryArch(path)
	return a != "" && a != "universal" && a != runtime.GOARCH
}

func run(out func(string), name string, args ...string) error {
	out("$ " + name + " " + strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			out("  " + strings.TrimRight(sc.Text(), "\r"))
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()
	return cmd.Wait()
}

// BuildFromSource clones (or updates) brian1917/workloader into ./workloader-src and builds a native
// ./workloader with the local Go toolchain (no Rosetta). tag "" = default branch head; otherwise a release tag.
func BuildFromSource(tag string, out func(string)) (string, error) {
	if GoVersion() == "" {
		return "", fmt.Errorf("Go is not installed (brew install go, or https://go.dev/dl)")
	}
	if !HaveGit() {
		return "", fmt.Errorf("git is not installed (xcode-select --install on macOS)")
	}
	if _, err := os.Stat(filepath.Join(SrcDir, ".git")); err == nil {
		if err := run(out, "git", "-C", SrcDir, "fetch", "--tags", "--quiet"); err != nil {
			return "", fmt.Errorf("git fetch failed: %w", err)
		}
		ref := "origin/HEAD"
		if tag != "" {
			ref = tag
		}
		if err := run(out, "git", "-C", SrcDir, "checkout", "--quiet", ref); err != nil {
			return "", fmt.Errorf("git checkout %s failed: %w", ref, err)
		}
		if tag == "" {
			run(out, "git", "-C", SrcDir, "pull", "--quiet", "--ff-only")
		}
	} else {
		args := []string{"clone", "--quiet", Repo, SrcDir}
		if tag != "" {
			args = []string{"clone", "--quiet", "--branch", tag, "--depth", "1", Repo, SrcDir}
		}
		if err := run(out, "git", args...); err != nil {
			return "", fmt.Errorf("git clone failed: %w", err)
		}
	}
	bin := "workloader"
	if runtime.GOOS == "windows" {
		bin = "workloader.exe"
	}
	abs, _ := filepath.Abs(bin)
	// same ldflags as the upstream release workflow (.github/workflows), otherwise `workloader version` prints nothing
	ver := ""
	if b, err := os.ReadFile(filepath.Join(SrcDir, "version")); err == nil {
		ver = strings.TrimSpace(string(b))
	}
	commit := ""
	if b, err := exec.Command("git", "-C", SrcDir, "rev-list", "-1", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(b))
	}
	ldflags := "-s -w -X github.com/brian1917/workloader/utils.Version=" + ver + " -X github.com/brian1917/workloader/utils.Commit=" + commit
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", abs, ".")
	cmd.Dir = SrcDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out("$ (cd " + SrcDir + " && CGO_ENABLED=0 go build -trimpath -ldflags '" + ldflags + "' -o " + abs + " .)")
	b, err := cmd.CombinedOutput()
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			out("  " + l)
		}
	}
	if err != nil {
		return "", fmt.Errorf("go build failed: %w", err)
	}
	os.Chmod(abs, 0o755)
	out("built " + abs + " " + ver + " for " + runtime.GOOS + "/" + runtime.GOARCH + " (" + BinaryArch(abs) + ")")
	return abs, nil
}

// InstallGoWithBrew runs `brew install go` (macOS/Linuxbrew). Returns an error when brew is absent.
func InstallGoWithBrew(out func(string)) error {
	if !HaveBrew() {
		return fmt.Errorf("Homebrew is not installed; install Go from https://go.dev/dl")
	}
	return run(out, "brew", "install", "go")
}
