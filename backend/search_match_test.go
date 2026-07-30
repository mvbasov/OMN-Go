package backend

// Tests for the fuzzy matcher.
//
// The first block is the worked examples from the plan, asserted to the exact
// point. They are not decoration: the constants in search_match.go ARE the
// ranking, so "did that tweak change what comes first" is otherwise an
// unanswerable question. Each case shows its arithmetic, so a failure tells you
// which term of the sum moved.
//
// The second block is properties that must hold whatever the constants are -
// tier ordering, the mask never lying, spans landing on real characters.

import (
	"math/rand"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------
// Worked examples (plan section 3.5)
// ---------------------------------------------------------------------

// E1 - exact substring, and why field weights matter.
func TestScore_E1_Substring(t *testing.T) {
	term := fold("bookmark")
	cases := []struct {
		what string
		cand string
		want int
		why  string
	}{
		{"title", "Bookmarks", 130, "100 +30 word start, no word end (followed by 's'), pos 0"},
		{"path", "Bookmarks", 130, "same string, same score - the FIELD weight differs, not this"},
		{"note content", "<script>bookmarks = [", 126, "100 +30 after '>', pos 8 -> -4"},
		{"asset path", "json/bookmarker-tags.json", 128, "100 +30 after '/', pos 5 -> -2"},
	}
	for _, c := range cases {
		got, spans, tier, ok := scoreTerm(term, fold(c.cand))
		if !ok {
			t.Errorf("%s: no match in %q", c.what, c.cand)
			continue
		}
		if tier != tierSubstring {
			t.Errorf("%s: tier %v, want substring", c.what, tier)
		}
		if got != c.want {
			t.Errorf("%s (%q): score %d, want %d (%s)", c.what, c.cand, got, c.want, c.why)
		}
		if len(spans) != 1 {
			t.Errorf("%s: %d spans, want 1", c.what, len(spans))
		}
	}
}

// E2 - subsequence: half-remembered names. "andint" finds "Android Intents".
func TestScore_E2_Subsequence(t *testing.T) {
	term := fold("andint")

	got, _, tier, ok := scoreTerm(term, fold("Android Intents & Termux"))
	if !ok {
		t.Fatal("andint did not match the title as a subsequence")
	}
	if tier != tierSubsequence {
		t.Fatalf("tier %v, want subsequence", tier)
	}
	// matched 6x16=96, consecutive [0,1,2]=+16 and [9,10]=+8, word start +12,
	// 2 gaps -6 and 5 skipped runes -5  => raw 121; ideal 16*6+8*5+12 = 148;
	// round(95*121/148) = 78.
	if want := 78; got != want {
		t.Errorf("title score %d, want %d", got, want)
	}

	// The same query against the path form scores the same: different gaps,
	// same normalised quality. (What separates them downstream is the field
	// weight, not this number.)
	if got, _, _, ok := scoreTerm(term, fold("AndroidIntents")); !ok || got != 78 {
		t.Errorf("path score %d (ok=%v), want 78", got, ok)
	}
}

// E3 - the typo rung, and why OSA rather than plain Levenshtein.
func TestScore_E3_Typo(t *testing.T) {
	term, token := fold("fecth"), fold("fetch")

	if d := osaDistance(term, token, 1); d != 1 {
		t.Errorf("OSA(fecth, fetch) = %d, want 1 (one adjacent transposition)", d)
	}
	// Plain Levenshtein charges 2 for a transposition. Proving the difference
	// keeps anyone from "simplifying" osaDistance into the textbook version:
	// at k=1 that would silently stop finding misspelled words.
	if d := levenshteinForTest(term, token); d != 2 {
		t.Errorf("plain Levenshtein(fecth, fetch) = %d, want 2 - the whole reason OSA is used", d)
	}

	got, tier, ok := scoreTypo(term, token)
	if !ok {
		t.Fatal("fecth did not match fetch at k=1")
	}
	if tier != tierTypo {
		t.Errorf("tier %v, want typo", tier)
	}
	if want := 45; got != want { // 60 - 15*1
		t.Errorf("score %d, want %d", got, want)
	}
}

// E4 - multi-term AND across fields, on the real line 15 of Test/OMN-Go/Fetch.md.
func TestScore_E4_MultiTerm(t *testing.T) {
	const line = "const response = await fetch('/json/test.json'); // Relative path to your JSON file"

	// "fetch" in the title: 100 +30 (after '/') +20 (ends the field) -5 (pos 11)
	if got, _, _, ok := scoreTerm(fold("fetch"), fold("Test/OMNGo/Fetch")); !ok || got != 145 {
		t.Errorf("title/fetch = %d (ok=%v), want 145", got, ok)
	}

	// "json" in the content line: three occurrences, all whole words, all past
	// the positional cap: 100 +30 +20 -15 = 135.
	got, spans, tier, ok := scoreTerm(fold("json"), fold(line))
	if !ok || tier != tierSubstring {
		t.Fatalf("content/json: ok=%v tier=%v", ok, tier)
	}
	if want := 135; got != want {
		t.Errorf("content/json = %d, want %d", got, want)
	}
	// Every occurrence is highlighted, including the case-insensitive one in
	// the comment.
	wantSpans := []span{{31, 4}, {41, 4}, {74, 4}}
	if len(spans) != len(wantSpans) {
		t.Fatalf("spans = %v, want %v", spans, wantSpans)
	}
	for i, s := range spans {
		if s != wantSpans[i] {
			t.Errorf("span %d = %+v, want %+v", i, s, wantSpans[i])
		}
	}

	// AND semantics are the caller's job, but the ingredient is here: a
	// document that fails one term contributes nothing.
	if _, _, _, ok := scoreTerm(fold("fetch"), fold("html/json/test.json")); ok {
		t.Error("'fetch' should not match html/json/test.json at all")
	}
}

// E5 - the whole-field equality bonus is what makes an exact tag hit win.
func TestScore_E5_FieldEquality(t *testing.T) {
	// tag "android" IS the term: 100 +50 +30 +20
	if got, _, _, ok := scoreTerm(fold("android"), fold("Android")); !ok || got != 200 {
		t.Errorf("exact tag = %d (ok=%v), want 200", got, ok)
	}
	// The same term inside a longer title scores less, by design:
	// 100 +30 (after a space), no word end ('s' follows), pos 8 -> -4 = 126.
	if got, _, _, ok := scoreTerm(fold("intent"), fold("Android Intents & Termux")); !ok || got != 126 {
		t.Errorf("title substring = %d (ok=%v), want 126", got, ok)
	}
}

// E6 - Cyrillic, and why every offset here is a rune offset.
func TestScore_E6_CyrillicRuneSpans(t *testing.T) {
	got, spans, _, ok := scoreTerm(fold("замет"), fold("Заметки"))
	if !ok {
		t.Fatal("Cyrillic substring did not match")
	}
	if want := 130; got != want { // 100 +30 word start, no word end, pos 0
		t.Errorf("score %d, want %d", got, want)
	}
	if len(spans) != 1 || spans[0] != (span{0, 5}) {
		t.Fatalf("spans %v, want [{0 5}] in RUNES - the same match is {0,10} in bytes", spans)
	}
	// Prove the offsets index the original text correctly.
	title := []rune("Заметки")
	if got := string(title[spans[0].Start : spans[0].Start+spans[0].Len]); got != "Замет" {
		t.Errorf("span slices to %q, want %q - byte offsets would cut a rune in half", got, "Замет")
	}
}

// E7 - thresholds, and honest non-matches.
func TestScore_E7_Thresholds(t *testing.T) {
	// 3 runes: below the typo threshold, but subsequence still applies.
	// r->0, s->2, p->3: 48 +8 consecutive +12 word start -4 gap => 64;
	// ideal 16*3+8*2+12 = 76; round(95*64/76) = 80.
	if got, _, tier, ok := scoreTerm(fold("rsp"), fold("response")); !ok || tier != tierSubsequence || got != 80 {
		t.Errorf("rsp/response = %d tier=%v ok=%v, want 80 subsequence", got, tier, ok)
	}

	// 2 runes: substring only. "js" is a subsequence of half the English
	// language, and matching it that way is pure noise.
	if _, _, _, ok := scoreTerm(fold("js"), fold("jumps over")); ok {
		t.Error("2-rune term matched as a subsequence; it must be substring-only")
	}
	if _, _, tier, ok := scoreTerm(fold("js"), fold("main.js")); !ok || tier != tierSubstring {
		t.Errorf("2-rune term should still match verbatim: ok=%v tier=%v", ok, tier)
	}

	// Two typos in one short word is out of scope by design: widening k here
	// costs precision on every query.
	if _, _, ok := scoreTypo(fold("fecthh"), fold("fetch")); ok {
		t.Error("fecthh matched fetch; k should be 1 for a 6-rune term")
	}
	// ... but a long term gets two edits.
	if typoBudget(8) != 2 || typoBudget(7) != 1 || typoBudget(3) != 0 {
		t.Errorf("typo budgets wrong: 8->%d 7->%d 3->%d", typoBudget(8), typoBudget(7), typoBudget(3))
	}
}

// E8 - the mask prefilter, including the numbers from the plan.
func TestScore_E8_Mask(t *testing.T) {
	term := fold("tok")
	tm := runeMask(term)
	for _, r := range []rune{'t', 'o', 'k'} {
		if tm&(1<<(uint32(r)%64)) == 0 {
			t.Errorf("mask missing bit for %q (bit %d)", r, uint32(r)%64)
		}
	}
	// A line without 'k' is rejected without looking at a single rune.
	if !maskRejects(tm, runeMask(fold("the other one"))) {
		t.Error("line lacking 'k' was not rejected by the mask")
	}
	// A line containing all three is not rejected (it may still fail to match,
	// which is fine - the mask is a filter, not an answer).
	if maskRejects(tm, runeMask(fold("take out kettle"))) {
		t.Error("mask rejected a line containing t, o and k")
	}
}

// ---------------------------------------------------------------------
// Properties
// ---------------------------------------------------------------------

// The rungs must never cross: any verbatim hit outranks any fuzzy one, however
// good the fuzzy one looks. This is what makes result order explainable.
//
// Note what this test does NOT assume - that the score ranges happen not to
// overlap. They DO overlap: the weakest substring hit scores 85 and a perfect
// subsequence scores 95. Separation is enforced by comparing tiers first
// (betterMatch), which is why that helper exists rather than a bare "sort by
// score" at each call site.
func TestTierSeparation(t *testing.T) {
	weakestSubstring := substringBase - maxPosPenalty/2
	if weakestSubstring > subseqCeiling {
		t.Logf("note: score ranges no longer overlap (substring floor %d > subsequence ceiling %d); "+
			"tier-first comparison is still the contract", weakestSubstring, subseqCeiling)
	}

	// The case that matters: a genuinely weak substring hit against the best
	// possible fuzzy one.
	weak, _, weakTier, ok := scoreTerm(fold("json"), fold(strings.Repeat("x", 40)+" and then json somewhere"))
	if !ok || weakTier != tierSubstring {
		t.Fatalf("setup: ok=%v tier=%v", ok, weakTier)
	}
	strong, _, strongTier, ok := scoreTerm(fold("json"), fold("json"))
	if !ok {
		t.Fatal("setup: exact match failed")
	}
	_ = strong
	_ = strongTier

	perfectFuzzy, _, fuzzyTier, ok := scoreSubsequenceTier(fold("jsn"), fold("jsn tail"))
	if !ok || fuzzyTier != tierSubsequence {
		t.Fatalf("setup: fuzzy ok=%v tier=%v", ok, fuzzyTier)
	}
	if perfectFuzzy <= weak {
		t.Logf("note: this test is only meaningful while a fuzzy score (%d) can exceed "+
			"a substring score (%d)", perfectFuzzy, weak)
	}
	if !betterMatch(weakTier, weak, fuzzyTier, perfectFuzzy) {
		t.Errorf("a weak substring hit (%d) lost to a perfect fuzzy one (%d); "+
			"tiers must be compared before scores", weak, perfectFuzzy)
	}

	// And a typo match loses to both.
	typo, typoTier, ok := scoreTypo(fold("fecth"), fold("fetch"))
	if !ok {
		t.Fatal("setup: typo match failed")
	}
	if !betterMatch(fuzzyTier, perfectFuzzy, typoTier, typo) {
		t.Error("a subsequence match must outrank a typo match")
	}
	if !betterMatch(tierSubstring, weak, typoTier, typo) {
		t.Error("a substring match must outrank a typo match")
	}
	// Within one tier, the score decides.
	if !betterMatch(tierSubstring, 130, tierSubstring, 126) {
		t.Error("within a tier, the higher score must win")
	}
	if betterMatch(tierNone, 999, tierSubstring, 1) {
		t.Error("a non-match must never outrank a match")
	}
}

// scoreSubsequenceTier is a thin test shim: scoreTerm tries substring first, so
// there is no way through the public entry point to obtain a subsequence score
// for text that also contains the term verbatim.
func scoreSubsequenceTier(term, cand []rune) (int, []span, matchTier, bool) {
	s, spans, ok := scoreSubsequence(term, cand)
	if !ok {
		return 0, nil, tierNone, false
	}
	return s, spans, tierSubsequence, true
}

// A longer query must not score lower for an equally good match - otherwise
// results visibly reorder while the user is still typing. This is the property
// the ideal-normalisation exists for; dividing by term length breaks it.
func TestSubsequenceNormalisationIsLengthStable(t *testing.T) {
	// Same shape of match at three lengths: every rune consecutive, at a word
	// start. All three should be the maximum score.
	for _, s := range []string{"abcd", "abcdefgh", "abcdefghijkl"} {
		got, _, ok := scoreSubsequence(fold(s), fold(s+" tail"))
		if !ok {
			t.Fatalf("%q did not match", s)
		}
		if got != subseqCeiling {
			t.Errorf("%q (len %d) scored %d, want the ceiling %d - a perfect match must "+
				"be worth the same at every length", s, len(s), got, subseqCeiling)
		}
	}
}

// The mask may never reject something that actually matches. Fuzzed against
// the real scorer, because a false negative here is invisible: the result
// simply never appears.
func TestMaskNeverRejectsARealMatch(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []rune("abcdefgпривет_-/. 1234")
	randStr := func(n int) string {
		b := make([]rune, n)
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(b)
	}
	for i := 0; i < 20000; i++ {
		term := fold(randStr(1 + rng.Intn(5)))
		cand := fold(randStr(1 + rng.Intn(40)))
		_, _, _, ok := scoreTerm(term, cand)
		if ok && maskRejects(runeMask(term), runeMask(cand)) {
			t.Fatalf("mask rejected a real match: term=%q cand=%q", string(term), string(cand))
		}
	}
}

// Spans must always describe real, in-bounds ranges - a renderer slices by
// them, and an off-by-one produces mojibake rather than an error.
func TestSpansAreInBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	alphabet := []rune("abcабв /._")
	randStr := func(n int) string {
		b := make([]rune, n)
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(b)
	}
	for i := 0; i < 20000; i++ {
		term := fold(randStr(1 + rng.Intn(4)))
		cand := fold(randStr(1 + rng.Intn(30)))
		_, spans, _, ok := scoreTerm(term, cand)
		if !ok {
			continue
		}
		for _, s := range spans {
			if s.Start < 0 || s.Len <= 0 || s.Start+s.Len > len(cand) {
				t.Fatalf("span %+v out of bounds for %q (len %d)", s, string(cand), len(cand))
			}
		}
	}
}

func TestFoldingIsLengthPreserving(t *testing.T) {
	// Every fold must be one rune to one rune, or spans point at the wrong
	// place in the original text. This is why the expanding folds are absent.
	for _, s := range []string{"Ünïcôde", "ЁЛКА", "Mixed Case", "ß and æ stay", "日本語"} {
		if got, want := len(fold(s)), len([]rune(s)); got != want {
			t.Errorf("fold(%q) has %d runes, want %d", s, got, want)
		}
	}
	if got := foldString("Ёлка"); got != "елка" {
		t.Errorf("foldString(Ёлка) = %q, want елка - the letter people omit when typing", got)
	}
	if got := foldString("Café"); got != "cafe" {
		t.Errorf("foldString(Café) = %q, want cafe", got)
	}
}

func TestTokenize(t *testing.T) {
	// Real line 13 of Test/OMN-Go/Fetch.md.
	got := tokenize("async function loadJSON() {")
	want := []string{"async", "function", "loadjson", "load", "json"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tokenize = %v, want %v", got, want)
	}

	// The length filter. "to" can never match anything at the allowed edit
	// budgets, so it is not worth a dictionary entry.
	for _, tok := range tokenize("go to the end") {
		if len(tok) < minTokenLen {
			t.Errorf("token %q shorter than the minimum %d", tok, minTokenLen)
		}
	}
	long := strings.Repeat("x", maxTokenLen+1)
	for _, tok := range tokenize(long) {
		t.Errorf("over-long token kept: %q", tok)
	}

	// A URL splits into its useful parts on the punctuation alone.
	got = tokenize("https://github.com/mvbasov/OMN-Go")
	for _, want := range []string{"https", "github", "com", "mvbasov", "omn"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("tokenize(URL) = %v, missing %q", got, want)
		}
	}
}

func TestMergeSpans(t *testing.T) {
	got := mergeSpans([]span{{10, 4}, {0, 5}, {4, 2}, {20, 1}})
	want := []span{{0, 6}, {10, 4}, {20, 1}}
	if len(got) != len(want) {
		t.Fatalf("merged to %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitQuery(t *testing.T) {
	terms := splitQuery("  Fecth   JSON  ")
	if len(terms) != 2 {
		t.Fatalf("got %d terms, want 2", len(terms))
	}
	if string(terms[0]) != "fecth" || string(terms[1]) != "json" {
		t.Errorf("terms = %q, %q, want folded fecth, json", string(terms[0]), string(terms[1]))
	}
	if len(splitQuery("   ")) != 0 {
		t.Error("whitespace-only query should produce no terms")
	}
}

func TestOSADistanceBounds(t *testing.T) {
	cases := []struct {
		a, b string
		k    int
		want int
	}{
		{"fetch", "fetch", 1, 0},
		{"fecth", "fetch", 1, 1},  // transposition
		{"fetc", "fetch", 1, 1},   // deletion
		{"fetchx", "fetch", 1, 1}, // insertion
		{"fatch", "fetch", 1, 1},  // substitution
		{"fecthh", "fetch", 1, 2}, // over budget -> k+1
		{"abcdefgh", "hgfedcba", 2, 3},
	}
	for _, c := range cases {
		if got := osaDistance(fold(c.a), fold(c.b), c.k); got != c.want {
			t.Errorf("osaDistance(%q,%q,k=%d) = %d, want %d", c.a, c.b, c.k, got, c.want)
		}
	}
}

// levenshteinForTest is the textbook algorithm, present ONLY so E3 can show
// what OSA buys. Not used by production code.
func levenshteinForTest(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
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
			cur[j] = v
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
