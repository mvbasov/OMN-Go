package backend

// ---------------------------------------------------------------------
// The log transport
//
// One line goes to two destinations. stdout is the destination a desktop
// user and `adb logcat` read. The /api/logs SSE stream is the destination
// every open page reads: omn-go-sse.js mirrors each line into the browser
// console, and the sync progress overlay reads the same stream for its
// stage text.
//
// broadcastLogLine is the only fan-out. Two callers reach it:
//
//	JSLogger.Write  the standard log package, for the two call sites that
//	                cannot reach an *App. See TestNoDirectLogPrintf.
//	App.emitLog     every other line, through a.logDebugf, a.logInfof or
//	                a.logErrf. See log_levels.go.
//
// THE SSE STREAM ALWAYS CARRIES EVERY LINE. Two reasons hold that rule. The
// sync progress overlay is fed by "[sync]" debug lines, and it must keep
// working when a reader asks for less noise. And the browser is the place a
// person can change a filter and see the effect at once, with no server
// restart. broadcastLogLine therefore takes a separate switch for stdout
// only.
//
// The stream is a live sample and not a complete transcript. A client with a
// full 10-slot channel loses the line rather than blocking the writer. That
// is correct for a progress display, and it means the stream must never
// drive state that has to see every event.
// ---------------------------------------------------------------------

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	logMutex   sync.Mutex
	logClients []chan string
)

// logTimeLayout is the prefix format of the standard log package with
// log.LstdFlags. emitLog writes the stamp itself, because it does not go
// through the log package. The two sources must look the same on stdout and
// on the stream, or the page has two shapes to parse.
const logTimeLayout = "2006/01/02 15:04:05 "

// broadcastLogLine sends one finished line to every SSE subscriber, and to
// stdout when toStdout is true. Both happen under logMutex, so two
// goroutines cannot interleave one line into another.
func broadcastLogLine(msg string, toStdout bool) {
	logMutex.Lock()
	for _, c := range logClients {
		select {
		case c <- msg:
		default:
		}
	}
	if toStdout {
		fmt.Print(msg)
	}
	logMutex.Unlock()
}

type JSLogger struct{}

func (l *JSLogger) Write(p []byte) (n int, err error) {
	// A line from the standard log package carries no level. It reaches
	// stdout, the same as before this file grew a filter.
	broadcastLogLine(string(p), true)
	return len(p), nil
}

// emitLog formats one line as "[tag] (level) message", stamps it, and hands
// it to broadcastLogLine. It is the only writer of a level-tagged line.
func (a *App) emitLog(lvl logLevel, tag logTag, format string, args ...any) {
	line := time.Now().Format(logTimeLayout) +
		"[" + string(tag) + "] (" + string(lvl) + ") " +
		fmt.Sprintf(format, args...) + "\n"
	broadcastLogLine(line, true)
}

func (a *App) InitLoggerAndRoute() {
	log.SetOutput(&JSLogger{})
	a.Router.HandleFunc("/api/logs", a.HandleLogsSSE)
}

func (a *App) HandleLogsSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 10)
	logMutex.Lock()
	logClients = append(logClients, ch)
	logMutex.Unlock()

	defer func() {
		logMutex.Lock()
		for i, c := range logClients {
			if c == ch {
				logClients = append(logClients[:i], logClients[i+1:]...)
				break
			}
		}
		logMutex.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
