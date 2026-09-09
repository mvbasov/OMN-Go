package backend

// ---------------------------------------------------------------------
// The log transport
//
// One line goes to two destinations. stdout is the destination a desktop
// user and `adb logcat` read. The /api/logs SSE stream is the destination
// that every open page reads. omn-go-sse.js mirrors each line into the
// browser console, and the sync progress overlay reads the same stream for
// its stage text.
//
// broadcastLogLine is the only fan-out. It has THREE destinations since
// 26.09.38: stdout, the SSE stream, and the history ring below. Two
// callers reach it:
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
	"encoding/json"
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

// ---------------------------------------------------------------------
// The history ring
// ---------------------------------------------------------------------
//
// The SSE stream is a live sample. A page that opens after an event
// never sees the lines of it. A person who reads a fault report must
// then make the fault happen again, with a page open.
//
// The ring holds the last logHistoryCap lines, and /api/logs/history
// answers with them.
//
// IT DOES NOT REPLAY ON THE STREAM. That was the first design, and it
// breaks the sync progress overlay. applySyncLogLine in omn-go-sse.js
// reads "[sync] (debug)" lines off the raw stream to drive the stages.
// A replay on connect feeds it the lines of a sync that ended an hour
// ago. Each page load would then show a sync that is not running. The
// ring therefore answers its own endpoint, and the stream carries live
// lines alone, exactly as before.
//
// IT HOLDS EVERY LINE, the same as the stream. The stdout switches say
// what a reader wants to SEE, and never what the application must keep.
// A person who turned debug off and then met a fault needs the debug
// lines of that moment more than anybody.
//
// THE SIZE. 500 lines, and a line is about 120 bytes, thus about 60
// kilobytes for the life of the process. That is the right order for a
// phone. It is a constant and not a setting: a person who needs another
// number is a person who is already reading this file.

// logHistoryCap is the number of lines that the ring holds.
const logHistoryCap = 500

var (
	logHistory      [logHistoryCap]string
	logHistoryNext  int
	logHistoryCount int
)

// recordLogLine writes one line into the ring. The caller holds
// logMutex.
//
// The ring writes over the oldest line when it is full. A log that stops
// at a cap keeps the start of the session and loses the fault, which is
// the wrong half.
func recordLogLine(msg string) {
	logHistory[logHistoryNext] = msg
	logHistoryNext = (logHistoryNext + 1) % logHistoryCap
	if logHistoryCount < logHistoryCap {
		logHistoryCount++
	}
}

// logHistorySnapshot answers a copy of the ring, oldest line first.
//
// It is a COPY. The caller reads it with no lock, and a writer can add a
// line while the caller still reads.
func logHistorySnapshot() []string {
	logMutex.Lock()
	defer logMutex.Unlock()

	out := make([]string, 0, logHistoryCount)
	start := (logHistoryNext - logHistoryCount + logHistoryCap) % logHistoryCap
	for i := 0; i < logHistoryCount; i++ {
		out = append(out, logHistory[(start+i)%logHistoryCap])
	}
	return out
}

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
	recordLogLine(msg)
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
	broadcastLogLine(line, a.logLineEnabled(lvl, tag))
}

// logFilter is the cached form of the three log switches: Config.LogDebug,
// Config.LogInfo and Config.LogTags.
type logFilter struct {
	debug bool
	info  bool
	tags  map[logTag]bool
}

// applyLogFilter caches the log switches of one configuration.
//
// A LOG LINE MUST NEVER TAKE THE CONFIG LOCK. loadConfig holds the config
// write lock while it runs, and it writes a log line when config.json is
// unreadable. A Go RWMutex is not reentrant, so a read of the configuration
// from inside emitLog would deadlock the startup path. An atomic value costs
// one load for each line and cannot deadlock.
//
// The configuration stays the one authority. This is a copy that two places
// refresh: loadConfig, at the end, and the POST branch of handleConfig,
// after it writes config.json.
func (a *App) applyLogFilter(c Config) {
	f := logFilter{
		debug: c.LogDebug,
		info:  c.LogInfo,
		tags:  make(map[logTag]bool, len(allLogTags)),
	}
	for _, t := range normalizeLogTags(c.LogTags) {
		f.tags[logTag(t)] = true
	}
	a.logFilter.Store(f)
}

// logLineEnabled says whether one line reaches stdout and the browser
// console. An error always does. A debug or an info line needs its level
// switched on and its tag ticked.
//
// Before loadConfig runs, the cache is empty and this answers the same as a
// fresh install: faults only. Every line the application writes that early
// is a fault, so nothing is lost.
func (a *App) logLineEnabled(lvl logLevel, tag logTag) bool {
	if lvl == levelError {
		return true
	}
	f, ok := a.logFilter.Load().(logFilter)
	if !ok {
		return false
	}
	if lvl == levelDebug && !f.debug {
		return false
	}
	if lvl == levelInfo && !f.info {
		return false
	}
	return f.tags[tag]
}

// initLogger sends the standard logger into the stream of /api/logs.
//
// It registered that route as well until 26.09.32, and it was the one
// route outside the block of server.go. registerRoutes holds each route
// now, thus a test reads the whole set from one place. Section 3 of
// CLAUDE.md asks for that one block.
//
// It is unexported since 26.09.32. Nothing outside this package ever
// called it: main_desktop.go and the Android layer use StartServer,
// AssetsRefreshed, GetServerPort, WaitUntilReady, SetLANAddresses and
// SetAndroidPackage. See section 3 of CLAUDE.md on the exported surface.
func (a *App) initLogger() {
	log.SetOutput(&JSLogger{})
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

// handleLogHistory answers the ring of the last logHistoryCap lines,
// oldest first.
//
// WHY THIS IS A SEPARATE ENDPOINT AND NOT A REPLAY ON /api/logs. See the
// banner of the ring above. A replay on the stream breaks the sync
// progress overlay.
//
// IT IS ADMIN ONLY, and /api/logs is not. That looks inconsistent, and
// it is the right pair. The stream carries what happens while a person
// watches. The ring carries what happened before the person arrived,
// which is the shape that a reader of another device would want. A LAN
// share therefore hands out no transcript.
//
// The answer follows section 1.4 of doc/API.md: JSON with a status word.
func (a *App) handleLogHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	lines := logHistorySnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"cap":    logHistoryCap,
		"lines":  lines,
	})
}
