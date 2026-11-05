package app

import (
	"sync/atomic"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/storage"
)

// The App side of the storage package. Each method gives package storage the
// storage directory of the App, and the logger or the settings that the call
// needs.

// layout answers the storage directory of the App as a storage.Layout.
func (a *App) layout() storage.Layout { return storage.Layout(a.StorageDir) }

// assetsRefreshed tells whether refreshEmbeddedAssets wrote a file in this
// process. The Android WebView keeps scripts and styles in its own disk
// cache, thus new pages can use old scripts after an update. The Android
// layer reads this value through AssetsRefreshed.
var assetsRefreshed atomic.Bool

// AssetsRefreshed tells whether this start installed or replaced a
// version-dependent file. gomobile exports it. MainActivity.java calls it
// before the first loadUrl, and on true it calls WebView.clearCache(true) one
// time. A start with no change keeps the cache.
func AssetsRefreshed() bool {
	return assetsRefreshed.Load()
}

// refreshEmbeddedAssets writes the application files of this build. See
// storage.RefreshEmbeddedAssets.
func (a *App) refreshEmbeddedAssets() {
	// The flag reports the work of this start only.
	assetsRefreshed.Store(false)
	if storage.AssetsVersion(a.layout()) != version {
		// Each page compiles again with the page shell of this build.
		n, err := storage.DropCompiledPages(a.layout())
		if err != nil {
			a.log(logx.Assets).Errf("cannot remove the compiled pages: %v", err)
		}
		if n > 0 {
			a.log(logx.Assets).Infof("removed %d compiled page(s) of the previous version", n)
		}
	}
	if storage.RefreshEmbeddedAssets(a.layout(), version, a.log(logx.Assets)) > 0 {
		// AssetsRefreshed tells the Android layer to clear the WebView cache
		// one time.
		assetsRefreshed.Store(true)
	}
}

// syncNoteFilesToHTML copies the plain files of md/ to html/. See
// storage.SyncNoteFilesToHTML.
func (a *App) syncNoteFilesToHTML() {
	storage.SyncNoteFilesToHTML(a.layout(), a.log(logx.NoteFiles))
}

// syncNoteFileToMD copies one saved file of html/ to md/. See
// storage.SyncNoteFileToMD.
func (a *App) syncNoteFileToMD(htmlPath string) {
	storage.SyncNoteFileToMD(a.layout(), htmlPath, a.log(logx.NoteFiles))
}

// resolvePageName answers the paths of a note or a file. The mime_types map
// of the configuration can make an extension a file. See
// storage.ResolvePageName.
func (a *App) resolvePageName(name string) (mdPath, htmlPath, baseName string, isPage bool) {
	return storage.ResolvePageName(a.layout(), a.config.Get().MimeTypes, name)
}
