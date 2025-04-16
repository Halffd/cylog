# Cylog - Cytube Chat Viewer

A desktop and web application for viewing Cytube chat messages with a clean, customizable interface.

## Features

- Real-time chat viewing with WebSocket connection
- Adjustable font size and chat width
- Modern, clean UI with dark theme
- Keyboard shortcuts for easy navigation
- Desktop and web interface
- File-based message logging with rotation
- Log viewing and management through web interface
- RESTful API for programmatic access to messages and logs
- Compatible with Cytube ChatStyleAdjuster Tampermonkey script

## Technologies

- Go (Golang) with Gin web framework
- WebSockets for real-time communication
- HTML/CSS/JavaScript for the frontend
- Desktop mode with WebView or browser fallback

## Getting Started

### Prerequisites

- Go 1.16 or higher

### Installation

1. Clone the repository
```
git clone https://github.com/yourusername/cylog.git
cd cylog
```

2. Build the application
```
go build
```

3. Run the application
```
./cylog
```

The application will automatically launch as a desktop app using WebView if available, or fall back to your default web browser.

## Tampermonkey Integration

Cylog is compatible with the "Cytube Chat Style Adjuster" Tampermonkey script. This allows you to:

1. Use the same keyboard shortcuts for adjusting font size and chat width
2. View a chat preview similar to the one on Cytube
3. Maintain consistent styling between Cytube and Cylog

### Installing the Tampermonkey Script

1. Install the [Tampermonkey](https://www.tampermonkey.net/) extension for your browser
2. Visit http://localhost:8080/api/v1/tampermonkey/bridge.user.js to install the bridge script
3. Install the Cylog-compatible version of the script from http://localhost:8080/scripts/cylog-compatible-cytube.js

## Configuration

You can modify the following constants in `main.go`:

- `appPort`: The HTTP server port (default: 8080)
- `webSocketURL`: The Cytube WebSocket URL to connect to
- `logsDir`: Directory for storing log files (default: "logs")
- `maxLogFileSize`: Maximum size for a log file before rotation (default: 10MB)
- `maxLogFiles`: Maximum number of log files to keep (default: 5)

## API Endpoints

Cylog provides a RESTful API for accessing chat messages and logs:

### Messages

- `GET /api/v1/messages` - Get all recent messages (JSON)
- `GET /api/messages` - Legacy endpoint for backwards compatibility

### Logs

- `GET /api/v1/logs` - Get list of available log files (JSON)
- `GET /api/v1/logs/:filename` - Get content of a specific log file
  - Optional query parameter `format=json` to get logs as structured JSON

### Tampermonkey

- `GET /api/v1/tampermonkey/bridge.user.js` - Get the Tampermonkey bridge script

## License

This project is licensed under the MIT License - see the LICENSE file for details. 