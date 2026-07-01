package backend

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

var (
	logMutex   sync.Mutex
	logClients []chan string
)

type JSLogger struct{}

func (l *JSLogger) Write(p []byte) (n int, err error) {
	msg := string(p)
	logMutex.Lock()
	for _, c := range logClients {
		select {
		case c <- msg:
		default:
		}
	}
	logMutex.Unlock()
	fmt.Print(msg)
	return len(p), nil
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
