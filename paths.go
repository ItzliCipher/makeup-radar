package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const appName = "Makeup Radar"

// Dirs are where Makeup Radar keeps its data on this Mac.
type Dirs struct {
	Data  string // ~/Library/Application Support/Makeup Radar
	Cache string // .../Cache
	Logs  string // ~/Library/Logs/Makeup Radar
}

func resolveDirs(override string) (Dirs, error) {
	var d Dirs
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return d, err
		}
		d = Dirs{Data: abs, Cache: filepath.Join(abs, "Cache"), Logs: filepath.Join(abs, "Logs")}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return d, err
		}
		if runtime.GOOS == "darwin" {
			d.Data = filepath.Join(home, "Library", "Application Support", appName)
			d.Logs = filepath.Join(home, "Library", "Logs", appName)
		} else {
			cfg, err := os.UserConfigDir()
			if err != nil {
				cfg = filepath.Join(home, ".config")
			}
			d.Data = filepath.Join(cfg, "makeup-radar")
			d.Logs = filepath.Join(d.Data, "logs")
		}
		d.Cache = filepath.Join(d.Data, "Cache")
	}
	for _, p := range []string{d.Data, d.Cache, d.Logs} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return d, err
		}
	}
	return d, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSONAtomic writes via a temp file and rename so a crash or power
// loss never leaves a half-written file behind.
func writeJSONAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// loadOrCreateToken returns the secret that ties the browser session to
// this Mac's Makeup Radar, creating it on first run.
func loadOrCreateToken(dir string) (string, error) {
	path := filepath.Join(dir, "session.key")
	if b, err := os.ReadFile(path); err == nil {
		t := strings.TrimSpace(string(b))
		if len(t) == 64 {
			return t, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	t := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(t), 0o600); err != nil {
		return "", err
	}
	return t, nil
}
