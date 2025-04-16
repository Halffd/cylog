package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/webview/webview"
	"github.com/zserge/lorca"
)

// Constants
const (
	appPort      = 8080
	appWidth     = 1000
	appHeight    = 700
	webSocketURL = "wss://cytube.net/ws" // Update with actual WebSocket URL
)

// Message represents a chat message
type Message struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	HTML      string    `json:"html"`
}

// ChatServer manages chat state and connections
type ChatServer struct {
	clients     map[*websocket.Conn]bool
	messages    []Message
	broadcast   chan Message
	register    chan *websocket.Conn
	unregister  chan *websocket.Conn
	cytubeConn  *websocket.Conn
	messagesMux sync.RWMutex
	upgrader    websocket.Upgrader
}

//go:embed static
var staticFiles embed.FS

// NewChatServer creates a new chat server
func NewChatServer() *ChatServer {
	return &ChatServer{
		clients:    make(map[*websocket.Conn]bool),
		messages:   make([]Message, 0, 100),
		broadcast:  make(chan Message),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all connections
			},
		},
	}
}

// Run starts the chat server
func (s *ChatServer) Run(ctx context.Context) {
	// Connect to Cytube WebSocket
	err := s.connectToCytube()
	if err != nil {
		log.Printf("Failed to connect to Cytube: %v", err)
	}

	// Start the server routines
	go s.handleMessages(ctx)
}

// connectToCytube establishes a connection to the Cytube WebSocket
func (s *ChatServer) connectToCytube() error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(webSocketURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Cytube WebSocket: %w", err)
	}

	s.cytubeConn = conn
	go s.readCytubeMessages()
	return nil
}

// readCytubeMessages reads messages from the Cytube WebSocket
func (s *ChatServer) readCytubeMessages() {
	defer s.cytubeConn.Close()

	for {
		_, data, err := s.cytubeConn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from Cytube: %v", err)
			// Try to reconnect after a short delay
			time.Sleep(5 * time.Second)
			err = s.connectToCytube()
			if err != nil {
				log.Printf("Failed to reconnect to Cytube: %v", err)
			}
			return
		}

		// Parse and handle the message
		// Note: The actual parsing would depend on the Cytube message format
		// This is a simplified example
		msg := Message{
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			Username:  "User", // Extract from message
			Timestamp: time.Now(),
			Content:   string(data),
			HTML:      string(data), // Assuming HTML content is provided
		}

		s.broadcast <- msg
	}
}

// handleMessages processes incoming messages and client registrations
func (s *ChatServer) handleMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-s.register:
			s.clients[client] = true
			s.sendRecentMessages(client)
		case client := <-s.unregister:
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				client.Close()
			}
		case message := <-s.broadcast:
			// Store the message
			s.messagesMux.Lock()
			// Keep only the most recent 100 messages
			if len(s.messages) >= 100 {
				s.messages = s.messages[1:]
			}
			s.messages = append(s.messages, message)
			s.messagesMux.Unlock()

			// Broadcast to all clients
			for client := range s.clients {
				err := client.WriteJSON(message)
				if err != nil {
					log.Printf("Error broadcasting message: %v", err)
					client.Close()
					delete(s.clients, client)
				}
			}
		}
	}
}

// sendRecentMessages sends recent messages to a newly connected client
func (s *ChatServer) sendRecentMessages(client *websocket.Conn) {
	s.messagesMux.RLock()
	defer s.messagesMux.RUnlock()

	for _, msg := range s.messages {
		err := client.WriteJSON(msg)
		if err != nil {
			log.Printf("Error sending recent message: %v", err)
			return
		}
	}
}

// handleWebSocket handles WebSocket connections from clients
func (s *ChatServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}

	// Register the client
	s.register <- conn

	// Read messages from the client
	go func() {
		defer func() {
			s.unregister <- conn
		}()

		for {
			var msg Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				break
			}

			// Process the message if needed
			// For now, we just echo it back
			s.broadcast <- msg
		}
	}()
}

// setupHTTPServer sets up the HTTP server for web UI
func setupHTTPServer(ctx context.Context, chatServer *ChatServer) *http.Server {
	mux := http.NewServeMux()

	// Serve static files
	staticContent, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("Failed to load static files: %v", err)
	}
	fileServer := http.FileServer(http.FS(staticContent))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	// Serve index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		tmpl, err := template.ParseFS(staticFiles, "static/index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.Execute(w, map[string]interface{}{
			"WebSocketURL": fmt.Sprintf("ws://%s/ws", r.Host),
		})
	})

	// WebSocket endpoint
	mux.HandleFunc("/ws", chatServer.handleWebSocket)

	// API endpoints
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		chatServer.messagesMux.RLock()
		defer chatServer.messagesMux.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatServer.messages)
	})

	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", appPort),
		Handler: mux,
	}

	// Start the HTTP server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	return server
}

// launchGUI creates and launches the desktop GUI using webview or lorca
func launchGUI(ctx context.Context, useWebView bool) {
	appURL := fmt.Sprintf("http://localhost:%d", appPort)

	if useWebView {
		// Use webview for cross-platform GUI
		w := webview.New(true)
		defer w.Destroy()
		w.SetTitle("Cytube Chat Viewer")
		w.SetSize(appWidth, appHeight, webview.HintNone)
		w.Navigate(appURL)

		// Run the webview window
		w.Run()
	} else {
		// Use lorca (Chromium-based) if available
		ui, err := lorca.New(appURL, "", appWidth, appHeight)
		if err != nil {
			log.Fatalf("Failed to create UI: %v", err)
		}
		defer ui.Close()

		// Wait for the window to be closed
		<-ui.Done()
	}
}

// createStaticFiles creates the necessary static files for the web UI
func createStaticFiles() error {
	staticDir := filepath.Join(".", "static")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		return err
	}

	// Create index.html
	indexHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Cytube Chat Viewer</title>
    <link rel="stylesheet" href="/static/styles.css">
</head>
<body>
    <div class="app-container">
        <header>
            <h1>Cytube Chat Viewer</h1>
            <div class="controls">
                <button id="fontSizeIncrease">A+</button>
                <button id="fontSizeDecrease">A-</button>
                <button id="chatWidthIncrease">W+</button>
                <button id="chatWidthDecrease">W-</button>
            </div>
        </header>
        <main>
            <div id="chatwrap">
                <div id="messagebuffer"></div>
            </div>
        </main>
    </div>
    <script>
        const wsUrl = "{{.WebSocketURL}}";
    </script>
    <script src="/static/app.js"></script>
</body>
</html>`

	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte(indexHTML), 0644); err != nil {
		return err
	}

	// Create CSS file
	cssContent := `body {
    font-family: Arial, sans-serif;
    margin: 0;
    padding: 0;
    background-color: #0f0f0f;
    color: #fff;
}

.app-container {
    display: flex;
    flex-direction: column;
    height: 100vh;
}

header {
    background-color: #1a1a1a;
    padding: 10px;
    display: flex;
    justify-content: space-between;
    align-items: center;
}

h1 {
    margin: 0;
    font-size: 20px;
}

.controls button {
    background-color: #333;
    color: white;
    border: none;
    padding: 5px 10px;
    margin-left: 5px;
    cursor: pointer;
}

.controls button:hover {
    background-color: #555;
}

main {
    flex: 1;
    overflow: hidden;
    display: flex;
    padding: 10px;
}

#chatwrap {
    width: 27em;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.5);
    border: 2px solid rgba(235, 155, 125, 0.5);
    overflow-y: auto;
    padding: 0;
}

#messagebuffer {
    padding: 10px;
}

.message {
    margin-bottom: 10px;
    word-wrap: break-word;
}

.timestamp {
    color: #999;
    font-size: 2.2em;
    margin-right: 5px;
}

.username {
    color: #66aaff;
    font-size: 2.2em;
    font-weight: bold;
    margin-right: 5px;
}

.content {
    font-size: 4.8em;
}

@media (max-width: 768px) {
    #chatwrap {
        width: 100%;
    }
}`

	if err := os.WriteFile(filepath.Join(staticDir, "styles.css"), []byte(cssContent), 0644); err != nil {
		return err
	}

	// Create JavaScript file
	jsContent := `document.addEventListener('DOMContentLoaded', () => {
    const messagebuffer = document.getElementById('messagebuffer');
    const chatwrap = document.getElementById('chatwrap');
    
    // Message cache for deduplication
    const messageIds = new Set();
    
    // WebSocket connection
    const socket = new WebSocket(wsUrl);
    
    socket.onopen = () => {
        console.log('Connected to server');
    };
    
    socket.onmessage = (event) => {
        const message = JSON.parse(event.data);
        addMessage(message);
    };
    
    socket.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
    
    socket.onclose = () => {
        console.log('Disconnected from server');
        // Attempt to reconnect after a delay
        setTimeout(() => {
            window.location.reload();
        }, 5000);
    };
    
    // Add a message to the chat
    function addMessage(message) {
        // Skip if we've already added this message
        if (messageIds.has(message.id)) {
            return;
        }
        
        // Add to our tracking set
        messageIds.add(message.id);
        
        const shouldScroll = isAtBottom();
        
        // If we have HTML content, use that directly
        if (message.html) {
            const tempDiv = document.createElement('div');
            tempDiv.innerHTML = message.html;
            tempDiv.classList.add('message');
            
            // Add a data attribute for tracking
            tempDiv.setAttribute('data-message-id', message.id);
            
            messagebuffer.appendChild(tempDiv);
        } else {
            // Otherwise create structured message
            const msgElement = document.createElement('div');
            msgElement.classList.add('message');
            msgElement.setAttribute('data-message-id', message.id);
            
            const timestamp = document.createElement('span');
            timestamp.classList.add('timestamp');
            timestamp.textContent = new Date(message.timestamp).toLocaleTimeString();
            
            const username = document.createElement('span');
            username.classList.add('username');
            username.textContent = message.username;
            
            const content = document.createElement('span');
            content.classList.add('content');
            content.textContent = message.content;
            
            msgElement.appendChild(timestamp);
            msgElement.appendChild(username);
            msgElement.appendChild(content);
            
            messagebuffer.appendChild(msgElement);
        }
        
        // Limit the number of messages (keep last 100)
        const messages = messagebuffer.querySelectorAll('.message');
        if (messages.length > 100) {
            messagebuffer.removeChild(messages[0]);
        }
        
        // Scroll to bottom if we were already at the bottom
        if (shouldScroll) {
            scrollToBottom();
        }
    }
    
    // Check if user is at the bottom of the chat
    function isAtBottom() {
        return messagebuffer.scrollHeight - messagebuffer.scrollTop <= messagebuffer.clientHeight + 10;
    }
    
    // Scroll to the bottom of the chat
    function scrollToBottom() {
        messagebuffer.scrollTop = messagebuffer.scrollHeight;
    }
    
    // Font size adjustment
    let fontSize = 16; // Base font size in pixels
    
    document.getElementById('fontSizeIncrease').addEventListener('click', () => {
        fontSize += 1;
        updateFontSize();
    });
    
    document.getElementById('fontSizeDecrease').addEventListener('click', () => {
        fontSize = Math.max(8, fontSize - 1);
        updateFontSize();
    });
    
    function updateFontSize() {
        document.documentElement.style.setProperty('--base-font-size', fontSize + 'px');
        document.querySelectorAll('#messagebuffer span:not(.timestamp):not(.username)').forEach(el => {
            el.style.fontSize = (fontSize * 1.2) + 'px';
        });
        document.querySelectorAll('#messagebuffer span.timestamp, #messagebuffer span.username').forEach(el => {
            el.style.fontSize = (fontSize * 0.8) + 'px';
        });
    }
    
    // Chat width adjustment
    let chatWidth = 27; // Width in em
    
    document.getElementById('chatWidthIncrease').addEventListener('click', () => {
        chatWidth += 1;
        updateChatWidth();
    });
    
    document.getElementById('chatWidthDecrease').addEventListener('click', () => {
        chatWidth = Math.max(10, chatWidth - 1);
        updateChatWidth();
    });
    
    function updateChatWidth() {
        chatwrap.style.width = chatWidth + 'em';
    }
    
    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        // Ignore if focus is on an input element
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) {
            return;
        }
        
        if (e.key === '=' || e.key === '+') {
            if (e.shiftKey) {
                // Increase font size
                fontSize += 1;
                updateFontSize();
            } else {
                // Increase chat width
                chatWidth += 1;
                updateChatWidth();
            }
            e.preventDefault();
        } else if (e.key === '-' || e.key === '_') {
            if (e.shiftKey) {
                // Decrease font size
                fontSize = Math.max(8, fontSize - 1);
                updateFontSize();
            } else {
                // Decrease chat width
                chatWidth = Math.max(10, chatWidth - 1);
                updateChatWidth();
            }
            e.preventDefault();
        }
    });
    
    // Fetch initial messages
    fetch('/api/messages')
        .then(response => response.json())
        .then(messages => {
            messages.forEach(message => addMessage(message));
            scrollToBottom();
        })
        .catch(error => console.error('Error fetching messages:', error));
});`

	if err := os.WriteFile(filepath.Join(staticDir, "app.js"), []byte(jsContent), 0644); err != nil {
		return err
	}

	return nil
}

func main() {
	// Create context that will be canceled on SIGINT or SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		cancel()
	}()

	// Create static files for the web UI
	if err := createStaticFiles(); err != nil {
		log.Fatalf("Failed to create static files: %v", err)
	}

	// Create and start the chat server
	chatServer := NewChatServer()
	chatServer.Run(ctx)

	// Setup and start HTTP server
	httpServer := setupHTTPServer(ctx, chatServer)

	// Launch the desktop GUI
	useWebView := true // Set to false to use lorca (Chromium) instead
	go launchGUI(ctx, useWebView)

	// Wait for context cancellation or application exit
	<-ctx.Done()

	// Gracefully shutdown the HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Application shutdown complete")
}