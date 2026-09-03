package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Settings are the few things umwl-tui remembers between runs. They never contain secrets (the PCE credentials
// live in workloader's pce.yaml). Two layers, the local one wins:
//
//	./umwl-tui.json                      — this working folder (one folder per Illumio account + PCE)
//	~/.config/umwl-tui/config.json       — this user, every working folder (os.UserConfigDir)
//
// Only keys that are set are written, so a local file can override a single key.
type Settings struct {
	Workloader string `json:"workloader,omitempty"` // path to the workloader binary (absolute or ~/…)
}

const LocalSettingsFile = "umwl-tui.json"

// UserSettingsPath is the per-user settings file ("" when the config dir is unknown).
func UserSettingsPath() string {
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		return ""
	}
	return filepath.Join(d, "umwl-tui", "config.json")
}

// SettingsPath returns the file a save with the given scope writes to.
func SettingsPath(user bool) string {
	if user {
		return UserSettingsPath()
	}
	abs, _ := filepath.Abs(LocalSettingsFile)
	return abs
}

func readSettings(path string) (Settings, bool) {
	var s Settings
	if path == "" {
		return s, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return s, false
	}
	if json.Unmarshal(b, &s) != nil {
		return s, false
	}
	return s, true
}

// LoadSettings merges the user file and the local file (local overrides). Source names the file that supplied
// the workloader path ("" when none does).
func LoadSettings() (s Settings, source string) {
	if u, ok := readSettings(UserSettingsPath()); ok {
		s = u
		if u.Workloader != "" {
			source = UserSettingsPath()
		}
	}
	if l, ok := readSettings(SettingsPath(false)); ok {
		if l.Workloader != "" {
			s.Workloader = l.Workloader
			source = SettingsPath(false)
		}
	}
	return s, source
}

// SaveSetting writes one key into the chosen layer, keeping the other keys of that file.
func SaveSettings(patch Settings, user bool) (string, error) {
	path := SettingsPath(user)
	if path == "" {
		return "", os.ErrNotExist
	}
	cur, _ := readSettings(path)
	if patch.Workloader != "" {
		cur.Workloader = patch.Workloader
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(cur, "", "  ")
	return path, os.WriteFile(path, append(b, '\n'), 0o644)
}

// ExpandHome turns ~/x into the absolute path under the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
