package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	defaultPort  = 47321
	cookieName   = "mr_session"
	idleShutdown = 25 * time.Minute
)

type serverInfo struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"startedAt"`
}

type Server struct {
	eng     *Engine
	store   *Store
	token   string
	port    int
	hosts   map[string]bool
	origins map[string]bool
	logf    func(string, ...any)

	lastHit atomic.Int64
	streams atomic.Int32
	quitMu  sync.Once
	quit    chan string

	// Website mode: a public, shared deployment.
	web       bool
	limiter   *ipLimiter
	searchSem chan struct{}
}

// ipLimiter is a per-visitor token bucket so one visitor can't flood the
// shared website (and, through it, the stores) with searches.
type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newIPLimiter(perMinute, burst float64) *ipLimiter {
	return &ipLimiter{buckets: map[string]*bucket{}, rate: perMinute / 60, burst: burst}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.buckets) > 20000 {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	b := l.buckets[ip]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[ip] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP is the visitor's address. Behind a hosting service's edge it
// comes from the edge's client-IP header, else the first X-Forwarded-For.
func (s *Server) clientIP(r *http.Request) string {
	if s.web {
		for _, hdr := range []string{"CF-Connecting-IP", "True-Client-IP"} {
			if ip := strings.TrimSpace(r.Header.Get(hdr)); ip != "" && net.ParseIP(ip) != nil {
				return ip
			}
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// webAllowed lists what visitors of the shared website may call: reading
// only. Settings, sources and saved items can't be changed by visitors
// (saved items and recent searches live in each visitor's browser).
func webAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	switch r.URL.Path {
	case "/api/health", "/api/search", "/api/product", "/api/sources":
		return true
	}
	return false
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

const csp = "default-src 'self'; img-src 'self' https: data:; style-src 'self'; script-src 'self'; connect-src 'self'; " +
	"font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

// middleware enforces that only this Mac's own browser session can use
// the app: the Host must be the local address (blocks DNS rebinding),
// API calls need the session cookie (SameSite=Strict, so other websites
// can't send it), and writes need a custom header and our Origin.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastHit.Store(time.Now().UnixNano())
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", csp)
		if s.web {
			h.Set("Strict-Transport-Security", "max-age=31536000")
			if strings.HasPrefix(r.URL.Path, "/api/") && !webAllowed(r) {
				writeErr(w, http.StatusForbidden, "That isn’t available on the shared website.")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !s.hosts[r.Host] {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				writeErr(w, http.StatusForbidden, "cross-site request refused")
				return
			}
			if r.URL.Path != "/api/health" && !s.validSession(r) {
				writeErr(w, http.StatusUnauthorized, "session")
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				if r.Header.Get("X-Requested-With") != "MakeupRadar" {
					writeErr(w, http.StatusForbidden, "missing request header")
					return
				}
				if o := r.Header.Get("Origin"); o != "" && !s.origins[o] {
					writeErr(w, http.StatusForbidden, "cross-origin request refused")
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	assets, _ := fs.Sub(webFiles, "web")
	files := http.FileServer(http.FS(assets))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") || r.URL.Path == "index.html" {
			http.NotFound(w, r)
			return
		}
		if s.web {
			w.Header().Set("Cache-Control", "public, max-age=300")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})))
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/icon.svg", http.StatusFound)
	})
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/product", s.handleProduct)
	mux.HandleFunc("GET /api/sources", s.handleSources)
	mux.HandleFunc("POST /api/sources/add", s.handleAddSource)
	mux.HandleFunc("POST /api/sources/toggle", s.handleToggleSource)
	mux.HandleFunc("POST /api/sources/remove", s.handleRemoveSource)
	mux.HandleFunc("POST /api/settings", s.handleSettings)
	mux.HandleFunc("GET /api/saved", s.handleSavedList)
	mux.HandleFunc("POST /api/saved", s.handleSavedAdd)
	mux.HandleFunc("DELETE /api/saved", s.handleSavedRemove)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("DELETE /api/history", s.handleHistoryClear)
	mux.HandleFunc("POST /api/ping", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("POST /api/quit", s.handleQuit)
	return s.middleware(mux)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if t := r.URL.Query().Get("t"); t != "" {
		if subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1 {
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: s.token, Path: "/", HttpOnly: true,
				SameSite: http.SameSiteStrictMode, MaxAge: 400 * 24 * 3600,
			})
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	b, err := fs.ReadFile(webFiles, "web/index.html")
	if err != nil {
		http.Error(w, "missing interface", http.StatusInternalServerError)
		return
	}
	html := strings.ReplaceAll(string(b), "{{VERSION}}", appVersion)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, html)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	mode := "local"
	if s.web {
		mode = "web"
	}
	resp := map[string]any{
		"ok": true, "app": appName, "version": appVersion,
		"session": s.web || s.validSession(r), "mode": mode,
	}
	if !s.web {
		resp["pid"] = os.Getpid() // lets the Mac launcher confirm it found its own service
	}
	writeJSON(w, 200, resp)
}

func oneOf(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return allowed[0]
}

func clampInt(v string, lo, hi, def int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return max(lo, min(hi, n))
}

func limitString(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	qv := r.URL.Query()
	sp := SearchParams{
		Q:        limitString(qv.Get("q"), 160),
		Sort:     oneOf(qv.Get("sort"), "top", "new", "advanced", "price"),
		Days:     clampInt(qv.Get("days"), 7, 365, 90),
		Cat:      oneOf(qv.Get("cat"), "all", CatFace, CatCheeks, CatEyes, CatBrows, CatLips, CatTools, CatSets, CatOther),
		Brand:    limitString(qv.Get("brand"), 80),
		HideSets: qv.Get("hideSets") == "1",
		Fresh:    qv.Get("fresh") == "1",
		Rescan:   qv.Get("rescan") == "1",
	}
	if s.web {
		// Visitors can't force full rescans; catalogs refresh on their own
		// schedule. Each visitor picks their own news edition.
		sp.Rescan = false
		if code := limitString(qv.Get("region"), 4); code != "" && regionByCode(code).Code == code {
			sp.Region = code
		}
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	s.streams.Add(1)
	defer s.streams.Add(-1)
	if s.web {
		problem := func(code, msg string) {
			b, _ := json.Marshal(map[string]string{"code": code, "message": msg})
			fmt.Fprintf(w, "event: problem\ndata: %s\n\n", b)
			fl.Flush()
		}
		if !s.limiter.allow(s.clientIP(r)) {
			problem("rate", "You’re searching very quickly. Wait a few seconds, then try again.")
			return
		}
		select {
		case s.searchSem <- struct{}{}:
			defer func() { <-s.searchSem }()
		case <-time.After(20 * time.Second):
			problem("busy", "Makeup Radar is busy with other searches. Try again in a moment.")
			return
		case <-r.Context().Done():
			return
		}
	}
	var mu sync.Mutex
	closed := false
	emit := func(event string, v any) {
		b, err := json.Marshal(v)
		if err != nil {
			s.logf("encode %s: %v", event, err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		fl.Flush()
	}
	ctx := r.Context()
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				mu.Lock()
				if !closed {
					fmt.Fprint(w, ": keep-alive\n\n")
					fl.Flush()
				}
				mu.Unlock()
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	s.eng.Search(ctx, sp, emit)
	close(stop)
	mu.Lock()
	closed = true
	mu.Unlock()
}

func (s *Server) handleProduct(w http.ResponseWriter, r *http.Request) {
	d := s.eng.ProductDetail(limitString(r.URL.Query().Get("id"), 120))
	if d == nil {
		writeErr(w, 404, "That product is no longer in the latest catalog check.")
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.eng.Sources())
}

func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return errors.New("couldn't read the request")
	}
	return nil
}

func (s *Server) handleAddSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st, err := s.eng.AddBrand(r.Context(), limitString(body.URL, 300))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleToggleSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.eng.SetEnabled(body.ID, body.Enabled); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleRemoveSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.eng.RemoveBrand(body.ID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Region     *string `json:"region"`
		NewsGoogle *bool   `json:"newsGoogle"`
		NewsBing   *bool   `json:"newsBing"`
		NewsPress  *bool   `json:"newsPress"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	set, err := s.store.UpdateSettings(body.Region, body.NewsGoogle, body.NewsBing, body.NewsPress)
	if err != nil {
		writeErr(w, 500, "couldn't save settings")
		return
	}
	writeJSON(w, 200, set)
}

func (s *Server) handleSavedList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"items": s.store.Saved()})
}

func (s *Server) handleSavedAdd(w http.ResponseWriter, r *http.Request) {
	var p ProductCard
	if err := decodeBody(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.store.AddSaved(p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleSavedRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RemoveSaved(limitString(r.URL.Query().Get("id"), 120)); err != nil {
		writeErr(w, 500, "couldn't update saved products")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"items": s.store.History()})
}

func (s *Server) handleHistoryClear(w http.ResponseWriter, r *http.Request) {
	s.store.ClearHistory()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]bool{"ok": true})
	go func() {
		time.Sleep(250 * time.Millisecond)
		s.requestQuit("quit from app")
	}()
}

func (s *Server) requestQuit(reason string) {
	s.quitMu.Do(func() { s.quit <- reason })
}

type serveOptions struct {
	Port      int
	IdleExit  bool
	Web       bool              // public website mode
	Transport http.RoundTripper // tests substitute fixture upstreams
}

// runServer is the long-running background process behind the app.
func runServer(dirs Dirs, opt serveOptions) error {
	lockFile, err := os.OpenFile(filepath.Join(dirs.Data, "server.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another Makeup Radar is already running")
	}

	var logFile *os.File
	if !opt.Web {
		logFile, _ = os.OpenFile(filepath.Join(dirs.Logs, "radar.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	}
	if logFile != nil {
		if st, err := logFile.Stat(); err == nil && st.Size() > 4<<20 {
			_ = logFile.Truncate(0)
		}
		defer logFile.Close()
	}
	var logOut io.Writer = os.Stderr
	if logFile != nil {
		logOut = io.MultiWriter(os.Stderr, logFile)
	}
	logger := log.New(logOut, "", log.LstdFlags)
	logf := logger.Printf

	token, err := loadOrCreateToken(dirs.Data)
	if err != nil {
		return err
	}

	port := opt.Port
	if port == 0 {
		port = defaultPort
	}
	var ln net.Listener
	if opt.Web {
		// Hosting services route public traffic to this port.
		ln, err = net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			return err
		}
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			ln, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
		}
	}
	port = ln.Addr().(*net.TCPAddr).Port

	client := newHTTPClient()
	if opt.Transport != nil {
		client.Transport = opt.Transport
	}
	store := NewStore(dirs.Data)
	eng := NewEngine(store, client, filepath.Join(dirs.Cache, "catalogs"), logf)
	if opt.Web {
		eng.web = true
		eng.freshMinAge = 5 * time.Minute // shared by everyone: don't refetch news per click
		eng.resultLimit = 120
	}

	s := &Server{
		eng: eng, store: store, token: token, port: port, logf: logf, quit: make(chan string, 1),
		hosts:   map[string]bool{fmt.Sprintf("127.0.0.1:%d", port): true, fmt.Sprintf("localhost:%d", port): true},
		origins: map[string]bool{fmt.Sprintf("http://127.0.0.1:%d", port): true, fmt.Sprintf("http://localhost:%d", port): true},
		web:     opt.Web,
	}
	if opt.Web {
		s.limiter = newIPLimiter(30, 30)
		s.searchSem = make(chan struct{}, 12)
	}
	s.lastHit.Store(time.Now().UnixNano())

	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	if !opt.Web {
		info := serverInfo{PID: os.Getpid(), Port: port, Version: appVersion, StartedAt: time.Now()}
		if err := writeJSONAtomic(filepath.Join(dirs.Data, "server.json"), info); err != nil {
			return err
		}
		defer func() {
			var cur serverInfo
			if readJSON(filepath.Join(dirs.Data, "server.json"), &cur) == nil && cur.PID == os.Getpid() {
				_ = os.Remove(filepath.Join(dirs.Data, "server.json"))
			}
		}()
		logf("Makeup Radar %s listening on http://127.0.0.1:%d", appVersion, port)
	} else {
		logf("Makeup Radar %s website listening on port %d", appVersion, port)
	}
	eng.Prewarm()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-sigs
		s.requestQuit("signal " + sig.String())
	}()
	if opt.IdleExit && !opt.Web {
		go func() {
			t := time.NewTicker(time.Minute)
			defer t.Stop()
			for range t.C {
				last := time.Unix(0, s.lastHit.Load())
				if s.streams.Load() == 0 && time.Since(last) > idleShutdown {
					s.requestQuit("idle")
					return
				}
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case reason := <-s.quit:
		logf("shutting down: %s", reason)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	return nil
}
