package search

import (
	"net/http"
	"testing"

	"net.basov.omngo/backend/internal/testkit"
)

// testApp stands in for the App of package app. testkit.App holds the
// storage directory, the settings and the log hub. It adds the index.
type testApp struct {
	*testkit.App
	search *Index
}

func newTestApp(t *testing.T) *testApp { return &testApp{App: testkit.New(t)} }

// newUnconfiguredApp answers the stand-in with the zero Config, the same as
// newUnconfiguredApp of package app.
func newUnconfiguredApp(t *testing.T) *testApp {
	return &testApp{App: testkit.NewUnconfigured(t)}
}

// searchService answers the Service of the stand-in, the same as
// searchService of package app.
func (a *testApp) searchService() Service {
	return Service{
		Index:      a.search,
		Layout:     a.Layout(),
		Config:     a.Config.Get(),
		Log:        a.Log,
		RenderPage: a.RenderPage,
	}
}

func (a *testApp) rebuildSearchIndex() { a.searchService().RebuildIndex() }

func (a *testApp) dropSearchIndex() { a.searchService().DropIndex() }

func (a *testApp) searchIndexStatus() string { return a.searchService().IndexStatus() }

func (a *testApp) markSearchIndexDirty() { a.search.MarkDirty() }

func (a *testApp) handleSearch(w http.ResponseWriter, r *http.Request) {
	a.searchService().HandleSearch(w, r)
}

func (a *testApp) serveSearchPage(w http.ResponseWriter, r *http.Request) {
	a.searchService().ServeSearchPage(w, r)
}
