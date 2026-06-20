//go:build !android

package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"
	"net.basov.omngo/backend"
)

func main() {
	go backend.StartServer()
	
	// Wait for server to bind
	time.Sleep(500 * time.Millisecond)
	url := fmt.Sprintf("http://localhost:%d", backend.GetServerPort())
	
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
