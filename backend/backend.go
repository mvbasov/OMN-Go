// Package backend is the facade of OMN-Go for gomobile and for
// main_desktop.go. It holds six functions, the version and the build
// commit. gomobile makes a Java binding for each exported name, thus this
// package exports nothing else. The application is in package internal/app.
package backend

import "net.basov.omngo/backend/internal/app"

// StartServer starts the server. storageDir replaces the default storage
// directory when it is not empty. defaultPort is the port of a config.json
// with none, and 0 gives 8080. ServerService.java calls it.
func StartServer(storageDir string, defaultPort int) {
	app.SetVersion(APP_VERSION)
	app.SetBuildCommit(buildCommit, buildCommitTime)
	app.StartServer(storageDir, defaultPort)
}

// WaitUntilReady blocks until the server listens, or until the bind fails.
// main_desktop.go calls it before it opens the browser.
func WaitUntilReady() { app.WaitUntilReady() }

// ServerPort answers the port of the server, or 0 before StartServer.
func ServerPort() int { return app.ServerPort() }

// AssetsRefreshed tells whether this start wrote an application file.
// MainActivity.java then clears the cache of the WebView one time.
func AssetsRefreshed() bool { return app.AssetsRefreshed() }

// SetAndroidPackage records the applicationId. ServerService.java calls it
// before StartServer.
func SetAndroidPackage(name string) { app.SetAndroidPackage(name) }

// SetLANAddresses records the LAN addresses of the device as one list with
// commas. ServerService.java calls it.
func SetLANAddresses(list string) { app.SetLANAddresses(list) }
