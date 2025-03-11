package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"strconv"

	"github.com/rs/cors"
)

var activeSessions = make(map[string]time.Time) // sessionID -> timestamp of last active page change
var sessionPages = make(map[string]string)      // sessionID -> active page
var sessionIPs = make(map[string]string)        // sessionID -> IP address
var mu sync.Mutex

// Timeout channels to handle cancellation of individual session timeouts
var timeoutChannels = make(map[string]chan struct{})

// SSE event channels to push updates to the frontend
var sseClients = make(map[chan<- string]struct{})

// Load HTML template
var templates = template.Must(template.ParseFiles("templates/index.html"))

// Get IP Address of Request
func getIPAddress(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	sessions := []map[string]string{}
	for sessionID, page := range sessionPages {
		sessions = append(sessions, map[string]string{
			"session_id": sessionID,
			"page":       page,
		})
	}
	mu.Unlock()

	err := templates.ExecuteTemplate(w, "index.html", sessions)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// SSE Event Handler
func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Encoding", "identity")

	clientChannel := make(chan string)

	// Register the client
	mu.Lock()
	sseClients[clientChannel] = struct{}{}
	mu.Unlock()

	// Cleanup client on close
	defer func() {
		mu.Lock()
		delete(sseClients, clientChannel)
		mu.Unlock()
		close(clientChannel)
	}()

	// Send updates to the client
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-clientChannel:
			// Send the message to the client as an SSE message
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

// Notify SSE clients when there's an update in active sessions
func notifySSEClients(msg string) {
	mu.Lock()
	defer mu.Unlock()

	// Send the message to all connected SSE clients
	for client := range sseClients {
		client <- msg
	}
}

// Set Active Page
func setActivePage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	page := r.URL.Query().Get("page")
	ip := getIPAddress(r)

	// Get the timeout from query params, default to 5 seconds if not provided
	timeoutParam := r.URL.Query().Get("timeout")
	timeout := 5 * time.Second // default timeout is 5 seconds
	if timeoutParam != "" {
		// Parse the timeout value from the query parameter
		parsedTimeout, err := strconv.Atoi(timeoutParam)
		if err == nil {
			timeout = time.Duration(parsedTimeout) * time.Second
		}
	}

	mu.Lock()

	// Set the session page and the current time
	sessionPages[sessionID] = page
	activeSessions[sessionID] = time.Now().UTC()
	sessionIPs[sessionID] = ip

	// Cancel any previous timeout if the session already exists
	if ch, exists := timeoutChannels[sessionID]; exists {
		close(ch) // Cancel previous timeout
	}

	// Create a new channel for the session timeout
	timeoutCh := make(chan struct{})
	timeoutChannels[sessionID] = timeoutCh

	mu.Unlock()

	// Notify all connected SSE clients that a new session page was updated
	notifySSEClients(fmt.Sprintf("Session updated: %s - Page: %s", sessionID, page))

	// Set a timeout to remove the session after the specified duration asynchronously
	go func() {
		select {
		case <-time.After(timeout):
			mu.Lock()
			delete(sessionPages, sessionID)
			delete(activeSessions, sessionID)
			delete(sessionIPs, sessionID)
			delete(timeoutChannels, sessionID)
			mu.Unlock()

			// Notify all connected SSE clients that the session was removed
			notifySSEClients(fmt.Sprintf("Session timed out: %s", sessionID))

		case <-timeoutCh: // Channel closes if the session is updated before timeout
			return
		}
	}()

	w.Write([]byte("Active page updated"))
}

// Admin API: Get Active Sessions
func getActiveSessions(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	sessions := []map[string]string{}
	for sessionID, page := range sessionPages {
		sessions = append(sessions, map[string]string{
			"session_id": sessionID,
			"page":       page,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func main() {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/sse", sseHandler)
	http.HandleFunc("/set_active", setActivePage)
	http.HandleFunc("/admin/sessions", getActiveSessions)

	log.Println("Switch Service Running on :8080")

	// Enable CORS
	http.ListenAndServe(":8080", cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"}, // Allow frontend URL
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}).Handler(http.DefaultServeMux))
}
