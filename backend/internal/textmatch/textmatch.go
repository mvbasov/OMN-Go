// Package textmatch scores a search term against a text. It has no state.
package textmatch

// ----------------------------------------------------------------------
// The fuzzy matcher
// ----------------------------------------------------------------------
//
// This file only scores. It has no I/O and no state, and it uses the standard
// library alone. Each offset in this file is a RUNE offset, because a byte
// offset can cut a Cyrillic character in half.
//
// The ladder has three rungs, and the first rung that hits wins:
//
//	1. ScoreSubstring    The folded term appears as it is.
//	2. scoreSubsequence  The runes of the term appear in order, as in fzf.
//	3. ScoreTypo         A bounded edit distance finds a misspelling.
//
// BetterMatch compares the rungs BEFORE the scores. The weakest substring hit
// scores 85, and a perfect subsequence scores 95. An order by score alone
// would thus put a vague match above the word itself. TierPhrase stands above
// the three rungs, and it belongs to a whole query. See scoreDocument in
// package search.
//
// ScoreTerm runs rungs 1 and 2. Rung 3 needs a set of candidate tokens, and
// only the caller can make that set: the index, or the search of one page.
// This file thus gives Tokenize, ScoreTypo and osaDistance to the caller.
//
// Each rung answers a different question: "I know what it says", "I remember
// the name roughly", and "I typed it wrong". One scorer for all three
// questions gives an order that nobody can predict.

import "unicode"

// Span is the range of a match in a candidate, in rune offsets.
type Span struct{ Start, Len int }

// Tier records the rung that gave a score. A test can thus check how an
// input matched, and not only that it matched.
type Tier uint8

const (
	TierNone Tier = iota
	// TierPhrase belongs to a whole query, and ScoreTerm never returns it.
	// scoreDocument gives it to a document, and to the line, that holds each
	// query word in order and side by side. It is a rung and not a bonus,
	// because a bonus cannot win against a sum. See scoreDocument.
	TierPhrase
	TierSubstring
	TierSubsequence
	TierTypo
)

// String names the tier in the failure message of a test.
func (t Tier) String() string {
	switch t {
	case TierPhrase:
		return "phrase"
	case TierSubstring:
		return "substring"
	case TierSubsequence:
		return "subsequence"
	case TierTypo:
		return "typo"
	default:
		return "none"
	}
}

// These are the scoring constants. TestScore_E1 to TestScore_E8 hold the
// exact numbers, thus a change here changes the order of the results.
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

// FoldTable holds the diacritic mappings that the fold applies after it
// lowercases a rune. Each entry maps ONE rune to ONE rune, because a change
// of length moves each span. An expanding fold, such as ß to ss, is absent on
// purpose. ё maps to е, because a person often types е for ё. OMN_FOLD_TABLE
// in omn-go-highlight.js is a copy, and TestFoldTableHasAFrontendCopy compares the
// two.
var FoldTable = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ñ': 'n', 'ç': 'c',
	'ё': 'е',
}

// foldRune lowercases a rune and removes the diacritics of FoldTable.
func foldRune(r rune) rune {
	if r < utf8SelfMax {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}
	r = unicode.ToLower(r)
	if f, ok := FoldTable[r]; ok {
		return f
	}
	return r
}

// utf8SelfMax is the ASCII bound. Below it, a fold needs one comparison and
// no map lookup. The fold runs over each line of each indexed file.
const utf8SelfMax = 0x80

// Fold answers s folded, as runes. It gives one rune for each input rune,
// thus an offset in the result is also an offset in s.
func Fold(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		out = append(out, foldRune(r))
	}
	return out
}

// IsShortTerm tells whether a term is too short to match a line alone. One
// Latin or Cyrillic rune hits almost each line. One Han, Hiragana, Katakana
// or Hangul rune is a whole word, thus it is never short.
func IsShortTerm(runes []rune) bool {
	if len(runes) > 1 || len(runes) == 0 {
		return false
	}
	r := runes[0]
	return !unicode.Is(unicode.Han, r) &&
		!unicode.Is(unicode.Hiragana, r) &&
		!unicode.Is(unicode.Katakana, r) &&
		!unicode.Is(unicode.Hangul, r)
}

// IsWordRune tells what is inside a word, for the word bonuses and for
// Tokenize. A letter or a digit of any script counts. '_' counts too, because
// it joins the parts of an identifier.
func IsWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// RuneMask answers a 64-bit signature of the runes in rs. Rungs 1 and 2 need
// EACH rune of the term, thus MaskRejects never rejects a real match. A
// collision costs only a check that finds nothing. Rung 3 cannot use the
// mask, because a typo is a rune that the text does not have.
func RuneMask(rs []rune) uint64 {
	var m uint64
	for _, r := range rs {
		m |= 1 << (uint32(r) % 64)
	}
	return m
}

// MaskRejects reports whether cand cannot possibly contain every rune of term.
func MaskRejects(termMask, candMask uint64) bool {
	return termMask&^candMask != 0
}

// Tokenize splits raw text into folded tokens for the typo rung. A token is a
// run of word runes. Tokenize also gives the camelCase parts of an
// identifier, thus a typo of "json" reaches "loadJSON". It reads the RAW
// text, because the fold removes the case that the split needs.
//
// Tokenize drops a token outside [minTokenLen, maxTokenLen]. The typo rung
// needs a term of 4 runes or more, and a token must be within k runes of the
// term in length. A token of 1 or 2 runes thus never matches. The upper bound
// drops a base64 blob.
func Tokenize(s string) []string {
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
		if IsWordRune(r) {
			word = append(word, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// camelSplit splits an identifier at a change of case: "loadJSON" gives load
// and JSON, and "JSONFile" gives JSON and File. With nothing to split, it
// answers one part, and the caller then emits no duplicate.
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

// indexRunes is strings.Index for rune slices. It answers a rune offset.
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

// ScoreSubstring scores each hit of a folded term in cand, and it answers a
// span for each hit. The score comes from the best hit, and not from the
// first. "xjson /json" thus gets the score of "/json".
func ScoreSubstring(term, cand []rune) (int, []Span, bool) {
	if len(term) == 0 {
		return 0, nil, false
	}
	var spans []Span
	best := 0
	for at := indexRunes(cand, term, 0); at >= 0; at = indexRunes(cand, term, at+1) {
		spans = append(spans, Span{Start: at, Len: len(term)})

		s := substringBase
		if len(cand) == len(term) {
			s += bonusFieldEqual
		}
		if at == 0 || !IsWordRune(cand[at-1]) {
			s += bonusWordStart
		}
		if end := at + len(term); end >= len(cand) || !IsWordRune(cand[end]) {
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

// scoreSubsequence scores the runes of term in order in cand, with the
// leftmost greedy match. Density and a word start add points, and a gap costs
// points.
//
// The total is normalized against the IDEAL match: the term as one run at a
// word start. A division by the length would lower the score of a longer
// query, and the results would move while the user types. The fold removed
// the case, thus only a separator marks a word start here.
func scoreSubsequence(term, cand []rune) (int, []Span, bool) {
	if len(term) == 0 {
		return 0, nil, false
	}
	raw, ti, gaps, skipped, prev := 0, 0, 0, 0, -2
	var spans []Span
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
			spans = append(spans, Span{Start: ci, Len: 1})
		}
		if ci == 0 || !IsWordRune(cand[ci-1]) {
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

// TypoBudget answers the number of edits for a term of this length. It
// answers 0 for a term that is too short to guess.
func TypoBudget(termLen int) int {
	switch {
	case termLen >= typoK2TermLen:
		return 2
	case termLen >= typoMinTermLen:
		return 1
	default:
		return 0
	}
}

// osaDistance is the Damerau-Levenshtein distance with the optimal string
// alignment rule, bounded by k. It answers k+1 as soon as the distance is
// above k, thus a miss costs little. Plain Levenshtein charges 2 for an
// adjacent transposition, the most frequent typing fault. "fecth" would then
// miss "fetch" at k=1.
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

// ScoreTypo scores a folded term against one token from Tokenize. The caller
// chooses the tokens. This rung cannot use the mask, thus a whole corpus
// would cost quadratic time. The index first narrows the tokens by trigram
// signature. A page search tries the tokens of its one file.
func ScoreTypo(term, token []rune) (int, Tier, bool) {
	k := TypoBudget(len(term))
	if k == 0 || len(token) < minTokenLen {
		return 0, TierNone, false
	}
	d := osaDistance(term, token, k)
	if d > k {
		return 0, TierNone, false
	}
	return typoBase - typoPerEdit*d, TierTypo, true
}

// ScoreTerm runs rungs 1 and 2 against one folded candidate: a title, a tag,
// a path or a line. Rung 3 needs tokens. See ScoreTypo.
func ScoreTerm(term, cand []rune) (int, []Span, Tier, bool) {
	if len(term) == 0 || len(cand) == 0 {
		return 0, nil, TierNone, false
	}
	if s, spans, ok := ScoreSubstring(term, cand); ok {
		return s, spans, TierSubstring, true
	}
	if len(term) < minFuzzyTermLen {
		// One or two runes match almost any text as a subsequence. Stop here,
		// or the list fills with noise before the third keystroke.
		return 0, nil, TierNone, false
	}
	if s, spans, ok := scoreSubsequence(term, cand); ok {
		return s, spans, TierSubsequence, true
	}
	return 0, nil, TierNone, false
}

// BetterMatch reports whether match A ranks above match B. It is the ONE
// place of the order rule. A better tier always wins, and within a tier the
// higher score wins. See the banner. TestTierSeparation holds the rule.
func BetterMatch(aTier Tier, aScore int, bTier Tier, bScore int) bool {
	if aTier != bTier {
		if aTier == TierNone {
			return false
		}
		if bTier == TierNone {
			return true
		}
		return aTier < bTier
	}
	return aScore > bScore
}

// MergeSpans sorts spans by start, and it merges a pair that overlaps or
// touches. A renderer then never gets a nested highlight.
func MergeSpans(spans []Span) []Span {
	if len(spans) < 2 {
		return spans
	}
	sorted := make([]Span, len(spans))
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
