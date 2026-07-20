//go:build !android

package main

import (
	"fmt"
	"log"
	"net.basov.omngo/backend"
	"os"
	"os/exec"
	"runtime"
)

func main() {
	app := backend.StartServer("", 0) // desktop: OS-appropriate storage default, historical port default (8080)

	// Block until the listener has actually bound, instead of guessing
	// with a fixed time.Sleep(500ms) that could fire too early on a slow
	// boot (Docker/Android) or waste time on a fast one.
	app.WaitUntilReady()
	url := fmt.Sprintf("http://localhost:%d", app.GetServerPort())

	// A replacement process spawned by /api/restart marks itself with this
	// env var: the user's browser tab already exists (the frontend reloads
	// it after the restart), so opening another one here would leave a
	// duplicate tab on every ShareLAN toggle.
	if os.Getenv("OMN_GO_RESTARTED") == "1" {
		log.Printf("Restarted instance; browser already open at %s", url)
		select {} // Block main thread
	}

	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}

	if err != nil {
		log.Printf("Could not auto-launch browser. Please visit %s manually.", url)
	}

	select {} // Block main thread
}
