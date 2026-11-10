package storage

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"net.basov.omngo/backend/internal/logx"
)

// version stands in for the build version of package app. A test of package
// app sees the same value.
const version = "dev"

// testApp stands in for the App of package app. It holds the storage
// directory and the log hub, the same fields that the App gives to the
// functions of this package. The tests of this package read these fields by
// the names of the App, thus a test reads the same here and in package app.
type testApp struct {
	StorageDir string
	filter     atomic.Value
	logs       logx.Hub
}

// newTestApp answers the stand-in of a fresh install. It makes md/ and html/,
// the same as newTestApp of package app. The functions of this package read
// no setting, thus the stand-in holds none.
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	a := &testApp{StorageDir: t.TempDir()}
	for _, d := range []string{"md", "html"} {
		if err := os.MkdirAll(filepath.Join(a.StorageDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func (a *testApp) layout() Layout { return Layout(a.StorageDir) }

func (a *testApp) log(tag logx.Tag) logx.Logger { return logx.New(tag, &a.filter, &a.logs) }

func (a *testApp) refreshEmbeddedAssets() {
	RefreshEmbeddedAssets(a.layout(), version, a.log(logx.Assets), nil)
}

func (a *testApp) syncNoteFilesToHTML() {
	SyncNoteFilesToHTML(a.layout(), a.log(logx.NoteFiles))
}

func (a *testApp) syncNoteFileToMD(htmlPath string) {
	SyncNoteFileToMD(a.layout(), htmlPath, a.log(logx.NoteFiles))
}
