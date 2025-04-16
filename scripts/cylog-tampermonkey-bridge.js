// ==UserScript==
// @name         Cylog-Tampermonkey Bridge
// @namespace    http://tampermonkey.net/
// @version      1.0
// @description  Bridge between Cylog desktop app and Cytube ChatStyleAdjuster Tampermonkey script
// @author       Cylog
// @match        http://localhost:8080/*
// @match        http://127.0.0.1:8080/*
// @grant        none
// ==/UserScript==

(function() {
    'use strict';
    
    // Configuration
    const config = {
        debug: true,
        isCylogApp: window.location.host.includes('localhost:8080') || window.location.host.includes('127.0.0.1:8080'),
        isCytubeOrigin: window.location.host.includes('cytube.') || window.location.host.includes('cytu.') || window.location.host.includes('om3tcw.com')
    };
    
    // Logging function
    function log(message, level = 'info') {
        if (!config.debug && level === 'debug') return;
        console[level](`[Cylog Bridge] ${message}`);
    }
    
    log('Bridge script loaded');
    
    // Track if we're running inside Cylog or Cytube
    if (config.isCylogApp) {
        log('Running in Cylog app - initializing bridge');
        initCylogBridge();
    } else if (config.isCytubeOrigin) {
        log('Running in Cytube - looking for ChatStyleAdjuster');
        initCytubeBridge();
    } else {
        log('Not running in either Cylog or Cytube - bridge inactive', 'warn');
    }
    
    // Initialize bridge in Cylog app
    function initCylogBridge() {
        // Expose necessary elements and functions for the ChatStyleAdjuster
        window.addEventListener('DOMContentLoaded', () => {
            log('DOM loaded in Cylog, setting up bridge elements');
            
            // Create a pollwrap element for compatibility
            if (!document.getElementById('pollwrap')) {
                const pollwrap = document.createElement('div');
                pollwrap.id = 'pollwrap';
                pollwrap.style.display = 'none';
                document.body.appendChild(pollwrap);
                log('Added pollwrap element for compatibility');
            }
            
            // Wait for Cylog to be ready
            document.addEventListener('cylog-ready', (event) => {
                log('Cylog app is ready, connecting to ChatStyleAdjuster');
                
                // Announce our presence to Tampermonkey scripts
                window.postMessage('tampermonkey-cytube-ready', '*');
                
                // Listen for style changes from ChatStyleAdjuster
                monitorTampermonkeyChanges();
            });
            
            // Listen for font size changes from Cylog and relay them to ChatStyleAdjuster
            document.addEventListener('cylog-font-size-change', (event) => {
                log(`Font size changed: ${event.detail.fontSize}px, action: ${event.detail.action}`);
                // This event is already dispatched by Cylog and will be picked up by ChatStyleAdjuster
            });
            
            // Listen for width changes from Cylog and relay them to ChatStyleAdjuster
            document.addEventListener('cylog-width-change', (event) => {
                log(`Chat width changed: ${event.detail.chatWidth}em, action: ${event.detail.action}`);
                // This event is already dispatched by Cylog and will be picked up by ChatStyleAdjuster
            });
        });
    }
    
    // Initialize bridge in Cytube
    function initCytubeBridge() {
        // Wait for ChatStyleAdjuster to be initialized
        const checkInterval = setInterval(() => {
            if (window.chatStyleAdjuster) {
                clearInterval(checkInterval);
                log('ChatStyleAdjuster found, connecting bridge');
                
                // Create a bridge to relay messages to Cylog
                createMessageBridge();
            }
        }, 1000);
        
        // Give up after 10 seconds if ChatStyleAdjuster isn't found
        setTimeout(() => {
            clearInterval(checkInterval);
            log('ChatStyleAdjuster not found after timeout', 'warn');
        }, 10000);
    }
    
    // Monitor for changes made by Tampermonkey script
    function monitorTampermonkeyChanges() {
        // Watch for changes to the chat area that might be made by Tampermonkey
        const chatwrap = document.getElementById('chatwrap');
        const observer = new MutationObserver((mutations) => {
            mutations.forEach(mutation => {
                if (mutation.type === 'attributes') {
                    log(`Attribute ${mutation.attributeName} changed on chatwrap`);
                    
                    // If width was changed externally, update our internal state
                    if (mutation.attributeName === 'style' && chatwrap.style.width) {
                        const width = chatwrap.style.width;
                        const match = width.match(/^([\d\.]+)(em|px)$/);
                        if (match) {
                            const value = parseFloat(match[1]);
                            const unit = match[2];
                            log(`Detected width change to ${value}${unit}`);
                            
                            // Update Cylog's internal state if available
                            if (window.cylogStyleManager) {
                                // Don't adjust width directly to avoid loops, just update the variable
                                if (unit === 'em') {
                                    window.chatWidth = value;
                                    log(`Updated Cylog chatWidth to ${value}em`);
                                }
                            }
                        }
                    }
                }
            });
        });
        
        // Start observing
        observer.observe(chatwrap, { 
            attributes: true,
            attributeFilter: ['style']
        });
        
        log('Now monitoring for Tampermonkey style changes');
    }
    
    // Create a bridge to relay messages from Cytube to Cylog
    function createMessageBridge() {
        // This would include code to relay messages from Cytube to Cylog
        // For example, via a WebSocket connection or localStorage
        
        log('Message bridge ready');
        
        // If you have a chat listener in ChatStyleAdjuster, hook into it
        if (window.chatListener) {
            const originalHandleMessage = window.chatListener.handleMessage;
            window.chatListener.handleMessage = function(msg) {
                // Call the original function
                originalHandleMessage.call(window.chatListener, msg);
                
                // Also send to Cylog if needed
                // This would depend on how you want to connect the two
                log('Message intercepted and relayed');
            };
            
            log('Chat listener hooked for message relay');
        }
    }
})(); 