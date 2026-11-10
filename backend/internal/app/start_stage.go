package app

import (
	"fmt"
	"sync"

	"net.basov.omngo/backend/internal/storage"
)

// The start stage is the work of StartServer that runs now, for example
// "Updating the application files: 34 of 120". The start screen of the
// Android app reads it with StartStage two times per second, and it shows
// the text under the spinner. The stage is empty after the bind.
var (
	startStageMu sync.Mutex
	startStage   string
)

// setStartStage records the work of the start that runs now.
func setStartStage(text string) {
	startStageMu.Lock()
	startStage = text
	startStageMu.Unlock()
}

// startProgress answers a storage.Progress that records "what: done of total",
// or "what: done" when the whole count is not known.
func startProgress(what string) storage.Progress {
	return func(done, total int) {
		if total > 0 {
			setStartStage(fmt.Sprintf("%s: %d of %d", what, done, total))
			return
		}
		setStartStage(fmt.Sprintf("%s: %d", what, done))
	}
}

// StartStage answers the work of the start that runs now, or "" after the
// bind and before StartServer. gomobile exports it.
func StartStage() string {
	startStageMu.Lock()
	defer startStageMu.Unlock()
	return startStage
}
