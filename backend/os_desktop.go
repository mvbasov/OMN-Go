//go:build !android

package backend

import (
	"log"
	"os/exec"
	"runtime"
)

// GetStorageDir returns the local data directory for Desktop environments.
func GetStorageDir() string {
	return "./data"
}

// OpenExternalEditor opens the markdown file using the OS's default editor or a custom command.
func OpenExternalEditor(path string) error {
	if appConfig.DesktopExtCmd != "" {
		log.Printf("Opening %s with custom command: %s", path, appConfig.DesktopExtCmd)
		return exec.Command(appConfig.DesktopExtCmd, path).Start()
	}

	log.Println("Opening with default system editor:", path)
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", path).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		err = exec.Command("open", path).Start()
	default:
		log.Println("Unsupported desktop OS for external editor.")
	}
	return err
}
