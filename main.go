// Makeup Radar finds the newest, most advanced makeup by scanning brand
// catalogs and the beauty press live, every time you search.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const appVersion = "1.0.0"

func main() {
	var args []string
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "-psn_") { // passed by older macOS launchers
			continue
		}
		args = append(args, a)
	}
	fs := flag.NewFlagSet("makeup-radar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	serve := fs.Bool("serve", false, "run the background search service")
	web := fs.Bool("web", false, "run as a public website (for hosting services)")
	dataDir := fs.String("data-dir", "", "use a different data folder")
	port := fs.Int("port", 0, "preferred local port")
	noIdle := fs.Bool("no-idle-exit", false, "keep running when idle")
	noBrowser := fs.Bool("no-browser", false, "print the address instead of opening the browser")
	showVersion := fs.Bool("version", false, "print the version")
	_ = fs.Parse(args)

	if *showVersion {
		fmt.Println(appName, appVersion)
		return
	}
	if *web || os.Getenv("MAKEUP_RADAR_WEB") == "1" {
		runWebsite(*dataDir, *port)
		return
	}
	dirs, err := resolveDirs(*dataDir)
	if err != nil {
		showError("Makeup Radar couldn't create its data folder.", err.Error())
		os.Exit(1)
	}
	if *serve {
		if err := runServer(dirs, serveOptions{Port: *port, IdleExit: !*noIdle, Transport: testTransport()}); err != nil {
			fmt.Fprintln(os.Stderr, "makeup radar:", err)
			os.Exit(1)
		}
		return
	}
	if err := runLauncher(dirs, launcherOptions{NoBrowser: *noBrowser, Port: *port, DataDir: *dataDir}); err != nil {
		showError("Makeup Radar couldn't open.", err.Error())
		os.Exit(1)
	}
}

// runWebsite serves Makeup Radar as a public website. Hosting services
// pass the port in $PORT; data is a disposable cache in the temp folder.
func runWebsite(dataDir string, port int) {
	if port == 0 {
		if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil && p > 0 {
			port = p
		} else {
			port = 8080
		}
	}
	if dataDir == "" {
		dataDir = filepath.Join(os.TempDir(), "makeup-radar-web")
	}
	dirs, err := resolveDirs(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "makeup radar:", err)
		os.Exit(1)
	}
	if err := runServer(dirs, serveOptions{Port: port, Web: true, Transport: testTransport()}); err != nil {
		fmt.Fprintln(os.Stderr, "makeup radar:", err)
		os.Exit(1)
	}
}
