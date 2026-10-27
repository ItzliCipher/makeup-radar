package main

import (
	"sort"
	"strconv"
	"strings"
)

// Category keys used across the app.
const (
	CatFace   = "face"
	CatCheeks = "cheeks"
	CatEyes   = "eyes"
	CatBrows  = "brows"
	CatLips   = "lips"
	CatTools  = "tools"
	CatSets   = "sets"
	CatOther  = "other"
)

var categoryOrder = []string{CatFace, CatCheeks, CatEyes, CatBrows, CatLips, CatTools, CatSets, CatOther}

var categoryLabels = map[string]string{
	CatFace: "Face", CatCheeks: "Cheeks", CatEyes: "Eyes", CatBrows: "Brows",
	CatLips: "Lips", CatTools: "Tools", CatSets: "Sets & gifts", CatOther: "Other",
}

type vocabEntry struct {
	phrase string
	label  string
	cat    string
	weight float64 // >= 0.8 is a specific product word; below that is generic
}

// productVocab maps product words to a category and a display label.
var productVocab = []vocabEntry{
	// Lips
	{"liquid lipstick", "Liquid lipstick", CatLips, 1.3}, {"lipstick", "Lipstick", CatLips, 1.2}, {"lipsticks", "Lipstick", CatLips, 1.2},
	{"lip stick", "Lipstick", CatLips, 1.2}, {"lip gloss", "Lip gloss", CatLips, 1.2}, {"lipgloss", "Lip gloss", CatLips, 1.2},
	{"gloss", "Gloss", CatLips, 0.9}, {"glosses", "Gloss", CatLips, 0.9}, {"lip oil", "Lip oil", CatLips, 1.3}, {"lip oils", "Lip oil", CatLips, 1.3},
	{"lip balm", "Lip balm", CatLips, 1.2}, {"lip butter", "Lip balm", CatLips, 1.2}, {"lip treatment", "Lip treatment", CatLips, 1.2},
	{"lip mask", "Lip mask", CatLips, 1.2}, {"lip sleeping mask", "Lip mask", CatLips, 1.3}, {"lip liner", "Lip liner", CatLips, 1.3},
	{"lipliner", "Lip liner", CatLips, 1.3}, {"lip pencil", "Lip liner", CatLips, 1.2}, {"lip tint", "Lip tint", CatLips, 1.3},
	{"lip stain", "Lip stain", CatLips, 1.3}, {"lip plumper", "Lip plumper", CatLips, 1.3}, {"plumper", "Lip plumper", CatLips, 0.9},
	{"lip lacquer", "Lip lacquer", CatLips, 1.2}, {"lip cream", "Lip cream", CatLips, 1.2}, {"liquid lip", "Liquid lip", CatLips, 1.2},
	{"lip color", "Lip color", CatLips, 1.1}, {"lip colour", "Lip color", CatLips, 1.1}, {"lip crayon", "Lip crayon", CatLips, 1.2},
	{"lip jelly", "Lip jelly", CatLips, 1.2}, {"lip glaze", "Lip glaze", CatLips, 1.2}, {"lip serum", "Lip serum", CatLips, 1.2},
	{"lip shine", "Lip shine", CatLips, 1.2}, {"lip topper", "Lip topper", CatLips, 1.2}, {"lip kit", "Lip kit", CatLips, 1.2},
	{"lip duo", "Lip duo", CatLips, 1.1}, {"lip blush", "Lip blush", CatLips, 1.2}, {"lip mousse", "Lip mousse", CatLips, 1.2},
	{"lip", "Lip", CatLips, 0.6}, {"lips", "Lip", CatLips, 0.6}, {"lippie", "Lip", CatLips, 0.8},
	// Eyes
	{"mascara", "Mascara", CatEyes, 1.3}, {"mascaras", "Mascara", CatEyes, 1.3}, {"lash primer", "Lash primer", CatEyes, 1.2},
	{"eyeliner", "Eyeliner", CatEyes, 1.3}, {"eye liner", "Eyeliner", CatEyes, 1.3}, {"liquid liner", "Liquid liner", CatEyes, 1.3},
	{"gel liner", "Gel liner", CatEyes, 1.3}, {"kohl", "Kohl liner", CatEyes, 1.2}, {"eye pencil", "Eye pencil", CatEyes, 1.2},
	{"eyeshadow palette", "Eyeshadow palette", CatEyes, 1.4}, {"eyeshadow", "Eyeshadow", CatEyes, 1.3}, {"eye shadow", "Eyeshadow", CatEyes, 1.3},
	{"eyeshadows", "Eyeshadow", CatEyes, 1.3}, {"shadow stick", "Shadow stick", CatEyes, 1.3}, {"eye palette", "Eyeshadow palette", CatEyes, 1.3},
	{"shadow palette", "Eyeshadow palette", CatEyes, 1.3}, {"liquid shadow", "Liquid shadow", CatEyes, 1.3}, {"shadow", "Eyeshadow", CatEyes, 0.8},
	{"eye primer", "Eye primer", CatEyes, 1.2}, {"eye gloss", "Eye gloss", CatEyes, 1.2}, {"glitter", "Glitter", CatEyes, 0.6},
	{"false lashes", "False lashes", CatEyes, 1.3}, {"lash clusters", "Lash clusters", CatEyes, 1.3}, {"lashes", "Lashes", CatEyes, 1.0},
	{"lash", "Lash", CatEyes, 0.8}, {"lash serum", "Lash serum", CatEyes, 1.3}, {"lash glue", "Lash glue", CatEyes, 1.0},
	{"liner", "Liner", CatEyes, 0.6}, {"palette", "Palette", CatEyes, 0.8}, {"eye", "Eye", CatEyes, 0.5}, {"eyes", "Eye", CatEyes, 0.5},
	{"face palette", "Face palette", CatCheeks, 1.4}, {"blush palette", "Blush palette", CatCheeks, 1.5}, {"contour palette", "Contour palette", CatCheeks, 1.5},
	{"highlighter palette", "Highlighter palette", CatCheeks, 1.5}, {"bronzer palette", "Bronzer palette", CatCheeks, 1.5}, {"lip palette", "Lip palette", CatLips, 1.4},
	{"concealer palette", "Concealer palette", CatFace, 1.5},
	// Brows
	{"brow gel", "Brow gel", CatBrows, 1.4}, {"brow pencil", "Brow pencil", CatBrows, 1.4}, {"brow pomade", "Brow pomade", CatBrows, 1.4},
	{"pomade", "Brow pomade", CatBrows, 0.8}, {"brow wax", "Brow wax", CatBrows, 1.4}, {"brow serum", "Brow serum", CatBrows, 1.4},
	{"brow powder", "Brow powder", CatBrows, 1.4}, {"brow pen", "Brow pen", CatBrows, 1.4}, {"brow", "Brow", CatBrows, 1.0},
	{"brows", "Brow", CatBrows, 1.0}, {"eyebrow", "Brow", CatBrows, 1.0}, {"eyebrows", "Brow", CatBrows, 1.0},
	// Cheeks
	{"liquid blush", "Liquid blush", CatCheeks, 1.4}, {"cream blush", "Cream blush", CatCheeks, 1.4}, {"powder blush", "Powder blush", CatCheeks, 1.4},
	{"blush", "Blush", CatCheeks, 1.3}, {"blushes", "Blush", CatCheeks, 1.3}, {"blusher", "Blush", CatCheeks, 1.3},
	{"bronzer", "Bronzer", CatCheeks, 1.3}, {"bronzers", "Bronzer", CatCheeks, 1.3}, {"bronzing", "Bronzer", CatCheeks, 0.9},
	{"highlighter", "Highlighter", CatCheeks, 1.3}, {"highlighters", "Highlighter", CatCheeks, 1.3}, {"highlight", "Highlighter", CatCheeks, 0.8},
	{"illuminator", "Illuminator", CatCheeks, 1.2}, {"luminizer", "Luminizer", CatCheeks, 1.2}, {"contour", "Contour", CatCheeks, 1.2},
	{"sculpt", "Contour", CatCheeks, 0.8}, {"sculpting", "Contour", CatCheeks, 0.8}, {"cheek tint", "Cheek tint", CatCheeks, 1.3},
	{"cheek", "Cheek", CatCheeks, 0.6}, {"cheeks", "Cheek", CatCheeks, 0.6},
	// Face
	{"serum foundation", "Serum foundation", CatFace, 1.5}, {"foundation stick", "Foundation stick", CatFace, 1.5},
	{"stick foundation", "Foundation stick", CatFace, 1.5}, {"powder foundation", "Powder foundation", CatFace, 1.5},
	{"foundation", "Foundation", CatFace, 1.4}, {"foundations", "Foundation", CatFace, 1.4}, {"concealer", "Concealer", CatFace, 1.4},
	{"concealers", "Concealer", CatFace, 1.4}, {"color corrector", "Color corrector", CatFace, 1.3}, {"colour corrector", "Color corrector", CatFace, 1.3},
	{"corrector", "Corrector", CatFace, 1.1}, {"skin tint", "Skin tint", CatFace, 1.4}, {"tinted moisturizer", "Tinted moisturizer", CatFace, 1.4},
	{"tinted moisturiser", "Tinted moisturizer", CatFace, 1.4}, {"tinted serum", "Tinted serum", CatFace, 1.4}, {"tinted spf", "Tinted SPF", CatFace, 1.3},
	{"tinted sunscreen", "Tinted SPF", CatFace, 1.3}, {"bb cream", "BB cream", CatFace, 1.3}, {"cc cream", "CC cream", CatFace, 1.3},
	{"cushion", "Cushion", CatFace, 1.1}, {"primer", "Primer", CatFace, 1.2}, {"primers", "Primer", CatFace, 1.2},
	{"setting powder", "Setting powder", CatFace, 1.4}, {"loose powder", "Loose powder", CatFace, 1.4}, {"pressed powder", "Pressed powder", CatFace, 1.4},
	{"finishing powder", "Finishing powder", CatFace, 1.4}, {"powder", "Powder", CatFace, 0.8}, {"setting spray", "Setting spray", CatFace, 1.4},
	{"setting mist", "Setting spray", CatFace, 1.4}, {"fixing spray", "Setting spray", CatFace, 1.4}, {"pore filler", "Pore primer", CatFace, 1.2},
	{"skin perfector", "Skin perfector", CatFace, 1.2}, {"complexion", "Complexion", CatFace, 0.6}, {"face", "Face", CatFace, 0.4},
	// Tools
	{"brush set", "Brush set", CatTools, 1.2}, {"brush", "Brush", CatTools, 1.0}, {"brushes", "Brushes", CatTools, 1.0},
	{"sponge", "Sponge", CatTools, 1.0}, {"blender", "Sponge", CatTools, 0.8}, {"powder puff", "Puff", CatTools, 1.1}, {"puff", "Puff", CatTools, 0.8},
	{"sharpener", "Sharpener", CatTools, 1.0}, {"eyelash curler", "Lash curler", CatTools, 1.3}, {"lash curler", "Lash curler", CatTools, 1.3},
	{"tweezers", "Tweezers", CatTools, 1.0}, {"makeup bag", "Makeup bag", CatTools, 1.0}, {"cosmetic bag", "Makeup bag", CatTools, 1.0},
	{"applicator", "Applicator", CatTools, 0.6}, {"tool", "Tool", CatTools, 0.5}, {"tools", "Tool", CatTools, 0.5},
}

// nonProductPhrases mark store items that aren't beauty products at all.
var nonProductPhrases = []string{
	"gift card", "giftcard", "e gift card", "egift card", "egift", "e gift", "digital gift", "gift certificate", "virtual gift",
	"shipping protection", "package protection", "order protection", "route protection", "shipping insurance", "insurance",
	"donation", "donate", "tote", "tote bag", "t shirt", "tshirt", "hoodie", "sweatshirt", "crewneck", "sweatpants", "beanie",
	"baseball cap", "trucker hat", "sticker", "stickers", "poster", "keychain", "phone case", "water bottle", "socks", "merch",
	"sample", "samples", "tester", "testers", "free gift", "gift with purchase", "gwp", "mystery box", "mystery bag",
	"subscription", "membership", "warranty", "ecard", "e card", "digital download", "gift wrap", "gift wrapping", "add on",
	"shopping bag", "paper bag", "display unit", "replacement part",
}

// nonMakeupPhrases are other beauty categories that Makeup Radar leaves
// out unless a product is clearly also makeup (e.g. "tinted serum").
var nonMakeupPhrases = []string{
	"cleanser", "cleansing", "face wash", "moisturizer", "moisturiser", "night cream", "day cream", "eye cream", "face cream",
	"toner", "essence", "serum", "face oil", "retinol", "exfoliant", "exfoliator", "peel", "face mask", "sheet mask", "clay mask",
	"sunscreen", "sun cream", "spf", "eye patch", "eye patches", "pimple patch", "makeup remover", "micellar", "cleansing balm",
	"cleansing oil", "mist", "facial", "skincare", "skin care", "body lotion", "body wash", "body oil", "body butter", "body cream",
	"body scrub", "shower", "deodorant", "hand cream", "bath", "body mist", "body spray", "shampoo", "conditioner", "hair",
	"scalp", "fragrance", "perfume", "eau de parfum", "eau de toilette", "parfum", "cologne", "candle", "nail polish", "nail",
	"nails", "manicure", "cuticle", "press on", "supplement", "gummies", "vitamins", "toothpaste", "lotion",
}

var setPhrases = []string{
	"set", "sets", "bundle", "bundles", "vault", "gift set", "gift box", "giftset", "advent calendar", "coffret",
	"collection box", "value pack", "value set", "starter kit", "discovery kit", "mini kit", "holiday kit", "trio set", "duo set",
}

// textIndex makes repeated phrase lookups cheap: single words hit a set,
// multi-word phrases check their first word before scanning the text.
type textIndex struct {
	pad  string
	toks map[string]bool
}

func newTextIndex(norm string) textIndex {
	toks := make(map[string]bool, 32)
	for _, t := range tokens(norm) {
		toks[t] = true
	}
	return textIndex{pad: padded(norm), toks: toks}
}

func (ix textIndex) has(phrase string) bool {
	if phrase == "" {
		return false
	}
	sp := strings.IndexByte(phrase, ' ')
	if sp < 0 {
		return ix.toks[phrase]
	}
	if !ix.toks[phrase[:sp]] {
		return false
	}
	return strings.Contains(ix.pad, " "+phrase+" ")
}

type classifyInput struct {
	Title, Type, Tags string // normalized
	Focus, SourceKind string
}

type classification struct {
	Category     string
	BaseCategory string
	Kind         string
	IsSet        bool
	Exclude      bool
	Reason       string
}

func classify(in classifyInput) classification {
	t, ty, tg := newTextIndex(in.Title), newTextIndex(in.Type), newTextIndex(in.Tags)

	for _, ph := range nonProductPhrases {
		if t.has(ph) || ty.has(ph) {
			return classification{Exclude: true, Reason: "not a product"}
		}
	}

	scores := map[string]float64{}
	specific := false
	type kindHit struct {
		v     vocabEntry
		field float64
	}
	var kindHits []kindHit
	for _, v := range productVocab {
		inType, inTitle, inTags := ty.has(v.phrase), t.has(v.phrase), tg.has(v.phrase)
		if !inType && !inTitle && !inTags {
			continue
		}
		s := 0.0
		field := 0.0
		if inType {
			s += 3 * v.weight
			field = 3
		}
		if inTitle {
			s += 2 * v.weight
			if field < 2 {
				field = 2
			}
		}
		if inTags {
			s += 0.6 * v.weight
			if field < 0.6 {
				field = 0.6
			}
		}
		scores[v.cat] += s
		if v.weight >= 0.8 && (inType || inTitle) {
			specific = true
			kindHits = append(kindHits, kindHit{v, field})
		}
	}

	nonMakeup := 0.0
	for _, ph := range nonMakeupPhrases {
		if ty.has(ph) {
			nonMakeup += 3
		}
		if t.has(ph) {
			nonMakeup += 2
		}
	}

	cat, best := "", 0.0
	for _, c := range categoryOrder {
		if scores[c] > best {
			cat, best = c, scores[c]
		}
	}

	switch {
	case !specific && nonMakeup > 0:
		return classification{Exclude: true, Reason: "not makeup"}
	case cat == "":
		if in.Focus == "makeup" && in.SourceKind == "brand" {
			cat = CatOther
		} else {
			return classification{Exclude: true, Reason: "not makeup"}
		}
	case !specific && (in.Focus != "makeup" || in.SourceKind == "shop"):
		// Only generic words like "face" or "eye" matched in a mixed catalog.
		return classification{Exclude: true, Reason: "not clearly makeup"}
	}

	// The display kind is the most specific phrase within the winning
	// category: "Serum foundation" beats a plain "Foundation" type.
	bestKind, bestKindScore := "", 0.0
	for _, h := range kindHits {
		if h.v.cat != cat {
			continue
		}
		ks := float64(len(h.v.phrase))*2 + h.field*3 + h.v.weight
		if ks > bestKindScore {
			bestKind, bestKindScore = h.v.label, ks
		}
	}
	c := classification{Category: cat, BaseCategory: cat, Kind: bestKind}
	for _, ph := range setPhrases {
		if t.has(ph) || ty.has(ph) {
			c.IsSet = true
			break
		}
	}
	if c.IsSet {
		c.Category = CatSets
		if c.Kind == "" {
			c.Kind = "Set"
		}
	}
	return c
}

// Highlight is one "what makes it advanced" signal found in a product
// or headline.
type Highlight struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Group  string  `json:"group"`
	Weight float64 `json:"-"`
}

type techRule struct {
	key, label, group string
	weight            float64
	phrases           []string
	phraseLabels      map[string]string
}

var techRules = []techRule{
	{key: "pdrn", label: "PDRN", group: "skincare", weight: 1.0, phrases: []string{"pdrn", "polynucleotide", "polynucleotides", "salmon dna"}},
	{key: "exosomes", label: "Exosomes", group: "skincare", weight: 1.0, phrases: []string{"exosome", "exosomes"}},
	{key: "peptides", label: "Peptides", group: "skincare", weight: 0.8, phrases: []string{"peptide", "peptides", "biopeptide", "biopeptides", "cyclopeptide", "cyclopeptides", "tripeptide", "hexapeptide", "oligopeptide", "polypeptide"}},
	{key: "nad", label: "NAD+", group: "skincare", weight: 0.9, phrases: []string{"nad"}},
	{key: "retinoid", label: "Retinoid", group: "skincare", weight: 0.8, phrases: []string{"retinol", "retinal", "retinoid", "bakuchiol", "hydroxypinacolone"}, phraseLabels: map[string]string{"bakuchiol": "Bakuchiol"}},
	{key: "tranexamic", label: "Tranexamic acid", group: "skincare", weight: 0.7, phrases: []string{"tranexamic"}},
	{key: "microbiome", label: "Microbiome care", group: "skincare", weight: 0.7, phrases: []string{"microbiome", "probiotic", "probiotics", "postbiotic", "postbiotics", "prebiotic", "prebiotics"}},
	{key: "niacinamide", label: "Niacinamide", group: "skincare", weight: 0.6, phrases: []string{"niacinamide", "vitamin b3"}},
	{key: "ceramides", label: "Ceramides", group: "skincare", weight: 0.6, phrases: []string{"ceramide", "ceramides"}},
	{key: "hyaluronic", label: "Hyaluronic acid", group: "skincare", weight: 0.5, phrases: []string{"hyaluronic", "hyaluronate"}},
	{key: "squalane", label: "Squalane", group: "skincare", weight: 0.4, phrases: []string{"squalane"}},
	{key: "vitaminc", label: "Vitamin C", group: "skincare", weight: 0.4, phrases: []string{"vitamin c", "ascorbic", "ascorbyl"}},
	{key: "centella", label: "Centella", group: "skincare", weight: 0.4, phrases: []string{"centella", "cica", "madecassoside"}},
	{key: "collagen", label: "Collagen support", group: "skincare", weight: 0.3, phrases: []string{"collagen"}},
	{key: "skincare", label: "Skincare hybrid", group: "skincare", weight: 0.7, phrases: []string{"skincare infused", "skin care infused", "skincare makeup", "skin care makeup", "serum foundation", "serum concealer", "serum blush", "tinted serum", "skincare hybrid", "makeup skincare hybrid", "treatment foundation"}},
	{key: "spf", label: "SPF", group: "protection", weight: 1.0, phrases: []string{"spf", "broad spectrum", "sunscreen", "uv protection", "uva uvb"}},
	{key: "bluelight", label: "Blue-light defense", group: "protection", weight: 0.4, phrases: []string{"blue light", "hev light", "anti pollution"}},
	{key: "adaptive", label: "Color-adapting", group: "adaptive", weight: 1.4, phrases: []string{"color adapting", "colour adapting", "color changing", "colour changing", "color adjusting", "colour adjusting", "shade adapting", "shade adjusting", "adapts to your", "ph adjusting", "ph reactive", "ph activated", "ph color", "ph colour", "color shifting", "colour shifting", "chameleon"}},
	{key: "transfer", label: "Transfer-proof", group: "wear", weight: 0.6, phrases: []string{"transfer proof", "transfer resistant", "transferproof", "kiss proof", "smudge proof", "smudgeproof", "budge proof", "mask proof", "no transfer"}},
	{key: "waterproof", label: "Waterproof", group: "wear", weight: 0.5, phrases: []string{"waterproof", "water resistant", "sweat proof", "sweatproof", "humidity proof", "tear proof", "sweat resistant"}},
	{key: "longwear", label: "Long-wear", group: "wear", weight: 0.5, phrases: []string{"long wear", "longwear", "long wearing", "long lasting", "longlasting", "all day wear"}},
	{key: "blur", label: "Blurring", group: "finish", weight: 0.6, phrases: []string{"blur", "blurring", "blurs", "soft focus", "pore blurring", "airbrush", "airbrushed", "filter finish", "filter effect", "hd finish", "photo ready", "optical blurring"}},
	{key: "hybrid", label: "Hybrid texture", group: "texture", weight: 0.5, phrases: []string{"cream to powder", "powder to cream", "liquid to powder", "gel to powder", "balm to powder", "water to powder", "mousse", "whipped", "bouncy", "gel cream", "water based", "waterless", "jelly"},
		phraseLabels: map[string]string{"cream to powder": "Cream-to-powder", "powder to cream": "Powder-to-cream", "liquid to powder": "Liquid-to-powder", "gel to powder": "Gel-to-powder", "balm to powder": "Balm-to-powder", "mousse": "Mousse texture", "jelly": "Jelly texture", "waterless": "Waterless"}},
	{key: "cushion", label: "Cushion tech", group: "texture", weight: 0.5, phrases: []string{"cushion"}},
	{key: "plump", label: "Plumping", group: "texture", weight: 0.4, phrases: []string{"plump", "plumping", "plumper", "lip volume", "volumizing lip"}},
	{key: "multiuse", label: "Multi-use", group: "texture", weight: 0.4, phrases: []string{"multi use", "multiuse", "multi purpose", "multipurpose", "lip and cheek", "cheek and lip", "lips and cheeks", "cheeks and lips", "eyes lips", "all over", "2 in 1", "3 in 1"}},
	{key: "tubing", label: "Tubing mascara", group: "lash", weight: 1.0, phrases: []string{"tubing"}},
	{key: "lashgrowth", label: "Lash-boosting", group: "lash", weight: 0.8, phrases: []string{"lash serum", "lash growth", "lash boosting", "brow serum", "lash conditioning", "lash enhancing", "brow growth"}},
	{key: "refill", label: "Refillable", group: "eco", weight: 0.7, phrases: []string{"refillable", "refill", "refills"}},
	{key: "patented", label: "Patented tech", group: "tech", weight: 1.4, phrases: []string{"patented", "patent pending", "proprietary technology", "proprietary complex", "proprietary blend", "exclusive technology"}},
	{key: "firstever", label: "First-of-its-kind", group: "tech", weight: 1.2, phrases: []string{"first ever", "first of its kind", "worlds first", "industry first", "never before seen", "breakthrough", "revolutionary", "next gen", "next generation"}},
	{key: "biotech", label: "Biotech", group: "tech", weight: 1.0, phrases: []string{"biotech", "biotechnology", "bio fermented", "fermented", "bioengineered", "lab grown", "biomimetic", "upcycled"}},
	{key: "encapsulated", label: "Encapsulated actives", group: "tech", weight: 0.9, phrases: []string{"encapsulated", "microencapsulated", "encapsulation", "microspheres", "micro spheres", "spherical powder"}},
	{key: "ai", label: "AI shade match", group: "tech", weight: 1.0, phrases: []string{"ai shade", "ai powered", "artificial intelligence", "virtual try on", "shade matching technology"}},
	{key: "clinical", label: "Clinically tested", group: "tech", weight: 0.5, phrases: []string{"clinically proven", "clinically tested", "clinical study", "clinical results", "clinically shown", "dermatologist tested", "ophthalmologist tested", "clinical trial"}},
	{key: "applicator", label: "Smart applicator", group: "tech", weight: 0.4, phrases: []string{"precision tip", "flexible wand", "silicone applicator", "mesh applicator", "magnetic", "self sharpening", "airless pump", "dual ended", "double ended", "built in brush", "built in sponge", "ergonomic"}},
	{key: "limited", label: "Limited edition", group: "drop", weight: 0.3, phrases: []string{"limited edition", "limited release", "exclusive collection", "collab", "collaboration"}},
}

var techDetails = map[string]string{
	"pdrn":         "PDRN (polynucleotides): DNA-fragment actives from Korean skincare, marketed for skin repair and bounce.",
	"exosomes":     "Exosomes: one of the newest “regenerative” skincare actives, now crossing into makeup.",
	"peptides":     "Peptides: amino-acid chains used in skincare to support firmness and hydration, built into the makeup itself.",
	"nad":          "NAD+: a cellular-energy molecule that has become a longevity-skincare favourite.",
	"retinoid":     "A retinoid or retinol alternative (such as bakuchiol), an anti-ageing active rarely seen in makeup.",
	"tranexamic":   "Tranexamic acid: targets the look of dark spots and uneven tone.",
	"microbiome":   "Pre-, pro- or postbiotics, formulated to support skin’s natural microbiome.",
	"niacinamide":  "Niacinamide (vitamin B3): helps the look of pores, redness and uneven tone over time.",
	"ceramides":    "Ceramides: lipids that reinforce the skin barrier, common in skin-care makeup.",
	"hyaluronic":   "Hyaluronic acid: a humectant that draws water into skin so the formula wears more hydrated.",
	"squalane":     "Squalane: a lightweight, skin-identical emollient for a comfortable, non-greasy feel.",
	"vitaminc":     "Vitamin C: an antioxidant used for brightening.",
	"centella":     "Centella (cica): a soothing botanical loved in K-beauty.",
	"collagen":     "Collagen-support claims: formulated to help skin look firmer.",
	"skincare":     "A skincare hybrid: makeup designed to treat skin while you wear it.",
	"spf":          "Built-in sun protection. Check the label for the broad-spectrum SPF value.",
	"bluelight":    "Claims defence against blue light or pollution.",
	"adaptive":     "Color-adapting: the shade shifts or adjusts on contact with your skin (pH- or moisture-reactive).",
	"transfer":     "Transfer-resistant: designed not to budge onto masks, cups or collars.",
	"waterproof":   "Water- and sweat-resistant formula.",
	"longwear":     "A long-wear formula engineered to last through the day.",
	"hours":        "The brand’s stated wear time.",
	"blur":         "Optical blurring: soft-focus pigments scatter light to soften the look of pores and texture.",
	"hybrid":       "A transforming texture (for example cream-to-powder) that blends easily and then sets.",
	"cushion":      "A cushion compact, the K-beauty format that sponges a fresh layer of liquid with each tap.",
	"plump":        "Creates a temporarily fuller-looking lip.",
	"multiuse":     "One product for several areas (eyes, cheeks and/or lips).",
	"tubing":       "Tubing mascara wraps each lash in tiny polymer tubes that won’t smudge and slide off with warm water.",
	"lashgrowth":   "A conditioning complex for fuller-looking lashes or brows over time.",
	"refill":       "Refillable packaging: buy the refill, keep the case.",
	"patented":     "Patented or proprietary technology, according to the brand.",
	"firstever":    "The brand calls it a first-of-its-kind or breakthrough formula.",
	"biotech":      "Biotech or fermentation-derived ingredients.",
	"encapsulated": "Encapsulated actives or microspheres, for gradual release or a special finish.",
	"ai":           "Comes with AI or virtual shade-matching support.",
	"clinical":     "Backed by brand-reported clinical, dermatologist or ophthalmologist testing.",
	"applicator":   "An engineered applicator or package design.",
	"limited":      "A limited-edition or collaboration release.",
	"inclusive":    "An unusually broad shade range.",
}

var groupCaps = map[string]float64{
	"skincare": 2.2, "protection": 1.3, "adaptive": 1.6, "wear": 1.5, "finish": 0.8, "texture": 1.0,
	"lash": 1.2, "eco": 0.8, "tech": 3.0, "inclusive": 1.2, "drop": 0.3,
}

var attributeRules = []struct {
	label   string
	phrases []string
}{
	{"Vegan", []string{"vegan"}},
	{"Cruelty-free", []string{"cruelty free", "crueltyfree", "leaping bunny"}},
	{"Clean", []string{"clean beauty", "clean at sephora", "clean formula", "clean makeup"}},
	{"Fragrance-free", []string{"fragrance free", "unscented"}},
	{"Non-comedogenic", []string{"non comedogenic", "noncomedogenic"}},
	{"Sensitive skin", []string{"sensitive skin", "hypoallergenic"}},
	{"Talc-free", []string{"talc free"}},
}

type analysis struct {
	Highlights []Highlight
	Score      float64
	Attributes []string
	FlaggedNew bool
}

// analyze finds innovation highlights and attributes in normalized text.
// shades and baseCat let it credit broad shade ranges.
func analyze(norm string, tagsNorm string, shades int, baseCat string) analysis {
	ix := newTextIndex(norm)
	var hs []Highlight
	for _, r := range techRules {
		for _, ph := range r.phrases {
			if ix.has(ph) {
				label := r.label
				if l, ok := r.phraseLabels[ph]; ok {
					label = l
				}
				hs = append(hs, Highlight{Key: r.key, Label: label, Group: r.group, Weight: r.weight})
				break
			}
		}
	}

	toks := tokens(norm)
	spfSeen := false
	for i := 0; i+1 < len(toks); i++ {
		// "spf 30" → label the SPF chip with its value.
		if toks[i] == "spf" && !spfSeen {
			if n, err := strconv.Atoi(toks[i+1]); err == nil && n >= 2 && n <= 100 {
				spfSeen = true
				for j := range hs {
					if hs[j].Key == "spf" {
						hs[j].Label = "SPF " + toks[i+1]
					}
				}
			}
		}
	}
	if h, ok := wearHours(toks); ok {
		hs = append(hs, h)
	}

	if shades > 0 {
		if (baseCat == CatFace && shades >= 30) || shades >= 45 {
			w := 0.8
			if shades >= 45 {
				w = 1.1
			}
			hs = append(hs, Highlight{Key: "inclusive", Label: strconv.Itoa(shades) + " shades", Group: "inclusive", Weight: w})
		}
	}

	// Score with per-group caps so one family of claims can't dominate.
	byGroup := map[string]float64{}
	for _, h := range hs {
		byGroup[h.Group] += h.Weight
	}
	score := 0.0
	for g, v := range byGroup {
		if c, ok := groupCaps[g]; ok && v > c {
			v = c
		}
		score += v
	}

	// Most meaningful first, one per group before repeats.
	sort.SliceStable(hs, func(i, j int) bool { return hs[i].Weight > hs[j].Weight })
	ordered := make([]Highlight, 0, len(hs))
	usedGroup := map[string]bool{}
	for _, h := range hs {
		if !usedGroup[h.Group] {
			ordered = append(ordered, h)
			usedGroup[h.Group] = true
		}
	}
	for _, h := range hs {
		dup := false
		for _, o := range ordered {
			if o.Key == h.Key {
				dup = true
				break
			}
		}
		if !dup {
			ordered = append(ordered, h)
		}
	}

	var attrs []string
	for _, a := range attributeRules {
		for _, ph := range a.phrases {
			if ix.has(ph) {
				attrs = append(attrs, a.label)
				break
			}
		}
	}

	tix := newTextIndex(tagsNorm)
	flagged := false
	for _, ph := range []string{"new", "new arrival", "new arrivals", "new in", "just dropped", "just launched", "new launch", "launch", "new release", "newness"} {
		if tix.has(ph) {
			flagged = true
			break
		}
	}

	return analysis{Highlights: ordered, Score: score, Attributes: attrs, FlaggedNew: flagged}
}

// wearHours spots claims like "24-hour wear" or "lasts up to 16 hours".
func wearHours(toks []string) (Highlight, bool) {
	unit := map[string]bool{"hour": true, "hours": true, "hr": true, "hrs": true, "h": true}
	wearWords := map[string]bool{"wear": true, "long": true, "longwear": true, "lasting": true, "staying": true, "stay": true, "coverage": true, "transfer": true, "smudge": true, "hold": true, "color": true, "colour": true, "wearing": true}
	best := 0
	for i := 0; i+1 < len(toks); i++ {
		n, err := strconv.Atoi(toks[i])
		if err != nil || n < 8 || n > 72 || !unit[toks[i+1]] {
			continue
		}
		ok := false
		for j := i + 2; j < len(toks) && j <= i+4; j++ {
			if wearWords[toks[j]] {
				ok = true
				break
			}
		}
		if !ok && i >= 1 {
			prev := toks[i-1]
			if prev == "lasts" || prev == "for" || (prev == "to" && i >= 2 && toks[i-2] == "up") {
				ok = true
			}
		}
		if ok && n > best {
			best = n
		}
	}
	if best == 0 {
		return Highlight{}, false
	}
	w := 0.5
	if best >= 24 {
		w = 0.8
	}
	return Highlight{Key: "hours", Label: strconv.Itoa(best) + "h wear", Group: "wear", Weight: w}, true
}
