package backend

// ----------------------------------------------------------------------
// The fuzzy matcher
// ----------------------------------------------------------------------
//
// Pure scoring: no I/O, no state, no dependency beyond the standard library
// (see the note in templates.go on why this codebase is careful about what it
// links). Everything works on []rune rather than bytes, because a byte offset
// into Cyrillic text is a way to cut a character in half - every offset this
// file produces or consumes is a RUNE offset.
//
// The ladder, first rung that hits wins. Rungs are compared BEFORE scores (see
// betterMatch): a verbatim hit always beats a fuzzy one, whatever the two
// numbers happen to be. That has to be structural rather than a property of
// the constants - with the constants below, the weakest substring hit scores 85
// while a perfect subsequence scores 95, so ordering by score alone would put
// "I remembered the name roughly" above "the word is right there".
//
//	1. scoreSubstring   - the term appears verbatim (folded)
//	2. scoreSubsequence - the term's runes appear in order, fzf-style
//	3. scoreTypo        - bounded edit distance, for a misspelling
//
// A fourth rung stands above these three, and this file does not produce it.
// tierPhrase belongs to a whole query. scoreDocument gives it to a document
// that holds every word of the query in order and next to each other. See
// the constant and the banner of scoreDocument.
//
// Rungs 1 and 2 are driven from here by scoreTerm. Rung 3 is NOT: it needs a
// set of candidate tokens to compare against, which only the caller (the index,
// or the single-file page search) can produce - so this file exports the
// pieces (tokenize, scoreTypo, osaDistance) and lets the caller decide which
// tokens are worth the distance computation.
//
// Why three rungs rather than one clever algorithm: they answer different
// questions. Substring is "I know what it says"; subsequence is "I remember
// roughly what it was called"; edit distance is "I typed it wrong". A single
// scorer tuned to do all three does none of them predictably, and predictable
// ordering is most of what makes a search box feel trustworthy.

import (
	"strings"
	"unicode"
)

// span is a matched range within a candidate, in rune offsets.
type span struct{ Start, Len int }

// matchTier records which rung produced a score. Exposed mainly so tests can
// assert that a given input is matched the way it is meant to be, not merely
// that it scores something.
type matchTier uint8

const (
	tierNone matchTier = iota
	// tierPhrase is the rung of a whole query, and never of one term.
	// scoreTerm cannot return it. scoreDocument gives it to a document,
	// and to the one line, that holds every word of the query in order
	// and next to each other.
	//
	// It is a rung and not a bonus, because the ladder already ranks by
	// quality before quantity. A bonus cannot win against a sum.
	// A title of five loose query words scored 2001 in the laboratory.
	// The note that held the sentence scored 718. A bonus large enough
	// to win one case is too large for the next one. See the banner of
	// scoreDocument.
	tierPhrase
	tierSubstring
	tierSubsequence
	tierTypo
)

func (t matchTier) String() string {
	switch t {
	case tierPhrase:
		return "phrase"
	case tierSubstring:
		return "substring"
	case tierSubsequence:
		return "subsequence"
	case tierTypo:
		return "typo"
	default:
		return "none"
	}
}

// Scoring constants. Named rather than inlined because the worked examples in
// the plan (and the table tests beside this file) assert exact arithmetic: a
// change here is a change to result ordering, and should have to be typed
// somewhere visible.
const (
	// Rung 1 - exact substring.
	substringBase   = 100 // any verbatim hit starts here
	bonusFieldEqual = 50  // the candidate IS the term
	bonusWordStart  = 30  // the hit starts a word
	bonusWordEnd    = 20  // ... and ends one, i.e. a whole word
	maxPosPenalty   = 30  // positional penalty is capped, then halved

	// Rung 2 - subsequence.
	subseqPerRune     = 16 // every matched rune
	subseqConsecutive = 8  // ... that directly follows the previous match
	subseqWordStart   = 12 // ... and starts a word
	subseqGapPenalty  = 3  // per gap (a run of skipped runes)
	subseqSkipPenalty = 1  // per skipped rune
	subseqMaxPenalty  = 20 // the two penalties together stop here
	subseqCeiling     = 95 // hard ceiling: always below the weakest rung 1 hit

	// Rung 3 - bounded edit distance.
	typoBase    = 60
	typoPerEdit = 15

	// Thresholds.
	minFuzzyTermLen = 3  // below this, substring only: fuzzy on 1-2 runes is noise
	typoMinTermLen  = 4  // below this, no edit-distance matching at all
	typoK2TermLen   = 8  // at or above this, two edits are allowed instead of one
	minTokenLen     = 3  // a token shorter than this can never match at k<=2
	maxTokenLen     = 32 // base64 blobs and minified identifiers are not queries
)

// ----------------------------------------------------------------------
// Folding
// ----------------------------------------------------------------------

// foldTable holds the only diacritic mappings applied on top of lowercasing.
// Every entry is deliberately ONE rune to ONE rune: folding must not change
// the length of the text, or every span this file returns would point at the
// wrong place in the original. That rules out the expanding folds (ß -> ss,
// æ -> ae), which are therefore absent rather than forgotten.
//
// Cyrillic ё -> е is here for the same reason the Latin accents are: it is the
// character people leave off when typing, so a note titled "Ёлка" has to be
// findable by "елка".
var foldTable = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ñ': 'n', 'ç': 'c',
	'ё': 'е',
}

// foldRune lowercases and strips the diacritics in foldTable. One rune in, one
// rune out - see the table comment.
func foldRune(r rune) rune {
	if r < utf8SelfMax {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}
	r = unicode.ToLower(r)
	if f, ok := foldTable[r]; ok {
		return f
	}
	return r
}

// utf8SelfMax is the ASCII fast-path bound: below it, folding is a single
// comparison and no map lookup, which matters because folding runs over every
// line of every indexed file.
const utf8SelfMax = 0x80

// fold returns s folded, as runes. The result has exactly one rune per rune of
// the input, so an offset into it is also an offset into the original.
func fold(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		out = append(out, foldRune(r))
	}
	return out
}

// foldString is fold for callers that want a string back.
func foldString(s string) string {
	return string(fold(s))
}

// isWordRune decides what counts as "inside a word" for the word-boundary
// bonuses and for tokenisation. Letters and digits in any script, plus '_'
// because it holds identifiers together in the code these notes contain.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// ----------------------------------------------------------------------
// The character mask
// ----------------------------------------------------------------------

// runeMask returns a 64-bit signature of which characters appear in rs.
//
// It exists to answer "could this text possibly contain that term?" without
// touching the text: rungs 1 and 2 both require EVERY rune of the term to be
// present, so termMask &^ candMask != 0 is a rejection that cannot produce a
// false negative. Collisions (many runes share a bit) only ever cost a wasted
// check, never a missed match.
//
// Rung 3 deliberately cannot use this - a typo is precisely a rune the term
// has and the text does not - which is why the typo rung is driven from a
// token dictionary by the caller instead.
func runeMask(rs []rune) uint64 {
	var m uint64
	for _, r := range rs {
		m |= 1 << (uint32(r) % 64)
	}
	return m
}

// maskRejects reports whether cand cannot possibly contain every rune of term.
func maskRejects(termMask, candMask uint64) bool {
	return termMask&^candMask != 0
}

// ----------------------------------------------------------------------
// Tokenisation
// ----------------------------------------------------------------------

// tokenize splits raw text into folded tokens for the typo rung.
//
// A token is a run of word runes (see isWordRune). Beyond the whole run, the
// camelCase pieces of an identifier are emitted too, so a typo of "json" can
// still reach the "JSON" inside "loadJSON" - code is half of what these notes
// contain, and its interesting words live inside identifiers.
//
// Case is why this works on RAW text rather than folded text: folding destroys
// exactly the signal camel splitting needs. The pieces come back folded.
//
// Tokens outside [minTokenLen, maxTokenLen] are dropped. The lower bound is
// not a guess: the typo rung needs terms of at least typoMinTermLen (4) runes
// and allows at most k edits, so a token can only match if its length is
// within k of the term's - which makes a 1- or 2-rune token unreachable at
// every allowed k. The upper bound drops base64 blobs, which nobody types.
func tokenize(s string) []string {
	var out []string
	emit := func(t []rune) {
		if len(t) < minTokenLen || len(t) > maxTokenLen {
			return
		}
		b := make([]rune, len(t))
		for i, r := range t {
			b[i] = foldRune(r)
		}
		out = append(out, string(b))
	}

	var word []rune
	flush := func() {
		if len(word) == 0 {
			return
		}
		emit(word)
		if pieces := camelSplit(word); len(pieces) > 1 {
			for _, p := range pieces {
				emit(p)
			}
		}
		word = word[:0]
	}

	for _, r := range s {
		if isWordRune(r) {
			word = append(word, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// camelSplit breaks an identifier at case transitions: lower-or-digit followed
// by upper ("loadJSON" -> load, JSON), and upper followed by upper-then-lower
// ("JSONFile" -> JSON, File). Returns a single piece when there is nothing to
// split, which the caller uses to skip emitting a duplicate.
func camelSplit(word []rune) [][]rune {
	var pieces [][]rune
	start := 0
	for i := 1; i < len(word); i++ {
		prev, cur := word[i-1], word[i]
		boundary := false
		if unicode.IsUpper(cur) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			boundary = true
		} else if unicode.IsUpper(prev) && unicode.IsUpper(cur) &&
			i+1 < len(word) && unicode.IsLower(word[i+1]) {
			boundary = true
		}
		if boundary {
			pieces = append(pieces, word[start:i])
			start = i
		}
	}
	pieces = append(pieces, word[start:])
	return pieces
}

// ----------------------------------------------------------------------
// Rung 1: exact substring
// ----------------------------------------------------------------------

// indexRunes is strings.Index for rune slices, returning a rune offset.
func indexRunes(hay, needle []rune, from int) int {
	if len(needle) == 0 || len(needle) > len(hay) {
		return -1
	}
	for i := from; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// scoreSubstring scores a verbatim (already folded) occurrence of term in cand.
//
// EVERY occurrence is reported as a span - a line mentioning the term three
// times should highlight three times - but the SCORE comes from the best one,
// not the first. Scoring the first would rank "xjson /json" below its own
// better second hit purely because of reading order.
func scoreSubstring(term, cand []rune) (int, []span, bool) {
	if len(term) == 0 {
		return 0, nil, false
	}
	var spans []span
	best := 0
	for at := indexRunes(cand, term, 0); at >= 0; at = indexRunes(cand, term, at+1) {
		spans = append(spans, span{Start: at, Len: len(term)})

		s := substringBase
		if len(cand) == len(term) {
			s += bonusFieldEqual
		}
		if at == 0 || !isWordRune(cand[at-1]) {
			s += bonusWordStart
		}
		if end := at + len(term); end >= len(cand) || !isWordRune(cand[end]) {
			s += bonusWordEnd
		}
		pos := at
		if pos > maxPosPenalty {
			pos = maxPosPenalty
		}
		s -= pos / 2
		if s > best {
			best = s
		}
	}
	if spans == nil {
		return 0, nil, false
	}
	return best, spans, true
}

// ----------------------------------------------------------------------
// Rung 2: subsequence (fzf-style)
// ----------------------------------------------------------------------

// scoreSubsequence scores term's runes appearing in order within cand, greedy
// leftmost, with bonuses for density and word starts and penalties for gaps.
//
// The raw total is normalised against the IDEAL match - the same term found as
// one consecutive run starting at a word boundary - rather than divided by the
// term length. Dividing by length makes a longer query score lower for the same
// quality of match, which would visibly reorder results as the user keeps
// typing; normalising against the ideal keeps a "perfect" match worth the same
// whatever its length.
//
// Word starts are detected from separators only. A camel boundary is not
// visible here because cand is already folded - that signal is used by
// tokenize (for the typo rung), where the raw text is still available.
func scoreSubsequence(term, cand []rune) (int, []span, bool) {
	if len(term) == 0 {
		return 0, nil, false
	}
	raw, ti, gaps, skipped, prev := 0, 0, 0, 0, -2
	var spans []span
	for ci := 0; ci < len(cand) && ti < len(term); ci++ {
		if cand[ci] != term[ti] {
			continue
		}
		raw += subseqPerRune
		if ci == prev+1 {
			raw += subseqConsecutive
			spans[len(spans)-1].Len++
		} else {
			if prev >= 0 {
				gaps++
				skipped += ci - prev - 1
			}
			spans = append(spans, span{Start: ci, Len: 1})
		}
		if ci == 0 || !isWordRune(cand[ci-1]) {
			raw += subseqWordStart
		}
		prev = ci
		ti++
	}
	if ti < len(term) {
		return 0, nil, false // not a subsequence at all
	}

	penalty := gaps*subseqGapPenalty + skipped*subseqSkipPenalty
	if penalty > subseqMaxPenalty {
		penalty = subseqMaxPenalty
	}
	raw -= penalty

	l := len(term)
	ideal := subseqPerRune*l + subseqConsecutive*(l-1) + subseqWordStart
	score := (raw*subseqCeiling + ideal/2) / ideal // rounded
	if score > subseqCeiling {
		score = subseqCeiling
	}
	if score < 1 {
		score = 1
	}
	return score, spans, true
}

// ----------------------------------------------------------------------
// Rung 3: bounded edit distance
// ----------------------------------------------------------------------

// typoBudget is how many edits a term of this length may be matched through,
// or 0 when the term is too short to guess at.
func typoBudget(termLen int) int {
	switch {
	case termLen >= typoK2TermLen:
		return 2
	case termLen >= typoMinTermLen:
		return 1
	default:
		return 0
	}
}

// osaDistance is Damerau-Levenshtein under the optimal string alignment
// restriction, bounded by k: it returns k+1 as soon as the true distance is
// known to exceed k, so a non-match costs almost nothing.
//
// OSA rather than plain Levenshtein because an adjacent TRANSPOSITION is the
// most common typing error and plain Levenshtein charges 2 for it - which puts
// "fecth" out of reach of "fetch" at k=1, i.e. exactly the case this rung
// exists to catch.
func osaDistance(a, b []rune, k int) int {
	la, lb := len(a), len(b)
	if la-lb > k || lb-la > k {
		return k + 1
	}
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev2 := make([]int, lb+1)
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := cur[j-1] + 1
			if prev[j]+1 < v {
				v = prev[j] + 1
			}
			if prev[j-1]+cost < v {
				v = prev[j-1] + cost
			}
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] && prev2[j-2]+1 < v {
				v = prev2[j-2] + 1
			}
			cur[j] = v
			if v < best {
				best = v
			}
		}
		if best > k {
			return k + 1 // every alignment from here on is already too expensive
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[lb]
}

// scoreTypo scores term against one candidate token. Both must already be
// folded; token is expected to come from tokenize.
//
// The caller decides WHICH tokens to try - this rung cannot use the character
// mask (see runeMask), so handing it a whole corpus would be quadratic. The
// index narrows by trigram signature first; page search simply tries the
// tokens of the one file it read.
func scoreTypo(term, token []rune) (int, matchTier, bool) {
	k := typoBudget(len(term))
	if k == 0 || len(token) < minTokenLen {
		return 0, tierNone, false
	}
	d := osaDistance(term, token, k)
	if d > k {
		return 0, tierNone, false
	}
	return typoBase - typoPerEdit*d, tierTypo, true
}

// ----------------------------------------------------------------------
// The ladder
// ----------------------------------------------------------------------

// scoreTerm runs rungs 1 and 2 against one candidate string: a title, a tag, a
// path, a line of text. term must already be folded (see fold); cand is folded
// here if needed by the caller passing pre-folded runes.
//
// Rung 3 is not attempted, because it needs tokens rather than a candidate
// string - see the file header and scoreTypo.
func scoreTerm(term, cand []rune) (int, []span, matchTier, bool) {
	if len(term) == 0 || len(cand) == 0 {
		return 0, nil, tierNone, false
	}
	if s, spans, ok := scoreSubstring(term, cand); ok {
		return s, spans, tierSubstring, true
	}
	if len(term) < minFuzzyTermLen {
		// One or two runes match nearly anything as a subsequence; allowing it
		// would fill the result list with noise on the way to the third
		// keystroke.
		return 0, nil, tierNone, false
	}
	if s, spans, ok := scoreSubsequence(term, cand); ok {
		return s, spans, tierSubsequence, true
	}
	return 0, nil, tierNone, false
}

// betterMatch reports whether match A should rank above match B. It is the ONE
// place the ordering contract lives, so a caller cannot accidentally compare
// scores across rungs:
//
//  1. a lower (better) tier always wins - substring over subsequence over typo
//  2. within a tier, the higher score wins
//
// Ordering by score alone is wrong and not by a small margin: a substring hit
// buried deep in a long line with no word boundaries scores 85, and a flawless
// subsequence match scores 95. The document that literally contains the word
// must not lose to the one that merely suggests it.
func betterMatch(aTier matchTier, aScore int, bTier matchTier, bScore int) bool {
	if aTier != bTier {
		if aTier == tierNone {
			return false
		}
		if bTier == tierNone {
			return true
		}
		return aTier < bTier
	}
	return aScore > bScore
}

// mergeSpans sorts spans by start and merges overlapping or touching ones, so
// a renderer can walk them in order and never has to handle a nested
// highlight. Used when several query terms hit the same line.
func mergeSpans(spans []span) []span {
	if len(spans) < 2 {
		return spans
	}
	sorted := make([]span, len(spans))
	copy(sorted, spans)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Start < sorted[j-1].Start; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	out := sorted[:1]
	for _, s := range sorted[1:] {
		last := &out[len(out)-1]
		if s.Start <= last.Start+last.Len {
			if end := s.Start + s.Len; end > last.Start+last.Len {
				last.Len = end - last.Start
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// splitQuery breaks a raw query into folded terms. Whitespace-separated, with
// AND semantics applied by the caller: every term must hit somewhere in a
// document for it to be a result.
func splitQuery(q string) [][]rune {
	var terms [][]rune
	for _, f := range strings.Fields(q) {
		if r := fold(f); len(r) > 0 {
			terms = append(terms, r)
		}
	}
	return terms
}
