package main

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(?:script|style|noscript|template)[^>]*>.*?</(?:script|style|noscript|template)\s*>`)
	reBlockTag    = regexp.MustCompile(`(?i)<\s*/?\s*(?:br|p|div|li|ul|ol|h[1-6]|tr|table|section|article|blockquote)\b[^>]*>`)
	reAnyTag      = regexp.MustCompile(`<[^>]*>`)
	reHSpace      = regexp.MustCompile(`[ \t\f\v\x{00A0}\x{2007}\x{202F}]+`)
)

// htmlToText turns an HTML fragment (a product description, an RSS
// snippet) into readable plain text with paragraph breaks preserved.
// The result is only ever rendered as text, never as markup.
func htmlToText(s string) string {
	if s == "" {
		return ""
	}
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reBlockTag.ReplaceAllString(s, "\n")
	s = reAnyTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(reHSpace.ReplaceAllString(ln, " "))
		if ln != "" {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// truncateRunes cuts s to at most n runes, ending on a word boundary
// with an ellipsis when it had to cut.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	cut := n
	for i := n; i > n-40 && i > 0; i-- {
		if unicode.IsSpace(r[i]) {
			cut = i
			break
		}
	}
	return strings.TrimRightFunc(string(r[:cut]), func(c rune) bool {
		return unicode.IsSpace(c) || c == ',' || c == ';' || c == ':' || c == '-'
	}) + "…"
}

var foldTable = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'æ': "ae",
	'ç': "c", 'č': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ì': "i",
	'í': "i", 'î': "i", 'ï': "i", 'ñ': "n", 'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o",
	'ö': "o", 'ø': "o", 'œ': "oe", 'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y",
	'ÿ': "y", 'ß': "ss", 'š': "s", 'ž': "z",
}

// normalize lowercases, folds accents, drops apostrophes and turns every
// other non-alphanumeric run into a single space, so "Juvia's Place",
// "Ami Colé" and "24-Hour" become "juvias place", "ami cole", "24 hour".
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r == '\'' || r == '’' || r == '‘' || r == '´' || r == '`' || r == 'ʼ':
			continue
		case r == '&':
			if !space {
				b.WriteByte(' ')
			}
			b.WriteString("and ")
			space = true
			continue
		}
		if f, ok := foldTable[r]; ok {
			b.WriteString(f)
			space = false
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			space = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			// Non-Latin scripts are kept as-is so they can still match.
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// padded wraps a normalized string in spaces so whole-word phrase checks
// are a simple substring search: containsPhrase(" lip oil ", "lip oil").
func padded(norm string) string { return " " + norm + " " }

func containsPhrase(paddedText, phrase string) bool {
	if phrase == "" {
		return false
	}
	return strings.Contains(paddedText, " "+phrase+" ")
}

func tokens(norm string) []string {
	if norm == "" {
		return nil
	}
	return strings.Fields(norm)
}

// wordVariants gives simple singular/plural forms for a keyword so
// "glosses" finds "gloss" and "lash" finds "lashes".
func wordVariants(w string) []string {
	out := []string{w}
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		out = append(out, w[:len(w)-3]+"y")
	case len(w) > 4 && (strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "sses") || strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "xes")):
		out = append(out, w[:len(w)-2])
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		out = append(out, w[:len(w)-1])
	default:
		if strings.HasSuffix(w, "sh") || strings.HasSuffix(w, "ss") || strings.HasSuffix(w, "ch") || strings.HasSuffix(w, "x") {
			out = append(out, w+"es")
		} else if strings.HasSuffix(w, "y") && len(w) > 3 && !strings.ContainsAny(w[len(w)-2:len(w)-1], "aeiou") {
			out = append(out, w[:len(w)-1]+"ies")
		} else {
			out = append(out, w+"s")
		}
	}
	return out
}

// jaccard returns the overlap of two token sets (0..1).
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

var titleStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true,
	"in": true, "on": true, "for": true, "with": true, "is": true, "are": true, "its": true,
	"it": true, "this": true, "that": true, "at": true, "by": true, "from": true, "as": true,
	"new": true, "just": true, "here": true, "your": true, "you": true, "we": true, "our": true,
	"be": true, "has": true, "have": true, "s": true, "my": true, "all": true, "into": true,
}

// significantTokens keeps the words that carry meaning in a headline or
// product title.
func significantTokens(norm string) map[string]bool {
	set := map[string]bool{}
	for _, t := range tokens(norm) {
		if len(t) < 2 || titleStopwords[t] {
			continue
		}
		set[t] = true
	}
	return set
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// phraseVariants pluralizes or singularizes the last word of a phrase.
func phraseVariants(phrase string) []string {
	i := strings.LastIndexByte(phrase, ' ')
	if i < 0 {
		return wordVariants(phrase)
	}
	head, last := phrase[:i+1], phrase[i+1:]
	var out []string
	for _, v := range wordVariants(last) {
		out = append(out, head+v)
	}
	return out
}
