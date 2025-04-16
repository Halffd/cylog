// ==UserScript==
// @name         Cytube Chat Style Adjuster for Cylog
// @namespace    http://tampermonkey.net/
// @version      1.3
// @description  Apply default chat styles and allow hotkeys to adjust text size and chat width. Compatible with Cylog desktop app.
// @author       ...
// @match        https://om3tcw.com/*
// @match        https://cytube.*/*
// @match        https://cytu.*/*
// @match        http://localhost:8080/*
// @match        http://127.0.0.1:8080/*
// @grant        none
// ==/UserScript==

class ChatStyleAdjuster {
    constructor() {
        this.loggingLevel = 'info'; // Set logging level: 'debug', 'info', 'warn', 'error'
        this.log("Initializing ChatStyleAdjuster...", 'info');
        
        // Detect if we're running in Cylog
        this.isCylog = window.location.host.includes('localhost:8080') || window.location.host.includes('127.0.0.1:8080');
        this.log(`Running in Cylog environment: ${this.isCylog}`, 'info');
        
        // Expose instance to window for bridge script
        window.chatStyleAdjuster = this;
        
        this.init();
    }

    log(message, level = 'info') {
        const levels = ['debug', 'info', 'warn', 'error'];
        if (levels.indexOf(level) >= levels.indexOf(this.loggingLevel)) {
            console[level](`[ChatStyleAdjuster] ${message}`);
        }
    }

    async waitForElement(selector) {
        this.log(`Waiting for element: ${selector}`, 'info');
        const timeout = 60000; // 60 seconds
        const interval = 100; // 100 ms
        let elapsedTime = 0;

        return new Promise((resolve, reject) => {
            const checkExist = setInterval(() => {
                const element = document.querySelector(selector);
                if (element) {
                    clearInterval(checkExist);
                    this.log(`Element found: ${selector}`, 'info');
                    resolve(element);
                } else if (elapsedTime >= timeout) {
                    clearInterval(checkExist);
                    this.log(`Element ${selector} not found within timeout`, 'error');
                    reject(new Error(`Element ${selector} not found within timeout`));
                }
                elapsedTime += interval;
            }, interval);
        });
    }

    async init() {
        this.log("Initializing chat components...", 'info');
        
        // In Cylog, wait for the cylog-ready event
        if (this.isCylog) {
            this.log("In Cylog environment, waiting for cylog-ready event", 'info');
            
            // Listen for cylog-ready event
            document.addEventListener('cylog-ready', (event) => {
                this.log("Cylog is ready, initializing with Cylog elements", 'info');
                this.initWithCylog(event.detail);
            });
            
            // Announce presence to Cylog
            window.postMessage('tampermonkey-cytube-ready', '*');
        } else {
            // Standard Cytube initialization
            this.chatWrap = await this.waitForElement("#chatwrap");
            this.chatBuffer = await this.waitForElement("#messagebuffer");
            this.chatPreview = this.createChatPreview();
            this.applyDefaultStyles();
            this.addEventListeners();
            const motdElement = await this.waitForElement("#pollwrap");
            motdElement.appendChild(this.chatPreview);
            this.startPreviewUpdate();
        }
        
        this.log("Chat components initialized successfully.", 'info');
    }
    
    // Initialize with Cylog elements
    async initWithCylog(detail) {
        // Use the element IDs provided by Cylog
        this.chatWrap = document.getElementById(detail.chatwrapId || 'chatwrap');
        this.chatBuffer = document.getElementById(detail.messagebufferId || 'messagebuffer');
        
        // Try to create a chat preview if we have a pollwrap
        try {
            const pollwrap = document.getElementById('pollwrap');
            if (pollwrap) {
                this.chatPreview = this.createChatPreview();
                pollwrap.appendChild(this.chatPreview);
                this.startPreviewUpdate();
            } else {
                this.log("No pollwrap element found for chat preview", 'warn');
            }
        } catch (error) {
            this.log(`Error creating chat preview: ${error.message}`, 'error');
        }
        
        // Apply styles and listeners
        this.applyDefaultStyles();
        this.addEventListeners();
        
        // Listen for Cylog-specific events
        this.addCylogEventListeners();
        
        this.log("Initialized with Cylog elements", 'info');
    }

    applyDefaultStyles() {
        this.log("Applying default styles...", 'info');
        const style = document.createElement("style");
        style.textContent = `
            /* Default chat panel width */
            #chatwrap {
                width: 27em;
            }

            /* Smaller font for timestamps/usernames */
            #messagebuffer span.timestamp,
            #messagebuffer span.username {
                font-size: 2.2em;
            }

            /* Larger font for actual message text (spans without timestamp/username classes) */
            #messagebuffer span:not(.timestamp):not(.username) {
                font-size: 4.8em;
            }
        `;
        document.head.appendChild(style);
        this.log("Default styles applied.", 'info');
    }

    isEditableElement(el) {
        return el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable);
    }

    addEventListeners() {
        this.log("Adding event listeners...", 'info');
        document.addEventListener("keydown", (e) => this.handleKeyDown(e));
        if (this.chatBuffer) {
            this.chatBuffer.addEventListener("scroll", () => this.checkScroll());
        }
        this.log("Event listeners added.", 'info');
    }
    
    // Add Cylog-specific event listeners
    addCylogEventListeners() {
        this.log("Adding Cylog-specific event listeners...", 'info');
        
        // Listen for font size changes from Cylog
        document.addEventListener('cylog-font-size-change', (event) => {
            this.log(`Received font size change from Cylog: ${event.detail.fontSize}px`, 'info');
            // We don't need to do anything, as Cylog already handles the style changes
        });
        
        // Listen for chat width changes from Cylog
        document.addEventListener('cylog-width-change', (event) => {
            this.log(`Received chat width change from Cylog: ${event.detail.chatWidth}em`, 'info');
            // We don't need to do anything, as Cylog already handles the style changes
        });
        
        // Listen for messages from Cylog
        document.addEventListener('cylog-message', (event) => {
            this.log(`Received message from Cylog`, 'debug');
            // Update the preview if we have one
            if (this.chatPreview) {
                this.updatePreviewWithMessage(event.detail.html);
            }
        });
        
        this.log("Cylog-specific event listeners added.", 'info');
    }
    
    // Update the preview with a new message
    updatePreviewWithMessage(html) {
        if (!this.chatPreview) return;
        
        const tempDiv = document.createElement('div');
        tempDiv.innerHTML = html;
        tempDiv.classList.add('message');
        
        this.chatPreview.appendChild(tempDiv);
        
        // Limit the number of messages (keep last 5)
        const messages = this.chatPreview.querySelectorAll('.message');
        if (messages.length > 5) {
            this.chatPreview.removeChild(messages[0]);
        }
        
        this.log("Preview updated with new message", 'debug');
    }

    handleKeyDown(e) {
        if (this.isEditableElement(e.target)) return;

        let action = null;
        if (e.key === "=" || e.key === "+") {
            action = "increase";
        } else if (e.key === "-" || e.key === "_") {
            action = "decrease";
        }
        if (!action) return;

        this.log(`Key pressed: ${e.key}, Action: ${action}`, 'debug');
        
        if (this.isCylog) {
            // In Cylog environment, dispatch events for Cylog to handle
            if (e.shiftKey) {
                // Font size change
                document.dispatchEvent(new CustomEvent('tampermonkey-font-size-change', {
                    detail: { action }
                }));
            } else {
                // Chat width change
                document.dispatchEvent(new CustomEvent('tampermonkey-chat-width-change', {
                    detail: { action }
                }));
            }
        } else {
            // In Cytube, handle normally
            if (e.shiftKey) {
                this.adjustFontSize(action);
            } else {
                this.adjustChatWidth(action);
            }
        }
        
        e.preventDefault();
    }

    adjustFontSize(action) {
        this.log(`Adjusting font size: ${action}`, 'info');
        const spans = document.querySelectorAll("#messagebuffer span:not(.timestamp):not(.username)");
        if (this.chatPreview) {
            spans.push(this.chatPreview);
        }
        if (!spans.length) return;

        const computedStyle = window.getComputedStyle(spans[0]);
        let currentSize = parseFloat(computedStyle.fontSize) || 16;
        const step = 1;

        currentSize = action === "increase" ? currentSize + step : Math.max(step, currentSize - step);
        spans.forEach((span) => {
            span.style.fontSize = currentSize + "px";
        });
        this.log(`Font size adjusted to: ${currentSize}px`, 'info');
    }

    adjustChatWidth(action) {
        this.log(`Adjusting chat width: ${action}`, 'info');
        if (!this.chatWrap) return;

        const computedStyle = window.getComputedStyle(this.chatWrap);
        let currentWidth = computedStyle.width;
        const emMatch = currentWidth.match(/^([\d\.]+)em$/);
        const pxMatch = currentWidth.match(/^([\d\.]+)px$/);
        let value;

        if (emMatch) {
            value = parseFloat(emMatch[1]);
            const step = 1;
            value = action === "increase" ? value + step : Math.max(1, value - step);
            this.chatWrap.style.width = value + "em";
        } else if (pxMatch) {
            value = parseFloat(pxMatch[1]);
            const step = 10;
            value = action === "increase" ? value + step : Math.max(step, value - step);
            this.chatWrap.style.width = value + "px";
        } else {
            value = parseFloat(currentWidth) || 0;
            const step = 10;
            value = action === "increase" ? value + step : Math.max(step, value - step);
            this.chatWrap.style.width = value + "px";
        }
        this.log(`Chat width adjusted to: ${this.chatWrap.style.width}`, 'info');
    }

    checkScroll() {
        if (!this.chatBuffer) return;
        this.isAtBottom = this.chatBuffer.scrollHeight - this.chatBuffer.scrollTop <= this.chatBuffer.clientHeight + 10;
        this.log(`Scroll checked: isAtBottom = ${this.isAtBottom}`, 'debug');
    }

    createChatPreview() {
        this.log("Creating chat preview...", 'info');
        const preview = document.createElement("div");
        preview.id = "chatPreview";
        preview.style.position = "relative";
        preview.style.display = "flex";
        preview.style.flexDirection = "column";
        preview.style.alignItems = "flex-start"; // Changed to left align
        preview.style.justifyContent = "center";
        preview.style.top = "0";
        preview.style.left = "0";
        preview.style.width = "100%";
        //preview.style.maxHeight = "800px"; // Set max height
        preview.style.backgroundColor = "rgba(0, 0, 0, 0.5)";
        preview.style.zIndex = "1000";
        preview.style.overflowY = "auto"; // Enable scrollbar
        preview.style.color = "white";
        preview.style.margin = "0";
        preview.style.padding = "0";
        preview.style.border = "2px solid rgba(235, 155, 125, 0.5)";
        preview.style.outline = "none";
        preview.style.boxShadow = "none";
        preview.style.backgroundImage = "none";
        preview.style.fontSize = "22px";
        this.log("Chat preview created.", 'info');
        return preview;
    }
    
    startPreviewUpdate() {
        if (!this.chatPreview || !this.chatBuffer) return;
        
        this.log("Starting preview update...", 'info');
        setInterval(() => {
            // Determine if we should stick to the bottom
            const shouldStickToBottom = this.chatPreview.scrollHeight - this.chatPreview.scrollTop <= this.chatPreview.clientHeight + 10;

            // Update content
            this.chatPreview.innerHTML = this.chatBuffer.innerHTML;

            // If we were at the bottom before, scroll to bottom again
            if (shouldStickToBottom) {
//                this.chatPreview.scrollTop = this.chatPreview.scrollHeight;
            }
        }, 100);
        this.log("Preview update started.", 'info');
    }

    addMessageToChatAndPreview(msg) {
        this.log(`Adding message to chat and preview: ${msg}`, 'info');
        if (this.chatBuffer) {
            this.chatBuffer.innerHTML += `<div class="message">${msg}</div>`;
        }
        if (this.chatPreview) {
            this.chatPreview.innerHTML += `<div>${msg}</div>`;
            const messages = this.chatPreview.querySelectorAll("div");
            if (messages.length > 5) {
                this.chatPreview.removeChild(messages[0]);
                this.log("Removed oldest message from chat preview.", 'info');
            }
        }
    }
}

/*
 * ChatListener
 * Listens for messages from the server and sends them to the socket
 * @param {ChatStyleAdjuster} chatStyleAdjuster - The chat style adjuster instance
*/
class ChatListener {
    constructor(chatStyleAdjuster) {
        this.loggingLevel = 'info';
        this.log("Initializing ChatListener...", 'info');
        this.chatStyleAdjuster = chatStyleAdjuster;
        this.socket = null; // Placeholder for socket connection
        this.useSocket = false; // Set to true to enable socket usage
        
        // Detect if we're running in Cylog
        this.isCylog = window.location.host.includes('localhost:8080') || window.location.host.includes('127.0.0.1:8080');
        
        // Expose to window for the bridge script
        window.chatListener = this;
        
        this.init();
        this.messageBuffer = []; // Buffer to hold messages
        
        // Only start the interval in Cytube, not in Cylog
        if (!this.isCylog) {
            this.updateInterval = setInterval(() => this.refreshMessages(), 10); // Update every 10ms
        }
        
        this.log("ChatListener initialized.", 'info');
    }

    log(message, level = 'info') {
        const levels = ['debug', 'info', 'warn', 'error'];
        if (levels.indexOf(level) >= levels.indexOf(this.loggingLevel)) {
            console[level](`[ChatListener] ${message}`);
        }
    }

    init() {
        this.log("Initializing ChatListener...", 'info');
        
        // Only connect to socket in Cytube, not in Cylog
        if (!this.isCylog && this.useSocket) {
            this.connectSocket();
        }
        
        // In Cylog, listen for messages via custom events
        if (this.isCylog) {
            document.addEventListener('cylog-message', (event) => {
                this.handleCylogMessage(event.detail);
            });
        }
    }

    connectSocket() {
        if (this.useSocket) {
            this.socket = new WebSocket("ws://cytube.net/ws");
            this.socket.onmessage = (event) => this.handleMessage(event.data);
            this.log("Socket connected.", 'info');
        } else {
            this.log("Socket usage is disabled.", 'warn');
        }
    }

    handleMessage(msg) {
        this.log(`Message received: ${msg}`, 'debug');
        // Get chatBuffer HTML and send to socket and chatPreview
        if (this.chatStyleAdjuster.chatBuffer) {
            const chatBufferHTML = this.chatStyleAdjuster.chatBuffer.innerHTML;
            if (this.socket && this.socket.readyState === WebSocket.OPEN) {
                this.socket.send(chatBufferHTML); // Send message to socket
                this.log("Message sent to socket.", 'info');
            }
            if (this.chatStyleAdjuster.chatPreview) {
                this.chatStyleAdjuster.chatPreview.innerHTML += `<div>${chatBufferHTML}</div>`; // Send HTML to chatPreview
            }
        }
    }
    
    // Handle messages from Cylog
    handleCylogMessage(detail) {
        this.log(`Message received from Cylog`, 'debug');
        
        // Add to buffer
        if (detail.html) {
            this.messageBuffer.push(detail.html);
        }
    }

    refreshMessages() {
        if (this.messageBuffer.length > 0) {
            const msg = this.messageBuffer.shift(); // Get the oldest message
            this.chatStyleAdjuster.addMessageToChatAndPreview(msg); // Update chat and preview
            this.log(`Message refreshed: ${msg}`, 'info');
        }
    }
}

(function () {
    "use strict";
    try {
        const chatStyleAdjuster = new ChatStyleAdjuster();
        const chatListener = new ChatListener(chatStyleAdjuster);
    } catch (error) {
        console.error("Error initializing chat style adjuster:", error);
        try {
            console.trace("Error trace:", error.stack, error.message, error.name, error.constructor, error.cause);
        } catch (error) {
            console.error("Error tracing error:", error);
        }
    }
})(); 