package search

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/textmatch"
)

// Only GLOBAL search has a setting. Page search has no cost when nobody uses
// it. A switch that saves nothing only gives one more way to break the
// feature.

// GlobalAvailable answers "can this server search each file now": the
// user asked for it AND an index exists. injectRuntimeVars gives it to each
// page as OMN_SEARCH_GLOBAL, thus the dialog never offers a scope that fails.
func (svc Service) GlobalAvailable() bool {
	return svc.Config.SearchEnabled && svc.Index.Built()
}

// defaultScope is the scope of a request without one. It follows the
// setting, but it falls back to page scope when global search cannot answer.
// A request with no preference must not get a scope that fails.
func (svc Service) defaultScope() string {
	if svc.GlobalAvailable() {
		return config.NormalizeSearchScope(svc.Config.SearchScope)
	}
	return config.SearchScopePage
}

// searchMatch is one snippet of the response. Spans are [start, len] pairs in
// RUNE offsets into Text. See the banner of package textmatch.
type searchMatch struct {
	Line    int            `json:"line"`
	Context string         `json:"context,omitempty"`
	Section *searchSection `json:"section,omitempty"`
	Text    string         `json:"text"`
	Spans   [][2]int       `json:"spans"`
}

// searchSection names the part of a document that holds a hit: a bookmark, a
// quick note, or a heading section. ID can be empty while Label is not, and
// the UI then shows the label with no link. See sections.go.
type searchSection struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
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

	// Highlight holds the terms of the query as typed. The client marks them
	// in an opened result. The terms belong to the QUESTION, thus they are
	// here one time and not on each result.
	Highlight []string `json:"highlight,omitempty"`
}

// HandleSearch answers GET /api/search. It has NO authMiddleware, the same as
// /api/note, because a search shows nothing that a remote caller cannot fetch
// file by file. See doc/API.md.
func (svc Service) HandleSearch(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	qs := r.URL.Query()

	scope := strings.ToLower(strings.TrimSpace(qs.Get("scope")))
	if scope == "" {
		scope = svc.defaultScope()
	}

	resp := searchResponse{
		Query:   qs.Get("q"),
		Scope:   scope,
		Results: []searchResult{},
	}

	code := http.StatusOK
	switch scope {
	case config.SearchScopePage:
		svc.searchPage(&resp, qs)
	case config.SearchScopeAll:
		switch {
		case !svc.Config.SearchEnabled:
			// This is not an empty result. "Nothing matched" is about the
			// notes, and this answer is about the settings.
			code = http.StatusServiceUnavailable
			resp.Status = "disabled"
			resp.Error = "global search is off (Settings -> Search)"
		case !svc.EnsureIndex():
			code = http.StatusServiceUnavailable
			resp.Status = "unavailable"
			resp.Error = "the search index is not ready"
		default:
			svc.searchGlobal(&resp, qs)
		}
	default:
		code = http.StatusBadRequest
		resp.Status = "error"
		resp.Error = "unknown scope " + strconv.Quote(scope)
	}

	if code == http.StatusOK {
		resp.Highlight = highlightTerms(parseQuery(resp.Query))
	}
	resp.TookMS = time.Since(started).Milliseconds()
	svc.writeJSON(w, code, resp)
}

// searchPage fills resp from the one document that "on" names.
func (svc Service) searchPage(resp *searchResponse, qs map[string][]string) {
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

	doc, err := svc.LoadPageDocument(get("on"))
	if err != nil {
		svc.Log(logx.Search).Errf("%s: %v", get("on"), err)
		return
	}
	if doc == nil {
		return // missing, outside storage, or binary: nothing to say
	}
	// The kind filters of the query apply, and Config.SearchKinds does NOT.
	// That setting tells what the INDEX holds, and page search must work on
	// each note.
	if !kindAllowed(doc.Kind, splitCSV(get("kind")), q.kinds) {
		return
	}

	score, _, hits, ok := scoreDocument(q, doc)
	if !ok {
		return
	}

	limit := clampInt(atoiOr(get("snippets"), searchDefaultSnippets), 1, searchMaxSnippets)
	hits = cutSnippets(q, hits, limit, nil)

	res := searchResult{
		Path: doc.Path, Kind: doc.Kind, Name: doc.Name, Title: doc.Title,
		Tags: doc.Tags, Score: score, URL: doc.URL, Truncated: doc.truncated,
	}
	res.Matches, _ = buildMatches(doc, hits)
	// Page scope gives no anchor. The reader is already on the page, and the
	// dialog marks the text there.

	resp.Results = append(resp.Results, res)
	resp.Total = 1
	resp.Truncated = doc.truncated
}

// cutSnippets answers the first limit lines, less each line that carries no
// word of the query. THE WINDOW COMES FIRST, and the drop second, thus a line
// can only leave and never move UP. TestCutSnippetsNeverPromotes holds the
// rule. A line leaves when the query has several terms, and each term that
// hits the line is short or common. See
// doc/decisions/0009-show-only-the-search-rows-that-carry-a-word-of-the-query.md.
func cutSnippets(q parsedQuery, hits []lineHit, limit int, common map[string]bool) []lineHit {
	if len(hits) > limit {
		hits = hits[:limit] // the window first
	}
	if len(q.terms) < 2 || len(hits) == 0 {
		return hits
	}
	kept := make([]lineHit, 0, len(hits))
	for _, h := range hits {
		carries := false
		for _, term := range q.terms {
			if textmatch.IsShortTerm(term.runes) || common[string(term.runes)] {
				continue
			}
			_, _, tier, ok := textmatch.ScoreTerm(term.runes, h.line.fold)
			if !ok || tier != textmatch.TierSubstring {
				continue
			}
			carries = true
			break
		}
		if carries {
			kept = append(kept, h)
		}
	}
	// A matching document has something to say. When no line of the window
	// carries a word, the window is the answer.
	if len(kept) == 0 {
		return hits
	}
	return kept
}

// searchGlobal answers a query of scope=all. It tests each document against
// the masks and the trigram signature with no I/O, and it reads only the
// documents that can match. The code of page search scores them.
func (svc Service) searchGlobal(resp *searchResponse, qs map[string][]string) {
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

	kindFilter := splitCSV(get("kind"))
	limit := clampInt(atoiOr(get("limit"), searchDefaultLimit), 1, searchMaxLimit)
	snippets := clampInt(atoiOr(get("snippets"), searchDefaultSnippets), 1, searchMaxSnippets)

	type scored struct {
		doc   *searchDocument
		index *indexedDoc
		score int
		tier  textmatch.Tier
		hits  []lineHit
	}
	var found []scored
	common := svc.Index.CommonWords()
	read := 0

	for _, d := range svc.Index.Docs() {
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

		// Only now does the search open a file.
		doc := svc.ReloadDocument(d)
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

	// Order the documents by tier and then by score, the same rule as for one
	// match. A document with each term as it is thus wins against one that
	// needed a typo match. A tie orders the newest first, and then by path.
	sortScored := func(i, j int) bool {
		a1, b1 := found[i], found[j]
		if a1.tier != b1.tier || a1.score != b1.score {
			return textmatch.BetterMatch(a1.tier, a1.score, b1.tier, b1.score)
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
		hits := cutSnippets(q, f.hits, snippets, common)
		res := searchResult{
			Path: f.doc.Path, Kind: f.doc.Kind, Name: f.doc.Name, Title: f.doc.Title,
			Tags: f.doc.Tags, Score: f.score, URL: f.doc.URL, Truncated: f.doc.truncated,
		}
		var anchor string
		res.Matches, anchor = buildMatches(f.doc, hits)
		res.URL += anchor
		if f.doc.truncated {
			resp.Truncated = true
		}
		resp.Results = append(resp.Results, res)
	}
	if read > 0 {
		svc.Log(logx.Search).Infof("%q: %d candidates read, %d matched", resp.Query, read, resp.Total)
	}
}

// buildMatches makes the snippets from the line hits, with the section of
// each hit. Both scopes use it. The document URL gets the fragment of the
// BEST hit, because the reader sees the ranked order.
func buildMatches(doc *searchDocument, hits []lineHit) ([]searchMatch, string) {
	var out []searchMatch
	anchor := ""
	for _, h := range hits {
		text, spans := snippetFor(h.line.raw, h.spans)
		m := searchMatch{Line: h.line.no, Context: h.line.context, Text: text}
		for _, sp := range spans {
			m.Spans = append(m.Spans, [2]int{sp.Start, sp.Len})
		}
		if sec := sectionFor(doc.sections, h.line.no); sec != nil && (sec.id != "" || sec.label != "") {
			m.Section = &searchSection{ID: sec.id, Label: sec.label}
			if anchor == "" && sec.id != "" {
				anchor = "#" + sec.id
			}
		}
		out = append(out, m)
	}
	return out, anchor
}

// ServeSearchPage renders /OMNGoSearch.html for each request. It has no md/
// source and no cache. The page is for GLOBAL search only. With global search
// off, it names the setting. Page search lives in the dialog.
func (svc Service) ServeSearchPage(w http.ResponseWriter, r *http.Request) {
	cfg := svc.Config

	// A note can link to the Search page, thus a person can reach it when
	// search is off. The page then says why it can do nothing, and where to
	// change that.
	if !cfg.SearchEnabled {
		body := renderSearchPage(searchPageView{Disabled: true})
		svc.RenderPage(w, http.StatusOK, "Search", render.PageHeader("Search", "System", "css/OMN-Go/omn-go-search.css"), body)
		return
	}

	query := r.URL.Query().Get("q")
	view := searchPageView{
		Query:        query,
		IndexedKinds: config.NormalizeSearchKinds(cfg.SearchKinds),
		Highlight:    highlightTerms(parseQuery(query)),
	}

	if strings.TrimSpace(query) != "" && svc.EnsureIndex() {
		// The page and the API use the same code, thus the page and the
		// dialog always agree.
		resp := searchResponse{Query: query, Scope: config.SearchScopeAll, Results: []searchResult{}}
		svc.searchGlobal(&resp, map[string][]string{"q": {query}})
		view.Results = resp.Results
		view.Total = resp.Total
		view.Truncated = resp.Truncated
	}

	title := "Search"
	if query != "" {
		title = "Search: " + query
	}
	body := renderSearchPage(view)
	svc.RenderPage(w, http.StatusOK, title, render.PageHeader(title, "System", "css/OMN-Go/omn-go-search.css"), body)
}
