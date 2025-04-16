package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Constants
const (
	appPort         = 8080
	appWidth        = 1000
	appHeight       = 700
	webSocketURL    = "wss://cytube.net/ws" // Update with actual WebSocket URL
	logsDir         = "logs"
	maxLogFileSize  = 10 * 1024 * 1024 // 10 MB
	maxLogFiles     = 5
	logDateFormat   = "2006-01-02"
	desktopAppTitle = "Cytube Chat Viewer"
)

// Message represents a chat message
type Message struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	HTML      string    `json:"html"`
}

// Logger handles logging to files
type Logger struct {
	currentLogFile *os.File
	logMutex       sync.Mutex
	logFilePath    string
}

// NewLogger creates a new logger instance
func NewLogger() (*Logger, error) {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logger := &Logger{}
	if err := logger.rotateLogFile(); err != nil {
		return nil, err
	}

	return logger, nil
}

// rotateLogFile creates a new log file with the current date
func (l *Logger) rotateLogFile() error {
	l.logMutex.Lock()
	defer l.logMutex.Unlock()

	// Close the current log file if it's open
	if l.currentLogFile != nil {
		l.currentLogFile.Close()
	}

	// Create a new log file with the current date
	currentDate := time.Now().Format(logDateFormat)
	logFileName := fmt.Sprintf("chat-%s.log", currentDate)
	l.logFilePath = filepath.Join(logsDir, logFileName)

	file, err := os.OpenFile(l.logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.currentLogFile = file

	// Clean old log files
	go l.cleanOldLogFiles()

	return nil
}

// cleanOldLogFiles removes old log files if there are more than maxLogFiles
func (l *Logger) cleanOldLogFiles() {
	files, err := filepath.Glob(filepath.Join(logsDir, "chat-*.log"))
	if err != nil {
		log.Printf("Error finding log files: %v", err)
		return
	}

	if len(files) <= maxLogFiles {
		return
	}

	// Sort files by modification time (oldest first)
	fileInfos := make(map[string]os.FileInfo, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			log.Printf("Error getting file info for %s: %v", file, err)
			continue
		}
		fileInfos[file] = info
	}

	// Sort files by modification time
	filesToDelete := len(files) - maxLogFiles
	for i := 0; i < filesToDelete; i++ {
		var oldestFile string
		var oldestTime time.Time
		first := true

		for file, info := range fileInfos {
			if first || info.ModTime().Before(oldestTime) {
				oldestFile = file
				oldestTime = info.ModTime()
				first = false
			}
		}

		if oldestFile != "" {
			os.Remove(oldestFile)
			delete(fileInfos, oldestFile)
			log.Printf("Deleted old log file: %s", oldestFile)
		}
	}
}

// LogMessage logs a message to the current log file
func (l *Logger) LogMessage(msg Message) error {
	l.logMutex.Lock()
	defer l.logMutex.Unlock()

	// Check if we need to rotate the log file based on size
	info, err := os.Stat(l.logFilePath)
	if err == nil && info.Size() > maxLogFileSize {
		if err := l.rotateLogFile(); err != nil {
			return err
		}
	}

	// Check if we need to rotate based on date
	currentDate := time.Now().Format(logDateFormat)
	if !strings.Contains(l.logFilePath, currentDate) {
		if err := l.rotateLogFile(); err != nil {
			return err
		}
	}

	// Format and write the log entry
	timestamp := msg.Timestamp.Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s: %s\n", timestamp, msg.Username, msg.Content)

	if _, err := l.currentLogFile.WriteString(logEntry); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	return nil
}

// GetAvailableLogs returns a list of available log files
func (l *Logger) GetAvailableLogs() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(logsDir, "chat-*.log"))
	if err != nil {
		return nil, fmt.Errorf("failed to find log files: %w", err)
	}

	// Extract just the filenames without the path
	logFiles := make([]string, len(files))
	for i, file := range files {
		logFiles[i] = filepath.Base(file)
	}

	return logFiles, nil
}

// GetLogContent returns the content of a specified log file
func (l *Logger) GetLogContent(filename string) (string, error) {
	// Validate the filename to ensure it's a log file
	if !strings.HasPrefix(filename, "chat-") || !strings.HasSuffix(filename, ".log") {
		return "", fmt.Errorf("invalid log filename")
	}

	// Construct the full path with directory
	filePath := filepath.Join(logsDir, filename)

	// Read the file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read log file: %w", err)
	}

	return string(content), nil
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
	logger      *Logger
}

// NewChatServer creates a new chat server
func NewChatServer(logger *Logger) *ChatServer {
	return &ChatServer{
		clients:    make(map[*websocket.Conn]bool),
		messages:   make([]Message, 0, 100),
		broadcast:  make(chan Message),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		logger:     logger,
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

		// Log the message to file
		if err := s.logger.LogMessage(msg); err != nil {
			log.Printf("Error logging message: %v", err)
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
func (s *ChatServer) handleWebSocket(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
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

			// Log the message to file
			if err := s.logger.LogMessage(msg); err != nil {
				log.Printf("Error logging message: %v", err)
			}

			// Process the message if needed
			// For now, we just echo it back
			s.broadcast <- msg
		}
	}()
}

// setupGinServer sets up the Gin server for web UI and API
func setupGinServer(ctx context.Context, chatServer *ChatServer) *gin.Engine {
	// Set Gin to release mode in production
	gin.SetMode(gin.ReleaseMode)

	// Create gin router
	router := gin.Default()

	// Load HTML templates
	router.LoadHTMLGlob("static/*.html")

	// Serve static files
	router.Static("/static", "./static")

	// Serve scripts directory
	router.Static("/scripts", "./scripts")

	// API group for v1
	api := router.Group("/api/v1")
	{
		// Messages endpoints
		api.GET("/messages", func(c *gin.Context) {
			chatServer.messagesMux.RLock()
			defer chatServer.messagesMux.RUnlock()

			c.JSON(http.StatusOK, chatServer.messages)
		})

		// Logs endpoints
		api.GET("/logs", func(c *gin.Context) {
			logs, err := chatServer.logger.GetAvailableLogs()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, logs)
		})

		api.GET("/logs/:filename", func(c *gin.Context) {
			filename := c.Param("filename")
			content, err := chatServer.logger.GetLogContent(filename)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			// Check if format=json is requested
			if c.Query("format") == "json" {
				// Parse log content into JSON array
				lines := strings.Split(content, "\n")
				logs := make([]map[string]string, 0)

				for _, line := range lines {
					if line == "" {
						continue
					}

					// Parse line like: [2025-04-16 15:04:05] Username: Message content
					re := regexp.MustCompile(`\[(.*?)\] (.*?): (.*)`)
					matches := re.FindStringSubmatch(line)

					if len(matches) == 4 {
						logs = append(logs, map[string]string{
							"timestamp": matches[1],
							"username":  matches[2],
							"content":   matches[3],
						})
					}
				}

				c.JSON(http.StatusOK, logs)
			} else {
				// Return as plain text
				c.String(http.StatusOK, content)
			}
		})
	}

	// Tampermonkey compatibility endpoints
	api.GET("/tampermonkey/bridge.user.js", func(c *gin.Context) {
		// Serve the Tampermonkey bridge script with the correct content type
		c.File("scripts/cylog-tampermonkey-bridge.js")
	})

	// Backwards compatibility for old API
	router.GET("/api/messages", func(c *gin.Context) {
		chatServer.messagesMux.RLock()
		defer chatServer.messagesMux.RUnlock()

		c.JSON(http.StatusOK, chatServer.messages)
	})

	// Serve index page
	router.GET("/", func(c *gin.Context) {
		host := c.Request.Host
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Host":                     host,
			"InjectTampermonkeyBridge": true,
		})
	})

	// WebSocket endpoint
	router.GET("/ws", chatServer.handleWebSocket)

	// Add a logs page
	router.GET("/logs", func(c *gin.Context) {
		logs, err := chatServer.logger.GetAvailableLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.HTML(http.StatusOK, "logs.html", gin.H{
			"Logs": logs,
		})
	})

	return router
}

// openBrowser opens the URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	// Detect the OS and set the appropriate command
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

// launchDesktopApp launches the desktop application using either webview or the system browser
func launchDesktopApp(url string) {
	// First try to use webview
	if webviewAvailable() {
		go func() {
			// Wait a moment for the server to start
			time.Sleep(500 * time.Millisecond)

			if err := startWebViewApp(url); err != nil {
				log.Printf("Failed to start WebView app: %v, falling back to browser", err)
				openBrowser(url)
			}
		}()
		return
	}

	// Fall back to system browser
	log.Println("WebView not available, opening in system browser")
	openBrowser(url)
}

// webviewAvailable checks if the WebView library is available
func webviewAvailable() bool {
	// Try running a simple command to check if WebView can be initialized
	cmd := exec.Command("go", "run", "-c", `
		package main
		import "github.com/webview/webview"
		func main() {
			w := webview.New(false)
			defer w.Destroy()
		}
	`)

	return cmd.Run() == nil
}

// startWebViewApp starts the WebView-based desktop app
func startWebViewApp(url string) error {
	// This requires the webview package to be available
	// Since we don't have it directly in our dependencies,
	// we'll create a small app that uses it and run it

	// Create a temporary go file
	tempDir, err := os.MkdirTemp("", "cylog-webview")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	webviewAppPath := filepath.Join(tempDir, "webview_app.go")
	webviewAppContent := fmt.Sprintf(`
package main

import (
	"github.com/webview/webview"
)

func main() {
	w := webview.New(true)
	defer w.Destroy()
	w.SetTitle("%s")
	w.SetSize(%d, %d, webview.HintNone)
	w.Navigate("%s")
	w.Run()
}
`, desktopAppTitle, appWidth, appHeight, url)

	if err := os.WriteFile(webviewAppPath, []byte(webviewAppContent), 0644); err != nil {
		return err
	}

	// Run the temporary WebView app
	cmd := exec.Command("go", "run", webviewAppPath)
	return cmd.Start()
}

// setupLogger configures the application logging to both file and console
func setupLogger() (*log.Logger, error) {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Open app log file
	appLogPath := filepath.Join(logsDir, "app.log")
	appLogFile, err := os.OpenFile(appLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open app log file: %w", err)
	}

	// Create a multi-writer to log to both file and console
	multiWriter := io.MultiWriter(os.Stdout, appLogFile)

	// Create and configure the logger
	logger := log.New(multiWriter, "", log.LstdFlags)

	// Replace the standard logger
	log.SetOutput(multiWriter)
	log.SetFlags(log.LstdFlags)

	return logger, nil
}

func main() {
	// Setup application logging
	appLogger, err := setupLogger()
	if err != nil {
		log.Fatalf("Failed to setup logger: %v", err)
	}

	appLogger.Println("Starting Cylog application")

	// Create context that will be canceled on SIGINT or SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		appLogger.Println("Shutting down due to signal")
		cancel()
	}()

	// Initialize chat logger
	chatLogger, err := NewLogger()
	if err != nil {
		appLogger.Fatalf("Failed to initialize chat logger: %v", err)
	}

	// Create and start the chat server
	chatServer := NewChatServer(chatLogger)
	chatServer.Run(ctx)

	// Setup Gin server
	router := setupGinServer(ctx, chatServer)

	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", appPort),
		Handler: router,
	}

	// Start the HTTP server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Fatalf("HTTP server error: %v", err)
		}
	}()

	appLogger.Printf("Server started at http://localhost:%d", appPort)

	// Launch the desktop application
	appURL := fmt.Sprintf("http://localhost:%d", appPort)
	launchDesktopApp(appURL)

	// Wait for context cancellation
	<-ctx.Done()
	appLogger.Println("Shutting down server...")

	// Gracefully shutdown the HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		appLogger.Printf("HTTP server shutdown error: %v", err)
	}

	appLogger.Println("Application shutdown complete")
}
