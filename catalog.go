package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	catalogCacheVersion = 3
	catalogTTL          = 30 * time.Minute
	maxCatalogAge       = 400 * 24 * time.Hour // older products can't be "new"
	userAgent           = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15 MakeupRadar/" + appVersion
)

// Product is one makeup product from a brand or shop catalog.
type Product struct {
	ID          string    `json:"id"`
	BrandID     string    `json:"brandId"`
	Brand       string    `json:"brand"`
	Source      string    `json:"source"`
	SourceKind  string    `json:"sourceKind"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Image       string    `json:"image,omitempty"`
	Images      []string  `json:"images,omitempty"`
	Type        string    `json:"type,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price"`
	PriceMax    float64   `json:"priceMax,omitempty"`
	CompareAt   float64   `json:"compareAt,omitempty"`
	Currency    string    `json:"currency"`
	Shades      int       `json:"shades,omitempty"`
	ShadeNames  []string  `json:"shadeNames,omitempty"`
	Available   bool      `json:"available"`
	CreatedAt   time.Time `json:"createdAt"`
	PublishedAt time.Time `json:"publishedAt"`

	// Derived on load; never trusted from disk.
	NewDate       time.Time   `json:"-"`
	DateUncertain bool        `json:"-"`
	Category      string      `json:"-"`
	BaseCategory  string      `json:"-"`
	Kind          string      `json:"-"`
	IsSet         bool        `json:"-"`
	Highlights    []Highlight `json:"-"`
	Innovation    float64     `json:"-"`
	Attributes    []string    `json:"-"`
	FlaggedNew    bool        `json:"-"`

	titleP, typeP, tagsP, brandP, descP string // padded normalized fields for matching
}

// derive recomputes everything Makeup Radar infers about a product.
// It reports false when the item isn't a makeup product.
func (p *Product) derive(b Brand) bool {
	if p.Price <= 0 || p.Title == "" {
		return false
	}
	titleN, typeN := normalize(p.Title), normalize(p.Type)
	tagsN := normalize(strings.Join(p.Tags, " "))
	c := classify(classifyInput{Title: titleN, Type: typeN, Tags: tagsN, Focus: b.Focus, SourceKind: b.Kind})
	if c.Exclude {
		return false
	}
	p.Category, p.BaseCategory, p.Kind, p.IsSet = c.Category, c.BaseCategory, c.Kind, c.IsSet
	if p.Kind == "" && p.Type != "" && len(p.Type) <= 28 {
		p.Kind = titleCase(strings.ToLower(p.Type))
	}
	descN := normalize(truncateRunes(p.Description, 2500))
	an := analyze(titleN+" "+typeN+" "+tagsN+" "+descN, tagsN, p.Shades, p.BaseCategory)
	p.Highlights, p.Innovation, p.Attributes, p.FlaggedNew = an.Highlights, an.Score, an.Attributes, an.FlaggedNew

	p.NewDate = p.PublishedAt
	if p.NewDate.IsZero() || (!p.CreatedAt.IsZero() && p.PublishedAt.Sub(p.CreatedAt) > 180*24*time.Hour) {
		// Re-published long after it was created: a restock, not a launch.
		p.NewDate = p.CreatedAt
	}
	if !p.CreatedAt.IsZero() && p.CreatedAt.After(p.NewDate) {
		p.NewDate = p.CreatedAt
	}
	p.titleP = padded(titleN)
	p.typeP = padded(normalize(p.Type + " " + p.Kind))
	p.tagsP = padded(tagsN)
	p.brandP = padded(normalize(p.Brand + " " + p.Source))
	descShort := descN
	if len(descShort) > 1200 {
		descShort = descShort[:1200]
	}
	p.descP = padded(descShort)
	return true
}

// ---- Shopify catalog parsing ----

type rawProduct struct {
	ID          json.RawMessage `json:"id"`
	Title       string          `json:"title"`
	Handle      string          `json:"handle"`
	BodyHTML    string          `json:"body_html"`
	PublishedAt string          `json:"published_at"`
	CreatedAt   string          `json:"created_at"`
	Vendor      string          `json:"vendor"`
	ProductType string          `json:"product_type"`
	Tags        json.RawMessage `json:"tags"`
	Variants    []struct {
		Title          string          `json:"title"`
		Price          json.RawMessage `json:"price"`
		CompareAtPrice json.RawMessage `json:"compare_at_price"`
		Available      *bool           `json:"available"`
	} `json:"variants"`
	Images []struct {
		Src string `json:"src"`
	} `json:"images"`
	Options []struct {
		Name   string   `json:"name"`
		Values []string `json:"values"`
	} `json:"options"`
}

func rawString(r json.RawMessage) string {
	s := strings.TrimSpace(string(r))
	if s == "" || s == "null" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		var out string
		if json.Unmarshal(r, &out) == nil {
			return out
		}
	}
	return s
}

func rawPrice(r json.RawMessage) float64 {
	s := rawString(r)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil || math.IsNaN(f) || f < 0 {
		return 0
	}
	return f
}

func rawTags(r json.RawMessage) []string {
	if len(r) == 0 {
		return nil
	}
	var arr []string
	if json.Unmarshal(r, &arr) == nil {
		return arr
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	return nil
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05-0700", "2006-01-02 15:04:05 -0700", time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 MST", "Mon, 2 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// sizedImage asks Shopify's image CDN for a resized rendition.
func sizedImage(src string, width int) string {
	if src == "" {
		return ""
	}
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}
	if !strings.HasPrefix(src, "https://") && !strings.HasPrefix(src, "http://") {
		return ""
	}
	if !strings.Contains(src, "cdn.shopify.com") && !strings.Contains(src, "/cdn/shop/") {
		return src
	}
	u, err := url.Parse(src)
	if err != nil {
		return src
	}
	qv := u.Query()
	qv.Set("width", strconv.Itoa(width))
	u.RawQuery = qv.Encode()
	return u.String()
}

var shadeOptionNames = []string{"shade", "color", "colour", "tone", "hue", "tint"}

// catalogPage is one products.json page: the products worth keeping plus
// facts about the whole page, used to decide whether to read more pages.
type catalogPage struct {
	products []*Product
	total    int    // products on the page before any filtering
	order    int    // +1 newest-first, -1 oldest-first, 0 mixed
	firstID  string // detects stores that ignore the page parameter
}

func parseShopifyProducts(body []byte, b Brand, currency string, cutoff time.Time) ([]*Product, error) {
	pg, err := parseShopifyPage(body, b, currency, cutoff)
	return pg.products, err
}

// parseShopifyPage reads a store's products.json. Products whose launch
// date is before cutoff are skipped before any expensive work.
func parseShopifyPage(body []byte, b Brand, currency string, cutoff time.Time) (catalogPage, error) {
	var doc struct {
		Products []rawProduct `json:"products"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return catalogPage{}, fmt.Errorf("catalog isn't valid JSON")
	}
	page := catalogPage{total: len(doc.Products)}
	createdTimes := make([]time.Time, 0, len(doc.Products))
	out := make([]*Product, 0, len(doc.Products))
	for _, rp := range doc.Products {
		id := rawString(rp.ID)
		if page.firstID == "" {
			page.firstID = id
		}
		created, published := parseTime(rp.CreatedAt), parseTime(rp.PublishedAt)
		createdTimes = append(createdTimes, created)
		if id == "" || rp.Handle == "" {
			continue
		}
		if !cutoff.IsZero() {
			nd := published
			if nd.IsZero() || (!created.IsZero() && published.Sub(created) > 180*24*time.Hour) {
				nd = created
			}
			if created.After(nd) {
				nd = created
			}
			if nd.IsZero() || nd.Before(cutoff) {
				continue
			}
		}
		p := &Product{
			ID:          b.ID + ":" + id,
			BrandID:     b.ID,
			Brand:       b.Name,
			Source:      b.Name,
			SourceKind:  b.Kind,
			Title:       strings.TrimSpace(rp.Title),
			URL:         "https://" + b.Domain + "/products/" + url.PathEscape(rp.Handle),
			Type:        strings.TrimSpace(rp.ProductType),
			Currency:    currency,
			CreatedAt:   created,
			PublishedAt: published,
		}
		if b.Kind == "shop" {
			if v := strings.TrimSpace(rp.Vendor); v != "" && !strings.EqualFold(v, b.Name) {
				p.Brand = v
			}
		}
		tags := rawTags(rp.Tags)
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if t == "" || strings.HasPrefix(t, "_") || len(t) > 60 {
				continue
			}
			p.Tags = append(p.Tags, t)
			if len(p.Tags) >= 30 {
				break
			}
		}
		p.Description = truncateRunes(htmlToText(rp.BodyHTML), 1500)

		minP, maxP, cmp := 0.0, 0.0, 0.0
		anyAvail, sawAvail := false, false
		for _, v := range rp.Variants {
			pr := rawPrice(v.Price)
			if pr > 0 && (minP == 0 || pr < minP) {
				minP = pr
			}
			if pr > maxP {
				maxP = pr
			}
			if c := rawPrice(v.CompareAtPrice); c > cmp {
				cmp = c
			}
			if v.Available != nil {
				sawAvail = true
				if *v.Available {
					anyAvail = true
				}
			}
		}
		p.Price = minP
		if maxP > minP {
			p.PriceMax = maxP
		}
		if cmp > minP {
			p.CompareAt = cmp
		}
		p.Available = anyAvail || !sawAvail

		for _, o := range rp.Options {
			on := strings.ToLower(o.Name)
			for _, s := range shadeOptionNames {
				if strings.Contains(on, s) {
					p.Shades = len(o.Values)
					names := o.Values
					if len(names) > 60 {
						names = names[:60]
					}
					p.ShadeNames = append([]string(nil), names...)
					break
				}
			}
			if p.Shades > 0 {
				break
			}
		}
		if p.Shades == 1 {
			p.Shades, p.ShadeNames = 0, nil
		}
		for i, im := range rp.Images {
			if i >= 6 {
				break
			}
			if s := sizedImage(im.Src, 1200); s != "" {
				p.Images = append(p.Images, s)
			}
		}
		if len(rp.Images) > 0 {
			p.Image = sizedImage(rp.Images[0].Src, 600)
		}
		out = append(out, p)
	}
	page.products = out
	page.order = timesOrder(createdTimes)
	return page, nil
}

// dateOrder reports +1 when a page is newest-first, -1 when oldest-first.
func dateOrder(ps []*Product) int {
	ts := make([]time.Time, len(ps))
	for i, p := range ps {
		ts[i] = p.CreatedAt
	}
	return timesOrder(ts)
}

func timesOrder(ts []time.Time) int {
	asc, desc := 0, 0
	for i := 1; i < len(ts); i++ {
		a, b := ts[i-1], ts[i]
		if a.IsZero() || b.IsZero() {
			continue
		}
		if b.Before(a) {
			desc++
		} else if b.After(a) {
			asc++
		}
	}
	total := asc + desc
	if total == 0 {
		return 0
	}
	switch {
	case float64(desc)/float64(total) >= 0.7:
		return 1
	case float64(asc)/float64(total) >= 0.7:
		return -1
	}
	return 0
}

// finalize keeps launch-dated makeup from the last ~13 months and flags
// catalogs that were bulk-imported on one day (a platform migration,
// not a launch).
func finalize(ps []*Product, b Brand, now time.Time) []*Product {
	seen := map[string]bool{}
	kept := make([]*Product, 0, len(ps))
	for _, p := range ps {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		if p.derive(b) {
			kept = append(kept, p)
		}
	}
	byDay := map[string]int{}
	for _, p := range kept {
		if !p.NewDate.IsZero() {
			byDay[p.NewDate.Format("2006-01-02")]++
		}
	}
	bulkDay, bulkCount := "", 0
	for d, c := range byDay {
		if c > bulkCount {
			bulkDay, bulkCount = d, c
		}
	}
	var bulk time.Time
	if bulkCount >= 25 && float64(bulkCount) >= 0.4*float64(len(kept)) {
		bulk, _ = time.Parse("2006-01-02", bulkDay)
	}
	out := kept[:0]
	for _, p := range kept {
		if p.NewDate.IsZero() {
			continue
		}
		if !bulk.IsZero() && math.Abs(p.NewDate.Sub(bulk).Hours()) < 48 {
			continue
		}
		if now.Sub(p.NewDate) > maxCatalogAge || p.NewDate.After(now.Add(48*time.Hour)) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NewDate.After(out[j].NewDate) })
	return out
}

// ---- Catalog manager ----

type brandCache struct {
	Version     int        `json:"v"`
	BrandID     string     `json:"brandId"`
	FetchedAt   time.Time  `json:"fetchedAt"`
	LastOK      time.Time  `json:"lastOk"`
	OK          bool       `json:"ok"`
	Error       string     `json:"error,omitempty"`
	Fails       int        `json:"fails"`
	Currency    string     `json:"currency,omitempty"`
	MetaChecked time.Time  `json:"metaChecked"`
	Products    []*Product `json:"products"`
}

type flight struct {
	done chan struct{}
}

type brandState struct {
	brand    Brand
	enabled  bool
	cache    *brandCache
	inflight *flight
}

// BrandStatus is what the Sources panel shows for each catalog.
type BrandStatus struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Domain    string    `json:"domain"`
	Kind      string    `json:"kind"`
	Custom    bool      `json:"custom"`
	Enabled   bool      `json:"enabled"`
	State     string    `json:"state"` // ok | error | pending | checking
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitzero"`
	Products  int       `json:"products"`
	New30     int       `json:"new30"`
}

type CatalogManager struct {
	mu     sync.RWMutex
	states map[string]*brandState
	order  []string
	client *http.Client
	dir    string
	sem    chan struct{}
	now    func() time.Time
	logf   func(string, ...any)
}

func NewCatalogManager(client *http.Client, dir string, logf func(string, ...any)) *CatalogManager {
	_ = os.MkdirAll(dir, 0o700)
	return &CatalogManager{
		states: map[string]*brandState{},
		client: client,
		dir:    dir,
		sem:    make(chan struct{}, 12),
		now:    time.Now,
		logf:   logf,
	}
}

// SetBrands syncs the manager with the current brand list and toggles.
func (m *CatalogManager) SetBrands(all []Brand, disabled map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := map[string]bool{}
	m.order = m.order[:0]
	for _, b := range all {
		keep[b.ID] = true
		m.order = append(m.order, b.ID)
		st, ok := m.states[b.ID]
		if !ok {
			st = &brandState{brand: b}
			st.cache = m.loadCacheLocked(b)
			m.states[b.ID] = st
		}
		st.brand = b
		st.enabled = !disabled[b.ID]
	}
	for id := range m.states {
		if !keep[id] {
			delete(m.states, id)
		}
	}
}

func (m *CatalogManager) cachePath(id string) string {
	return filepath.Join(m.dir, id+".json")
}

func (m *CatalogManager) loadCacheLocked(b Brand) *brandCache {
	var c brandCache
	if err := readJSON(m.cachePath(b.ID), &c); err != nil || c.Version != catalogCacheVersion {
		return nil
	}
	c.Products = finalize(c.Products, b, m.now())
	return &c
}

func (m *CatalogManager) needsRefresh(st *brandState, now time.Time, force bool) bool {
	if !st.enabled || st.inflight != nil {
		return false
	}
	c := st.cache
	if c == nil {
		return true
	}
	if c.OK {
		return force || now.Sub(c.FetchedAt) > catalogTTL
	}
	// Failed before: back off 10 min, doubling, up to a day.
	backoff := 10 * time.Minute * time.Duration(1<<min(c.Fails-1, 7))
	if backoff > 24*time.Hour {
		backoff = 24 * time.Hour
	}
	if force && c.Fails <= 1 {
		return true
	}
	return now.Sub(c.FetchedAt) > backoff
}

// Stale lists enabled catalogs that should be refreshed now.
func (m *CatalogManager) Stale(force bool) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := m.now()
	var out []string
	for _, id := range m.order {
		st := m.states[id]
		if st != nil && (m.needsRefresh(st, now, force) || st.inflight != nil && st.enabled) {
			out = append(out, id)
		}
	}
	return out
}

// Ensure refreshes the given catalogs (joining refreshes already running)
// and calls progress as each finishes. It returns when all are done or
// ctx ends; unfinished refreshes keep going in the background.
func (m *CatalogManager) Ensure(ctx context.Context, ids []string, force bool, progress func(BrandStatus)) {
	type waiter struct {
		id string
		f  *flight
	}
	var ws []waiter
	var finished []string // completed between Stale() and now
	m.mu.Lock()
	now := m.now()
	for _, id := range ids {
		st := m.states[id]
		if st == nil || !st.enabled {
			continue
		}
		if st.inflight == nil && m.needsRefresh(st, now, force) {
			f := &flight{done: make(chan struct{})}
			st.inflight = f
			go m.run(st.brand, f)
		}
		if st.inflight != nil {
			ws = append(ws, waiter{id, st.inflight})
		} else {
			finished = append(finished, id)
		}
	}
	m.mu.Unlock()
	if progress != nil {
		for _, id := range finished {
			progress(m.Status(id))
		}
	}

	doneCh := make(chan string, len(ws))
	for _, w := range ws {
		go func(w waiter) {
			select {
			case <-w.f.done:
				doneCh <- w.id
			case <-ctx.Done():
			}
		}(w)
	}
	for range ws {
		select {
		case id := <-doneCh:
			if progress != nil {
				progress(m.Status(id))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *CatalogManager) run(b Brand, f *flight) {
	defer close(f.done)
	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	m.mu.RLock()
	prev := m.states[b.ID].cache
	m.mu.RUnlock()

	start := time.Now()
	res, err := m.fetch(ctx, b, prev)
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.states[b.ID]
	if st != nil {
		st.inflight = nil
	}
	var c brandCache
	if prev != nil {
		c = *prev
	}
	c.Version, c.BrandID, c.FetchedAt = catalogCacheVersion, b.ID, now
	if err != nil {
		c.OK = false
		c.Error = err.Error()
		c.Fails++
		m.logf("catalog %s: %v (%.1fs)", b.Domain, err, time.Since(start).Seconds())
	} else {
		c.OK, c.Error, c.Fails, c.LastOK = true, "", 0, now
		c.Products = res.products
		if res.currency != "" {
			c.Currency = res.currency
		}
		if res.metaChecked {
			c.MetaChecked = now
		}
		m.logf("catalog %s: %d makeup products (%.1fs)", b.Domain, len(c.Products), time.Since(start).Seconds())
	}
	if st != nil {
		st.cache = &c
	}
	if err := writeJSONAtomic(m.cachePath(b.ID), &c); err != nil {
		m.logf("cache write %s: %v", b.ID, err)
	}
}

type fetchResult struct {
	products    []*Product
	currency    string
	metaChecked bool
}

var errNoCatalog = errors.New("no public catalog at this address")

func (m *CatalogManager) fetch(ctx context.Context, b Brand, prev *brandCache) (*fetchResult, error) {
	base := "https://" + b.Domain
	res := &fetchResult{}
	currency := b.Currency
	if prev != nil && prev.Currency != "" {
		currency = prev.Currency
	}
	// Look up the store's currency (weekly) alongside the catalog download.
	var metaCh chan string
	if prev == nil || m.now().Sub(prev.MetaChecked) > 7*24*time.Hour {
		res.metaChecked = true
		metaCh = make(chan string, 1)
		go func() { metaCh <- m.shopCurrency(ctx, base) }()
	}
	if currency == "" {
		currency = "USD"
	}

	cutoff := m.now().Add(-maxCatalogAge)
	page1, err := m.getPage(ctx, base+"/products.json?limit=250", b, currency, cutoff)
	if err != nil {
		return nil, err
	}
	all := page1.products
	if page1.total >= 250 && page1.order <= 0 {
		// Not newest-first: ask for the catalog sorted by creation date.
		if sorted, err := m.getPage(ctx, base+"/collections/all/products.json?limit=250&sort_by=created-descending", b, currency, cutoff); err == nil && sorted.total > 0 && sorted.order > 0 {
			all = append(all, sorted.products...)
		} else if page1.order < 0 {
			// Oldest-first and unsortable: walk forward to the newest pages.
			for n := 2; n <= 16; n++ {
				pg, err := m.getPage(ctx, base+"/products.json?limit=250&page="+strconv.Itoa(n), b, currency, cutoff)
				if err != nil || pg.total == 0 || pg.firstID == page1.firstID {
					break
				}
				all = append(all, pg.products...)
				if pg.total < 250 {
					break
				}
			}
		}
	}
	if metaCh != nil {
		if cur := <-metaCh; cur != "" {
			res.currency = cur
			if cur != currency {
				for _, p := range all {
					p.Currency = cur
				}
			}
		}
	}
	res.products = finalize(all, b, m.now())
	return res, nil
}

func (m *CatalogManager) request(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json,text/javascript,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return m.client.Do(req)
}

func (m *CatalogManager) getPage(ctx context.Context, u string, b Brand, currency string, cutoff time.Time) (catalogPage, error) {
	body, err := m.fetchCatalogBody(ctx, u)
	if err != nil {
		return catalogPage{}, err
	}
	return parseShopifyPage(body, b, currency, cutoff)
}

// fetchCatalogBody downloads one catalog page, retrying once on rate
// limits or dropped connections, and turns failures into plain English.
func (m *CatalogManager) fetchCatalogBody(ctx context.Context, u string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := m.request(ctx, u)
		if err != nil {
			lastErr = friendlyNetErr(err)
			if ctx.Err() != nil {
				break
			}
			continue
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 24<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
			lastErr = errors.New("store is rate-limiting, will retry later")
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil, lastErr
			}
			continue
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			return nil, errNoCatalog
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
			return nil, errors.New("store blocked automated catalog access")
		case resp.StatusCode != http.StatusOK:
			return nil, fmt.Errorf("store answered HTTP %d", resp.StatusCode)
		}
		if rerr != nil {
			lastErr = errors.New("connection dropped while downloading")
			continue
		}
		trim := strings.TrimSpace(string(body[:min(len(body), 64)]))
		if !strings.HasPrefix(trim, "{") {
			return nil, errNoCatalog
		}
		return body, nil
	}
	if lastErr == nil {
		lastErr = errors.New("store didn't respond")
	}
	return nil, lastErr
}

func (m *CatalogManager) shopCurrency(ctx context.Context, base string) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	resp, err := m.request(ctx, base+"/meta.json")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var meta struct {
		Currency string `json:"currency"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta) != nil {
		return ""
	}
	cur := strings.ToUpper(strings.TrimSpace(meta.Currency))
	if len(cur) != 3 {
		return ""
	}
	return cur
}

// Products returns every product from enabled catalogs.
func (m *CatalogManager) Products() []*Product {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Product
	for _, id := range m.order {
		st := m.states[id]
		if st == nil || !st.enabled || st.cache == nil {
			continue
		}
		out = append(out, st.cache.Products...)
	}
	return out
}

func (m *CatalogManager) Product(id string) *Product {
	brandID, _, _ := strings.Cut(id, ":")
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := m.states[brandID]
	if st == nil || st.cache == nil {
		return nil
	}
	for _, p := range st.cache.Products {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (m *CatalogManager) Status(id string) BrandStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statusLocked(m.states[id], m.now())
}

func (m *CatalogManager) statusLocked(st *brandState, now time.Time) BrandStatus {
	if st == nil {
		return BrandStatus{}
	}
	s := BrandStatus{ID: st.brand.ID, Name: st.brand.Name, Domain: st.brand.Domain, Kind: st.brand.Kind, Custom: st.brand.Custom, Enabled: st.enabled, State: "pending"}
	if st.inflight != nil {
		s.State = "checking"
	}
	if c := st.cache; c != nil {
		s.CheckedAt = c.FetchedAt
		s.Products = len(c.Products)
		for _, p := range c.Products {
			if now.Sub(p.NewDate) <= 30*24*time.Hour {
				s.New30++
			}
		}
		if st.inflight == nil {
			if c.OK {
				s.State = "ok"
			} else {
				s.State = "error"
				s.Error = c.Error
				if !c.LastOK.IsZero() {
					s.Error += " (showing products from the last good check)"
				}
			}
		}
	}
	return s
}

func (m *CatalogManager) Statuses() []BrandStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := m.now()
	out := make([]BrandStatus, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.statusLocked(m.states[id], now))
	}
	return out
}

// LastChecked is the most recent successful catalog check.
func (m *CatalogManager) LastChecked() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var t time.Time
	for _, st := range m.states {
		if st.enabled && st.cache != nil && st.cache.OK && st.cache.FetchedAt.After(t) {
			t = st.cache.FetchedAt
		}
	}
	return t
}

// OldestChecked is the least recent successful check among enabled catalogs.
func (m *CatalogManager) OldestChecked() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var t time.Time
	for _, st := range m.states {
		if st.enabled && st.cache != nil && st.cache.OK && (t.IsZero() || st.cache.FetchedAt.Before(t)) {
			t = st.cache.FetchedAt
		}
	}
	return t
}

func (m *CatalogManager) Counts() (enabled, ok int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, st := range m.states {
		if st.enabled {
			enabled++
			if st.cache != nil && st.cache.OK {
				ok++
			}
		}
	}
	return
}

// ProbeStore checks that a website exposes a readable catalog and
// returns the brand name most of its products carry.
func (m *CatalogManager) ProbeStore(ctx context.Context, domain string) (name string, count int, err error) {
	body, err := m.fetchCatalogBody(ctx, "https://"+domain+"/products.json?limit=50")
	if err != nil {
		return "", 0, err
	}
	b := Brand{ID: "probe", Name: domain, Domain: domain, Kind: "brand", Focus: "makeup"}
	ps, err := parseShopifyProducts(body, b, "USD", time.Time{})
	if err != nil {
		return "", 0, err
	}
	if len(ps) == 0 {
		return "", 0, errors.New("the store's catalog is empty")
	}
	var doc struct {
		Products []struct {
			Vendor string `json:"vendor"`
		} `json:"products"`
	}
	_ = json.Unmarshal(body, &doc)
	votes := map[string]int{}
	for _, p := range doc.Products {
		if v := strings.TrimSpace(p.Vendor); v != "" {
			votes[v]++
		}
	}
	best := 0
	for v, n := range votes {
		if n > best || (n == best && v < name) {
			name, best = v, n
		}
	}
	return name, len(ps), nil
}

func friendlyNetErr(err error) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "no such host"):
		return errors.New("website not found")
	case strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Timeout") || strings.Contains(s, "timeout"):
		return errors.New("store took too long to respond")
	case strings.Contains(s, "refusing to connect"):
		return errors.New("address isn't a public website")
	case strings.Contains(s, "certificate"):
		return errors.New("store's secure connection failed")
	case strings.Contains(s, "connection refused") || strings.Contains(s, "network is unreachable") || strings.Contains(s, "no route"):
		return errors.New("couldn't connect (are you online?)")
	}
	return errors.New("couldn't reach the store")
}
