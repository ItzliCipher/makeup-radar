package main

import (
	"math"
	"sort"
	"strings"
	"time"
)

// ProductCard is the compact product shape the results grid renders.
type ProductCard struct {
	ID         string      `json:"id"`
	Brand      string      `json:"brand"`
	Source     string      `json:"source"`
	SourceKind string      `json:"sourceKind"`
	Title      string      `json:"title"`
	URL        string      `json:"url"`
	Image      string      `json:"image,omitempty"`
	Price      float64     `json:"price"`
	PriceMax   float64     `json:"priceMax,omitempty"`
	CompareAt  float64     `json:"compareAt,omitempty"`
	Currency   string      `json:"currency"`
	Shades     int         `json:"shades,omitempty"`
	Available  bool        `json:"available"`
	Category   string      `json:"category"`
	Kind       string      `json:"kind,omitempty"`
	IsSet      bool        `json:"isSet,omitempty"`
	NewDate    time.Time   `json:"newDate"`
	AgeDays    float64     `json:"ageDays"`
	Highlights []Highlight `json:"highlights,omitempty"`
	Innovation float64     `json:"innovation"`
	Attributes []string    `json:"attributes,omitempty"`
	FlaggedNew bool        `json:"flaggedNew,omitempty"`
	InNews     []NewsRef   `json:"inNews,omitempty"`
	Score      float64     `json:"score"`
}

type NewsRef struct {
	Title     string    `json:"title"`
	Publisher string    `json:"publisher"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
}

type HighlightDetail struct {
	Label  string `json:"label"`
	Group  string `json:"group"`
	Detail string `json:"detail"`
}

// ProductDetail adds everything the detail panel shows.
type ProductDetail struct {
	ProductCard
	Description      string            `json:"description"`
	Images           []string          `json:"images"`
	ShadeNames       []string          `json:"shadeNames,omitempty"`
	Type             string            `json:"type,omitempty"`
	PublishedAt      time.Time         `json:"publishedAt"`
	CreatedAt        time.Time         `json:"createdAt"`
	HighlightDetails []HighlightDetail `json:"highlightDetails"`
}

func toCard(p *Product, now time.Time) *ProductCard {
	age := now.Sub(p.NewDate).Hours() / 24
	if age < 0 {
		age = 0
	}
	hs := p.Highlights
	if len(hs) > 4 {
		hs = hs[:4]
	}
	return &ProductCard{
		ID: p.ID, Brand: p.Brand, Source: p.Source, SourceKind: p.SourceKind, Title: p.Title, URL: p.URL,
		Image: p.Image, Price: p.Price, PriceMax: p.PriceMax, CompareAt: p.CompareAt, Currency: p.Currency,
		Shades: p.Shades, Available: p.Available, Category: p.Category, Kind: p.Kind, IsSet: p.IsSet,
		NewDate: p.NewDate, AgeDays: math.Round(age*10) / 10, Highlights: hs, Innovation: math.Round(p.Innovation*10) / 10,
		Attributes: p.Attributes, FlaggedNew: p.FlaggedNew,
	}
}

func toDetail(p *Product, now time.Time) *ProductDetail {
	d := &ProductDetail{
		ProductCard: *toCard(p, now), Description: p.Description, Images: p.Images, ShadeNames: p.ShadeNames,
		Type: p.Type, PublishedAt: p.PublishedAt, CreatedAt: p.CreatedAt,
	}
	d.Highlights = p.Highlights
	for _, h := range p.Highlights {
		d.HighlightDetails = append(d.HighlightDetails, HighlightDetail{Label: h.Label, Group: h.Group, Detail: techDetails[h.Key]})
	}
	if len(d.Images) == 0 && p.Image != "" {
		d.Images = []string{p.Image}
	}
	return d
}

type SearchOptions struct {
	Sort     string // top | new | advanced | price
	Days     int
	Cat      string
	Brand    string
	HideSets bool
	Limit    int
}

type BrandCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ProductMeta struct {
	Total          int            `json:"total"`
	Shown          int            `json:"shown"`
	Days           int            `json:"days"`
	RequestedDays  int            `json:"requestedDays"`
	Expanded       bool           `json:"expanded"`
	Tier           int            `json:"tier"`
	Relaxed        []string       `json:"relaxed,omitempty"`
	Categories     map[string]int `json:"categories"`
	Brands         []BrandCount   `json:"brands"`
	BrandNoCatalog bool           `json:"brandNoCatalog,omitempty"`
	BrandOffline   bool           `json:"brandOffline,omitempty"`
}

func productIsBrand(p *Product, q *Query) bool {
	if q.BrandID != "" && p.BrandID == q.BrandID {
		return true
	}
	vendor := padded(normalize(p.Brand))
	for _, a := range q.BrandAliases {
		if weakAliases[a] && strings.Count(a, " ") == 0 && normalize(p.Brand) != a {
			continue
		}
		if containsPhrase(vendor, a) {
			return true
		}
	}
	return false
}

func inCats(p *Product, cats []string) bool {
	for _, c := range cats {
		if p.Category == c || p.BaseCategory == c {
			return true
		}
	}
	return false
}

// groupWeight scores how well a product matches one requested term:
// title beats product type, then brand, tags and description.
func groupWeight(p *Product, g TermGroup) float64 {
	best := 0.0
	crossCat := g.Kind == "product" && g.Cat != "" && p.BaseCategory != g.Cat && p.BaseCategory != CatOther
	for _, v := range g.Variants {
		weak := strings.HasPrefix(v, "~")
		if weak {
			v = v[1:]
			if g.Cat != "" && p.BaseCategory != g.Cat {
				continue
			}
		}
		// A one-word product term inside another category's product is
		// usually a shade name ("Blush" lip gloss), not what was asked for.
		if crossCat && !strings.Contains(v, " ") {
			continue
		}
		w := 0.0
		switch {
		case containsPhrase(p.titleP, v):
			w = 3
		case containsPhrase(p.typeP, v):
			w = 2.5
		case g.Kind == "keyword" && containsPhrase(p.brandP, v):
			w = 2
		case containsPhrase(p.tagsP, v):
			w = 1.2
		case containsPhrase(p.descP, v):
			w = 0.8
		}
		if weak {
			w *= 0.6
		}
		if w > best {
			best = w
		}
	}
	return best
}

func matchGroups(p *Product, groups []TermGroup) (full, core, prod bool, score float64) {
	full, core, prod = true, true, true
	for _, g := range groups {
		if w := groupWeight(p, g); w > 0 {
			score += w
			continue
		}
		full = false
		switch g.Kind {
		case "product":
			core, prod = false, false
		case "keyword":
			core = false
		}
	}
	return
}

func softBoost(p *Product, soft []TermGroup) float64 {
	total := 0.0
	for _, g := range soft {
		for _, v := range g.Variants {
			if containsPhrase(p.titleP, v) || containsPhrase(p.typeP, v) || containsPhrase(p.tagsP, v) || containsPhrase(p.descP, v) {
				total += 0.4
				break
			}
		}
	}
	return math.Min(total, 1)
}

var windowSteps = []int{7, 30, 90, 180, 365}

func nextWindow(d int) int {
	for _, s := range windowSteps {
		if s > d {
			return s
		}
	}
	return d
}

type candidate struct {
	p          *Product
	full, core bool
	prod       bool
	match      float64
	soft       float64
}

// searchProducts filters, matches and ranks catalog products.
func searchProducts(all []*Product, q *Query, o SearchOptions, now time.Time) ([]*ProductCard, ProductMeta) {
	meta := ProductMeta{Categories: map[string]int{}}
	days := o.Days
	if q.Days > 0 {
		days = q.Days
	}
	if days <= 0 {
		days = 90
	}
	meta.RequestedDays = days

	var cands []candidate
	brandHit := false
	for _, p := range all {
		if q.PriceMax > 0 && p.Price > q.PriceMax {
			continue
		}
		if q.PriceMin > 0 && p.Price < q.PriceMin {
			continue
		}
		if q.BrandName != "" {
			if !productIsBrand(p, q) {
				continue
			}
			brandHit = true
		}
		if len(q.Categories) > 0 && !inCats(p, q.Categories) {
			continue
		}
		c := candidate{p: p}
		c.full, c.core, c.prod, c.match = matchGroups(p, q.Groups)
		c.soft = softBoost(p, q.Soft)
		cands = append(cands, c)
	}
	if q.BrandName != "" && !brandHit && q.BrandID == "" {
		meta.BrandNoCatalog = true
	}

	within := func(p *Product, d int) bool { return now.Sub(p.NewDate) <= time.Duration(d)*24*time.Hour }
	// Prefer exact matches; relax optional words only when nothing
	// matches exactly, and widen the time window when results are sparse.
	pick := func(d int) (tier, count int) {
		nFull, nCore, nProd := 0, 0, 0
		for _, c := range cands {
			if !within(c.p, d) {
				continue
			}
			if c.full {
				nFull++
			}
			if c.core {
				nCore++
			}
			if c.prod {
				nProd++
			}
		}
		// Relaxing only makes sense when something concrete remains: a
		// product or keyword for tier 2, a product type for tier 3.
		hasProd, hasCore := false, false
		for _, g := range q.Groups {
			if g.Kind == "product" {
				hasProd, hasCore = true, true
			}
			if g.Kind == "keyword" {
				hasCore = true
			}
		}
		switch {
		case nFull > 0 || len(q.Groups) == 0:
			return 1, nFull
		case nCore > 0 && hasCore:
			return 2, nCore
		case nProd > 0 && hasProd:
			return 3, nProd
		}
		return 1, 0
	}
	tier, count := pick(days)
	for count < 6 && days < 365 {
		days = nextWindow(days)
		tier, count = pick(days)
		meta.Expanded = true
	}
	if count == 0 && meta.Expanded {
		days, meta.Expanded = meta.RequestedDays, false
		tier, _ = pick(days)
	}
	meta.Days, meta.Tier = days, tier
	if tier >= 2 {
		for _, g := range q.Groups {
			if g.Kind == "attr" || (tier == 3 && g.Kind == "keyword") {
				meta.Relaxed = append(meta.Relaxed, titleCase(g.Label))
			}
		}
	}

	var pool []candidate
	maxMatch := 0.0
	for _, c := range cands {
		if !within(c.p, days) {
			continue
		}
		ok := (tier == 1 && c.full) || (tier == 2 && c.core) || (tier == 3 && c.prod)
		if !ok {
			continue
		}
		pool = append(pool, c)
		if c.match > maxMatch {
			maxMatch = c.match
		}
	}

	// De-duplicate the same product listed by the brand and by a shop.
	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i].p, pool[j].p
		if (a.SourceKind == "brand") != (b.SourceKind == "brand") {
			return a.SourceKind == "brand"
		}
		return a.NewDate.After(b.NewDate)
	})
	seen := map[string]bool{}
	dedup := pool[:0]
	for _, c := range pool {
		k := normalize(c.p.Brand) + "|" + normalize(c.p.Title)
		if seen[k] {
			continue
		}
		seen[k] = true
		dedup = append(dedup, c)
	}
	pool = dedup

	for _, c := range pool {
		if o.HideSets && c.p.IsSet {
			continue
		}
		meta.Categories[c.p.Category]++
	}

	brandCounts := map[string]int{}
	var cards []*ProductCard
	for _, c := range pool {
		p := c.p
		if o.HideSets && p.IsSet {
			continue
		}
		if o.Cat != "" && o.Cat != "all" && p.Category != o.Cat {
			continue
		}
		brandCounts[p.Brand]++
		if o.Brand != "" && p.Brand != o.Brand {
			continue
		}
		card := toCard(p, now)
		r := math.Exp(-card.AgeDays / 45)
		inn := math.Min(p.Innovation/5, 1)
		m := 0.0
		if maxMatch > 0 {
			m = c.match / maxMatch
		} else {
			m = 1
		}
		score := 0.45*r + 0.30*inn + 0.15*m + 0.10*c.soft
		if p.FlaggedNew {
			score += 0.03
		}
		if !p.Available {
			score -= 0.02
		}
		if p.IsSet {
			score -= 0.05
		}
		card.Score = score
		cards = append(cards, card)
	}
	for name, n := range brandCounts {
		meta.Brands = append(meta.Brands, BrandCount{name, n})
	}
	sort.Slice(meta.Brands, func(i, j int) bool {
		if meta.Brands[i].Count != meta.Brands[j].Count {
			return meta.Brands[i].Count > meta.Brands[j].Count
		}
		return meta.Brands[i].Name < meta.Brands[j].Name
	})
	meta.Total = len(cards)
	sortCards(cards, o.Sort)
	limit := o.Limit
	if limit <= 0 {
		limit = 240
	}
	if len(cards) > limit {
		cards = cards[:limit]
	}
	meta.Shown = len(cards)
	return cards, meta
}

func sortCards(cards []*ProductCard, mode string) {
	switch mode {
	case "new":
		sort.SliceStable(cards, func(i, j int) bool {
			if !cards[i].NewDate.Equal(cards[j].NewDate) {
				return cards[i].NewDate.After(cards[j].NewDate)
			}
			return cards[i].Score > cards[j].Score
		})
	case "advanced":
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].Innovation != cards[j].Innovation {
				return cards[i].Innovation > cards[j].Innovation
			}
			return cards[i].NewDate.After(cards[j].NewDate)
		})
	case "price":
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].Price != cards[j].Price {
				return cards[i].Price < cards[j].Price
			}
			return cards[i].NewDate.After(cards[j].NewDate)
		})
	default:
		sort.SliceStable(cards, func(i, j int) bool { return cards[i].Score > cards[j].Score })
		diversify(cards)
	}
}

// diversify gently spreads brands out near the top so one big launch
// doesn't fill the whole first screen.
func diversify(cards []*ProductCard) {
	n := min(len(cards), 72)
	if n < 3 {
		return
	}
	head := append([]*ProductCard(nil), cards[:n]...)
	used := map[string]int{}
	out := make([]*ProductCard, 0, n)
	for len(head) > 0 {
		bi, bs := 0, math.Inf(-1)
		for i, c := range head {
			s := c.Score - 0.05*float64(used[c.Brand])
			if s > bs {
				bi, bs = i, s
			}
		}
		c := head[bi]
		used[c.Brand]++
		out = append(out, c)
		head = append(head[:bi], head[bi+1:]...)
	}
	copy(cards, out)
}

var genericTitleWords = func() map[string]bool {
	m := map[string]bool{}
	for _, v := range productVocab {
		if !strings.Contains(v.phrase, " ") {
			m[v.phrase] = true
		}
	}
	for _, w := range []string{"liquid", "cream", "matte", "glow", "mini", "set", "collection", "edition", "limited", "shade",
		"color", "colour", "duo", "kit", "spf", "soft", "stick", "balm", "oil", "tint", "skin", "tinted", "pressed", "loose",
		"jelly", "plumping", "plump", "peptide", "treatment", "butter", "mousse", "pigment", "glaze", "satin", "velvet",
		"shine", "luminous", "radiant", "dewy", "sheer", "hydrating", "long", "wear", "waterproof", "volume", "gel",
		"pencil", "spray", "setting", "serum", "makeup", "beauty", "launch", "launches", "holiday", "pro", "super", "ultra",
		"perfect", "natural", "nude", "glossy", "blur", "blurring", "multi", "pocket", "travel", "size", "refill", "refillable"} {
		m[w] = true
	}
	return m
}()

// crossReference links products to launch stories that name them.
func crossReference(cards []*ProductCard, news []*NewsItem) {
	byBrand := map[string][]*NewsItem{}
	for _, n := range news {
		for _, b := range n.Brands {
			k := normalize(b)
			byBrand[k] = append(byBrand[k], n)
		}
	}
	if len(byBrand) == 0 {
		return
	}
	for _, c := range cards {
		c.InNews = nil
		items := byBrand[normalize(c.Brand)]
		if len(items) == 0 {
			continue
		}
		brandToks := significantTokens(normalize(c.Brand))
		ptoks := significantTokens(normalize(c.Title))
		for t := range brandToks {
			delete(ptoks, t)
		}
		if len(ptoks) == 0 {
			continue
		}
		for _, n := range items {
			sig := n.sig
			if sig == nil {
				sig = significantTokens(normalize(n.Title))
			}
			overlap, distinctive := 0, false
			for t := range ptoks {
				if sig[t] {
					overlap++
					if !genericTitleWords[t] && len(t) >= 4 {
						distinctive = true
					}
				}
			}
			if distinctive && (overlap >= 2 || len(ptoks) <= 2) {
				c.InNews = append(c.InNews, NewsRef{Title: n.Title, Publisher: n.Publisher, URL: n.URL, Published: n.Published})
				if len(c.InNews) >= 3 {
					break
				}
			}
		}
		if len(c.InNews) > 0 {
			c.Score += 0.12
		}
	}
}
