package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type launcherOptions struct {
	NoBrowser bool
	Port      int
	DataDir   string
}

var localClient = &http.Client{
	Timeout:   1500 * time.Millisecond,
	Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
}

// runningServer finds a healthy Makeup Radar service started earlier.
func runningServer(dirs Dirs) (serverInfo, bool) {
	var info serverInfo
	if readJSON(filepath.Join(dirs.Data, "server.json"), &info) != nil || info.Port == 0 {
		return info, false
	}
	resp, err := localClient.Get("http://127.0.0.1:" + strconv.Itoa(info.Port) + "/api/health")
	if err != nil {
		return info, false
	}
	defer resp.Body.Close()
	var h struct {
		OK      bool   `json:"ok"`
		App     string `json:"app"`
		Version string `json:"version"`
		PID     int    `json:"pid"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&h) != nil || !h.OK || h.App != appName {
		return info, false
	}
	info.Version, info.PID = h.Version, h.PID
	return info, true
}

func askServerToQuit(info serverInfo, token string) {
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(info.Port)+"/api/quit", strings.NewReader("{}"))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "MakeupRadar")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	if resp, err := localClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

// runLauncher is what runs when you open the app: it starts the
// background service if needed and opens the interface in your browser.
func runLauncher(dirs Dirs, o launcherOptions) error {
	token, err := loadOrCreateToken(dirs.Data)
	if err != nil {
		return err
	}
	info, ok := runningServer(dirs)
	if ok && info.Version != appVersion {
		// An older version is still running after an update.
		askServerToQuit(info, token)
		for i := 0; i < 30; i++ {
			time.Sleep(150 * time.Millisecond)
			if _, still := runningServer(dirs); !still {
				break
			}
		}
		ok = false
	}
	if !ok {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		args := []string{"--serve"}
		if o.DataDir != "" {
			args = append(args, "--data-dir", dirs.Data)
		}
		if o.Port != 0 {
			args = append(args, "--port", strconv.Itoa(o.Port))
		}
		cmd := exec.Command(exe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0); err == nil {
			cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
			defer devnull.Close()
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("couldn't start the search service: %w", err)
		}
		_ = cmd.Process.Release()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(120 * time.Millisecond)
			if inf, up := runningServer(dirs); up {
				info, ok = inf, true
				break
			}
		}
		if !ok {
			return errors.New("the search service didn't start in time. Please try opening Makeup Radar again")
		}
	}
	u := "http://127.0.0.1:" + strconv.Itoa(info.Port) + "/?t=" + token
	if o.NoBrowser {
		fmt.Println(u)
		return nil
	}
	return openBrowser(u)
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("/usr/bin/open", u).Run()
	case "linux":
		if p, err := exec.LookPath("xdg-open"); err == nil {
			return exec.Command(p, u).Start()
		}
	}
	fmt.Println(u)
	return nil
}

// showError tells the user what went wrong in a native dialog on macOS.
func showError(title, detail string) {
	fmt.Fprintln(os.Stderr, title, detail)
	if runtime.GOOS != "darwin" {
		return
	}
	esc := func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		return strings.ReplaceAll(s, `"`, `\"`)
	}
	script := `display alert "` + esc(title) + `" message "` + esc(detail) + `" as critical`
	_ = exec.Command("/usr/bin/osascript", "-e", script).Run()
}
