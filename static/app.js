document.addEventListener('DOMContentLoaded', () => {
    const messagebuffer = document.getElementById('messagebuffer');
    const chatwrap = document.getElementById('chatwrap');
    
    // Message cache for deduplication
    const messageIds = new Set();
    
    // Default font and chat width settings
    let fontSize = 16;
    let chatWidth = 27;
    
    // Initialize the StyleManager - compatible with ChatStyleAdjuster
    initializeStyleManager(messagebuffer, chatwrap);
    
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
        
        // Dispatch a custom event that the Tampermonkey script can listen for
        dispatchMessageEvent(message);
    }
    
    // Dispatch a custom event when a message is added
    function dispatchMessageEvent(message) {
        const event = new CustomEvent('cylog-message', {
            detail: {
                message: message,
                html: message.html || createMessageHTML(message)
            }
        });
        document.dispatchEvent(event);
    }
    
    // Create HTML representation of a message (for Tampermonkey compatibility)
    function createMessageHTML(message) {
        return `<span class="timestamp">${new Date(message.timestamp).toLocaleTimeString()}</span> <span class="username">${message.username}</span>: <span class="content">${message.content}</span>`;
    }
    
    // Check if user is at the bottom of the chat
    function isAtBottom() {
        return messagebuffer.scrollHeight - messagebuffer.scrollTop <= messagebuffer.clientHeight + 10;
    }
    
    // Scroll to the bottom of the chat
    function scrollToBottom() {
        messagebuffer.scrollTop = messagebuffer.scrollHeight;
    }
    
    // Initialize the StyleManager to be compatible with ChatStyleAdjuster
    function initializeStyleManager(chatBuffer, chatWrap) {
        // Font size adjustment buttons
        document.getElementById('fontSizeIncrease').addEventListener('click', () => {
            adjustFontSize('increase');
        });
        
        document.getElementById('fontSizeDecrease').addEventListener('click', () => {
            adjustFontSize('decrease');
        });
        
        // Chat width adjustment buttons
        document.getElementById('chatWidthIncrease').addEventListener('click', () => {
            adjustChatWidth('increase');
        });
        
        document.getElementById('chatWidthDecrease').addEventListener('click', () => {
            adjustChatWidth('decrease');
        });
        
        // Keyboard shortcuts for compatibility with ChatStyleAdjuster
        document.addEventListener('keydown', (e) => {
            // Skip if focus is on an input element
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) {
                return;
            }
            
            let action = null;
            if (e.key === '=' || e.key === '+') {
                action = 'increase';
            } else if (e.key === '-' || e.key === '_') {
                action = 'decrease';
            }
            
            if (!action) return;
            
            if (e.shiftKey) {
                adjustFontSize(action);
            } else {
                adjustChatWidth(action);
            }
            e.preventDefault();
        });
        
        // Expose functions globally for Tampermonkey script access
        window.cylogStyleManager = {
            adjustFontSize,
            adjustChatWidth,
            isAtBottom,
            scrollToBottom
        };
    }
    
    // Adjust font size (compatible with ChatStyleAdjuster)
    function adjustFontSize(action) {
        const step = 1; // Pixel step
        
        fontSize = action === 'increase' ? 
            fontSize + step : 
            Math.max(8, fontSize - step);
        
        // Update timestamp and username size
        document.querySelectorAll('#messagebuffer span.timestamp, #messagebuffer span.username').forEach(el => {
            el.style.fontSize = (fontSize * 0.8) + 'px';
        });
        
        // Update content size
        document.querySelectorAll('#messagebuffer span:not(.timestamp):not(.username)').forEach(el => {
            el.style.fontSize = (fontSize * 1.2) + 'px';
        });
        
        // Dispatch custom event for Tampermonkey script
        document.dispatchEvent(new CustomEvent('cylog-font-size-change', { 
            detail: { fontSize, action } 
        }));
    }
    
    // Adjust chat width (compatible with ChatStyleAdjuster)
    function adjustChatWidth(action) {
        const step = 1; // em step
        
        chatWidth = action === 'increase' ? 
            chatWidth + step : 
            Math.max(10, chatWidth - step);
        
        chatwrap.style.width = chatWidth + 'em';
        
        // Dispatch custom event for Tampermonkey script
        document.dispatchEvent(new CustomEvent('cylog-width-change', { 
            detail: { chatWidth, action } 
        }));
    }
    
    // Fetch initial messages
    fetch('/api/messages')
        .then(response => response.json())
        .then(messages => {
            messages.forEach(message => addMessage(message));
            scrollToBottom();
        })
        .catch(error => console.error('Error fetching messages:', error));
    
    // Listen for Tampermonkey script to announce its presence
    window.addEventListener('message', (event) => {
        if (event.data === 'tampermonkey-cytube-ready') {
            document.dispatchEvent(new CustomEvent('cylog-ready', {
                detail: {
                    chatwrapId: 'chatwrap',
                    messagebufferId: 'messagebuffer'
                }
            }));
        }
    });
    
    // Announce that Cylog is ready
    window.parent.postMessage('cylog-ready', '*');
});
