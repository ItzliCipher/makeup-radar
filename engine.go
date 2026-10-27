package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Engine ties the catalogs, the news feeds and the user's store together.
type Engine struct {
	store    *Store
	catalogs *CatalogManager
	news     *NewsManager
	logf     func(string, ...any)
	now      func() time.Time

	// Website-mode tuning (zero values mean the Mac app's defaults).
	web         bool
	freshMinAge time.Duration
	resultLimit int

	mu      sync.Mutex
	phrases []brandPhrase
}

func NewEngine(store *Store, client *http.Client, cacheDir string, logf func(string, ...any)) *Engine {
	e := &Engine{
		store:    store,
		catalogs: NewCatalogManager(client, cacheDir, logf),
		news:     NewNewsManager(client, logf),
		logf:     logf,
		now:      time.Now,
	}
	e.syncBrands()
	return e
}

// syncBrands pushes the current brand list into every component.
func (e *Engine) syncBrands() {
	all := e.store.AllBrands()
	e.catalogs.SetBrands(all, e.store.DisabledSet())
	set := e.store.Settings()
	rebuildQueryDict(set.CustomBrands)
	e.mu.Lock()
	e.phrases = buildBrandPhrases(all)
	e.mu.Unlock()
}

func (e *Engine) brandPhrases() []brandPhrase {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.phrases
}

// Prewarm refreshes stale catalogs in the background at startup so the
// first search is quick.
func (e *Engine) Prewarm() {
	go e.catalogs.Ensure(context.Background(), e.catalogs.Stale(false), false, nil)
}

type SearchParams struct {
	Q        string
	Sort     string
	Days     int
	Cat      string
	Brand    string
	HideSets bool
	Fresh    bool
	Rescan   bool
	Region   string // website visitors choose their own news edition
}

type newsResult struct {
	items []*NewsItem
	stats NewsStats
}

// Search runs one live search and streams results through emit as they
// arrive: plan, catalog progress, news, products, done.
func (e *Engine) Search(ctx context.Context, sp SearchParams, emit func(event string, v any)) {
	start := time.Now()
	brands := e.store.AllBrands()
	q := ParseQuery(sp.Q, e.now(), brands)
	set := e.store.Settings()
	if sp.Region != "" {
		set.Region = sp.Region
	}
	enabled, _ := e.catalogs.Counts()
	var feeds []string
	if set.NewsGoogle {
		feeds = append(feeds, "Google News")
	}
	if set.NewsBing {
		feeds = append(feeds, "Bing News")
	}
	if set.NewsPress {
		feeds = append(feeds, "Beauty press")
	}
	emit("plan", map[string]any{
		"q": q.Raw, "chips": q.Chips(), "generic": q.Generic, "brandName": q.BrandName, "newsTerms": q.NewsTerms,
		"catalogs": enabled, "feeds": feeds, "region": regionByCode(set.Region).Label,
	})

	newsDays := sp.Days
	if q.Days > 0 {
		newsDays = q.Days
	}
	if newsDays <= 0 || newsDays > 60 {
		newsDays = 60
	}
	if newsDays < 30 && q.Days == 0 {
		newsDays = 30
	}
	newsCh := make(chan newsResult, 1)
	go func() {
		if len(feeds) == 0 {
			newsCh <- newsResult{}
			return
		}
		nctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		items, stats := e.news.Search(nctx, q, NewsOptions{
			Days: newsDays, Fresh: sp.Fresh, FreshMinAge: e.freshMinAge, Region: regionByCode(set.Region),
			Google: set.NewsGoogle, Bing: set.NewsBing, Press: set.NewsPress, Brands: e.brandPhrases(),
		})
		newsCh <- newsResult{items, stats}
	}()

	stale := e.catalogs.Stale(sp.Rescan)
	catCh := make(chan struct{})
	go func() {
		defer close(catCh)
		if len(stale) == 0 {
			return
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		done := 0
		emit("progress", map[string]any{"phase": "catalogs", "done": 0, "total": len(stale)})
		e.catalogs.Ensure(cctx, stale, sp.Rescan, func(st BrandStatus) {
			done++
			emit("progress", map[string]any{"phase": "catalogs", "done": done, "total": len(stale), "brand": st.Name, "ok": st.State == "ok"})
		})
	}()

	opts := SearchOptions{Sort: sp.Sort, Days: sp.Days, Cat: sp.Cat, Brand: sp.Brand, HideSets: sp.HideSets, Limit: e.resultLimit}
	var (
		news            []*NewsItem
		newsStats       NewsStats
		cards           []*ProductCard
		meta            ProductMeta
		newsDone, catOK bool
	)
	sendProducts := func() {
		if cards == nil {
			cards = []*ProductCard{}
		}
		enabledNow, okNow := e.catalogs.Counts()
		emit("products", map[string]any{
			"items": cards, "meta": meta, "checkedAt": e.catalogs.OldestChecked(),
			"catalogsOk": okNow, "catalogsTotal": enabledNow,
		})
	}
	for !(newsDone && catOK) {
		select {
		case r := <-newsCh:
			newsCh = nil // a nil channel never fires again in select
			newsDone, news, newsStats = true, r.items, r.stats
			if news == nil {
				news = []*NewsItem{}
			}
			if newsStats.Feeds == nil {
				newsStats.Feeds = []string{}
			}
			emit("news", map[string]any{"items": news, "stats": newsStats, "fetchedAt": e.now()})
			if catOK && len(cards) > 0 {
				crossReference(cards, news)
				if opts.Sort == "" || opts.Sort == "top" {
					sortCards(cards, "top")
				}
				sendProducts()
			}
		case <-catCh:
			catCh = nil // closed channels fire forever; stop listening
			catOK = true
			cards, meta = searchProducts(e.catalogs.Products(), q, opts, e.now())
			if q.BrandID != "" && len(cards) == 0 {
				if st := e.catalogs.Status(q.BrandID); st.State != "ok" || !st.Enabled {
					meta.BrandOffline = true
				}
			}
			if newsDone {
				crossReference(cards, news)
				if opts.Sort == "" || opts.Sort == "top" {
					sortCards(cards, "top")
				}
			}
			sendProducts()
		case <-ctx.Done():
			return
		}
	}
	enabledNow, okNow := e.catalogs.Counts()
	emit("done", map[string]any{
		"tookMs": time.Since(start).Milliseconds(), "catalogsOk": okNow, "catalogsTotal": enabledNow,
		"pending": len(e.catalogs.Stale(false)), "newsFailed": newsStats.Failed, "newsFeeds": newsStats.Feeds,
	})
	// The shared website never records searches; each visitor's browser
	// keeps its own recent searches.
	if sp.Fresh && !e.web && strings.TrimSpace(sp.Q) != "" {
		e.store.AddHistory(sp.Q)
	}
}

func (e *Engine) ProductDetail(id string) *ProductDetail {
	p := e.catalogs.Product(id)
	if p == nil {
		return nil
	}
	return toDetail(p, e.now())
}

func (e *Engine) Sources() map[string]any {
	set := e.store.Settings()
	var regionList []map[string]string
	for _, r := range regions {
		regionList = append(regionList, map[string]string{"code": r.Code, "label": r.Label})
	}
	return map[string]any{
		"brands":     e.catalogs.Statuses(),
		"region":     set.Region,
		"regions":    regionList,
		"newsGoogle": set.NewsGoogle,
		"newsBing":   set.NewsBing,
		"newsPress":  set.NewsPress,
		"pressFeeds": pressFeedNames(),
		"readOnly":   e.web,
	}
}

func (e *Engine) SetEnabled(id string, on bool) error {
	if err := e.store.SetEnabled(id, on); err != nil {
		return err
	}
	e.syncBrands()
	if on {
		go e.catalogs.Ensure(context.Background(), []string{id}, false, nil)
	}
	return nil
}

func prettyNameFromDomain(domain string) string {
	d := strings.TrimPrefix(domain, "www.")
	if i := strings.Index(d, "."); i > 0 {
		d = d[:i]
	}
	d = strings.ReplaceAll(d, "-", " ")
	parts := strings.Fields(d)
	for i, p := range parts {
		parts[i] = titleCase(p)
	}
	return strings.Join(parts, " ")
}

// AddBrand checks a website for a readable catalog and adds it.
func (e *Engine) AddBrand(ctx context.Context, input string) (BrandStatus, error) {
	domain, err := validStoreDomain(input)
	if err != nil {
		return BrandStatus{}, err
	}
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	name, _, err := e.catalogs.ProbeStore(pctx, domain)
	if err != nil && !strings.HasPrefix(domain, "www.") {
		if n2, _, err2 := e.catalogs.ProbeStore(pctx, "www."+domain); err2 == nil {
			name, domain, err = n2, "www."+domain, nil
		}
	}
	if err != nil {
		if errors.Is(err, errNoCatalog) {
			return BrandStatus{}, errors.New("this store doesn't publish a catalog Makeup Radar can read")
		}
		return BrandStatus{}, err
	}
	if name == "" || len(name) > 40 {
		name = prettyNameFromDomain(domain)
	}
	b, err := e.store.AddCustomBrand(name, domain)
	if err != nil {
		return BrandStatus{}, err
	}
	e.syncBrands()
	cctx, ccancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer ccancel()
	e.catalogs.Ensure(cctx, []string{b.ID}, true, nil)
	return e.catalogs.Status(b.ID), nil
}

func (e *Engine) RemoveBrand(id string) error {
	if err := e.store.RemoveCustomBrand(id); err != nil {
		return err
	}
	e.syncBrands()
	return nil
}

func pressFeedNames() []string {
	out := make([]string, 0, len(pressFeeds))
	for _, f := range pressFeeds {
		out = append(out, f.Name)
	}
	return out
}
