//go:build android

package main

import (
	"fmt"
	"os/exec"
	"time"
	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/gl"
)

func main() {
	go runServer()

	// Automatically launch the Android default browser to view the UI
	go func() {
		time.Sleep(1 * time.Second)
		url := fmt.Sprintf("http://localhost:%d", appConfig.ServerPort)
		exec.Command("am", "start", "-a", "android.intent.action.VIEW", "-d", url).Start()
	}()

	// High-performance canvas avoiding AppCompat.
	// Render a color block to represent Server Status due to 5MB size constraints
	// and to avoid heavyweight FreeType/font libraries inside the GL loop.
	app.Main(func(a app.App) {
		var glctx gl.Context
		for e := range a.Events() {
			switch e := a.Filter(e).(type) {
			case lifecycle.Event:
				if e.Crosses(lifecycle.StageVisible) == lifecycle.CrossOn {
					glctx, _ = e.DrawContext.(gl.Context)
					a.Send(paint.Event{})
				} else if e.Crosses(lifecycle.StageVisible) == lifecycle.CrossOff {
					glctx = nil
				}
			case paint.Event:
				if glctx == nil || e.External {
					continue
				}
				
				// Print to logcat so server IP is visible to debugging tools
				fmt.Printf("GoOMN Active: Port %d, Connections: %d\n", appConfig.ServerPort, activeConns)
				
				// Clear to a Green/Dark theme to signify active state
				glctx.ClearColor(0.0, 0.2, 0.1, 1.0)
				glctx.Clear(gl.COLOR_BUFFER_BIT)
				a.Publish()
			}
		}
	})
}
