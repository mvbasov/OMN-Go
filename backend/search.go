package backend

// ----------------------------------------------------------------------
// Search: the query layer
// ----------------------------------------------------------------------
//
// This file turns a query string into results. It contains TWO searches that
// share one matcher (search_match.go):
//
//   - PAGE search (scope=page) - the open note only. Reads that one file,
//     scores it, returns, and keeps nothing. No index, no configuration, no
//     gate: there is no standing cost to opt out of, so it is always
//     available. That is this phase.
//   - GLOBAL search (scope=all) - everything, through the index. A later
//     phase; until then it answers 503 rather than pretending.
//
// Same matcher, same scoring, same response shape for both, so a result means
// the same thing whichever scope produced it - only the haystack differs.
//
// Nothing here holds state between requests. A page-scope query allocates a
// document, scores it and drops it; on a 2.5 KB note that is microseconds and
// a few kilobytes of garbage.

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Limits and defaults for one request.
const (
	searchDefaultSnippets = 3
	searchMaxSnippets     = 10
	searchDefaultLimit    = 50
	searchMaxLimit        = 200

	// maxIndexFileBytes caps how much of any one file is searched: the first
	// 500 KiB, cut at a line boundary. A larger file is NOT skipped - it is
	// searched up to the cap and flagged truncated, so the answer is "found
	// nothing in the part I looked at" rather than silence.
	maxIndexFileBytes = 500 << 10

	// snippetMaxRunes / snippetLead shape a snippet around its first hit.
	snippetMaxRunes = 160
	snippetLead     = 60
)

// Field weights, x10 so the arithmetic stays in integers and is exactly
// reproducible (see the worked examples in the plan). A title hit is worth
// three content hits; the ordering these produce IS the feature, so they are
// named here rather than inlined.
const (
	weightTitle   = 30
	weightTags    = 25
	weightPath    = 20
	weightHeader  = 15
	weightContent = 10
)

// Kind weights, x100. A note beats a config blob at equal score.
var kindWeight = map[string]int{
	"md":        100,
	"bookmarks": 100,
	"json":      90,
	"user_json": 90,
	"js":        85,
}

// ----------------------------------------------------------------------
// The query
// ----------------------------------------------------------------------

// queryTerm is one whitespace-separated term, folded, with its character mask
// precomputed and its optional field restriction resolved.
type queryTerm struct {
	runes []rune
	mask  uint64
	field string // "" = any field; otherwise "title", "tag", "path"
}

// parsedQuery is a query string after parsing: the terms that must ALL match,
// plus any kind filter pulled out of a "kind:" prefix.
type parsedQuery struct {
	terms []queryTerm
	kinds []string
}

// parseQuery splits a raw query into terms and pulls out the field prefixes.
// "tag:hydro title:manual json" restricts the first two terms and leaves the
// third free. An unknown prefix is NOT treated as a prefix - "http://x" must
// stay a search for "http://x", not a search of a field called "http".
func parseQuery(q string) parsedQuery {
	var out parsedQuery
	for _, f := range strings.Fields(q) {
		field := ""
		if k, v, found := strings.Cut(f, ":"); found && v != "" {
			switch strings.ToLower(k) {
			case "title", "tag", "tags", "path", "name":
				field = normalizeQueryField(strings.ToLower(k))
				f = v
			case "kind":
				for _, kind := range strings.Split(v, ",") {
					if kind = strings.TrimSpace(strings.ToLower(kind)); kind != "" {
						out.kinds = append(out.kinds, kind)
					}
				}
				continue
			}
		}
		runes := fold(f)
		if len(runes) == 0 {
			continue
		}
		out.terms = append(out.terms, queryTerm{runes: runes, mask: runeMask(runes), field: field})
	}
	return out
}

func normalizeQueryField(k string) string {
	switch k {
	case "tags":
		return "tag"
	case "name":
		return "path"
	default:
		return k
	}
}

// ----------------------------------------------------------------------
// The document being searched
// ----------------------------------------------------------------------

// docField is one short, weighted piece of metadata: a title, a tag, the path.
type docField struct {
	name   string
	text   []rune // folded
	weight int    // x10, see the weight constants
}

// docLine is one line of content: its number in the file AS STORED, its raw
// text (kept for snippets, so the original case survives), the folded form the
// matcher works on, and the character mask that lets most lines be rejected
// without looking at them.
type docLine struct {
	no      int
	raw     string
	fold    []rune
	mask    uint64
	context string // "", "code" or "script" - see classifyContexts
}

// searchDocument is one searchable thing. Page search builds exactly one of
// these per request and throws it away; the index will build many and keep a
// reduced form of them.
type searchDocument struct {
	Path      string // storage-relative, slash form ("md/Test/OMN-Go/Fetch.md")
	Kind      string
	Name      string // page name for md, file path for an asset
	Title     string
	Tags      []string
	URL       string
	fields    []docField
	lines     []docLine
	truncated bool
}

// loadPageDocument reads and parses the single file a page-scope query targets.
//
// name is whatever the frontend was looking at - "Note", "Note.html",
// "Note.md" or "js/thing.js" - resolved through resolvePageName, the one place
// that decision lives. Returns nil (no error) when the target does not exist or
// resolves outside the storage directory: a query for something that is not
// there is an empty result, not a failure, and never a way to probe the
// filesystem.
func (a *App) loadPageDocument(name string) (*searchDocument, error) {
	if name == "" {
		return nil, nil
	}
	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	filePath := htmlPath
	if isPage {
		filePath = mdPath
	}
	if !a.withinStorage(filePath) {
		return nil, nil
	}

	data, truncated, err := readCapped(filePath, maxIndexFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if isBinary(data) {
		return nil, nil
	}

	if isPage {
		return newMarkdownDocument(baseName, string(data), truncated), nil
	}
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(name)), "/")
	return newAssetDocument(rel, string(data), truncated), nil
}

// newMarkdownDocument builds the searchable form of a note. Shared by page
// search (which builds one per request and drops it) and the index (which
// builds one per file, keeps a reduced form, and rebuilds this one on demand
// when a query actually reaches the document) - so both see identical fields,
// identical line numbering and identical context labels.
func newMarkdownDocument(baseName, content string, truncated bool) *searchDocument {
	doc := &searchDocument{
		Path:      "md/" + baseName + ".md",
		Kind:      SearchKindMD,
		Name:      baseName,
		URL:       "/" + baseName + ".html",
		truncated: truncated,
	}
	doc.parseMarkdown(content)
	return doc
}

// newAssetDocument is the same for a file with no front matter: a script, a
// JSON blob. rel is storage-relative below html/ ("js/mine.js").
func newAssetDocument(rel, content string, truncated bool) *searchDocument {
	doc := &searchDocument{
		Path:      "html/" + rel,
		Kind:      assetKind(rel),
		Name:      rel,
		URL:       "/" + rel,
		truncated: truncated,
	}
	doc.parsePlain(content)
	return doc
}

// parseMarkdown fills a document from note source: the front-matter header
// becomes weighted fields, and only the BODY becomes content lines - so a
// "Category: Notes" header never shows up as a content hit. Line numbers stay
// true to the file, header included, because that is what the reader sees.
func (d *searchDocument) parseMarkdown(content string) {
	fm := splitFrontMatter(content)
	title, tags := extractTitleTags(content)
	if title == "" {
		title = d.Name
	}
	d.Title = title
	d.Tags = tags

	d.fields = append(d.fields,
		docField{name: "title", text: fold(title), weight: weightTitle},
		docField{name: "path", text: fold(d.Name), weight: weightPath})
	for _, t := range tags {
		d.fields = append(d.fields, docField{name: "tag", text: fold(t), weight: weightTags})
	}
	if fm.HasHeader {
		for _, h := range strings.Split(fm.Header, "\n") {
			k, v, found := strings.Cut(h, ":")
			if !found {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "title", "tags":
				continue // already weighted above, at their own weights
			}
			if v = strings.TrimSpace(v); v != "" {
				d.fields = append(d.fields, docField{
					name: strings.ToLower(strings.TrimSpace(k)), text: fold(v), weight: weightHeader})
			}
		}
	}

	// Body line numbers continue from the header rather than restarting.
	firstBodyLine := 1 + strings.Count(content[:fm.BodyOffset], "\n")
	d.addLines(fm.Body, firstBodyLine)
}

// parsePlain fills a document from a file with no front matter (a script, a
// JSON blob): every line is content, and the path is the only field.
func (d *searchDocument) parsePlain(content string) {
	d.Title = path.Base(d.Name)
	d.fields = append(d.fields,
		docField{name: "path", text: fold(d.Name), weight: weightPath})
	d.addLines(content, 1)
}

func (d *searchDocument) addLines(content string, firstLineNo int) {
	raw := strings.Split(content, "\n")
	contexts := classifyContexts(raw)
	for i, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue // an empty line can never match, and costs memory to keep
		}
		f := fold(line)
		d.lines = append(d.lines, docLine{
			no:      firstLineNo + i,
			raw:     line,
			fold:    f,
			mask:    runeMask(f),
			context: contexts[i],
		})
	}
}

// classifyContexts labels every line prose, "code" or "script".
//
// A note's body legitimately contains JavaScript, and a hit inside it is a
// different kind of answer from a hit in prose - useful, but not the same, and
// a result list that presents them identically reads as noise. Marking is all
// this does: no score penalty, because down-ranking code would hide the answer
// from anyone searching FOR code, which in a notes app whose notes run scripts
// is a normal thing to want.
//
// Fence state wins over tag state, deliberately: a "<script>" MENTIONED inside
// a fenced example must not mark the rest of the file as script. That is the
// same failure the combined-scan comment in markdown.go records for the
// renderer.
//
// Line granularity means an inline `code` span is not marked - only fenced
// blocks, <pre> and <script>. Marking part of a line would need a span-level
// model that nothing in the UI can show today.
func classifyContexts(lines []string) []string {
	out := make([]string, len(lines))
	inFence, inScript, inPre := false, false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			out[i] = "code"
			inFence = !inFence
			continue
		}
		if inFence {
			out[i] = "code"
			continue
		}
		if inScript {
			out[i] = "script"
			if strings.Contains(lower, "</script>") {
				inScript = false
			}
			continue
		}
		if inPre {
			out[i] = "code"
			if strings.Contains(lower, "</pre>") {
				inPre = false
			}
			continue
		}
		if strings.Contains(lower, "<script") {
			out[i] = "script"
			if !strings.Contains(lower, "</script>") {
				inScript = true
			}
			continue
		}
		if strings.Contains(lower, "<pre") {
			out[i] = "code"
			if !strings.Contains(lower, "</pre>") {
				inPre = true
			}
			continue
		}
	}
	return out
}

// ----------------------------------------------------------------------
// Scoring a document
// ----------------------------------------------------------------------

// lineHit is one content line that matched, with every term's spans merged.
type lineHit struct {
	line  *docLine
	score int
	tier  matchTier
	spans []span
}

// scoreDocument applies AND semantics: every term must hit somewhere in the
// document, or the document is not a result at all. Within a term, the best
// (tier, weighted score) across all fields and lines wins - see betterMatch,
// which is why tiers are compared before numbers.
func scoreDocument(q parsedQuery, d *searchDocument) (int, matchTier, []lineHit, bool) {
	if len(q.terms) == 0 {
		return 0, tierNone, nil, false
	}

	total := 0
	worst := tierSubstring // the document's tier is its WEAKEST term's tier
	hits := map[int]*lineHit{}

	for _, term := range q.terms {
		bestScore, bestTier := 0, tierNone

		for _, f := range d.fields {
			if term.field != "" && term.field != f.name {
				continue
			}
			s, _, tier, ok := scoreTerm(term.runes, f.text)
			if !ok {
				continue
			}
			weighted := s * f.weight / 10
			if betterMatch(tier, weighted, bestTier, bestScore) {
				bestScore, bestTier = weighted, tier
			}
		}

		if term.field == "" {
			for i := range d.lines {
				ln := &d.lines[i]
				if maskRejects(term.mask, ln.mask) {
					continue // no rune loop, no allocation
				}
				s, spans, tier, ok := scoreTerm(term.runes, ln.fold)
				if !ok {
					continue
				}
				weighted := s * weightContent / 10
				if betterMatch(tier, weighted, bestTier, bestScore) {
					bestScore, bestTier = weighted, tier
				}
				h := hits[ln.no]
				if h == nil {
					h = &lineHit{line: ln}
					hits[ln.no] = h
				}
				h.spans = append(h.spans, spans...)
				// SUM across distinct terms, don't keep the best one. A line
				// carrying every term of the query is the line worth showing,
				// even when some other line matches a single term more
				// strongly: "await fetch('/json/test.json')" beats a heading
				// that merely says "fetch". Summing is what expresses that,
				// because each term contributes to a line at most once.
				h.score += weighted
				if tier > h.tier {
					h.tier = tier // a line is only as good as its weakest term
				}
			}
		}

		if bestTier == tierNone && term.field == "" {
			// Nothing matched verbatim or as a subsequence. Before giving up
			// on the document, try the typo rung - this is the rung that
			// finds "fetch" when the query said "fecth", and it is the only
			// one that cannot work from the character mask, so it runs last
			// and only when everything cheaper has failed.
			if s, spans, ok := scoreTypoInDocument(term.runes, d); ok {
				bestScore, bestTier = s.score, tierTypo
				for _, lh := range spans {
					h := hits[lh.line.no]
					if h == nil {
						h = &lineHit{line: lh.line}
						hits[lh.line.no] = h
					}
					h.spans = append(h.spans, lh.spans...)
					h.score += lh.score
					if lh.tier > h.tier {
						h.tier = lh.tier
					}
				}
			}
		}
		if bestTier == tierNone {
			return 0, tierNone, nil, false // AND: one miss drops the document
		}
		if bestTier > worst {
			worst = bestTier
		}
		total += bestScore
	}

	kw, ok := kindWeight[d.Kind]
	if !ok {
		kw = 100
	}
	total = total * kw / 100

	ordered := make([]lineHit, 0, len(hits))
	for _, h := range hits {
		h.spans = mergeSpans(h.spans)
		ordered = append(ordered, *h)
	}
	sortLineHits(ordered)
	return total, worst, ordered, true
}

// typoResult is the best token match found for a term in a document.
type typoResult struct {
	score int
	token string
}

// scoreTypoInDocument runs the edit-distance rung over the document's own
// tokens.
//
// Tokens are built here rather than kept in the index, because keeping them
// was the one structure that scaled with VOCABULARY rather than with text -
// measured at ~44k unique tokens per MB, which for a large collection means
// map entries in the millions. The index narrows to a handful of candidate
// documents by trigram signature (see search_index.go); this then pays the
// tokenisation cost only for those.
//
// Spans come from a plain substring scan for the matched TOKEN on the lines it
// appears on: the token is what is really in the text, so highlighting it is
// truthful in a way that highlighting the misspelled query would not be.
func scoreTypoInDocument(term []rune, d *searchDocument) (typoResult, []lineHit, bool) {
	if typoBudget(len(term)) == 0 {
		return typoResult{}, nil, false
	}

	best := typoResult{}
	bestWeight := 0
	seen := map[string]bool{}

	consider := func(text string, weight int) {
		for _, tok := range tokenize(text) {
			if seen[tok] {
				continue
			}
			seen[tok] = true
			s, _, ok := scoreTypo(term, []rune(tok))
			if !ok {
				continue
			}
			if weighted := s * weight / 10; weighted > bestWeight {
				bestWeight = weighted
				best = typoResult{score: weighted, token: tok}
			}
		}
	}

	for _, f := range d.fields {
		consider(string(f.text), f.weight)
	}
	for i := range d.lines {
		consider(d.lines[i].raw, weightContent)
	}
	if best.token == "" {
		return typoResult{}, nil, false
	}

	var hits []lineHit
	needle := []rune(best.token)
	for i := range d.lines {
		ln := &d.lines[i]
		if _, spans, ok := scoreSubstring(needle, ln.fold); ok {
			hits = append(hits, lineHit{line: ln, score: best.score, tier: tierTypo, spans: spans})
		}
	}
	return best, hits, true
}

// sortLineHits orders by (tier, score) descending, then by line number so the
// order is stable and reads top-to-bottom within a tie.
func sortLineHits(hits []lineHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			a, b := hits[j], hits[j-1]
			if betterMatch(a.tier, a.score, b.tier, b.score) ||
				(a.tier == b.tier && a.score == b.score && a.line.no < b.line.no) {
				hits[j], hits[j-1] = hits[j-1], hits[j]
				continue
			}
			break
		}
	}
}

// ----------------------------------------------------------------------
// Snippets
// ----------------------------------------------------------------------

// snippetFor trims a line down to something a result row can show, and shifts
// the spans to match. Rules, in order: strip surrounding whitespace; if what is
// left fits, use it; otherwise take a window around the first hit, marked with
// ellipses. Spans that fall outside the window are dropped rather than
// clamped - a highlight that does not cover its match is worse than none.
func snippetFor(raw string, spans []span) (string, []span) {
	runes := []rune(raw)

	lead := 0
	for lead < len(runes) && isSpace(runes[lead]) {
		lead++
	}
	end := len(runes)
	for end > lead && isSpace(runes[end-1]) {
		end--
	}
	runes = runes[lead:end]
	shifted := make([]span, 0, len(spans))
	for _, s := range spans {
		s.Start -= lead
		if s.Start >= 0 && s.Start+s.Len <= len(runes) {
			shifted = append(shifted, s)
		}
	}

	if len(runes) <= snippetMaxRunes {
		return string(runes), shifted
	}

	start := 0
	if len(shifted) > 0 && shifted[0].Start > snippetLead {
		start = shifted[0].Start - snippetLead
	}
	stop := start + snippetMaxRunes
	if stop > len(runes) {
		stop = len(runes)
		if start = stop - snippetMaxRunes; start < 0 {
			start = 0
		}
	}

	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if stop < len(runes) {
		suffix = "…"
	}

	out := make([]span, 0, len(shifted))
	for _, s := range shifted {
		if s.Start < start || s.Start+s.Len > stop {
			continue
		}
		s.Start += len([]rune(prefix)) - start
		out = append(out, s)
	}
	return prefix + string(runes[start:stop]) + suffix, out
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f'
}

// ----------------------------------------------------------------------
// Gating
// ----------------------------------------------------------------------
//
// Only GLOBAL search is gated. Page search has no setting because it has no
// standing cost: a switch whose "off" position saves nothing is just a way to
// break the feature by accident.

// globalSearchAvailable is the one answer to "can this server search
// everything right now": the user asked for it AND there is an index to ask.
// Injected into every page as OMN_SEARCH_GLOBAL (see injectRuntimeVars), so
// the dialog never offers a scope that would fail.
func (a *App) globalSearchAvailable() bool {
	return a.GetConfig().SearchEnabled && a.searchIndexBuilt()
}

// defaultSearchScope is the scope a request without an explicit one gets.
//
// It follows the configured preference, EXCEPT that it falls back to page
// scope whenever global search cannot answer - defaulting an unscoped query
// to a scope that can only fail would be a strange way to treat a caller who
// expressed no preference at all.
func (a *App) defaultSearchScope() string {
	if a.globalSearchAvailable() {
		return normalizeSearchScope(a.GetConfig().SearchScope)
	}
	return SearchScopePage
}

// ----------------------------------------------------------------------
// The HTTP surface
// ----------------------------------------------------------------------

// searchMatch is one snippet in the response. Spans are [start, len] pairs in
// RUNE offsets into Text - see the matcher's note on why byte offsets are a
// way to render mojibake.
type searchMatch struct {
	Line    int      `json:"line"`
	Context string   `json:"context,omitempty"`
	Text    string   `json:"text"`
	Spans   [][2]int `json:"spans"`
}

type searchResult struct {
	Path      string        `json:"path"`
	Kind      string        `json:"kind"`
	Name      string        `json:"name"`
	Title     string        `json:"title"`
	Tags      []string      `json:"tags,omitempty"`
	Score     int           `json:"score"`
	URL       string        `json:"url"`
	Truncated bool          `json:"truncated,omitempty"`
	Matches   []searchMatch `json:"matches,omitempty"`
}

type searchResponse struct {
	Query     string         `json:"query"`
	Scope     string         `json:"scope"`
	TookMS    int64          `json:"took_ms"`
	Total     int            `json:"total"`
	Truncated bool           `json:"truncated"`
	Results   []searchResult `json:"results"`
	Status    string         `json:"status,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// handleSearch answers GET /api/search.
//
// Registered WITHOUT authMiddleware, matching /api/note and every page and
// static route (see doc/API.md): search aggregates nothing a LAN guest could
// not already fetch file by file, so gating it would buy no confidentiality
// while breaking the guest experience.
func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	qs := r.URL.Query()

	scope := strings.ToLower(strings.TrimSpace(qs.Get("scope")))
	if scope == "" {
		scope = a.defaultSearchScope()
	}

	resp := searchResponse{
		Query:   qs.Get("q"),
		Scope:   scope,
		Results: []searchResult{},
	}

	switch scope {
	case SearchScopePage:
		a.searchPage(&resp, qs)
	case SearchScopeAll:
		if !a.GetConfig().SearchEnabled {
			// Not an empty result set: "nothing matched" is a claim about the
			// notes, and this is a claim about the settings.
			resp.Status = "disabled"
			resp.Error = "global search is off (Settings -> Search)"
			writeSearchJSON(w, http.StatusServiceUnavailable, &resp, started)
			return
		}
		if !a.ensureSearchIndex() {
			resp.Status = "unavailable"
			resp.Error = "the search index is not ready"
			writeSearchJSON(w, http.StatusServiceUnavailable, &resp, started)
			return
		}
		a.searchGlobal(&resp, qs)
	default:
		resp.Status = "error"
		resp.Error = "unknown scope " + strconv.Quote(scope)
		writeSearchJSON(w, http.StatusBadRequest, &resp, started)
		return
	}

	writeSearchJSON(w, http.StatusOK, &resp, started)
}

// searchPage fills resp from the single document named by "on".
func (a *App) searchPage(resp *searchResponse, qs map[string][]string) {
	get := func(k string) string {
		if v, ok := qs[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}

	q := parseQuery(resp.Query)
	if len(q.terms) == 0 {
		return // an empty query is an empty result, not an error
	}

	doc, err := a.loadPageDocument(get("on"))
	if err != nil {
		log.Printf("[search] %s: %v", get("on"), err)
		return
	}
	if doc == nil {
		return // missing, outside storage, or binary: nothing to say
	}
	// The query's own kind filters apply; Config.SearchKinds deliberately does
	// NOT. That setting says what the global INDEX covers - what it costs to
	// hold in memory - and has nothing to say about a file the user is looking
	// at right now. Letting it reach here would make page search stop working
	// on a note the moment someone unticked "Notes" for the index.
	if !kindAllowed(doc.Kind, splitCSV(get("kind")), q.kinds) {
		return
	}

	score, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		return
	}

	limit := clampInt(atoiOr(get("snippets"), searchDefaultSnippets), 1, searchMaxSnippets)
	if len(hits) > limit {
		hits = hits[:limit]
	}

	res := searchResult{
		Path: doc.Path, Kind: doc.Kind, Name: doc.Name, Title: doc.Title,
		Tags: doc.Tags, Score: score, URL: doc.URL, Truncated: doc.truncated,
	}
	for _, h := range hits {
		text, spans := snippetFor(h.line.raw, h.spans)
		m := searchMatch{Line: h.line.no, Context: h.line.context, Text: text}
		for _, s := range spans {
			m.Spans = append(m.Spans, [2]int{s.Start, s.Len})
		}
		res.Matches = append(res.Matches, m)
	}

	resp.Results = append(resp.Results, res)
	resp.Total = 1
	resp.Truncated = doc.truncated
}

// searchGlobal answers a scope=all query from the index.
//
// The shape is: reject cheaply, then read what survives. Every document is
// tested against the query's character masks and trigram signature - no I/O at
// all - and only the ones that could possibly match are read from disk and
// scored by exactly the same code page search uses. That is what keeps the
// index small enough to hold: it decides WHICH files to read, rather than
// being a copy of them.
func (a *App) searchGlobal(resp *searchResponse, qs map[string][]string) {
	get := func(k string) string {
		if v, ok := qs[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}

	q := parseQuery(resp.Query)
	if len(q.terms) == 0 {
		return
	}

	cfg := a.GetConfig()
	kindFilter := splitCSV(get("kind"))
	limit := clampInt(atoiOr(get("limit"), searchDefaultLimit), 1, searchMaxLimit)
	snippets := clampInt(atoiOr(get("snippets"), searchDefaultSnippets), 1, searchMaxSnippets)

	type scored struct {
		doc   *searchDocument
		index *indexedDoc
		score int
		tier  matchTier
		hits  []lineHit
	}
	var found []scored
	read := 0

	for _, d := range a.snapshotDocs() {
		if !kindAllowed(d.Kind, kindFilter, q.kinds) {
			continue
		}
		feasible := true
		for _, term := range q.terms {
			if !d.couldMatchTerm(term) {
				feasible = false
				break
			}
		}
		if !feasible {
			continue
		}

		// Only now is a file opened.
		doc := a.reloadDocument(d)
		if doc == nil {
			continue
		}
		read++
		score, tier, hits, ok := scoreDocument(q, doc)
		if !ok {
			continue
		}
		found = append(found, scored{doc: doc, index: d, score: score, tier: tier, hits: hits})
	}

	// Documents order by (tier, score) - the same tier-first rule a single
	// match uses, lifted to the document: one containing every term verbatim
	// outranks one that needed a typo correction, whatever the sums say.
	// Ties break on newest first, then path, so the order is stable.
	sortScored := func(i, j int) bool {
		a1, b1 := found[i], found[j]
		if a1.tier != b1.tier || a1.score != b1.score {
			return betterMatch(a1.tier, a1.score, b1.tier, b1.score)
		}
		if !a1.index.ModTime.Equal(b1.index.ModTime) {
			return a1.index.ModTime.After(b1.index.ModTime)
		}
		return a1.index.Path < b1.index.Path
	}
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && sortScored(j, j-1); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}

	resp.Total = len(found)
	if len(found) > limit {
		found = found[:limit]
		resp.Truncated = true
	}

	for _, f := range found {
		hits := f.hits
		if len(hits) > snippets {
			hits = hits[:snippets]
		}
		res := searchResult{
			Path: f.doc.Path, Kind: f.doc.Kind, Name: f.doc.Name, Title: f.doc.Title,
			Tags: f.doc.Tags, Score: f.score, URL: f.doc.URL, Truncated: f.doc.truncated,
		}
		for _, h := range hits {
			text, spans := snippetFor(h.line.raw, h.spans)
			m := searchMatch{Line: h.line.no, Context: h.line.context, Text: text}
			for _, s := range spans {
				m.Spans = append(m.Spans, [2]int{s.Start, s.Len})
			}
			res.Matches = append(res.Matches, m)
		}
		if f.doc.truncated {
			resp.Truncated = true
		}
		resp.Results = append(resp.Results, res)
	}
	_ = cfg
	if read > 0 {
		log.Printf("[search] %q: %d candidates read, %d matched", resp.Query, read, resp.Total)
	}
}

func writeSearchJSON(w http.ResponseWriter, status int, resp *searchResponse, started time.Time) {
	resp.TookMS = time.Since(started).Milliseconds()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[search] encode: %v", err)
	}
}

// serveSearchPage renders /OMNGoSearch.html.
//
// Dynamic like Config, not generated like OMNGoTags: there is no
// md/OMNGoSearch.md, nothing is written to the html/ cache, and ?refresh means
// nothing here. The results are computed per request from the index.
//
// GLOBAL ONLY. This page exists to show a ranked list across everything, which
// is exactly what needs the index; with global search off it does not exist,
// because a permanently empty page is worse than an honest 404. Page search
// lives in the dialog, where the answer is short enough not to need a page of
// its own.
func (a *App) serveSearchPage(w http.ResponseWriter, r *http.Request) {
	if !a.GetConfig().SearchEnabled {
		a.serveNotFound(w, r)
		return
	}

	query := r.URL.Query().Get("q")
	view := searchPageView{
		Query:        query,
		IndexedKinds: normalizeSearchKinds(a.GetConfig().SearchKinds),
	}

	if strings.TrimSpace(query) != "" && a.ensureSearchIndex() {
		// Same code path as the API, so the page and the dialog can never
		// disagree about what matches or in what order.
		resp := searchResponse{Query: query, Scope: SearchScopeAll, Results: []searchResult{}}
		a.searchGlobal(&resp, map[string][]string{"q": {query}})
		view.Results = resp.Results
		view.Total = resp.Total
		view.Truncated = resp.Truncated
	}

	title := "Search"
	if query != "" {
		title = "Search: " + query
	}
	body := renderSearchPage(view)
	compiled := a.compilePageWithBody(title,
		[]byte("Title: "+title+"\nCategory: System\n\n"), body)
	w.Header().Set("Content-Type", "text/html")
	w.Write(a.injectRuntimeVars(compiled))
}

// ----------------------------------------------------------------------
// Small shared helpers
// ----------------------------------------------------------------------

// withinStorage reports whether p stays inside StorageDir. resolvePageName
// already cleans its input, so this is defence in depth - the same containment
// notFoundSuggestion applies before it admits a file exists.
func (a *App) withinStorage(p string) bool {
	root, err := filepath.Abs(a.StorageDir)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// readCapped reads at most max bytes, cut back to the last complete line so no
// snippet is ever a half line. truncated says whether anything was left out.
func readCapped(p string, max int) (data []byte, truncated bool, err error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	// max+1 so a file of exactly max bytes is not misreported as truncated.
	data, err = io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) <= max {
		return data, false, nil
	}
	cut := data[:max]
	if nl := bytes.LastIndexByte(cut, '\n'); nl > 0 {
		cut = cut[:nl]
	}
	return cut, true, nil
}

// isBinary rejects a file whose first 8 KiB contains a NUL byte. Cheap, and
// wrong only for text that has no business being in a notes directory.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8<<10 {
		n = 8 << 10
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// assetKind maps a storage-relative html/ path to its search kind.
func assetKind(rel string) string {
	switch {
	case strings.HasPrefix(rel, "js/"):
		return "js"
	case strings.HasPrefix(rel, "user_json/"):
		return "user_json"
	case strings.HasPrefix(rel, "json/"):
		return "json"
	default:
		return "asset"
	}
}

// kindAllowed applies the kind filters. Both come from the caller (the query
// parameter and a "kind:" prefix inside the query itself); an empty filter
// allows everything.
func kindAllowed(kind string, filters ...[]string) bool {
	for _, f := range filters {
		if len(f) == 0 {
			continue
		}
		found := false
		for _, k := range f {
			if k == kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
