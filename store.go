package main

import (
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Settings are the user's choices, kept in settings.json.
type Settings struct {
	Version      int      `json:"version"`
	CustomBrands []Brand  `json:"customBrands"`
	Disabled     []string `json:"disabled"`
	Region       string   `json:"region"`
	NewsGoogle   bool     `json:"newsGoogle"`
	NewsBing     bool     `json:"newsBing"`
	NewsPress    bool     `json:"newsPress"`
}

type SavedItem struct {
	Product ProductCard `json:"product"`
	SavedAt time.Time   `json:"savedAt"`
}

type HistoryEntry struct {
	Q  string    `json:"q"`
	At time.Time `json:"at"`
}

// Store persists settings, saved products and recent searches.
type Store struct {
	mu       sync.Mutex
	dir      string
	settings Settings
	saved    map[string]SavedItem
	history  []HistoryEntry
}

func NewStore(dir string) *Store {
	s := &Store{dir: dir, saved: map[string]SavedItem{}}
	if err := readJSON(s.path("settings.json"), &s.settings); err != nil || s.settings.Version == 0 {
		s.settings = Settings{Version: 1, Region: "US", NewsGoogle: true, NewsBing: true, NewsPress: true}
	}
	var saved []SavedItem
	if readJSON(s.path("saved.json"), &saved) == nil {
		for _, it := range saved {
			s.saved[it.Product.ID] = it
		}
	}
	_ = readJSON(s.path("history.json"), &s.history)
	return s
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.settings
	c.CustomBrands = append([]Brand(nil), s.settings.CustomBrands...)
	c.Disabled = append([]string(nil), s.settings.Disabled...)
	return c
}

func (s *Store) saveSettingsLocked() error {
	return writeJSONAtomic(s.path("settings.json"), s.settings)
}

func (s *Store) UpdateSettings(region *string, google, bing, press *bool) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if region != nil {
		s.settings.Region = regionByCode(*region).Code
	}
	if google != nil {
		s.settings.NewsGoogle = *google
	}
	if bing != nil {
		s.settings.NewsBing = *bing
	}
	if press != nil {
		s.settings.NewsPress = *press
	}
	return s.settings, s.saveSettingsLocked()
}

// AllBrands is the built-in list plus brands the user added.
func (s *Store) AllBrands() []Brand {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Brand(nil), builtinBrands...)
	return append(out, s.settings.CustomBrands...)
}

func (s *Store) DisabledSet() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]bool{}
	for _, id := range s.settings.Disabled {
		m[id] = true
	}
	return m
}

func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, b := range builtinBrands {
		if b.ID == id {
			found = true
		}
	}
	for _, b := range s.settings.CustomBrands {
		if b.ID == id {
			found = true
		}
	}
	if !found {
		return errors.New("unknown source")
	}
	out := s.settings.Disabled[:0]
	for _, d := range s.settings.Disabled {
		if d != id {
			out = append(out, d)
		}
	}
	if !enabled {
		out = append(out, id)
	}
	s.settings.Disabled = out
	return s.saveSettingsLocked()
}

var reSlug = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Store) AddCustomBrand(name, domain string) (Brand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range append(append([]Brand(nil), builtinBrands...), s.settings.CustomBrands...) {
		if strings.TrimPrefix(b.Domain, "www.") == strings.TrimPrefix(domain, "www.") {
			return b, errors.New(b.Name + " is already one of your sources")
		}
	}
	id := "custom-" + strings.Trim(reSlug.ReplaceAllString(strings.ToLower(strings.TrimPrefix(domain, "www.")), "-"), "-")
	b := Brand{ID: id, Name: name, Domain: domain, Kind: "brand", Focus: "makeup", Currency: "", Custom: true, Aliases: []string{normalize(name)}}
	s.settings.CustomBrands = append(s.settings.CustomBrands, b)
	return b, s.saveSettingsLocked()
}

func (s *Store) RemoveCustomBrand(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.settings.CustomBrands[:0]
	found := false
	for _, b := range s.settings.CustomBrands {
		if b.ID == id {
			found = true
			continue
		}
		out = append(out, b)
	}
	if !found {
		return errors.New("only brands you added can be removed")
	}
	s.settings.CustomBrands = out
	return s.saveSettingsLocked()
}

func (s *Store) Saved() []SavedItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SavedItem, 0, len(s.saved))
	for _, it := range s.saved {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	return out
}

func (s *Store) saveSavedLocked() error {
	list := make([]SavedItem, 0, len(s.saved))
	for _, it := range s.saved {
		list = append(list, it)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].SavedAt.After(list[j].SavedAt) })
	return writeJSONAtomic(s.path("saved.json"), list)
}

func (s *Store) AddSaved(p ProductCard) error {
	if p.ID == "" || p.Title == "" || !strings.HasPrefix(p.URL, "https://") {
		return errors.New("that product can't be saved")
	}
	if p.Image != "" && !strings.HasPrefix(p.Image, "https://") {
		p.Image = ""
	}
	p.InNews = nil
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.saved) >= 500 {
		return errors.New("you've saved 500 products; remove some to save more")
	}
	s.saved[p.ID] = SavedItem{Product: p, SavedAt: time.Now()}
	return s.saveSavedLocked()
}

func (s *Store) RemoveSaved(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.saved, id)
	return s.saveSavedLocked()
}

func (s *Store) AddHistory(q string) {
	q = strings.TrimSpace(q)
	if q == "" || len(q) > 120 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []HistoryEntry{{Q: q, At: time.Now()}}
	for _, h := range s.history {
		if !strings.EqualFold(h.Q, q) && len(out) < 12 {
			out = append(out, h)
		}
	}
	s.history = out
	_ = writeJSONAtomic(s.path("history.json"), s.history)
}

func (s *Store) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]HistoryEntry(nil), s.history...)
}

func (s *Store) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	_ = writeJSONAtomic(s.path("history.json"), []HistoryEntry{})
}
