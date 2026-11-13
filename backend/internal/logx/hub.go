// Package logx writes the log lines of the app: the tags, the levels, the
// filter, the stream and the history ring.
package logx

// ----------------------------------------------------------------------
// The log transport
// ----------------------------------------------------------------------
//
// Hub.Broadcast is the only fan-out. It sends each line to THREE places:
// stdout, the /api/logs stream and the history ring. stdout is for a desktop
// user and for adb logcat. Each open page reads the stream, copies it into
// the browser console, and the sync overlay reads its stage text there. See
// doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md.
// Each App holds one Hub. Two callers reach Broadcast:
//
//	StdWriter       The standard log package, for the two call sites that
//	                cannot reach an *App. See TestNoDirectLogPrintf.
//	Logger.emit     Each other line, through Debugf, Infof or Errf.
//
// THE STREAM ALWAYS CARRIES EACH LINE. The sync overlay needs the "[sync]"
// debug lines also when a reader asks for less. The browser can also change
// its filter with no restart. The switches thus control stdout only.
//
// The stream is a live sample. A client with a full channel loses the line,
// and the writer never waits. The stream must thus never drive a state that
// needs each event.

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// ----------------------------------------------------------------------
// The history ring
// ----------------------------------------------------------------------
//
// A page that opens after an event never sees its lines on the stream. The
// ring holds the last HistoryCap lines, and /api/logs/history answers with
// them.
//
// THE RING DOES NOT REPLAY ON THE STREAM. applySyncLogLine in omn-go-api.js
// reads the "[sync] (debug)" lines of the stream. A replay would show an old
// sync on each page load.
//
// THE RING HOLDS EACH LINE, the same as the stream. A person who turned debug
// off and then met a fault needs those debug lines most.
//
// 500 lines of about 120 bytes use about 60 KB for the life of the process.
// That size suits a phone. It is a constant and not a setting.

// HistoryCap is the number of lines that the ring holds.
const HistoryCap = 500

// Hub holds the stream clients and the history ring of one App. mu keeps
// two lines apart, and it guards each field.
type Hub struct {
	mu      sync.Mutex
	clients []chan string
	history [HistoryCap]string
	next    int
	count   int
}

// record writes one line into the ring. The caller holds mu. When the ring
// is full, the new line replaces the oldest one. A log that stops at a limit
// keeps the start and loses the fault.
func (h *Hub) record(msg string) {
	h.history[h.next] = msg
	h.next = (h.next + 1) % HistoryCap
	if h.count < HistoryCap {
		h.count++
	}
}

// Snapshot answers a COPY of the ring, oldest line first. The caller reads
// it without the lock while a writer adds lines.
func (h *Hub) Snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]string, 0, h.count)
	start := (h.next - h.count + HistoryCap) % HistoryCap
	for i := 0; i < h.count; i++ {
		out = append(out, h.history[(start+i)%HistoryCap])
	}
	return out
}

// TimeLayout is the time prefix of the standard log package with
// log.LstdFlags. Logger.emit writes the stamp itself. Both sources must look
// the same, or the page must parse two shapes. Both are UTC: emit converts
// the time, and initLogger of package app sets log.LUTC.
const TimeLayout = "2006/01/02 15:04:05 "

// Broadcast sends one line to each stream client and to the ring, and to
// stdout when toStdout is true. It holds mu, thus two lines cannot mix.
func (h *Hub) Broadcast(msg string, toStdout bool) {
	h.mu.Lock()
	h.record(msg)
	for _, c := range h.clients {
		select {
		case c <- msg:
		default:
		}
	}
	if toStdout {
		fmt.Print(msg)
	}
	h.mu.Unlock()
}

// Subscribe adds one stream client. The channel holds 10 lines.
func (h *Hub) Subscribe() chan string {
	ch := make(chan string, 10)
	h.mu.Lock()
	h.clients = append(h.clients, ch)
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes one stream client.
func (h *Hub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, c := range h.clients {
		if c == ch {
			h.clients = append(h.clients[:i], h.clients[i+1:]...)
			return
		}
	}
}

// ClientCount answers the number of stream clients.
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// StdWriter answers the writer of the standard log package for h.
func (h *Hub) StdWriter() io.Writer { return stdWriter{h} }

type stdWriter struct{ hub *Hub }

func (w stdWriter) Write(p []byte) (n int, err error) {
	// A line from the standard log package has no level. It always goes to
	// stdout, because no filter applies to it.
	w.hub.Broadcast(string(p), true)
	return len(p), nil
}

// emit makes one line "[tag] (level) message", stamps it, and gives it to
// the hub. It is the only writer of a line with a level.
func (l Logger) emit(lvl Level, format string, args ...any) {
	line := time.Now().UTC().Format(TimeLayout) +
		"[" + string(l.tag) + "] (" + string(lvl) + ") " +
		fmt.Sprintf(format, args...) + "\n"
	l.hub.Broadcast(line, l.Enabled(lvl))
}

// Filter is the cached form of Config.LogDebug, Config.LogInfo and
// Config.LogTags.
type Filter struct {
	Debug bool
	Info  bool
	Tags  map[Tag]bool
}

// Enabled tells whether a line of level lvl with the tag of l reaches stdout
// and the browser console. An error always does. A debug or info line needs
// its level on and its tag checked. Before the first filter is stored, it
// allows faults only, the same as a fresh install.
func (l Logger) Enabled(lvl Level) bool {
	if lvl == LevelError {
		return true
	}
	f, ok := l.filter.Load().(Filter)
	if !ok {
		return false
	}
	if lvl == LevelDebug && !f.Debug {
		return false
	}
	if lvl == LevelInfo && !f.Info {
		return false
	}
	return f.Tags[l.tag]
}
