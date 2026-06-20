//go:build android

package backend

// GetStorageDir returns the isolated media storage directory for Android.
func GetStorageDir() string {
	return "/storage/emulated/0/Android/media/net.basov.omngo"
}

// OpenExternalEditor triggers an Android intent to open the markdown file.
func OpenExternalEditor(path string) error {
	// Android WebView wrapper intercepts omngo:// URLs natively,
	// so the Go backend doesn't need to execute anything here.
	return nil
}
