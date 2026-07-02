//go:build !android

package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"net.basov.omngo/backend"
)

func main() {
	app := backend.StartServer()

	// Block until the listener has actually bound, instead of guessing
	// with a fixed time.Sleep(500ms) that could fire too early on a slow
	// boot (Docker/Android) or waste time on a fast one.
	app.WaitUntilReady()
	url := fmt.Sprintf("http://localhost:%d", app.GetServerPort())
	
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
