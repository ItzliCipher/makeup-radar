package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// TermGroup is one thing the shopper asked for. A product must contain
// at least one of its variants. Variants starting with "~" are weak: they
// only count when the product is in the group's category.
type TermGroup struct {
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // product | attr | keyword | concern
	Cat      string   `json:"cat,omitempty"`
	Variants []string `json:"-"`
}

// Query is the structured reading of what someone typed.
type Query struct {
	Raw          string      `json:"raw"`
	Groups       []TermGroup `json:"groups"`
	Soft         []TermGroup `json:"soft"`
	BrandID      string      `json:"brandId,omitempty"`
	BrandName    string      `json:"brandName,omitempty"`
	BrandAliases []string    `json:"-"`
	Categories   []string    `json:"categories,omitempty"`
	PriceMin     float64     `json:"priceMin,omitempty"`
	PriceMax     float64     `json:"priceMax,omitempty"`
	Days         int         `json:"days,omitempty"`
	NewsTerms    []string    `json:"newsTerms"`
	Generic      bool        `json:"generic"`
}

type termDef struct {
	label    string
	cat      string
	triggers []string
	variants []string
}

var queryProductTerms = []termDef{
	{"Lip oil", CatLips, []string{"lip oil", "lip oils"}, []string{"lip oil", "lip oils"}},
	{"Lip gloss", CatLips, []string{"lip gloss", "lip glosses", "lipgloss", "gloss", "glosses"}, []string{"lip gloss", "lipgloss", "gloss", "glosses", "lip lacquer", "lip shine"}},
	{"Lipstick", CatLips, []string{"lipstick", "lipsticks", "lip stick"}, []string{"lipstick", "lipsticks", "lip stick", "lip color", "lip colour", "liquid lip"}},
	{"Lip liner", CatLips, []string{"lip liner", "lip liners", "lipliner", "lip pencil", "lip pencils"}, []string{"lip liner", "lipliner", "lip pencil", "lip liners"}},
	{"Lip tint", CatLips, []string{"lip tint", "lip tints", "lip stain", "lip stains"}, []string{"lip tint", "lip stain", "lip tints", "lip stains", "lip and cheek tint", "cheek and lip tint", "tinted lip"}},
	{"Lip balm", CatLips, []string{"lip balm", "lip balms", "lip butter", "lip treatment", "lip mask", "lip masks"}, []string{"lip balm", "lip butter", "lip treatment", "lip mask", "lip sleeping mask", "~balm"}},
	{"Lip plumper", CatLips, []string{"lip plumper", "lip plumpers", "plumper", "plumping gloss", "lip plump", "plumping lip"}, []string{"plumper", "plump", "plumping"}},
	{"Tubing mascara", CatEyes, []string{"tubing mascara", "tubing"}, []string{"tubing"}},
	{"Mascara", CatEyes, []string{"mascara", "mascaras"}, []string{"mascara", "mascaras"}},
	{"Eyeliner", CatEyes, []string{"eyeliner", "eye liner", "eyeliners", "liquid liner", "gel liner", "kohl", "eye pencil", "liner"}, []string{"eyeliner", "eye liner", "liquid liner", "gel liner", "kohl", "eye pencil", "~liner"}},
	{"Eyeshadow", CatEyes, []string{"eyeshadow", "eye shadow", "eyeshadows", "shadow", "shadows", "shadow stick", "shadow sticks"}, []string{"eyeshadow", "eye shadow", "eyeshadows", "shadow", "shadows", "shadow stick", "eye palette"}},
	{"Palette", "", []string{"palette", "palettes"}, []string{"palette", "palettes"}},
	{"Lash serum", CatEyes, []string{"lash serum", "lash growth", "lash boosting", "lash serums"}, []string{"lash serum", "lash growth", "lash boosting", "lash enhancing"}},
	{"Lashes", CatEyes, []string{"lashes", "false lashes", "falsies", "lash clusters", "lash extensions", "strip lashes", "lash"}, []string{"lashes", "false lashes", "lash clusters", "falsies", "lash"}},
	{"Brow gel", CatBrows, []string{"brow gel", "brow gels", "brow wax", "brow lamination", "brow glue"}, []string{"brow gel", "brow wax", "brow glue", "lamination", "brow styling"}},
	{"Brow pencil", CatBrows, []string{"brow pencil", "brow pencils", "brow pen", "brow pens"}, []string{"brow pencil", "brow pen", "micro pencil", "brow definer"}},
	{"Blush", CatCheeks, []string{"blush", "blushes", "blusher", "cheek tint", "cheek color", "cheek colour"}, []string{"blush", "blushes", "blusher", "cheek tint", "cheek color", "cheek colour", "cheek stain", "lip and cheek", "cheek and lip"}},
	{"Bronzer", CatCheeks, []string{"bronzer", "bronzers", "bronzing"}, []string{"bronzer", "bronzers", "bronzing", "bronze"}},
	{"Highlighter", CatCheeks, []string{"highlighter", "highlighters", "highlight", "illuminator", "strobe", "strobing"}, []string{"highlighter", "highlighters", "highlight", "illuminator", "illuminating", "luminizer", "strobe", "~glow"}},
	{"Contour", CatCheeks, []string{"contour", "contouring", "sculpt", "sculpting"}, []string{"contour", "sculpt", "sculpting", "sculptor"}},
	{"Skin tint", CatFace, []string{"skin tint", "skin tints", "tinted moisturizer", "tinted moisturiser", "tinted serum", "tinted spf", "tinted sunscreen", "bb cream", "cc cream"}, []string{"skin tint", "skin tints", "tinted moisturizer", "tinted moisturiser", "tinted serum", "tinted spf", "tinted sunscreen", "bb cream", "cc cream", "sheer tint"}},
	{"Foundation", CatFace, []string{"foundation", "foundations"}, []string{"foundation", "foundations"}},
	{"Concealer", CatFace, []string{"concealer", "concealers", "corrector", "correctors", "color corrector", "colour corrector"}, []string{"concealer", "concealers", "corrector", "correcting"}},
	{"Primer", CatFace, []string{"primer", "primers", "pore primer", "grip primer"}, []string{"primer", "primers", "pore filler", "~base"}},
	{"Setting spray", CatFace, []string{"setting spray", "setting sprays", "setting mist", "fixing spray", "finishing spray"}, []string{"setting spray", "setting mist", "fixing spray", "finishing spray"}},
	{"Powder", CatFace, []string{"setting powder", "loose powder", "pressed powder", "finishing powder", "powder", "powders"}, []string{"powder", "powders"}},
	{"Cushion", CatFace, []string{"cushion", "cushions", "cushion foundation", "cushion compact"}, []string{"cushion"}},
	{"Brushes", CatTools, []string{"brush", "brushes", "makeup brush", "makeup brushes", "brush set"}, []string{"brush", "brushes"}},
	{"Sponge", CatTools, []string{"sponge", "sponges", "beauty blender", "beautyblender", "blending sponge", "puff", "powder puff"}, []string{"sponge", "sponges", "blender", "puff"}},
	{"Glitter", "", []string{"glitter", "sparkle", "shimmer", "glittery", "sparkly"}, []string{"glitter", "sparkle", "shimmer", "metallic", "glitters", "sparkly"}},
}

var queryAttrTerms = []termDef{
	{"SPF", "", []string{"spf", "sunscreen", "sun protection", "uv"}, []string{"spf", "broad spectrum", "sunscreen", "sun protection", "uv protection"}},
	{"Waterproof", "", []string{"waterproof", "water resistant", "water proof", "sweatproof", "sweat proof"}, []string{"waterproof", "water resistant", "sweat proof", "sweatproof", "smudge proof", "humidity proof"}},
	{"Long-wear", "", []string{"long wear", "longwear", "long lasting", "long wearing", "24 hour", "24hr", "24h", "all day", "transfer proof", "transfer resistant", "smudge proof"}, []string{"long wear", "longwear", "long lasting", "long wearing", "24 hour", "24hr", "24h", "16 hour", "12 hour", "36 hour", "48 hour", "all day", "transfer proof", "transfer resistant", "smudge proof", "budge proof"}},
	{"Vegan", "", []string{"vegan"}, []string{"vegan"}},
	{"Clean", "", []string{"clean", "non toxic", "nontoxic"}, []string{"clean", "non toxic"}},
	{"Fragrance-free", "", []string{"fragrance free", "unscented", "no fragrance"}, []string{"fragrance free", "unscented"}},
	{"Cruelty-free", "", []string{"cruelty free"}, []string{"cruelty free", "leaping bunny"}},
	{"Matte", "", []string{"matte", "mattifying", "velvet"}, []string{"matte", "mattifying", "velvet"}},
	{"Dewy", "", []string{"dewy", "glowy", "glow", "luminous", "radiant", "glass skin", "glazed"}, []string{"dewy", "glow", "glowy", "luminous", "radiant", "radiance", "glass skin", "glazed", "glossy"}},
	{"Satin", "", []string{"satin"}, []string{"satin"}},
	{"Hydrating", "", []string{"hydrating", "moisturizing", "moisturising", "hydration"}, []string{"hydrating", "hydration", "moisturizing", "moisturising", "hyaluronic", "moisture"}},
	{"Peptides", "", []string{"peptide", "peptides"}, []string{"peptide", "peptides", "biopeptide", "biopeptides"}},
	{"Hyaluronic acid", "", []string{"hyaluronic", "hyaluronic acid"}, []string{"hyaluronic", "hyaluronate"}},
	{"Niacinamide", "", []string{"niacinamide"}, []string{"niacinamide"}},
	{"PDRN", "", []string{"pdrn", "salmon dna", "polynucleotide", "polynucleotides"}, []string{"pdrn", "polynucleotide", "polynucleotides"}},
	{"Skincare-infused", "", []string{"skincare", "skin care", "skincare infused", "skin care infused", "serum", "skincare makeup"}, []string{"skincare", "skin care", "serum", "peptide", "peptides", "hyaluronic", "niacinamide", "ceramide", "ceramides", "squalane"}},
	{"Refillable", "", []string{"refillable", "refill", "refills"}, []string{"refillable", "refill"}},
	{"Cream", "", []string{"cream", "creamy"}, []string{"cream", "creamy", "balm"}},
	{"Liquid", "", []string{"liquid"}, []string{"liquid", "fluid", "serum"}},
	{"Stick", "", []string{"stick", "sticks"}, []string{"stick", "sticks"}},
	{"Jelly", "", []string{"jelly", "jellies"}, []string{"jelly", "jellies", "gel"}},
	{"K-beauty", "", []string{"korean", "k beauty", "kbeauty", "korean beauty"}, []string{"korean", "k beauty", "kbeauty", "korea"}},
	{"J-beauty", "", []string{"japanese", "j beauty", "jbeauty"}, []string{"japanese", "j beauty", "japan"}},
	{"Color-adapting", "", []string{"color changing", "colour changing", "color adapting", "colour adapting", "ph", "color adjusting", "colour adjusting"}, []string{"color changing", "colour changing", "color adapting", "colour adapting", "color adjusting", "colour adjusting", "ph", "adapts"}},
	{"Blurring", "", []string{"blurring", "blur", "soft focus", "airbrush", "filter"}, []string{"blur", "blurring", "blurs", "soft focus", "airbrush", "airbrushed", "filter"}},
	{"Plumping", "", []string{"plumping", "plump"}, []string{"plump", "plumping", "plumper"}},
	{"Mineral", "", []string{"mineral"}, []string{"mineral"}},
	{"Natural look", "", []string{"natural", "no makeup", "no makeup makeup", "skin like", "sheer"}, []string{"natural", "no makeup", "skin like", "sheer", "second skin"}},
	{"Full coverage", "", []string{"full coverage", "high coverage"}, []string{"full coverage", "high coverage", "buildable coverage"}},
}

type concernDef struct {
	label    string
	triggers []string
	boosts   []string
}

var queryConcernTerms = []concernDef{
	{"Dry skin", []string{"dry skin", "dry", "dehydrated skin", "flaky skin"}, []string{"hydrating", "hydration", "moisturizing", "hyaluronic", "dewy", "nourishing", "squalane", "ceramide", "dry skin"}},
	{"Oily skin", []string{"oily skin", "oily", "shine control", "oil control"}, []string{"matte", "mattifying", "oil control", "shine control", "oil absorbing", "oily skin", "pore"}},
	{"Combination skin", []string{"combination skin", "combo skin"}, []string{"balancing", "satin", "natural finish", "combination"}},
	{"Mature skin", []string{"mature skin", "mature", "aging skin", "ageing skin", "over 40", "over 50", "fine lines", "wrinkles"}, []string{"hydrating", "blur", "fine lines", "peptide", "plumping", "radiant", "mature", "firming", "luminous"}},
	{"Sensitive skin", []string{"sensitive skin", "sensitive", "rosacea", "eczema"}, []string{"sensitive", "fragrance free", "hypoallergenic", "soothing", "calming", "dermatologist tested", "centella", "gentle"}},
	{"Acne-prone skin", []string{"acne", "acne prone", "breakouts", "blemish", "blemishes"}, []string{"non comedogenic", "salicylic", "acne", "blemish", "oil free", "breathable"}},
	{"Deep skin tones", []string{"deep skin", "dark skin", "deep skin tones", "dark skin tones", "deep tones", "melanin", "deep skin tone", "dark skin tone"}, []string{"deep", "rich", "shade range", "inclusive", "all skin tones", "melanin"}},
	{"Fair skin", []string{"fair skin", "pale skin", "light skin"}, []string{"fair", "light", "porcelain"}},
	{"Hooded eyes", []string{"hooded eyes", "hooded"}, []string{"smudge proof", "transfer proof", "waterproof", "long wear"}},
	{"Sensitive eyes", []string{"sensitive eyes", "contact lenses", "contacts"}, []string{"ophthalmologist", "sensitive eyes", "contact lens"}},
	{"Short lashes", []string{"thin lashes", "short lashes", "sparse lashes", "straight lashes"}, []string{"lengthening", "volumizing", "lifting", "curl", "tubing", "lash serum"}},
	{"Beginners", []string{"beginner", "beginners", "easy", "foolproof", "for beginners"}, []string{"easy", "buildable", "blendable", "foolproof", "stick", "sheer"}},
	{"Travel", []string{"travel", "travel size", "mini", "minis"}, []string{"mini", "travel", "travel size", "pocket"}},
	{"Hot weather", []string{"humid", "humidity", "summer", "heat", "sweat"}, []string{"humidity proof", "sweat proof", "waterproof", "long wear", "transfer proof", "oil control"}},
}

var broadCategoryWords = map[string]string{
	"lip": CatLips, "lips": CatLips, "lip products": CatLips, "lip makeup": CatLips,
	"eye": CatEyes, "eyes": CatEyes, "eye makeup": CatEyes, "eye products": CatEyes,
	"brow": CatBrows, "brows": CatBrows, "eyebrow": CatBrows, "eyebrows": CatBrows, "brow products": CatBrows,
	"cheek": CatCheeks, "cheeks": CatCheeks, "cheek products": CatCheeks,
	"face": CatFace, "face makeup": CatFace, "complexion": CatFace, "base makeup": CatFace,
	"tools": CatTools, "makeup tools": CatTools,
	"gift set": CatSets, "gift sets": CatSets, "sets": CatSets, "kits": CatSets, "gifts": CatSets, "gift": CatSets,
	"holiday sets": CatSets, "holiday gifts": CatSets, "advent calendar": CatSets, "advent calendars": CatSets, "bundles": CatSets, "set": CatSets,
}

var queryStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "for": true, "with": true, "and": true, "or": true, "to": true, "of": true,
	"in": true, "on": true, "at": true, "by": true, "best": true, "good": true, "top": true, "new": true, "newest": true,
	"latest": true, "recent": true, "recently": true, "most": true, "advanced": true, "innovative": true, "product": true,
	"products": true, "makeup": true, "beauty": true, "cosmetics": true, "cosmetic": true, "launch": true, "launches": true,
	"launched": true, "release": true, "releases": true, "released": true, "drop": true, "drops": true, "dropped": true,
	"me": true, "my": true, "find": true, "show": true, "search": true, "any": true, "all": true, "some": true, "that": true,
	"this": true, "which": true, "what": true, "whats": true, "who": true, "is": true, "are": true, "i": true, "want": true,
	"need": true, "looking": true, "like": true, "items": true, "item": true, "stuff": true, "things": true, "please": true,
	"dollars": true, "dollar": true, "usd": true, "bucks": true, "pounds": true, "gbp": true, "eur": true, "euros": true,
	"trending": true, "viral": true, "hot": true, "popular": true, "tiktok": true, "hyped": true, "brand": true, "brands": true,
	"just": true, "out": true, "coming": true, "upcoming": true, "today": true, "week": true, "month": true, "year": true,
	"so": true, "very": true, "really": true, "great": true, "nice": true, "cool": true, "now": true, "available": true,
}

var multiStopwords = []string{"make up", "just dropped", "new in", "new arrivals", "new arrival", "new launches", "new releases", "right now"}

var (
	rePriceMax   = regexp.MustCompile(`\b(?:under|below|less than|cheaper than|max|up to|upto|within|no more than|at most)\s+(\d{1,4})\b`)
	rePriceMin   = regexp.MustCompile(`\b(?:over|above|more than|at least|min|minimum)\s+(\d{1,4})\b`)
	rePriceRange = regexp.MustCompile(`\b(?:between\s+)?(\d{1,4})\s+(?:to|and)\s+(\d{1,4})\s+(?:dollars|usd|bucks|pounds)\b|\bbetween\s+(\d{1,4})\s+(?:to|and)\s+(\d{1,4})\b`)
	reYear       = regexp.MustCompile(`\b(20[2-3][0-9])\b`)
)

type dictEntry struct {
	kind  string // brand | otherbrand | cat | product | attr | concern | stop
	index int
	name  string
	id    string
}

type queryDictionary struct {
	m        map[string]dictEntry
	maxWords int
}

// queryDict is swapped atomically when the user adds or removes a brand.
var queryDict atomic.Pointer[queryDictionary]

func init() { rebuildQueryDict(nil) }

// rebuildQueryDict indexes every phrase the parser understands. Custom
// brands added in Sources are folded in so they can be searched by name.
func rebuildQueryDict(custom []Brand) {
	d := map[string]dictEntry{}
	add := func(phrase string, e dictEntry) {
		phrase = normalize(phrase)
		if phrase == "" {
			return
		}
		if _, exists := d[phrase]; !exists {
			d[phrase] = e
		}
	}
	for _, s := range multiStopwords {
		add(s, dictEntry{kind: "stop"})
	}
	for _, b := range append(append([]Brand{}, builtinBrands...), custom...) {
		add(b.Name, dictEntry{kind: "brand", id: b.ID, name: b.Name})
		for _, a := range b.Aliases {
			add(a, dictEntry{kind: "brand", id: b.ID, name: b.Name})
		}
	}
	for name, aliases := range otherBrands {
		add(name, dictEntry{kind: "otherbrand", name: name})
		for _, a := range aliases {
			add(a, dictEntry{kind: "otherbrand", name: name})
		}
	}
	for i, t := range queryProductTerms {
		for _, tr := range t.triggers {
			add(tr, dictEntry{kind: "product", index: i})
		}
	}
	for i, t := range queryAttrTerms {
		for _, tr := range t.triggers {
			add(tr, dictEntry{kind: "attr", index: i})
		}
	}
	for i, c := range queryConcernTerms {
		for _, tr := range c.triggers {
			add(tr, dictEntry{kind: "concern", index: i})
		}
	}
	for w, c := range broadCategoryWords {
		add(w, dictEntry{kind: "cat", name: c})
	}
	maxW := 1
	for k := range d {
		if n := strings.Count(k, " ") + 1; n > maxW {
			maxW = n
		}
	}
	queryDict.Store(&queryDictionary{m: d, maxWords: maxW})
}

// brandAliasesByID lets matching recognise a catalog brand by any name.
func brandAliasesByID(id string, all []Brand) []string {
	for _, b := range all {
		if b.ID == id {
			out := []string{normalize(b.Name)}
			for _, a := range b.Aliases {
				out = append(out, normalize(a))
			}
			return uniqueStrings(out)
		}
	}
	return nil
}

func otherBrandAliases(name string) []string {
	out := []string{normalize(name)}
	for _, a := range otherBrands[name] {
		out = append(out, normalize(a))
	}
	return uniqueStrings(out)
}

// ParseQuery reads a free-text search into structured intent.
func ParseQuery(raw string, now time.Time, brands []Brand) *Query {
	q := &Query{Raw: strings.TrimSpace(raw)}
	norm := normalize(q.Raw)

	// Prices first, before numbers get mistaken for keywords.
	if m := rePriceRange.FindStringSubmatch(norm); m != nil {
		a, b := m[1], m[2]
		if a == "" {
			a, b = m[3], m[4]
		}
		lo, _ := strconv.Atoi(a)
		hi, _ := strconv.Atoi(b)
		if lo > hi {
			lo, hi = hi, lo
		}
		if hi > 0 {
			q.PriceMin, q.PriceMax = float64(lo), float64(hi)
			norm = strings.Replace(norm, m[0], " ", 1)
		}
	}
	if m := rePriceMax.FindStringSubmatch(norm); m != nil {
		if v, _ := strconv.Atoi(m[1]); v > 0 {
			q.PriceMax = float64(v)
			norm = strings.Replace(norm, m[0], " ", 1)
		}
	}
	if m := rePriceMin.FindStringSubmatch(norm); m != nil {
		if v, _ := strconv.Atoi(m[1]); v > 0 && !strings.HasPrefix(m[0], "over 40") && !strings.HasPrefix(m[0], "over 50") {
			q.PriceMin = float64(v)
			norm = strings.Replace(norm, m[0], " ", 1)
		}
	}
	pn := padded(norm)
	for _, w := range []string{"drugstore", "affordable", "cheap", "budget", "inexpensive", "low cost"} {
		if containsPhrase(pn, w) {
			if q.PriceMax == 0 {
				q.PriceMax = 20
			}
			norm = strings.TrimSpace(strings.ReplaceAll(padded(norm), " "+w+" ", " "))
			pn = padded(norm)
		}
	}
	for _, w := range []string{"luxury", "high end", "splurge", "prestige", "designer"} {
		if containsPhrase(pn, w) {
			if q.PriceMin == 0 {
				q.PriceMin = 40
			}
			norm = strings.TrimSpace(strings.ReplaceAll(padded(norm), " "+w+" ", " "))
			pn = padded(norm)
		}
	}

	// Time words.
	timeWords := []struct {
		phrase string
		days   int
	}{
		{"today", 2}, {"this week", 7}, {"past week", 7}, {"last week", 7}, {"this month", 31}, {"past month", 31},
		{"last month", 31}, {"last 30 days", 30}, {"past 30 days", 30}, {"this season", 92}, {"this year", 366},
		{"past year", 366}, {"last year", 366},
	}
	for _, tw := range timeWords {
		if containsPhrase(pn, tw.phrase) {
			if q.Days == 0 || tw.days < q.Days {
				q.Days = tw.days
			}
			norm = strings.TrimSpace(strings.ReplaceAll(padded(norm), " "+tw.phrase+" ", " "))
			pn = padded(norm)
		}
	}
	if m := reYear.FindStringSubmatch(norm); m != nil {
		y, _ := strconv.Atoi(m[1])
		if y == now.Year() {
			start := time.Date(y, 1, 1, 0, 0, 0, 0, now.Location())
			if d := int(now.Sub(start).Hours()/24) + 1; d > 0 && (q.Days == 0 || d < q.Days) {
				q.Days = max(d, 30)
			}
		}
		norm = strings.Replace(norm, m[1], " ", 1)
	}

	dict := queryDict.Load()
	toks := tokens(normalize(norm))
	catSet := map[string]bool{}
	seenGroup := map[string]bool{}
	var newsTerms []string
	for i := 0; i < len(toks); {
		matched := false
		for n := min(dict.maxWords, len(toks)-i); n >= 1; n-- {
			phrase := strings.Join(toks[i:i+n], " ")
			e, ok := dict.m[phrase]
			if !ok {
				continue
			}
			matched = true
			i += n
			switch e.kind {
			case "brand":
				if q.BrandName == "" {
					q.BrandID, q.BrandName = e.id, e.name
					q.BrandAliases = brandAliasesByID(e.id, brands)
					newsTerms = append(newsTerms, e.name)
				}
			case "otherbrand":
				if q.BrandName == "" {
					q.BrandName = e.name
					q.BrandAliases = otherBrandAliases(e.name)
					newsTerms = append(newsTerms, e.name)
				}
			case "cat":
				if !catSet[e.name] {
					catSet[e.name] = true
					q.Categories = append(q.Categories, e.name)
					if e.name != CatSets && e.name != CatTools {
						newsTerms = append(newsTerms, phrase)
					} else if e.name == CatSets {
						newsTerms = append(newsTerms, "gift set")
					}
				}
			case "product":
				t := queryProductTerms[e.index]
				if !seenGroup["p"+t.label] {
					seenGroup["p"+t.label] = true
					q.Groups = append(q.Groups, TermGroup{Label: t.label, Kind: "product", Cat: t.cat, Variants: t.variants})
					newsTerms = append(newsTerms, phrase)
				}
			case "attr":
				t := queryAttrTerms[e.index]
				if !seenGroup["a"+t.label] {
					seenGroup["a"+t.label] = true
					q.Groups = append(q.Groups, TermGroup{Label: t.label, Kind: "attr", Variants: t.variants})
					newsTerms = append(newsTerms, phrase)
				}
			case "concern":
				c := queryConcernTerms[e.index]
				if !seenGroup["c"+c.label] {
					seenGroup["c"+c.label] = true
					q.Soft = append(q.Soft, TermGroup{Label: c.label, Kind: "concern", Variants: c.boosts})
				}
			}
			break
		}
		if matched {
			continue
		}
		w := toks[i]
		i++
		if queryStopwords[w] || len(w) < 2 {
			continue
		}
		if _, err := strconv.Atoi(w); err == nil {
			continue // stray numbers rarely help a text match
		}
		if !seenGroup["k"+w] {
			seenGroup["k"+w] = true
			q.Groups = append(q.Groups, TermGroup{Label: w, Kind: "keyword", Variants: wordVariants(w)})
			newsTerms = append(newsTerms, w)
		}
	}
	sort.Strings(q.Categories)
	q.NewsTerms = uniqueStrings(newsTerms)
	q.Generic = len(q.Groups) == 0 && q.BrandName == "" && len(q.Categories) == 0
	return q
}

// Chips describes the parsed query for the interface.
func (q *Query) Chips() []map[string]string {
	var out []map[string]string
	if q.BrandName != "" {
		out = append(out, map[string]string{"type": "brand", "label": q.BrandName})
	}
	for _, c := range q.Categories {
		out = append(out, map[string]string{"type": "category", "label": categoryLabels[c]})
	}
	for _, g := range q.Groups {
		out = append(out, map[string]string{"type": g.Kind, "label": titleCase(g.Label)})
	}
	for _, g := range q.Soft {
		out = append(out, map[string]string{"type": "concern", "label": "For " + strings.ToLower(g.Label)})
	}
	switch {
	case q.PriceMin > 0 && q.PriceMax > 0:
		out = append(out, map[string]string{"type": "price", "label": "$" + strconv.Itoa(int(q.PriceMin)) + "–$" + strconv.Itoa(int(q.PriceMax))})
	case q.PriceMax > 0:
		out = append(out, map[string]string{"type": "price", "label": "Under $" + strconv.Itoa(int(q.PriceMax))})
	case q.PriceMin > 0:
		out = append(out, map[string]string{"type": "price", "label": "Over $" + strconv.Itoa(int(q.PriceMin))})
	}
	if q.Days > 0 {
		out = append(out, map[string]string{"type": "time", "label": "Last " + strconv.Itoa(q.Days) + " days"})
	}
	return out
}
