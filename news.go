package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// NewsItem is one launch story from the beauty press.
type NewsItem struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	URL        string      `json:"url"`
	Publisher  string      `json:"publisher"`
	Domain     string      `json:"domain,omitempty"`
	Published  time.Time   `json:"published"`
	Snippet    string      `json:"snippet,omitempty"`
	Image      string      `json:"image,omitempty"`
	Feeds      []string    `json:"feeds"`
	AlsoIn     []string    `json:"alsoIn,omitempty"`
	Brands     []string    `json:"brands,omitempty"`
	Highlights []Highlight `json:"highlights,omitempty"`
	Launch     bool        `json:"launch"`
	Score      float64     `json:"-"`

	titleN string
	sig    map[string]bool
}

// Region picks the Google News / Bing edition.
type Region struct {
	Code, Label, HL, GL, CEID, Market string
}

var regions = []Region{
	{"US", "United States", "en-US", "US", "US:en", "en-US"},
	{"GB", "United Kingdom", "en-GB", "GB", "GB:en", "en-GB"},
	{"CA", "Canada", "en-CA", "CA", "CA:en", "en-CA"},
	{"AU", "Australia", "en-AU", "AU", "AU:en", "en-AU"},
	{"IN", "India", "en-IN", "IN", "IN:en", "en-IN"},
}

func regionByCode(code string) Region {
	for _, r := range regions {
		if r.Code == code {
			return r
		}
	}
	return regions[0]
}

var beautyPublishers = map[string]bool{
	"allure": true, "byrdie": true, "glamour": true, "vogue": true, "elle": true, "harpers bazaar": true, "bazaar": true,
	"cosmopolitan": true, "instyle": true, "refinery29": true, "wwd": true, "womens wear daily": true, "beauty independent": true,
	"cosmetics business": true, "cosmetics design": true, "cosmetics design europe": true, "into the gloss": true,
	"the zoe report": true, "bustle": true, "who what wear": true, "marie claire": true, "teen vogue": true, "popsugar": true,
	"womens health": true, "shape": true, "seventeen": true, "good housekeeping": true, "real simple": true, "glossy": true,
	"beautymatter": true, "fashionista": true, "the cut": true, "coveteur": true, "temptalia": true, "hello giggles": true,
	"essence": true, "stylist": true, "vogue business": true, "the business of fashion": true, "business of fashion": true,
	"hypebae": true, "nylon": true, "grazia": true, "people": true, "self": true, "well good": true, "makeup com": true,
	"cosmetics and toiletries": true, "global cosmetic industry": true, "happi": true, "premium beauty news": true,
	"beauty packaging": true, "cew": true, "harpers bazaar uk": true, "vogue uk": true, "elle uk": true, "glamour uk": true,
	"cosmetify": true, "beauty bay": true, "the independent": true, "evening standard": true, "nbc select": true,
}

var launchWords = []string{
	"launch", "launches", "launched", "launching", "debut", "debuts", "debuted", "unveil", "unveils", "unveiled",
	"introduces", "introducing", "introduced", "drop", "drops", "dropped", "release", "releases", "released", "reveal",
	"reveals", "revealed", "first look", "just dropped", "arrives", "arrived", "is here", "now available", "teases",
	"sneak peek", "announces", "announced", "expands", "rolls out", "collab", "collaboration", "new collection", "coming soon",
}

var makeupNoise = []string{
	"makeup game", "make up game", "makeup games", "makeup date", "makeup exam", "makeup exams", "makeup test", "makeup tests",
	"makeup class", "makeup classes", "makeup session", "makeup day", "makeup days", "makeup race", "makeup match",
	"makeup work", "makeup assignment", "makeup lesson", "makeup call", "makeup of", "racial makeup", "genetic makeup",
	"political makeup", "roster makeup", "demographic makeup", "ethnic makeup", "partisan makeup", "makeup vote",
	"makeup election", "makeup tournament", "makeup start", "makeup shift", "makeup sex", "makeup classroom",
}

var beautyContext = []string{"makeup", "make up", "beauty", "cosmetics", "cosmetic", "skincare", "skin care", "sephora", "ulta"}

// brandPhrase maps a normalized alias to the brand's display name. Weak
// aliases are everyday words or celebrity names ("rhode", "kylie",
// "dior"): they only count as a brand mention when a story is already
// clearly about beauty.
type brandPhrase struct {
	alias, name string
	weak        bool
}

var weakAliases = map[string]bool{
	"merit": true, "mario": true, "rhode": true, "hailey bieber": true, "kylie": true, "kylie jenner": true,
	"fenty": true, "rihanna": true, "lady gaga": true, "ariana grande": true, "halsey": true, "gwen stefani": true,
	"patrick starrr": true, "harry styles": true, "selena gomez": true, "millie bobby brown": true,
	"victoria beckham": true, "pleasing": true, "violette": true, "hourglass": true, "kaja": true, "freck": true,
	"uoma": true, "mented": true, "kulfi": true, "refy": true, "lawless": true, "one size": true, "about face": true,
	"ilia": true, "jones road": true, "westman": true, "danessa": true, "summer fridays": true, "half magic": true,
	"jeffree star": true, "dior": true, "chanel": true, "ysl": true, "armani": true, "tom ford": true, "hermes": true,
	"givenchy": true, "abh": true, "kiko": true, "huda": true, "sisley": true, "rabanne": true, "milani": true,
	"yves saint laurent": true, "pomade": true, "onesize": true, "hince": true, "fwee": true, "thrive": true,
}

func buildBrandPhrases(all []Brand) []brandPhrase {
	var out []brandPhrase
	add := func(alias, name string) {
		a := normalize(alias)
		if a == "" {
			return
		}
		out = append(out, brandPhrase{alias: a, name: name, weak: weakAliases[a]})
	}
	for _, b := range all {
		add(b.Name, b.Name)
		for _, a := range b.Aliases {
			add(a, b.Name)
		}
	}
	for name, aliases := range otherBrands {
		add(name, name)
		for _, a := range aliases {
			add(a, name)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].alias) > len(out[j].alias) })
	return out
}

// detectBrands returns brands named in the text. Strong mentions are
// returned separately so callers can tell a beauty story from, say, a
// celebrity's music news.
func detectBrands(norm string, phrases []brandPhrase) (all []string, strong int) {
	pad := padded(norm)
	seen := map[string]bool{}
	for _, bp := range phrases {
		if seen[bp.name] {
			continue
		}
		if containsPhrase(pad, bp.alias) {
			seen[bp.name] = true
			all = append(all, bp.name)
			if !bp.weak {
				strong++
			}
		}
	}
	return all, strong
}

type rssDoc struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Source      struct {
		Name string `xml:",chardata"`
		URL  string `xml:"url,attr"`
	} `xml:"source"`
	BingSource string     `xml:"Source"`
	BingImage  string     `xml:"Image"`
	Thumbnails []mediaRef `xml:"thumbnail"`
	Contents   []mediaRef `xml:"content"`
	Enclosures []mediaRef `xml:"enclosure"`
}

type mediaRef struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Medium string `xml:"medium,attr"`
}

// pressFeed is a beauty publication's own RSS feed. These keep launch
// news flowing even if a news search engine changes its feed.
type pressFeed struct{ Name, Domain, URL string }

var pressFeeds = []pressFeed{
	{"Allure", "allure.com", "https://www.allure.com/feed/rss"},
	{"Byrdie", "byrdie.com", "https://feeds-api.dotdashmeredith.com/v1/rss/google/6772aca0-2ce6-4ccc-8a40-d5556ba3a9c7"},
	{"Glamour", "glamour.com", "https://www.glamour.com/feed/rss"},
	{"WWD", "wwd.com", "https://wwd.com/beauty-industry-news/feed/"},
	{"Temptalia", "temptalia.com", "https://www.temptalia.com/feed/"},
	{"Musings of a Muse", "musingsofamuse.com", "https://www.musingsofamuse.com/feed"},
}

func firstImage(it rssItem) string {
	for _, group := range [][]mediaRef{it.Thumbnails, it.Contents, it.Enclosures} {
		for _, m := range group {
			u := strings.TrimSpace(m.URL)
			if !strings.HasPrefix(u, "https://") {
				continue
			}
			if m.Type != "" && !strings.HasPrefix(m.Type, "image/") {
				continue
			}
			if m.Medium != "" && m.Medium != "image" {
				continue
			}
			return u
		}
	}
	return ""
}

func pressItems(items []rssItem, f pressFeed) []*NewsItem {
	out := make([]*NewsItem, 0, len(items))
	for _, it := range items {
		title := strings.TrimSpace(htmlToText(it.Title))
		link := strings.TrimSpace(it.Link)
		if title == "" || !strings.HasPrefix(link, "http") {
			continue
		}
		out = append(out, &NewsItem{
			ID: itemID(link), Title: title, URL: link, Publisher: f.Name, Domain: f.Domain,
			Published: parseTime(it.PubDate), Snippet: truncateRunes(htmlToText(it.Description), 260),
			Image: firstImage(it), Feeds: []string{"Beauty press"},
		})
	}
	return out
}

func parseRSS(body []byte) ([]rssItem, error) {
	var doc rssDoc
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.Strict = false
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) { return input, nil }
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("feed isn't valid RSS")
	}
	return doc.Channel.Items, nil
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func itemID(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:8])
}

func googleItems(items []rssItem) []*NewsItem {
	out := make([]*NewsItem, 0, len(items))
	for _, it := range items {
		title := strings.TrimSpace(htmlToText(it.Title))
		pub := strings.TrimSpace(it.Source.Name)
		if pub != "" && strings.HasSuffix(title, " - "+pub) {
			title = strings.TrimSpace(strings.TrimSuffix(title, " - "+pub))
		} else if i := strings.LastIndex(title, " - "); i > 20 && pub == "" {
			pub = strings.TrimSpace(title[i+3:])
			title = strings.TrimSpace(title[:i])
		}
		link := strings.TrimSpace(it.Link)
		if title == "" || !strings.HasPrefix(link, "http") {
			continue
		}
		out = append(out, &NewsItem{
			ID: itemID(link), Title: title, URL: link, Publisher: pub, Domain: hostOf(it.Source.URL),
			Published: parseTime(it.PubDate), Feeds: []string{"Google News"},
		})
	}
	return out
}

func bingItems(items []rssItem) []*NewsItem {
	out := make([]*NewsItem, 0, len(items))
	for _, it := range items {
		title := strings.TrimSpace(htmlToText(it.Title))
		link := strings.TrimSpace(it.Link)
		if u, err := url.Parse(link); err == nil && strings.Contains(u.Host, "bing.com") {
			if target := u.Query().Get("url"); strings.HasPrefix(target, "http") {
				link = target
			}
		}
		if title == "" || !strings.HasPrefix(link, "http") {
			continue
		}
		img := strings.TrimSpace(it.BingImage)
		if img != "" && !strings.HasPrefix(img, "https://") {
			if strings.HasPrefix(img, "http://") {
				img = "https://" + strings.TrimPrefix(img, "http://")
			} else {
				img = ""
			}
		}
		out = append(out, &NewsItem{
			ID: itemID(link), Title: title, URL: link, Publisher: strings.TrimSpace(it.BingSource), Domain: hostOf(link),
			Published: parseTime(it.PubDate), Snippet: truncateRunes(htmlToText(it.Description), 260), Image: img,
			Feeds: []string{"Bing News"},
		})
	}
	return out
}

func quoteTerm(t string) string {
	if strings.Contains(t, " ") {
		return `"` + t + `"`
	}
	return t
}

func googleWhen(days int) string {
	switch {
	case days <= 7:
		return "when:7d"
	case days <= 14:
		return "when:14d"
	case days <= 30:
		return "when:30d"
	}
	return "when:60d"
}

// newsQueries builds the Google News and Bing searches for a query.
func newsQueries(q *Query, days int) (google []string, bing string) {
	launch := "launch OR launches OR launched OR debuts OR unveils OR introduces OR new"
	when := googleWhen(days)
	if q.Generic {
		google = []string{
			"makeup OR cosmetics launch OR launches OR launched OR debuts OR unveils " + when,
			`lipstick OR mascara OR foundation OR blush OR concealer OR eyeshadow OR "lip gloss" OR bronzer launches OR debuts OR "new" ` + when,
		}
		return google, "new makeup launch"
	}
	var parts []string
	hasBeauty := q.BrandName != ""
	for _, t := range q.NewsTerms {
		parts = append(parts, quoteTerm(t))
		if hasAny(newTextIndex(normalize(t)), newsMakeupWords) {
			hasBeauty = true
		}
	}
	terms := strings.Join(parts, " ")
	ctx := ""
	if !hasBeauty {
		ctx = " makeup OR beauty OR cosmetics"
	}
	google = []string{terms + " " + launch + ctx + " " + when}
	bing = terms
	if !hasBeauty {
		bing += " makeup"
	}
	bing += " launch"
	return google, bing
}

// NewsManager fetches and caches launch news per query.
type NewsManager struct {
	client *http.Client
	logf   func(string, ...any)
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]newsEntry

	press      []*NewsItem
	pressAt    time.Time
	pressOK    int
	pressFirst sync.Mutex // one press refresh at a time
}

type newsEntry struct {
	at    time.Time
	items []*NewsItem
	stats NewsStats
}

type NewsStats struct {
	Feeds  []string `json:"feeds"`
	Failed []string `json:"failed,omitempty"`
	Raw    int      `json:"raw"`
}

func NewNewsManager(client *http.Client, logf func(string, ...any)) *NewsManager {
	return &NewsManager{client: client, logf: logf, now: time.Now, cache: map[string]newsEntry{}}
}

func (n *NewsManager) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, friendlyNetErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 6<<20))
}

func (n *NewsManager) fetchGoogle(ctx context.Context, query string, r Region) ([]*NewsItem, error) {
	u := "https://news.google.com/rss/search?q=" + url.QueryEscape(query) + "&hl=" + r.HL + "&gl=" + r.GL + "&ceid=" + url.QueryEscape(r.CEID)
	body, err := n.get(ctx, u)
	if err != nil {
		return nil, err
	}
	items, err := parseRSS(body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 && strings.Contains(query, "when:") {
		// Fall back if the time operator isn't understood.
		i := strings.LastIndex(query, " when:")
		if i > 0 {
			return n.fetchGoogle(ctx, query[:i], r)
		}
	}
	return googleItems(items), nil
}

// fetchPress reads the publications' own feeds, cached for 20 minutes
// because they aren't specific to a query.
func (n *NewsManager) fetchPress(ctx context.Context, fresh bool) ([]*NewsItem, int) {
	n.pressFirst.Lock()
	defer n.pressFirst.Unlock()
	n.mu.Lock()
	age := n.now().Sub(n.pressAt)
	if n.press != nil && (age < 20*time.Minute && !fresh || age < 3*time.Minute) {
		items, ok := n.press, n.pressOK
		n.mu.Unlock()
		return items, ok
	}
	n.mu.Unlock()

	type res struct {
		items []*NewsItem
		ok    bool
	}
	ch := make(chan res, len(pressFeeds))
	for _, f := range pressFeeds {
		go func(f pressFeed) {
			body, err := n.get(ctx, f.URL)
			if err != nil {
				n.logf("press %s: %v", f.Name, err)
				ch <- res{}
				return
			}
			items, err := parseRSS(body)
			if err != nil {
				n.logf("press %s: %v", f.Name, err)
				ch <- res{}
				return
			}
			ch <- res{pressItems(items, f), true}
		}(f)
	}
	var all []*NewsItem
	ok := 0
	for range pressFeeds {
		r := <-ch
		if r.ok {
			ok++
			all = append(all, r.items...)
		}
	}
	if ok > 0 {
		n.mu.Lock()
		n.press, n.pressAt, n.pressOK = all, n.now(), ok
		n.mu.Unlock()
	}
	return all, ok
}

func (n *NewsManager) fetchBing(ctx context.Context, query string, r Region) ([]*NewsItem, error) {
	u := "https://www.bing.com/news/search?q=" + url.QueryEscape(query) + "&qft=" + url.QueryEscape(`sortbydate="1"`) + "&format=rss&setlang=" + r.Market + "&cc=" + r.GL + "&mkt=" + r.Market
	body, err := n.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(body[:min(len(body), 512)]), "<rss") {
		return nil, errors.New("feed unavailable")
	}
	items, err := parseRSS(body)
	if err != nil {
		return nil, err
	}
	return bingItems(items), nil
}

type NewsOptions struct {
	Days        int
	Fresh       bool
	FreshMinAge time.Duration // a "fresh" search reuses news younger than this
	Region      Region
	Google      bool
	Bing        bool
	Press       bool
	Brands      []brandPhrase
}

// Search fetches live launch news for a query, then filters, scores and
// de-duplicates it.
func (n *NewsManager) Search(ctx context.Context, q *Query, o NewsOptions) ([]*NewsItem, NewsStats) {
	googleQs, bingQ := newsQueries(q, o.Days)
	key := strings.Join(googleQs, "|") + "|" + bingQ + "|" + o.Region.Code + fmt.Sprint(o.Google, o.Bing, o.Press)
	now := n.now()
	n.mu.Lock()
	minAge := o.FreshMinAge
	if minAge <= 0 {
		minAge = 45 * time.Second
	}
	if e, ok := n.cache[key]; ok {
		age := now.Sub(e.at)
		if (!o.Fresh && age < 15*time.Minute) || age < minAge {
			n.mu.Unlock()
			return e.items, e.stats
		}
	}
	n.mu.Unlock()

	type result struct {
		feed  string
		items []*NewsItem
		err   error
	}
	var jobs []func() result
	if o.Google {
		for _, gq := range googleQs {
			gq := gq
			jobs = append(jobs, func() result {
				items, err := n.fetchGoogle(ctx, gq, o.Region)
				return result{"Google News", items, err}
			})
		}
	}
	if o.Bing && bingQ != "" {
		jobs = append(jobs, func() result {
			items, err := n.fetchBing(ctx, bingQ, o.Region)
			return result{"Bing News", items, err}
		})
	}
	if o.Press {
		jobs = append(jobs, func() result {
			items, ok := n.fetchPress(ctx, o.Fresh)
			if ok == 0 {
				return result{"Beauty press", nil, errors.New("publication feeds unavailable")}
			}
			return result{"Beauty press", items, nil}
		})
	}
	ch := make(chan result, len(jobs))
	for _, j := range jobs {
		go func(j func() result) { ch <- j() }(j)
	}
	var raw []*NewsItem
	stats := NewsStats{}
	okFeeds, failedFeeds := map[string]bool{}, map[string]bool{}
	for range jobs {
		r := <-ch
		if r.err != nil {
			failedFeeds[r.feed] = true
			n.logf("news %s: %v", r.feed, r.err)
			continue
		}
		okFeeds[r.feed] = true
		raw = append(raw, r.items...)
	}
	for f := range okFeeds {
		stats.Feeds = append(stats.Feeds, f)
	}
	for f := range failedFeeds {
		if !okFeeds[f] {
			stats.Failed = append(stats.Failed, f)
		}
	}
	sort.Strings(stats.Feeds)
	sort.Strings(stats.Failed)
	stats.Raw = len(raw)

	items := rankNews(raw, q, o, now)
	if len(okFeeds) > 0 {
		n.mu.Lock()
		if len(n.cache) > 200 {
			n.cache = map[string]newsEntry{}
		}
		n.cache[key] = newsEntry{at: now, items: items, stats: stats}
		n.mu.Unlock()
	}
	return items, stats
}

func hasAny(ix textIndex, phrases []string) bool {
	for _, p := range phrases {
		if ix.has(p) {
			return true
		}
	}
	return false
}

// newsMakeupWords are product words that mean makeup even in a general
// news headline. Words like "foundation", "powder", "primer", "palette"
// or "brush" are left out because they're common outside beauty.
var newsMakeupWords = []string{
	"lipstick", "lipsticks", "lip gloss", "lip glosses", "lipgloss", "lip oil", "lip oils", "lip balm", "lip liner",
	"lip tint", "lip stain", "lip plumper", "lip kit", "liquid lipstick", "mascara", "mascaras", "eyeliner", "eyeliners",
	"eyeshadow", "eye shadow", "eyeshadows", "eyeshadow palette", "concealer", "concealers", "bronzer", "bronzers",
	"blush", "blushes", "liquid blush", "cream blush", "highlighter", "skin tint", "tinted moisturizer", "tinted serum",
	"setting spray", "setting powder", "bb cream", "cc cream", "false lashes", "lash serum", "brow gel", "brow pencil",
	"cushion foundation", "serum foundation", "foundation shades", "foundation range", "makeup artist", "makeup line",
	"makeup collection", "makeup brand", "makeup launch", "makeup launches", "beauty launch", "beauty launches",
}

func isSpecificMakeupWord(ix textIndex) bool {
	return hasAny(ix, newsMakeupWords)
}

// rankNews keeps launch stories that are about makeup and match the
// query, merges duplicates across feeds and outlets, and orders them.
func rankNews(raw []*NewsItem, q *Query, o NewsOptions, now time.Time) []*NewsItem {
	window := time.Duration(max(o.Days, 7)) * 24 * time.Hour
	if window > 75*24*time.Hour {
		window = 75 * 24 * time.Hour
	}
	var termToks []string
	for _, t := range q.NewsTerms {
		termToks = append(termToks, normalize(t))
	}
	var kept []*NewsItem
	for _, orig := range raw {
		// Work on a copy: raw items can be shared, cached feed stories.
		cp := *orig
		it := &cp
		it.Feeds = append([]string(nil), orig.Feeds...)
		it.AlsoIn = append([]string(nil), orig.AlsoIn...)
		if it.Published.IsZero() || now.Sub(it.Published) > window || it.Published.After(now.Add(6*time.Hour)) {
			continue
		}
		it.titleN = normalize(it.Title)
		tix := newTextIndex(it.titleN)
		full := it.titleN + " " + normalize(it.Snippet)
		fix := newTextIndex(full)

		brands, strong := detectBrands(full, o.Brands)
		specific := isSpecificMakeupWord(fix)
		beauty := specific || strong > 0 || hasAny(fix, beautyContext)
		if !beauty {
			continue
		}
		it.Brands = brands
		if hasAny(fix, makeupNoise) && !specific && len(it.Brands) == 0 {
			continue
		}

		score := 0.0
		if hasAny(tix, launchWords) {
			score += 2
			it.Launch = true
		} else if tix.has("new") || tix.has("newest") || tix.has("latest") {
			score += 1
			it.Launch = true
		}
		if isSpecificMakeupWord(tix) {
			score += 1
		}
		if len(it.Brands) > 0 {
			score += 0.8
		}
		an := analyze(full, "", 0, "")
		it.Highlights = an.Highlights
		if len(it.Highlights) > 3 {
			it.Highlights = it.Highlights[:3]
		}
		score += math.Min(an.Score*0.3, 1)

		if !q.Generic {
			matched := 0
			for _, t := range termToks {
				switch {
				case tix.has(t):
					score += 1.2
					matched++
				case fix.has(t):
					score += 0.5
					matched++
				default:
					// Try singular/plural forms ("lip oil" finds "lip oils").
					for _, v := range phraseVariants(t)[1:] {
						if tix.has(v) {
							score += 1.2
							matched++
							break
						}
						if fix.has(v) {
							score += 0.5
							matched++
							break
						}
					}
				}
			}
			if matched == 0 {
				continue
			}
			if q.BrandName != "" {
				found := false
				for _, b := range it.Brands {
					if b == q.BrandName {
						found = true
					}
				}
				if !found {
					continue
				}
			}
		} else if !it.Launch && !specific {
			continue
		}
		if beautyPublishers[normalize(it.Publisher)] {
			score += 0.7
		}
		age := now.Sub(it.Published).Hours() / 24
		score += 2 * math.Exp(-age/10)
		it.Score = score
		it.sig = significantTokens(it.titleN)
		kept = append(kept, it)
	}

	// Merge the same story from different feeds/outlets.
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Score > kept[j].Score })
	var merged []*NewsItem
	for _, it := range kept {
		dup := false
		for _, m := range merged {
			if m.URL == it.URL || jaccard(m.sig, it.sig) >= 0.6 {
				dup = true
				if m.Image == "" && it.Image != "" {
					m.Image = it.Image
				}
				if m.Snippet == "" && it.Snippet != "" {
					m.Snippet = it.Snippet
				}
				for _, f := range it.Feeds {
					if !containsString(m.Feeds, f) {
						m.Feeds = append(m.Feeds, f)
					}
				}
				if it.Publisher != "" && it.Publisher != m.Publisher && !containsString(m.AlsoIn, it.Publisher) {
					m.AlsoIn = append(m.AlsoIn, it.Publisher)
				}
				break
			}
		}
		if !dup {
			merged = append(merged, it)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Published.After(merged[j].Published) })
	if len(merged) > 40 {
		// Keep the strongest 40, still shown newest first.
		sort.SliceStable(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
		merged = merged[:40]
		sort.SliceStable(merged, func(i, j int) bool { return merged[i].Published.After(merged[j].Published) })
	}
	return merged
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
