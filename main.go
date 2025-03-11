package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rs/cors"
)

var (
	activeSessions  = make(map[string]time.Time)
	sessionPages    = make(map[string]string)
	sessionIPs      = make(map[string]string)
	sessionDataSize = make(map[string]map[string]int64) // sessionID -> {request_size, response_size}
	mu              sync.Mutex
	timeoutChannels = make(map[string]chan struct{})
	sseClients      = make(map[chan<- string]struct{})
	templates       = template.Must(template.ParseFiles("templates/index.html"))
)

// Custom response writer to track response size
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	writtenBytes int64
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.writtenBytes += int64(n)
	return n, err
}

func (rw *responseWriterWrapper) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}


// Middleware to track request and response size
func trackTrafficSize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			next.ServeHTTP(w, r)
			return
		}

		requestSize := r.ContentLength
		wrappedWriter := &responseWriterWrapper{ResponseWriter: w, statusCode: 200}

		// Process the request
		next.ServeHTTP(wrappedWriter, r)

		mu.Lock()
		if _, exists := sessionDataSize[sessionID]; !exists {
			sessionDataSize[sessionID] = map[string]int64{"request_size": 0, "response_size": 0}
		}
		sessionDataSize[sessionID]["request_size"] += requestSize
		sessionDataSize[sessionID]["response_size"] += wrappedWriter.writtenBytes
		mu.Unlock()
	})
}

// Capture Request Size
func getRequestSize(r *http.Request) int {
	requestSize := 0
	if r.ContentLength > 0 {
		requestSize = int(r.ContentLength)
	}
	for key, values := range r.Header {
		for _, value := range values {
			requestSize += len(key) + len(value)
		}
	}
	return requestSize
}


// Get IP Address of Request
func getIPAddress(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Serve HTML Page
func homeHandler(w http.ResponseWriter, r *http.Request) {
	templates.Execute(w, nil)
}

// SSE Event Handler
func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChannel := make(chan string)
	mu.Lock()
	sseClients[clientChannel] = struct{}{}
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(sseClients, clientChannel)
		mu.Unlock()
		close(clientChannel)
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-clientChannel:
			jsonData, err := json.Marshal(msg)
			if err != nil {
				log.Println("Error marshaling data to JSON:", err)
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			w.(http.Flusher).Flush()
		}
	}
}

// Notify SSE clients when there's an update
func notifySSEClients(msg string) {
	mu.Lock()
	defer mu.Unlock()
	for client := range sseClients {
		client <- msg
	}
}

// Set Active Page
func setActivePage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	page := r.URL.Query().Get("page")
	ip := getIPAddress(r)

	timeoutParam := r.URL.Query().Get("timeout")
	timeout := 5 * time.Second
	if timeoutParam != "" {
		parsedTimeout, err := strconv.Atoi(timeoutParam)
		if err == nil {
			timeout = time.Duration(parsedTimeout) * time.Second
		}
	}

	mu.Lock()
	sessionPages[sessionID] = page
	activeSessions[sessionID] = time.Now().UTC()
	sessionIPs[sessionID] = ip
	sessionDataSize[sessionID] = map[string]int64{"request_size": 0, "response_size": 0}

	if ch, exists := timeoutChannels[sessionID]; exists {
		close(ch)
	}

	timeoutCh := make(chan struct{})
	timeoutChannels[sessionID] = timeoutCh
	mu.Unlock()

	notifySSEClients(fmt.Sprintf("Session updated: %s - Page: %s - IP: %s", sessionID, page, ip))

	go func() {
		select {
		case <-time.After(timeout):
			mu.Lock()
			delete(sessionPages, sessionID)
			delete(activeSessions, sessionID)
			delete(sessionIPs, sessionID)
			delete(sessionDataSize, sessionID)
			delete(timeoutChannels, sessionID)
			mu.Unlock()

			notifySSEClients(fmt.Sprintf("Session timed out: %s", sessionID))

		case <-timeoutCh:
			return
		}
	}()

	w.Write([]byte("Active page updated"))
}

// Admin API: Get Active Sessions with Data Size
func getActiveSessions(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	sessions := []map[string]interface{}{}
	for sessionID, page := range sessionPages {
		sessions = append(sessions, map[string]interface{}{
			"session_id":   sessionID,
			"page":         page,
			"ip":           sessionIPs[sessionID],
			"request_size": sessionDataSize[sessionID]["request_size"],
			"response_size": sessionDataSize[sessionID]["response_size"],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/sse", sseHandler)
	mux.HandleFunc("/set_active", setActivePage)
	mux.HandleFunc("/admin/sessions", getActiveSessions)

	log.Println("Switch Service Running on :http://localhost:6748")

	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}).Handler(trackTrafficSize(mux))

	http.ListenAndServe(":6748", handler)
}
