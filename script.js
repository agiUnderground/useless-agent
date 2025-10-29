// Global variables
let popOutWSWindow = null;
// Task Management
let currentTask = null;
let isFirstMessage = true;

// Network data tracking - per session tracking
let totalNetworkBytes = 0;
let networkMutex = false;
let sessionNetworkData = new Map(); // Map to track network data per session
let lastUpdateTime = Date.now();

// Function to format network data usage
function formatNetworkBytes(bytes) {
    if (bytes < 1024) {
        return `${bytes} B`;
    } else if (bytes < 1024 * 1024) {
        return `${(bytes / 1024).toFixed(2)} KB`;
    } else if (bytes < 1024 * 1024 * 1024) {
        return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
    } else {
        return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
    }
}

// Function to track network data for a specific session
function trackSessionNetworkData(sessionId, bytes) {
    if (networkMutex) return;
    networkMutex = true;
    
    // Initialize session data if it doesn't exist
    if (!sessionNetworkData.has(sessionId)) {
        sessionNetworkData.set(sessionId, {
            totalBytes: 0,
            lastUpdate: Date.now()
        });
    }
    
    const sessionData = sessionNetworkData.get(sessionId);
    sessionData.totalBytes += bytes;
    sessionNetworkData.set(sessionId, sessionData);
    
    // Update total network bytes
    totalNetworkBytes += bytes;
    
    // Update display immediately for real-time feedback
    updateNetworkCounter();
    
    networkMutex = false;
}

// Function to update network counter display
function updateNetworkCounter() {
    const networkCounter = document.getElementById('networkCounter');
    if (networkCounter) {
        networkCounter.textContent = formatNetworkBytes(totalNetworkBytes);
    }
}

// Function to reset network counter
function resetNetworkCounter() {
    totalNetworkBytes = 0;
    sessionNetworkData.clear();
    lastUpdateTime = Date.now();
    const networkCounter = document.getElementById('networkCounter');
    if (networkCounter) {
        networkCounter.textContent = '0 B';
    }
}

// Function to get network usage for a specific session
function getSessionNetworkUsage(sessionId) {
    if (sessionNetworkData.has(sessionId)) {
        return sessionNetworkData.get(sessionId).totalBytes;
    }
    return 0;
}

// Function to get total network usage across all sessions
function getTotalNetworkUsage() {
    return totalNetworkBytes;
}

// Toast notification functionality
function showToast(message, type = 'error', duration = 4000) {
  const toastContainer = document.getElementById('toastContainer');
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  
  const toastContent = document.createElement('div');
  toastContent.className = 'toast-content';
  toastContent.textContent = message;
  
  const closeButton = document.createElement('button');
  closeButton.className = 'toast-close';
  closeButton.innerHTML = '×';
  closeButton.addEventListener('click', () => {
    removeToast(toast);
  });
  
  toast.appendChild(toastContent);
  toast.appendChild(closeButton);
  toastContainer.appendChild(toast);
  
  // Auto-remove after duration unless hovered
  let timeoutId = setTimeout(() => {
    removeToast(toast);
  }, duration);
  
  // Reset timer on hover
  toast.addEventListener('mouseenter', () => {
    clearTimeout(timeoutId);
  });
  
  // Restart timer when mouse leaves
  toast.addEventListener('mouseleave', () => {
    timeoutId = setTimeout(() => {
      removeToast(toast);
    }, duration);
  });
}

function removeToast(toast) {
  toast.classList.add('fade-out');
  setTimeout(() => {
    if (toast.parentNode) {
      toast.parentNode.removeChild(toast);
    }
  }, 300);
}

// Remove old screenshot elements since we're using dynamic sessions
const oldScreenshotImg = document.getElementById('screenshotImg');
const oldScreenshotPlaceholder = document.getElementById('screenshotPlaceholder');
if (oldScreenshotImg && oldScreenshotPlaceholder) {
  oldScreenshotImg.remove();
  oldScreenshotPlaceholder.remove();
}

// Toggle debug options functionality
let debugOptionsVisible = false;
function toggleDebugOptions() {
  const mediaButtonsGroup = document.getElementById('mediaButtonsGroup');
  debugOptionsVisible = !debugOptionsVisible;
  
  if (debugOptionsVisible) {
    mediaButtonsGroup.classList.remove('hidden');
  } else {
    mediaButtonsGroup.classList.add('hidden');
  }
}

function formatIp(ip) {
  if (ip.includes(':')) {
    return `[${ip}]`;
  } else {
    return `${ip}`;
  }
}

// Multi-session management
const sessions = new Map();
const mainContent = document.getElementById('mainContent');
let currentFPS = 5; // Default FPS
let selectedSessionId = null;

// Session-specific settings state management
const sessionTabStates = new Map(); // Stores last used tab per session
const defaultTab = 'execution'; // Default tab when no session-specific state exists

// Session-specific state management - COMPLETELY REWRITTEN FOR LOG ISOLATION
const sessionStates = new Map(); // Complete state per session including logs, settings, etc.

// Initialize session state
function initializeSessionState(sessionId) {
  if (!sessionStates.has(sessionId)) {
    sessionStates.set(sessionId, {
      logs: [], // Session-specific log messages
      isStreaming: false, // Session-specific streaming state
      lastUsedTab: defaultTab, // Session-specific last used tab
      settings: {}, // Session-specific settings data
      lastDisplayedLogIndex: 0, // Track position where logs were last displayed
      lastReadLogCount: 0 // Track how many logs have been read for this session
    });
  }
  return sessionStates.get(sessionId);
}

// Get session state
function getSessionState(sessionId) {
  return sessionStates.get(sessionId) || initializeSessionState(sessionId);
}

// Update session state
function updateSessionState(sessionId, updates) {
  const state = getSessionState(sessionId);
  Object.assign(state, updates);
  sessionStates.set(sessionId, state);
}

// Function to get the last used tab for a session
function getLastUsedTab(sessionId) {
  const state = getSessionState(sessionId);
  return state.lastUsedTab || defaultTab;
}

// Function to set the last used tab for a session
function setLastUsedTab(sessionId, tabName) {
  updateSessionState(sessionId, { lastUsedTab: tabName });
}

// Function to update settings sidebar when session changes
function updateSettingsSidebarForNewSession(sessionId) {
  if (!settingsOpen) return;
  
  // Get the last used tab for the new session
  const targetTab = getLastUsedTab(sessionId);
  console.log(`Session switched to ${sessionId}, switching to tab: ${targetTab}`);
  
  // Find the tab button for the target tab
  const targetTabButton = document.querySelector(`[data-tab="${targetTab}"]`);
  if (targetTabButton) {
    // Remove active class from all tabs and contents
    document.querySelectorAll('.settings-tab').forEach(btn => btn.classList.remove('active'));
    document.querySelectorAll('.settings-tab-content').forEach(content => content.classList.remove('active'));
    
    // Add active class to target tab and corresponding content
    targetTabButton.classList.add('active');
    const targetContent = document.getElementById(`${targetTab}-tab`);
    if (targetContent) {
      targetContent.classList.add('active');
    }
    
    // CRITICAL FIX: Force complete logs tab reset when switching to logs
    if (targetTab === 'logs') {
      const logsTab = document.getElementById('logs-tab');
      if (logsTab) {
        // Clear all content immediately to prevent showing old session logs
        logsTab.innerHTML = '';
        // Force immediate DOM update
        logsTab.offsetHeight; // Force reflow
        
        // CRITICAL FIX: Force complete refresh by resetting display session flag
        // This ensures logs are completely isolated between sessions
        logsTab.dataset.currentDisplayingSession = null;
      }
    }
    
    // Initialize the tab content for the new session
    initializeTabContent(targetTab);
  }
}

// Function to initialize tab content based on tab type
function initializeTabContent(tabName) {
  if (tabName === 'logs') {
    // Initialize logs tab for current session
    const currentSessionId = selectedSessionId;
    if (currentSessionId) {
      // Always clear logs display first to prevent cross-contamination
      const logsPre = document.querySelector('.logs-content');
      if (logsPre) {
        logsPre.textContent = ''; // Clear immediately
      }
      
      // CRITICAL FIX: Always update display to show current session's logs
      // This ensures logs are displayed even when switching to logs tab for active session
      updateLogsDisplayFromState();
      
      // Check if we need to start streaming for this session using new session state system
      const sessionState = getSessionState(currentSessionId);
      if (!sessionState.isStreaming) {
        // CRITICAL FIX: Start streaming for current session if not already streaming
        // This ensures logs start printing immediately for active sessions
        console.log(`Starting log streaming for session ${currentSessionId} when switching to logs tab`);
        startLogStreaming(); // This will set isStreaming to true and update display
      }
    }
  } else if (tabName === 'notifications') {
    // Initialize notifications tab
    console.log('Notifications tab initialized');
    // No special initialization needed for notifications tab at this time
  }
}

// Function to select a session and apply wave animation
function selectSession(sessionId) {
  console.log(`Attempting to select session: ${sessionId}`);
  
  // CRITICAL FIX: Clean logs tab immediately when switching sessions
  // This ensures no cross-contamination between sessions
  const logsTab = document.getElementById('logs-tab');
  if (logsTab) {
    // Clear logs tab completely
    logsTab.innerHTML = '';
    logsTab.dataset.currentDisplayingSession = null;
  }
  
  // CRITICAL FIX: Stop ALL UI activity for previous session when switching
  if (selectedSessionId && selectedSessionId !== sessionId) {
    const previousSessionState = getSessionState(selectedSessionId);
    
    // Stop log streaming for previous session
    if (previousSessionState.isStreaming) {
      console.log(`Stopping log streaming for previous session: ${selectedSessionId}`);
      updateSessionState(selectedSessionId, { isStreaming: false });
    }
    
    // CRITICAL FIX: Reset log read count for previous session to ensure clean state
    // This saves the current position and allows fast switching
    updateSessionState(selectedSessionId, {
      lastReadLogCount: previousSessionState.logs ? previousSessionState.logs.length : 0
    });
    
    // CRITICAL FIX: Clear the logs display immediately to prevent any lingering content
    const logsPre = document.querySelector('.logs-content');
    if (logsPre) {
      logsPre.textContent = '';
    }
  }
  
  // Remove selected class from all sessions
  document.querySelectorAll('.session-container').forEach(container => {
    container.classList.remove('selected', 'user-assist-selected');
  });
  
  // Clear any existing server offline error when switching to another session
  clearServerOfflineError();
  
  // Add selected class to the chosen session
  const session = sessions.get(sessionId);
  if (session && session.container) {
    console.log(`Found session, adding selected class to container`);
    
    // Apply appropriate styling based on user-assist mode
    if (userAssistActive && userAssistTaskCard) {
      const taskSessionId = userAssistTaskCard.dataset.sessionId;
      if (taskSessionId === sessionId) {
        // This is the user-assist session - apply golden yellow/orange styling
        session.container.classList.add('user-assist-selected');
        console.log(`Applied user-assist styling to session: ${sessionId}`);
      } else {
        // User manually selected a different session while user-assist is active
        console.log('User manually selected different session, deactivating user-assist');
        // Deactivate user-assist without restoring green outline to old session
        deactivateUserAssist(false);
        session.container.classList.add('selected');
      }
    } else {
      // Normal selection - apply green styling
      session.container.classList.add('selected');
    }
    
    selectedSessionId = sessionId;
    console.log(`Selected session: ${sessionId}`);
    
    // Update the selected session IP display
    updateSelectedSessionIp(sessionId);
    
    // CRITICAL FIX: Force complete log refresh when switching sessions
    // This ensures complete isolation between sessions
    setTimeout(() => {
      if (logsTab && logsTab.classList.contains('active')) {
        // Force refresh by clearing the displaying session flag
        logsTab.dataset.currentDisplayingSession = null;
        updateLogsDisplayFromState();
      }
    }, 10); // Small delay to ensure DOM is updated
    
    // Update settings sidebar if it's open to reflect new session
    if (settingsOpen) {
      updateSettingsSidebarForNewSession(sessionId);
    }
  } else {
    console.log(`Session not found or container missing for sessionId: ${sessionId}`);
    console.log(`Available sessions:`, Array.from(sessions.keys()));
  }
}

// Function to update the selected session IP display
function updateSelectedSessionIp(sessionId) {
  const selectedSessionIpElement = document.getElementById('selectedSessionIp');
  const selectedIpTextElement = document.getElementById('selectedIpText');
  
  if (selectedSessionIpElement && selectedIpTextElement) {
    if (sessionId) {
      // Show the selected session IP
      selectedIpTextElement.textContent = sessionId;
      selectedSessionIpElement.style.display = 'block';
    } else {
      // Hide the IP display if no session is selected
      selectedSessionIpElement.style.display = 'none';
    }
  }
}

// Function to get the currently selected session
function getSelectedSession() {
  return selectedSessionId ? sessions.get(selectedSessionId) : null;
}

// Create a new session
function createSession(ipAddress) {
  const sessionId = ipAddress;
  
  if (sessions.has(sessionId)) {
    console.log(`Session ${sessionId} already exists`);
    return sessions.get(sessionId);
  }

  // Create session container
  const sessionContainer = document.createElement('div');
  sessionContainer.className = 'session-container';
  sessionContainer.dataset.sessionId = sessionId;

  // Create session header
  const sessionHeader = document.createElement('div');
  sessionHeader.className = 'session-header';
  
  // Create status indicator
  const statusIndicator = document.createElement('span');
  statusIndicator.className = 'connection-status';
  
  const sessionTitle = document.createElement('span');
  sessionTitle.textContent = `Session: ${sessionId}`;
  
  const closeButton = document.createElement('button');
  closeButton.className = 'session-close';
  closeButton.innerHTML = '×';
  closeButton.title = 'Close session';
  closeButton.addEventListener('click', () => {
    closeSession(sessionId);
  });

  sessionHeader.appendChild(statusIndicator);
  sessionHeader.appendChild(sessionTitle);
  sessionHeader.appendChild(closeButton);

  // Create session content
  const sessionContent = document.createElement('div');
  sessionContent.className = 'session-content';

  // Create screenshot placeholder
  const placeholder = document.createElement('div');
  placeholder.className = 'screenshot-placeholder';
  placeholder.textContent = 'Video stream will appear here';

  // Create screenshot image
  const screenshotImg = document.createElement('img');
  screenshotImg.className = 'screenshot-img hidden';
  screenshotImg.alt = `Video stream from ${sessionId}`;

  // Create screenshot container
  const screenshotContainer = document.createElement('div');
  screenshotContainer.className = 'screenshot-container';

  // Create image size container and overlay
  const imageSizeContainer = document.createElement('div');
  imageSizeContainer.className = 'image-size-container';

  const screenshotOverlay = document.createElement('div');
  screenshotOverlay.className = 'screenshot-overlay';

  // Create maximize button (different icon from fullscreen)
  const maximizeBtn = document.createElement('button');
  maximizeBtn.className = 'fullscreen-button';
  maximizeBtn.title = 'Maximize session (M)';
  maximizeBtn.innerHTML = `
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M8 3v3a2 2 0 0 1-2 2H3m18 0h-3a2 2 0 0 1-2-2V3m0 18v-3a2 2 0 0 1 2-2h3M3 16h3a2 2 0 0 1 2 2v3"></path>
    </svg>
  `;

  const fullscreenBtn = document.createElement('button');
  fullscreenBtn.className = 'fullscreen-button';
  fullscreenBtn.title = 'Toggle fullscreen (F)';
  fullscreenBtn.innerHTML = `
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"></path>
    </svg>
  `;

  screenshotOverlay.appendChild(maximizeBtn);
  screenshotOverlay.appendChild(fullscreenBtn);
  imageSizeContainer.appendChild(screenshotOverlay);
  screenshotContainer.appendChild(screenshotImg);
  screenshotContainer.appendChild(imageSizeContainer);
  
  sessionContent.appendChild(placeholder);
  sessionContent.appendChild(screenshotContainer);
  
  sessionContainer.appendChild(sessionHeader);
  sessionContainer.appendChild(sessionContent);

  // Add to main content
  mainContent.appendChild(sessionContainer);

  // Add click event listener to select session
  sessionContainer.addEventListener('click', () => {
    selectSession(sessionId);
  });

  // Create session object
  const session = {
    id: sessionId,
    ip: ipAddress,
    ws: null,
    container: sessionContainer,
    content: sessionContent,
    img: screenshotImg,
    placeholder: placeholder,
    screenshotContainer: screenshotContainer,
    fullscreenBtn: fullscreenBtn,
    videoInterval: null,
    isConnected: false
  };
  
  sessions.set(sessionId, session);
  updateSessionLayout();

  // Automatically select the newly created session
  selectSession(sessionId);

  // Add fullscreen functionality for this session
  setupSessionFullscreen(session);

  return session;
}

// Close a session
function closeSession(sessionId) {
  const session = sessions.get(sessionId);
  if (!session) return;

  // Check if the session being closed is maximized
  const isMaximized = session.container.classList.contains('maximized');
  
  // Check if the session being closed is the currently selected one
  const isSelectedSession = selectedSessionId === sessionId;
  
  // Disconnect WebSocket if connected
  if (session.ws) {
    session.ws.close();
  }

  // Stop video stream
  if (session.videoInterval) {
    clearInterval(session.videoInterval);
  }

  // Remove from DOM
  if (session.container && session.container.parentNode) {
    session.container.parentNode.removeChild(session.container);
  }

  // Clear any existing ping interval
  if (session.pingInterval) {
    clearInterval(session.pingInterval);
    session.pingInterval = null;
  }
  
  // Remove from sessions map
  sessions.delete(sessionId);
  
  // If the closed session was maximized, restore other sessions
  if (isMaximized) {
    sessions.forEach((otherSession, otherSessionId) => {
      otherSession.container.style.display = 'flex';
    });
  }
  
  // If the closed session was the selected one, select another session if available
  if (isSelectedSession && sessions.size > 0) {
    // Use setTimeout to ensure DOM updates are complete before selecting new session
    setTimeout(() => {
      // Get the first available session
      const firstSessionId = Array.from(sessions.keys())[0];
      // Ensure the session still exists before selecting it
      const newSession = sessions.get(firstSessionId);
      if (newSession && newSession.container) {
        // Clear selection first to ensure proper visual update
        selectedSessionId = null;
        // Then select the new session
        selectSession(firstSessionId);
      }
    }, 0);
  } else if (sessions.size === 0) {
    // No sessions left, clear selection
    selectedSessionId = null;
    clearConnectionLine();
    // Hide IP display when no sessions are left
    updateSelectedSessionIp(null);
  }
  
  updateSessionLayout();
}

// Update layout based on session count
function updateSessionLayout() {
  const sessionCount = sessions.size;
  mainContent.setAttribute('data-session-count', sessionCount);
}

// Update connection status for a session
function updateSessionConnectionStatus(session, status) {
  // Get the status indicator from the session header
  const statusIndicator = session.container.querySelector('.session-header .connection-status');
  
  if (statusIndicator) {
    statusIndicator.className = 'connection-status';
    
    switch(status) {
      case 'connected':
        statusIndicator.classList.add('connected');
        session.isConnected = true;
        break;
      case 'connecting':
        statusIndicator.classList.add('connecting');
        session.isConnected = false;
        break;
      case 'disconnected':
        statusIndicator.classList.add('disconnected');
        session.isConnected = false;
        // When disconnected, reset the screenshot container and show placeholder
        resetScreenshotContainer(session);
        break;
    }
  }
}

// Reset screenshot container when connection is lost
function resetScreenshotContainer(session) {
  // Hide image and show placeholder
  session.img.classList.add('hidden');
  session.placeholder.classList.remove('hidden');
  
  // Reset image source to prevent memory leaks
  if (session.img.src) {
    URL.revokeObjectURL(session.img.src);
    session.img.src = '';
  }
  
  // Reset image size container to ensure placeholder is centered
  const imageSizeContainer = session.content.querySelector('.image-size-container');
  if (imageSizeContainer) {
    imageSizeContainer.style.width = '';
    imageSizeContainer.style.height = '';
    imageSizeContainer.style.position = '';
    imageSizeContainer.style.top = '';
    imageSizeContainer.style.left = '';
    imageSizeContainer.style.transform = '';
  }
  
  // Reset screenshot container to ensure proper centering
  session.screenshotContainer.style.width = '';
  session.screenshotContainer.style.height = '';
}

// Setup WebSocket for a session
function setupSessionWebSocket(session) {
  // Clear any existing ping interval before creating new WebSocket
  if (session.pingInterval) {
    clearInterval(session.pingInterval);
    session.pingInterval = null;
  }
  
  if (session.ws) session.ws.close();
  
  updateSessionConnectionStatus(session, 'connecting');
  
  session.ws = new WebSocket(`ws://${session.ip}:8080/ws`);

  session.ws.onopen = () => {
    console.log(`Connected to server ${session.ip}`);
    updateSessionConnectionStatus(session, 'connected');
    
    // Clear any server offline error states
    clearServerOfflineError();
    
    // Reset session state after successful reconnection
    resetSessionState(session);
    
    // Automatically start video when connection is established
    startSessionVideo(session, 'screenshot');
  };
  
  session.ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      // Commented out to improve performance - logs were slowing down the system
      // console.log(`WebSocket message from ${session.ip}:`, data);
      
      if (data.type === 'tokenUpdate') {
        document.getElementById('tokenCounter').textContent = data.total;
      } else if (data.type === 'taskUpdate') {
        handleTaskUpdate(data);
      } else if (data.type === 'log') {
        handleLogMessage(data.data, session.id);
      }
    } catch (error) {
      // Commented out to improve performance - WebSocket error messages were slowing down system
      // console.log(`WebSocket message from ${session.ip} (non-JSON):`, event.data);
      
      // Try to parse as JSON for non-JSON messages too (in case of parsing issues above)
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'log') {
          handleLogMessage(data.data, session.id);
        }
      } catch (jsonError) {
        // If it's not JSON, ignore
      }
    }
  };
  
  session.ws.onclose = () => {
    console.log(`Disconnected from server ${session.ip}`);
    updateSessionConnectionStatus(session, 'disconnected');
    // showToast(`Connection to server ${session.ip} was closed`, 'error');
    
    // Start ping mechanism to check when server comes back online
    startServerPing(session);
  };
  
  session.ws.onerror = (error) => {
    console.error(`WebSocket error for ${session.ip}:`, error);
    updateSessionConnectionStatus(session, 'connecting');
    showToast(`Cannot establish connection to server ${session.ip}. Server is not responding.`, 'error');
  };
}

// Disconnect WebSocket for a session
function disconnectSessionWebSocket(session) {
  // Clear any existing ping interval
  if (session.pingInterval) {
    clearInterval(session.pingInterval);
    session.pingInterval = null;
  }
  
  if (session.ws) {
    session.ws.close();
    updateSessionConnectionStatus(session, 'disconnected');
  }
  
  // Stop video stream
  if (session.videoInterval) {
    clearInterval(session.videoInterval);
    session.videoInterval = null;
  }
  
  // Reset screenshot container to ensure proper centering
  resetScreenshotContainer(session);
}

// Start/stop video for a session
function toggleSessionVideoLoop(session, endpoint) {
  if (session.videoInterval) {
    clearInterval(session.videoInterval);
    session.videoInterval = null;
    return false;
  } else {
    session.videoInterval = setInterval(() => {
      fetchSessionImage(session, endpoint);
    }, 1000 / currentFPS);
    return true;
  }
}

function startSessionVideo(session, endpoint) {
  if (!session.videoInterval) {
    session.videoInterval = setInterval(() => {
      fetchSessionImage(session, endpoint);
    }, 1000 / currentFPS);
  }
}

// Fetch image for a specific session
async function fetchSessionImage(session, endpoint, shouldSample = false) {
  try {
    const response = await fetch(`http://${session.ip}:8080/${endpoint}`);
    if (!response.ok) throw new Error(`Failed to fetch ${endpoint} from ${session.ip}`);

    // Get the blob first
    const blob = await response.blob();
    
    // Track network data usage for this session
    const contentLength = response.headers.get('Content-Length');
    let bytes = 0;
    
    if (contentLength) {
      bytes = parseInt(contentLength);
      if (!isNaN(bytes)) {
        trackSessionNetworkData(session.id, bytes);
      }
    } else {
      // If Content-Length is not available, use blob size
      bytes = blob.size;
      trackSessionNetworkData(session.id, bytes);
    }

    const imageUrl = URL.createObjectURL(blob);
    if (session.img.src) URL.revokeObjectURL(session.img.src);
    session.img.src = imageUrl;
    
    session.img.onload = function() {
      const imageSizeContainer = session.content.querySelector('.image-size-container');
      
      // Check if we're in fullscreen mode
      const isFullscreen = document.fullscreenElement || 
                          document.webkitFullscreenElement || 
                          document.mozFullScreenElement || 
                          document.msFullscreenElement;
      
      if (!isFullscreen && imageSizeContainer) {
        // Set the container size to match the actual image dimensions for normal mode
        imageSizeContainer.style.width = this.width + 'px';
        imageSizeContainer.style.height = this.height + 'px';
        
        // Center the container in the session content
        imageSizeContainer.style.position = 'absolute';
        imageSizeContainer.style.top = '50%';
        imageSizeContainer.style.left = '50%';
        imageSizeContainer.style.transform = 'translate(-50%, -50%)';
      } else if (isFullscreen && imageSizeContainer) {
        // In fullscreen mode, let the CSS handle the positioning completely
        // Clear all inline styles to allow CSS fullscreen rules to take over
        imageSizeContainer.style.width = '';
        imageSizeContainer.style.height = '';
        imageSizeContainer.style.position = '';
        imageSizeContainer.style.top = '';
        imageSizeContainer.style.left = '';
        imageSizeContainer.style.transform = '';
      }
    };
    
    // Show image and hide placeholder
    session.img.classList.remove('hidden');
    session.placeholder.classList.add('hidden');
  } catch (error) {
    console.error(`Error fetching image from ${session.ip}:`, error);
    // Reset the screenshot container to ensure proper centering
    resetScreenshotContainer(session);
  }
}

// FPS Selection Function
function setFPS(fps) {
  currentFPS = fps;
  
  // Update button states
  document.querySelectorAll('.fps-button').forEach(btn => {
    if (parseInt(btn.dataset.fps) === fps) {
      btn.classList.add('active');
    } else {
      btn.classList.remove('active');
    }
  });
  
  // Restart all active video streams with new FPS
  sessions.forEach(session => {
    if (session.videoInterval) {
      clearInterval(session.videoInterval);
      session.videoInterval = setInterval(() => {
        fetchSessionImage(session, 'screenshot');
      }, 1000 / currentFPS);
    }
  });
  
  console.log(`FPS set to: ${currentFPS}`);
}

// Mouse Functions for specific session
async function sendSessionMouseClick(session) {
  try {
    const response = await fetch(`http://${session.ip}:8080/mouse-click`);
    if (!response.ok) throw new Error(`Failed to send mouse click to ${session.ip}`);
  } catch (error) {
    console.error(`Error sending mouse click to ${session.ip}:`, error);
  }
}

async function sendSessionMouseInput(session, x, y) {
  try {
    const response = await fetch(`http://${session.ip}:8080/mouse-input?x=${x}&y=${y}`);
    if (!response.ok) throw new Error(`Failed to send mouse input to ${session.ip}`);
  } catch (error) {
    console.error(`Error sending mouse input to ${session.ip}:`, error);
  }
}

function formatIp(ip) {
  if (ip.includes(':')) {
    return `[${ip}]`;
  } else {
    return `${ip}`;
  }
}

// Function to validate IP address format
function isValidIP(ip) {
  // IPv4 validation: 4 octets, each 0-255
  const ipv4Regex = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
  
  // IPv6 validation (simplified - allows various valid IPv6 formats)
  const ipv6Regex = /^([\da-fA-F]{1,4}:){7}[\da-fA-F]{1,4}$|^::([\da-fA-F]{1,4}:){0,6}[\da-fA-F]{1,4}$|^[\da-fA-F]{1,4}::([\da-fA-F]{1,4}:){0,5}[\da-fA-F]{1,4}$|^([\da-fA-F]{1,4}:){1}:([\da-fA-F]{1,4}:){0,5}[\da-fA-F]{1,4}$|^([\da-fA-F]{1,4}:){2}:([\da-fA-F]{1,4}:){0,4}[\da-fA-F]{1,4}$|^([\da-fA-F]{1,4}:){3}:([\da-fA-F]{1,4}:){0,3}[\da-fA-F]{1,4}$|^([\da-fA-F]{1,4}:){4}:([\da-fA-F]{1,4}:){0,2}[\da-fA-F]{1,4}$|^([\da-fA-F]{1,4}:){5}:([\da-fA-F]{1,4}:){0,1}[\da-fA-F]{1,4}$|^([\da-fA-F]{1,4}:){6}:[\da-fA-F]{1,4}$/;
  
  // Check for IPv4
  if (ipv4Regex.test(ip)) {
    const parts = ip.split('.');
    return parts.every(part => {
      const num = parseInt(part, 10);
      return num >= 0 && num <= 255 && part === num.toString();
    });
  }
  
  // Check for IPv6
  return ipv6Regex.test(ip);
}

// Global variable for validation error timeout
let validationErrorTimeout = null;

// Function to show validation error message
function showValidationError(message) {
  // Remove any existing error message
  hideValidationError();
  
  // Clear any existing timeout
  if (validationErrorTimeout) {
    clearTimeout(validationErrorTimeout);
    validationErrorTimeout = null;
  }
  
  const inputGroup = document.querySelector('.input-group');
  const errorDiv = document.createElement('div');
  errorDiv.className = 'validation-error';
  errorDiv.textContent = message;
  errorDiv.style.cssText = 'display: flex; color: #ff6b6b; font-size: 11px; margin-top: 5px; padding: 4px 8px; background-color: rgba(255, 107, 107, 0.1); border-radius: 4px;';
  
  // Insert after the input group (below both input field and button)
  inputGroup.parentNode.insertBefore(errorDiv, inputGroup.nextSibling);
  
  // Auto-hide after 3 seconds, store timeout ID
  validationErrorTimeout = setTimeout(hideValidationError, 3000);
}

// Function to hide validation error message
function hideValidationError() {
  const existingError = document.querySelector('.validation-error');
  if (existingError) {
    existingError.remove();
  }
  
  // Clear the timeout if it exists
  if (validationErrorTimeout) {
    clearTimeout(validationErrorTimeout);
    validationErrorTimeout = null;
  }
}

// Event Listeners
document.getElementById("setTargetIP").addEventListener("click", () => {
  const ipInput = document.getElementById('ipv4');
  const ipValue = ipInput.value.trim();
  
  if (!isValidIP(ipValue)) {
    showValidationError('Please enter a valid IPv4 or IPv6 address');
    return;
  }

  // Hide validation error immediately if valid IP is entered
  hideValidationError();
  
  // Clear any existing server offline error when connecting to a new server
  clearServerOfflineError();
  
  const ipAddress = formatIp(ipValue);
  
  // Create new session
  const session = createSession(ipAddress);
  
  // Setup WebSocket connection
  setupSessionWebSocket(session);
  
  // Clear IP input field and keep focus
  ipInput.value = '';
  ipInput.focus();
  
  // Button remains "Connect" for new connections
  const connectButton = document.getElementById('setTargetIP');
  connectButton.textContent = 'Connect';
});

// IP input field Enter key and Escape key functionality
document.getElementById("ipv4").addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    
    const ipInput = document.getElementById('ipv4');
    const ipValue = ipInput.value.trim();
    
    // Validate IP format
    if (!isValidIP(ipValue)) {
      showValidationError('Please enter a valid IPv4 or IPv6 address');
      return;
    }

    // Hide validation error immediately if valid IP is entered
    hideValidationError();
    
    // Clear any existing server offline error when connecting to a new server
    clearServerOfflineError();
    
    const ipAddress = formatIp(ipValue);

    // Create new session
    const session = createSession(ipAddress);
    
    // Setup WebSocket connection
    setupSessionWebSocket(session);
    
    // Clear IP input field and keep focus
    ipInput.value = '';
    ipInput.focus();
    
    // Button remains "Connect" for new connections
    const connectButton = document.getElementById('setTargetIP');
    connectButton.textContent = 'Connect';
  }
  
  // Esc - lose focus from IP input
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation(); // Prevent event from bubbling up to avoid closing settings sidebar
    document.getElementById('ipv4').blur();
  }
});

// Toggle button event listener
document.getElementById("toggleButtonsBtn").addEventListener("click", toggleDebugOptions);

// Remove old event listeners that reference single-session functions
// These will be replaced with session-specific controls
document.getElementById("screenshotBtn").addEventListener("click", () => {
  const selectedSession = getSelectedSession();
  if (selectedSession) {
    fetchSessionImage(selectedSession, 'screenshot');
  } else {
    console.log("Please select a session first");
    alert("Please select a session first by clicking on it");
  }
});

document.getElementById("sendMouseClick").addEventListener("click", () => {
  const selectedSession = getSelectedSession();
  if (selectedSession) {
    sendSessionMouseClick(selectedSession);
  } else {
    console.log("Please select a session first");
    alert("Please select a session first by clicking on it");
  }
});

document.getElementById("sendMouseInput").addEventListener("click", () => {
  const x = parseInt(document.getElementById('x-coord').value) || 0;
  const y = parseInt(document.getElementById('y-coord').value) || 0;
  
  const selectedSession = getSelectedSession();
  if (selectedSession) {
    sendSessionMouseInput(selectedSession, x, y);
  } else {
    console.log("Please select a session first");
    alert("Please select a session first by clicking on it");
  }
});

// Video Controls - session-specific
document.getElementById("videoBtn").addEventListener('click', function() {
  const selectedSession = getSelectedSession();
  if (selectedSession) {
    const isRunning = toggleSessionVideoLoop(selectedSession, 'screenshot');
    this.textContent = isRunning ? 'Stop Video' : 'Video';
  } else {
    console.log("Please select a session first");
    alert("Please select a session first by clicking on it");
  }
});

document.getElementById("video2").addEventListener('click', function() {
  const selectedSession = getSelectedSession();
  if (selectedSession) {
    const isRunning = toggleSessionVideoLoop(selectedSession, 'video2');
    this.textContent = isRunning ? 'Stop Video 2' : 'Video 2';
  } else {
    console.log("Please select a session first");
    alert("Please select a session first by clicking on it");
  }
});

// User-assist functionality
let userAssistActive = false;
let userAssistTaskCard = null;
let userAssistConnectionPath = null;
let userAssistPulseDots = [];

// Function to calculate user-assist connection path (straight line from task card to chat)
function calculateUserAssistPath(taskCardElement, chatElement) {
  const taskRect = taskCardElement.getBoundingClientRect();
  const chatRect = chatElement.getBoundingClientRect();
  const overlayRect = connectionSvg.getBoundingClientRect();
  
  // Calculate positions relative to SVG overlay
  const taskCenterX = taskRect.left + taskRect.width / 2 - overlayRect.left;
  const taskBottomY = taskRect.bottom - overlayRect.top;
  
  const chatCenterX = chatRect.left + chatRect.width / 2 - overlayRect.left;
  const chatTopY = chatRect.top - overlayRect.top;
  
  // Straight line from bottom center of task card to top center of chat
  return `M ${taskCenterX} ${taskBottomY} L ${chatCenterX} ${chatTopY}`;
}

// Function to draw user-assist connection line
function drawUserAssistConnectionLine(taskCardElement, chatElement) {
  // Clear any existing user-assist connection
  clearUserAssistConnectionLine();
  
  const pathData = calculateUserAssistPath(taskCardElement, chatElement);
  
  // Create SVG path
  userAssistConnectionPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  userAssistConnectionPath.setAttribute('d', pathData);
  userAssistConnectionPath.setAttribute('class', 'connection-path user-assist');
  userAssistConnectionPath.setAttribute('stroke', '#4CAF50'); // Use same green color as main lines
  
  connectionSvg.appendChild(userAssistConnectionPath);
  
  // Create traveling pulse dots
  createUserAssistPulseDots(pathData);
  
}

// Function to create user-assist line (no traveling dots)
function createUserAssistPulseDots(pathData) {
  // Start synchronized animation if not already running
  startSynchronizedAnimation();
  
  // No traveling dots - just the line itself
}

// Function to clear user-assist connection line
function clearUserAssistConnectionLine() {
  if (userAssistConnectionPath) {
    connectionSvg.removeChild(userAssistConnectionPath);
    userAssistConnectionPath = null;
  }
  
  // Clear pulse dots
  userAssistPulseDots.forEach(dot => {
    if (dot.parentNode === connectionSvg) {
      connectionSvg.removeChild(dot);
    }
  });
  userAssistPulseDots = [];
}

// Function to add user-assist badge to chat legend
function addUserAssistBadge() {
  const chatLegend = document.querySelector('#chatFieldset legend');
  if (chatLegend && !chatLegend.querySelector('.user-assist-badge')) {
    const badge = document.createElement('span');
    badge.className = 'user-assist-badge';
    badge.textContent = 'user-assist';
    chatLegend.appendChild(badge);
  }
}

// Function to remove user-assist badge from chat legend
function removeUserAssistBadge() {
  const chatLegend = document.querySelector('#chatFieldset legend');
  if (chatLegend) {
    const badge = chatLegend.querySelector('.user-assist-badge');
    if (badge) {
      chatLegend.removeChild(badge);
    }
  }
}

// Function to activate user-assist mode
function activateUserAssist(taskCard) {
  // Only allow activation for in-progress tasks
  if (!taskCard.classList.contains('in-progress')) {
    return;
  }
  
  if (userAssistActive && userAssistTaskCard === taskCard) {
    // If already active for this task card, deactivate it
    deactivateUserAssist();
    return;
  }
  
  // Deactivate any existing user-assist
  deactivateUserAssist();
  
  userAssistActive = true;
  userAssistTaskCard = taskCard;
  
  // Get the session ID from the task card and switch session focus
  const sessionId = taskCard.dataset.sessionId;
  if (sessionId && sessionId !== 'null' && sessionId !== 'undefined') {
    selectSession(sessionId);
  }
  
  // Draw connection line
  const chatFieldset = document.getElementById('chatFieldset');
  drawUserAssistConnectionLine(taskCard, chatFieldset);
  
  // Add badge to chat legend
  addUserAssistBadge();
  
  // Change chat input color
  const chatInput = document.getElementById('llmChatInput');
  const chatFieldsetElement = document.getElementById('chatFieldset');
  chatInput.classList.add('user-assist-active');
  chatFieldsetElement.classList.add('user-assist-active');
  
}

// Function to deactivate user-assist mode
function deactivateUserAssist(restoreGreenOutline = true) {
  if (!userAssistActive) return;
  
  // Clear both connection lines (chat-to-session and chat-to-task)
  clearUserAssistConnectionLine();
  clearConnectionLine();
  
  // Remove badge from chat legend
  removeUserAssistBadge();
  
  // Restore chat input color
  const chatInput = document.getElementById('llmChatInput');
  const chatFieldsetElement = document.getElementById('chatFieldset');
  chatInput.classList.remove('user-assist-active');
  chatFieldsetElement.classList.remove('user-assist-active');
  
  // Update session selection to remove user-assist styling
  if (selectedSessionId) {
    const session = sessions.get(selectedSessionId);
    if (session && session.container) {
      session.container.classList.remove('user-assist-selected');
      // Add back the 'selected' class to restore the green outline only if requested
      if (restoreGreenOutline) {
        session.container.classList.add('selected');
      }
      console.log('User-assist activated');
    }
  }
  
  // Force a complete redraw of the normal connection line to ensure no golden styling remains
  if (selectedSessionId) {
    // Clear first to ensure clean state
    clearConnectionLine();
    
    // Then redraw with normal styling by temporarily setting userAssistActive to false
    const session = sessions.get(selectedSessionId);
    const chatFieldset = document.getElementById('chatFieldset');
    
    if (session && chatFieldset) {
      // Temporarily set userAssistActive to false to ensure normal styling
      const wasUserAssistActive = userAssistActive;
      userAssistActive = false;
      drawConnectionLine(chatFieldset, session.container);
      userAssistActive = wasUserAssistActive;
    }
  }
  
  userAssistActive = false;
  userAssistTaskCard = null;
  
  console.log('User-assist deactivated');
}

// Function to toggle user-assist mode for current session (Ctrl+U hotkey)
function toggleUserAssistForCurrentSession() {
  console.log('=== CTRL+U PRESSED ===');
  console.log('toggleUserAssistForCurrentSession called', { userAssistActive, selectedSessionId });
  
  if (userAssistActive) {
    // User-assist is active, deactivate it
    console.log('✓ User-assist is active, deactivating...');
    deactivateUserAssist();
    console.log('✓ User-assist mode deactivated via Ctrl+U');
    return;
  }
  
  // User-assist is not active, try to activate it for current session's in-progress task
  const selectedSession = getSelectedSession();
  console.log('✓ Selected session:', selectedSession);
  
  if (!selectedSession) {
    console.log('✗ No session selected');
    showToast('No session selected', 'warning');
    return;
  }
  
  // Find all task cards
  const tasksContainer = document.getElementById('tasksContainer');
  if (!tasksContainer) {
    console.log('✗ No tasks container found');
    showToast('No tasks available', 'warning');
    return;
  }
  
  const taskCards = Array.from(tasksContainer.querySelectorAll('.task-card'));
  console.log(`✓ Found ${taskCards.length} task cards`);
  
  // Find the first in-progress task for the selected session
  let targetTaskCard = null;
  for (const taskCard of taskCards) {
    const taskSessionId = taskCard.dataset.sessionId;
    const isInProgress = taskCard.classList.contains('in-progress');
    
    console.log(`  Checking task card:`, {
      taskId: taskCard.dataset.taskId,
      taskSessionId,
      isInProgress,
      selectedSessionId: selectedSession.id
    });
    
    // Check if this task belongs to the selected session and is in progress
    if (isInProgress && taskSessionId === selectedSession.id) {
      targetTaskCard = taskCard;
      console.log('✓ FOUND TARGET TASK CARD:', targetTaskCard);
      break; // Found the target task
    }
  }
  
  if (targetTaskCard) {
    // Scroll to the task card first, then activate user-assist
    console.log('✓ Scrolling to target task card before activating user-assist');
    smartScrollToTask(targetTaskCard);
    
    // Wait a bit for scroll to complete, then activate user-assist
    setTimeout(() => {
      console.log('✓ Activating user-assist for target task');
      activateUserAssist(targetTaskCard);
      console.log(`✓ User-assist mode activated via Ctrl+U for task in session ${selectedSession.id}`);
    }, 300); // Small delay to ensure scroll animation completes
  } else {
    // No in-progress task found for this session
    console.log('✗ No in-progress task found for this session');
    showToast('No in-progress task found for selected session', 'warning');
  }
  
  console.log('=== END CTRL+U HANDLING ===');
}

// Global task counter for sequence numbers
let taskSequenceNumber = 1;

// Function for smart scrolling based on task position
function smartScrollToTask(taskCard) {
  const tasksContainer = document.getElementById('tasksContainer');
  const taskCards = Array.from(tasksContainer.querySelectorAll('.task-card'));
  const taskIndex = taskCards.indexOf(taskCard);
  const totalTasks = taskCards.length;
  
  if (taskIndex === 0) {
    // First task - scroll to top
    tasksContainer.scrollTo({ top: 0, behavior: 'smooth' });
  } else if (taskIndex === totalTasks - 1) {
    // Last task - scroll to bottom
    tasksContainer.scrollTo({ top: tasksContainer.scrollHeight, behavior: 'smooth' });
  } else {
    // Middle task - scroll to center
    const taskRect = taskCard.getBoundingClientRect();
    const containerRect = tasksContainer.getBoundingClientRect();
    const containerCenter = containerRect.height / 2;
    const taskCenter = taskRect.top - containerRect.top + taskRect.height / 2;
    const scrollOffset = taskCenter - containerCenter;
    
    tasksContainer.scrollBy({ top: scrollOffset, behavior: 'smooth' });
  }
}

// Update task card click handler to include user-assist functionality
function createTaskCard(message, status = 'in-progress', sessionId = null) {
  const taskCard = document.createElement('div');
  taskCard.className = `task-card ${status}`;
  
  // Store original message as data attribute for later reference
  taskCard.dataset.originalMessage = message;
  taskCard.dataset.sequenceNumber = taskSequenceNumber;
  if (sessionId) {
    taskCard.dataset.sessionId = sessionId;
  }
  
  // Add click event to handle user-assist activation and smart scrolling
  taskCard.addEventListener('click', (event) => {
    // Check if the click was on an actionable element (buttons, icons)
    const actionableElements = [
      '.task-session-window-icon',
      '.task-cancel-btn',
      '.task-info-icon'
    ];
    
    const clickedElement = event.target;
    const isActionable = actionableElements.some(selector => 
      clickedElement.closest(selector)
    );
    
    // Only handle user-assist if not clicking on actionable elements
    if (!isActionable) {
      // Smart scroll based on task position
      smartScrollToTask(taskCard);
      
      // Toggle user-assist only for in-progress tasks
      if (taskCard.classList.contains('in-progress')) {
        activateUserAssist(taskCard);
      } else if (userAssistActive) {
        // If user-assist is already active and clicking on a non-in-progress task, deactivate it
        deactivateUserAssist();
      }
    }
  });
  
  // Increment sequence number for next task
  taskSequenceNumber++;
  
  const taskHeader = document.createElement('div');
  taskHeader.className = 'task-header';
  
  // Create left side container for sequence number and status
  const taskHeaderLeft = document.createElement('div');
  taskHeaderLeft.className = 'task-header-left';
  
  // Add sequence number
  const sequenceNumber = document.createElement('span');
  sequenceNumber.className = 'task-sequence-number';
  sequenceNumber.textContent = `#${taskCard.dataset.sequenceNumber}`;
  sequenceNumber.style.fontSize = '10px';
  sequenceNumber.style.color = '#888';
  sequenceNumber.style.marginRight = '8px';
  
  const taskStatus = document.createElement('span');
  taskStatus.className = 'task-status';
  taskStatus.textContent = status === 'in-progress' ? 'In Progress' : 
                         status === 'completed' ? 'Completed' :
                         status === 'broken' ? 'Broken' : 
                         status === 'canceled' ? 'Canceled' : 'In Queue';
  
  taskHeaderLeft.appendChild(sequenceNumber);
  taskHeaderLeft.appendChild(taskStatus);
  
  taskHeader.appendChild(taskHeaderLeft);
  
  const controlsContainer = document.createElement('div');
  controlsContainer.className = 'task-header-controls';
  
  // Add session window icon for all tasks with session ID
  if (sessionId) {
    const fullscreenIcon = document.createElement('div');
    fullscreenIcon.className = 'task-session-window-icon';
    fullscreenIcon.innerHTML = '⛶'; // Fullscreen-like icon
    fullscreenIcon.title = 'Open session window';
    fullscreenIcon.addEventListener('click', (event) => {
      event.stopPropagation(); // Prevent triggering task selection
      openTaskSessionWindow(sessionId);
    });
    controlsContainer.appendChild(fullscreenIcon);
  }
  
  // Add cancel button for in-progress and queued tasks
  if (status === 'in-progress' || status === 'in-the-queue') {
    const cancelBtn = document.createElement('button');
    cancelBtn.className = 'task-cancel-btn';
    cancelBtn.textContent = 'Cancel';
    cancelBtn.addEventListener('click', () => {
      cancelTask(taskCard);
    });
    controlsContainer.appendChild(cancelBtn);
  }
  
  // Add info icon for non-in-progress tasks
  if (status !== 'in-progress') {
    const infoIcon = document.createElement('div');
    infoIcon.className = 'task-info-icon';
    infoIcon.textContent = '?';
    let infoTitle = message;
    if (sessionId) {
      infoTitle += `\nSession: ${sessionId}`;
    }
    infoIcon.title = infoTitle;
    controlsContainer.appendChild(infoIcon);
  }
  
  taskHeader.appendChild(controlsContainer);
  taskCard.appendChild(taskHeader);
  
  // Add session info if available
  const sessionInfo = document.createElement('div');
  sessionInfo.className = 'task-session-info';
  sessionInfo.style.fontSize = '10px';
  sessionInfo.style.color = '#888';
  sessionInfo.style.marginBottom = '4px';
  
  if (sessionId) {
    sessionInfo.textContent = `Session: ${sessionId}`;
  } else {
    sessionInfo.textContent = 'Session: Unknown';
  }
  
  const taskMessage = document.createElement('div');
  taskMessage.className = 'task-message';
  taskMessage.textContent = message.length > 100 ? message.substring(0, 100) + '...' : message;
  
  taskCard.appendChild(sessionInfo);
  taskCard.appendChild(taskMessage);
  
  // Add timer for in-progress and queued tasks
  if (status === 'in-progress' || status === 'in-the-queue') {
    const timerContainer = document.createElement('div');
    timerContainer.className = 'task-timer';
    
    const timerText = document.createElement('span');
    timerText.className = 'timer-text';
    timerText.textContent = '00:00:00:00:000';
    
    timerContainer.appendChild(timerText);
    taskCard.appendChild(timerContainer);
    
    // Store creation time for timer
    taskCard.dataset.createdAt = Date.now();
    
    // Start timer
    startTaskTimer(taskCard);
  }
  
  return taskCard;
}

function startTaskTimer(taskCard) {
  const timerElement = taskCard.querySelector('.timer-text');
  if (!timerElement) return;
  
  const createdAt = parseInt(taskCard.dataset.createdAt);
  const timerInterval = setInterval(() => {
    const elapsed = Date.now() - createdAt;
    timerElement.textContent = formatElapsedTime(elapsed);
  }, 10); // Update every 10ms for milliseconds precision
  
  // Store interval ID for cleanup
  taskCard.dataset.timerInterval = timerInterval;
}

function stopTaskTimer(taskCard) {
  const intervalId = taskCard.dataset.timerInterval;
  if (intervalId) {
    clearInterval(intervalId);
    delete taskCard.dataset.timerInterval;
  }
}

function formatElapsedTime(milliseconds) {
  const days = Math.floor(milliseconds / (1000 * 60 * 60 * 24));
  const hours = Math.floor((milliseconds % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
  const minutes = Math.floor((milliseconds % (1000 * 60 * 60)) / (1000 * 60));
  const seconds = Math.floor((milliseconds % (1000 * 60)) / 1000);
  const ms = milliseconds % 1000;
  
  return `${days.toString().padStart(2, '0')}:${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}:${ms.toString().padStart(3, '0')}`;
}

function showTasksSection() {
  const tasksSection = document.getElementById('tasksSection');
  const chatFieldset = document.getElementById('chatFieldset');
  
  // Show tasks section with flexible sizing
  tasksSection.classList.add('visible');
  
  // Calculate dynamic flex value based on tasks content
  const tasksContainer = document.getElementById('tasksContainer');
  const taskCount = tasksContainer.children.length;
  const dynamicFlex = Math.max(1.5, 3 - (taskCount * 0.3)); // Adjust flex based on task count
  
  // Apply dynamic flex to chat section
  chatFieldset.style.flex = `${dynamicFlex}`;
}

function cancelTask(taskCard) {
  const taskId = taskCard.dataset.taskId;
  const sessionId = taskCard.dataset.sessionId;
  if (!taskId) return;

  // Find the session to send the cancellation request to
  let targetSession = null;
  if (sessionId) {
    targetSession = sessions.get(sessionId);
  }
  
  if (!targetSession && sessions.size > 0) {
    // If no specific session is associated, use the first available session
    const firstSessionId = Array.from(sessions.keys())[0];
    targetSession = sessions.get(firstSessionId);
  }

  if (!targetSession) {
    console.error('No active session available to cancel task');
    return;
  }

  // Send cancellation request to the appropriate session
  fetch(`http://${targetSession.ip}:8080/task-cancel?taskId=${taskId}`)
    .then(response => response.json())
    .then(data => {
      console.log('Task cancellation response:', data);
      
      // Update UI based on backend response
      if (data.result === 'Task canceled successfully') {
        // Remove both possible status classes
        taskCard.classList.remove('in-progress', 'in-the-queue');
        taskCard.classList.add('canceled');
        
        const statusElement = taskCard.querySelector('.task-status');
        statusElement.textContent = 'Canceled';
        
        // Remove cancel button
        const cancelBtn = taskCard.querySelector('.task-cancel-btn');
        if (cancelBtn) {
          cancelBtn.remove();
        }
        
        // Stop timer
        stopTaskTimer(taskCard);
        
        // Update current task status
        if (currentTask === taskCard) {
          currentTask = null;
        }
      }
    })
    .catch(error => {
      console.error('Error canceling task:', error);
    });
}

function handleTaskUpdate(data) {
  console.log('Task update received:', data);
  
  // Debug: Check if we have the expected data structure
  if (!data.taskId || !data.status) {
    console.error('Invalid task update data:', data);
    return;
  }
  
  const tasksContainer = document.getElementById('tasksContainer');
  const taskCards = tasksContainer.querySelectorAll('.task-card');
  
  // First, check if we have a pending task that needs to be updated with real task ID
  let taskCard = null;
  for (const card of taskCards) {
    if (card.dataset.taskId === 'pending') {
      // Found a pending task - update it with the real task ID
      card.dataset.taskId = data.taskId;
      
      // Enable cancel button now that we have a real task ID
      const cancelBtn = card.querySelector('.task-cancel-btn');
      if (cancelBtn) {
        cancelBtn.disabled = false;
        cancelBtn.textContent = 'Cancel';
      }
      
      taskCard = card;
      break;
    } else if (card.dataset.taskId === data.taskId) {
      taskCard = card;
      break;
    }
  }
  
  if (!taskCard) {
    // Create new task card if it doesn't exist
    taskCard = createTaskCard(data.message, data.status);
    taskCard.dataset.taskId = data.taskId;
    tasksContainer.appendChild(taskCard);
    
    // Show tasks section if it's the first task
    if (tasksContainer.children.length === 1) {
      showTasksSection();
    }
  } else {
    // Update existing task card
    console.log('Updating task card with status:', data.status);
    
    // Remove all status classes first
    taskCard.classList.remove('in-progress', 'completed', 'broken', 'canceled', 'in-the-queue');
    // Add the new status class based on the status field
    taskCard.classList.add(data.status);
    
    const statusElement = taskCard.querySelector('.task-status');
    const messageElement = taskCard.querySelector('.task-message');
    
    if (statusElement) {
      statusElement.textContent = data.status === 'in-progress' ? 'In Progress' : 
                                data.status === 'completed' ? 'Completed' :
                                data.status === 'broken' ? 'Broken' : 
                                data.status === 'canceled' ? 'Canceled' : 'In Queue';
    }
    
    if (messageElement && data.message) {
      messageElement.textContent = data.message.length > 100 ? 
                                 data.message.substring(0, 100) + '...' : data.message;
    }
    
    // Remove cancel button if task is completed, broken, or canceled
    if (data.status !== 'in-progress' && data.status !== 'in-the-queue') {
      const cancelBtn = taskCard.querySelector('.task-cancel-btn');
      if (cancelBtn) {
        cancelBtn.remove();
      }
    }
    
    // Add cancel button for queued tasks
    if (data.status === 'in-the-queue') {
      const controlsContainer = taskCard.querySelector('.task-header-controls');
      const existingCancelBtn = taskCard.querySelector('.task-cancel-btn');
      
      if (!existingCancelBtn && controlsContainer) {
        const cancelBtn = document.createElement('button');
        cancelBtn.className = 'task-cancel-btn';
        cancelBtn.textContent = 'Cancel';
        cancelBtn.addEventListener('click', () => {
          cancelTask(taskCard);
        });
        controlsContainer.appendChild(cancelBtn);
      }
    }
    
    // Add or remove info icon based on new status
    const controlsContainer = taskCard.querySelector('.task-header-controls');
    const existingInfoIcon = taskCard.querySelector('.task-info-icon');
    
    if (data.status !== 'in-progress') {
      // Add info icon if it doesn't exist
      if (!existingInfoIcon && controlsContainer) {
        const infoIcon = document.createElement('div');
        infoIcon.className = 'task-info-icon';
        infoIcon.textContent = '?';
        infoIcon.title = taskCard.dataset.originalMessage;
        
        controlsContainer.appendChild(infoIcon);
      } else if (existingInfoIcon) {
        // Update title if info icon already exists
        existingInfoIcon.title = taskCard.dataset.originalMessage;
      }
    } else {
      // Remove info icon if status is 'in-progress'
      if (existingInfoIcon) {
        existingInfoIcon.remove();
      }
    }
    
    // Handle timer based on status changes
    if (data.status === 'in-progress' || data.status === 'in-the-queue') {
      // Start or continue timer for in-progress or queued tasks
      if (!taskCard.dataset.timerInterval) {
        // Timer is not running, start it
        startTaskTimer(taskCard);
      }
      // If timer is already running, it will continue automatically
    } else {
      // Stop timer for completed, broken, or canceled tasks
      stopTaskTimer(taskCard);
      
      // Deactivate user-assist if this was the active task
      if (userAssistActive && userAssistTaskCard === taskCard) {
        deactivateUserAssist();
      }
    }
  }
  
  // Update current task reference
  if (data.status === 'in-progress') {
    currentTask = taskCard;
  } else if (currentTask === taskCard) {
    currentTask = null;
  }
  
  // Scroll to bottom to show the updated/created task
  setTimeout(scrollTasksToBottom, 0);
}

function scrollTasksToBottom() {
  const tasksContainer = document.getElementById('tasksContainer');
  if (tasksContainer) {
    tasksContainer.scrollTop = tasksContainer.scrollHeight;
  }
}

// Function to check server availability
async function checkServerAvailability(ip) {
  try {
    const response = await fetch(`http://${ip}:8080/ping`, {
      method: 'GET',
      signal: AbortSignal.timeout(3000) // 3 second timeout
    });
    return response.ok;
  } catch (error) {
    console.error(`Server ${ip} is not available:`, error);
    return false;
  }
}

// Function to start server ping when disconnected
function startServerPing(session) {
  // Clear any existing ping interval
  if (session.pingInterval) {
    clearInterval(session.pingInterval);
  }
  
  session.pingInterval = setInterval(async () => {
    const isServerAvailable = await checkServerAvailability(session.ip);
    
    if (isServerAvailable) {
      console.log(`Server ${session.ip} is back online, attempting to reconnect...`);
      
      // Clear the ping interval
      clearInterval(session.pingInterval);
      session.pingInterval = null;
      
      // Attempt to reconnect
      setupSessionWebSocket(session);
    }
  }, 1000); // Ping every second
}

// Function to reset session state after reconnection
function resetSessionState(session) {
  console.log(`Resetting session state for ${session.ip}`);
  
  // Reset any pending tasks that might be in incorrect state
  const tasksContainer = document.getElementById('tasksContainer');
  const taskCards = tasksContainer.querySelectorAll('.task-card');
  
  taskCards.forEach(card => {
    // If task is associated with this session and is in incorrect state, update it
    if (card.dataset.sessionId === session.ip) {
      const taskId = card.dataset.taskId;
      
      // If task is marked as in-progress but server just reconnected,
      // it might be in an inconsistent state
      if (card.classList.contains('in-progress') && taskId !== 'pending') {
        // We'll let the server send the correct status updates
        // but mark it for potential resync
        console.log(`Task ${taskId} for session ${session.ip} may need status resync`);
        
        // Temporarily mark as in-queue until server confirms status
        card.classList.remove('in-progress');
        card.classList.add('in-the-queue');
        
        const statusElement = card.querySelector('.task-status');
        if (statusElement) {
          statusElement.textContent = 'In Queue';
        }
      }
    }
  });
  
  // Clear any stale connection state
  session.isConnected = true;
  
  // Update UI to reflect proper connection status
  updateSessionConnectionStatus(session, 'connected');
  
  // Clear any server offline error states
  const chatFieldset = document.getElementById('chatFieldset');
  if (chatFieldset) {
    chatFieldset.classList.remove('server-offline');
  }
}

// Function to open session window for a task
async function openTaskSessionWindow(sessionId) {
  console.log(`Opening session window for: ${sessionId}`);
  
  // First check if server is available
  const isServerAvailable = await checkServerAvailability(sessionId);
  
  if (!isServerAvailable) {
    showToast(`Cannot establish connection to server ${sessionId}. Server is not responding.`, 'error');
    return;
  }
  
  // Check if session already exists
  let session = sessions.get(sessionId);
  
  if (!session) {
    // Session doesn't exist, create a new one
    session = createSession(sessionId);
    setupSessionWebSocket(session);
    console.log(`Created new session for: ${sessionId}`);
  } else {
    // Session exists, select it and ensure it's connected
    selectSession(sessionId);
    if (!session.isConnected) {
      setupSessionWebSocket(session);
      console.log(`Reconnected to existing session: ${sessionId}`);
    } else {
      console.log(`Session already connected: ${sessionId}`);
    }
  }
  
  // Ensure the session is visible and focused
  if (session.container) {
    session.container.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }
}

function handleTaskCreation(message, sessionId = null) {
  // Create task card for every message with temporary ID
  const tasksContainer = document.getElementById('tasksContainer');
  
  const taskCard = createTaskCard(message, 'in-the-queue', sessionId);
  taskCard.dataset.taskId = 'pending'; // Temporary ID until we get real one
  tasksContainer.appendChild(taskCard);
  currentTask = taskCard;
  
  // Disable cancel button until we get real task ID
  const cancelBtn = taskCard.querySelector('.task-cancel-btn');
  if (cancelBtn) {
    cancelBtn.disabled = true;
    cancelBtn.textContent = 'Cancel';
  }
  
  // Show tasks section if it's the first task
  if (tasksContainer.children.length === 1) {
    showTasksSection();
  }
  
  // Scroll to bottom to show the new task
  setTimeout(scrollTasksToBottom, 0);
  
  isFirstMessage = false;
}

// Function to show server offline error
function showServerOfflineError(ip) {
  // Make chat fieldset blink with red and show error message
  const chatFieldset = document.getElementById('chatFieldset');
  const chatInput = document.getElementById('llmChatInput');
  
  // Add blinking red border to chat fieldset
  chatFieldset.classList.add('server-offline');
  
  // Show error message in chat input
  const originalPlaceholder = chatInput.placeholder;
  chatInput.placeholder = `Cannot create task - server ${ip} is offline`;
  
  // Store the original placeholder to restore later
  if (!chatInput.dataset.originalPlaceholder) {
    chatInput.dataset.originalPlaceholder = originalPlaceholder;
  }
  
  // Clear the blinking after exactly 1 second (two blinks)
  setTimeout(() => {
    chatFieldset.classList.remove('server-offline');
  }, 1000);
  
  showToast(`Cannot create task - server ${ip} is offline. Please wait for server to reconnect.`, 'error');
}

// Function to clear server offline error
function clearServerOfflineError() {
  const chatFieldset = document.getElementById('chatFieldset');
  const chatInput = document.getElementById('llmChatInput');
  
  if (chatFieldset) {
    chatFieldset.classList.remove('server-offline');
  }
  
  if (chatInput && chatInput.dataset.originalPlaceholder) {
    chatInput.placeholder = chatInput.dataset.originalPlaceholder;
    delete chatInput.dataset.originalPlaceholder;
  }
}

// LLM Chat
document.getElementById("llmSendButton").addEventListener("click", () => {
  const inputText = document.getElementById('llmChatInput').value;
  if (!inputText) return;

  // Get the currently selected session or use the first available session
  let targetSession = getSelectedSession();
  if (!targetSession && sessions.size > 0) {
    // If no session is selected, use the first session
    const firstSessionId = Array.from(sessions.keys())[0];
    targetSession = sessions.get(firstSessionId);
  }

  if (!targetSession) {
    console.error('No active session available to send task to');
    alert('Please connect to a session first');
    return;
  }

  // Check if session is connected
  if (!targetSession.isConnected) {
    showServerOfflineError(targetSession.ip);
    return;
  }

  // Check if user-assist mode is active
  if (userAssistActive && userAssistTaskCard) {
    // Send as user-assist message
    const taskId = userAssistTaskCard.dataset.taskId;
    if (taskId && taskId !== 'pending') {
      fetch(`http://${targetSession.ip}:8080/user-assist`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          taskId: taskId,
          message: inputText
        })
      }).then(response => response.json())
      .then(data => {
        if (!data.accepted) {
          showToast('User-assist message was not accepted. Task may be completed or not found.', 'warning');
        }
      })
      .catch(error => {
        console.error('Error sending user-assist message:', error);
        showToast('Failed to send user-assist message', 'error');
      });
    } else {
      showToast('Cannot send user-assist message: task not ready', 'warning');
    }
  } else {
    // Send as regular task
    fetch(`http://${targetSession.ip}:8080/llm-input`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        text: inputText,
        sessionId: targetSession.id
      })
    }).catch(error => {
      console.error('Error sending task to session:', error);
    });

    // Handle task creation with session ID
    handleTaskCreation(inputText, targetSession.id);
  }
  
  document.getElementById('llmChatInput').value = "";
});

// Hotkey functionality for chat input
document.getElementById("llmChatInput").addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    if (event.shiftKey) {
      // Shift+Enter - allow default behavior (new line)
      return;
    } else {
      // Enter without Shift - send message if input is not empty
      event.preventDefault();
      const inputText = document.getElementById('llmChatInput').value.trim();
      if (inputText) {
        // Check if session is connected before sending
        const targetSession = getSelectedSession();
        if (!targetSession) {
          alert('Please connect to a session first');
          return;
        }
        if (!targetSession.isConnected) {
          showServerOfflineError(targetSession.ip);
          return;
        }
        document.getElementById("llmSendButton").click();
      }
    }
  }
  
  // Ctrl+L - clear the chat input
  if (event.ctrlKey && event.key === "l") {
    event.preventDefault();
    document.getElementById('llmChatInput').value = "";
  }
  
  // Esc - lose focus from chat input
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation(); // Prevent event from bubbling up to avoid closing settings sidebar
    document.getElementById('llmChatInput').blur();
  }
  
  // Ctrl+U - toggle user-assist mode (handled by global listener)
  // Removed from here to avoid duplicate event handling
});

// Pop Out WS
document.getElementById("popOutWSButton").addEventListener("click", function() {
  // Open window with specific features for better visibility
  popOutWSWindow = window.open('', '_blank', 'width=800,height=600,resizable=yes,scrollbars=yes');

  if (popOutWSWindow) {
    const html = `<!DOCTYPE html>
<html>
<head>
  <title>WebSocket Output</title>
  <style>
    body {
      margin: 0;
      padding: 20px;
      background-color: #1e1e1e;
      color: #fff;
      font-family: monospace;
    }
    #poppedTextarea {
      width: 100%;
      height: calc(100vh - 40px);
      background-color: #000;
      color: #fff;
      border: 1px solid #333;
      padding: 10px;
      font-size: 14px;
      line-height: 1.4;
      resize: none;
      box-sizing: border-box;
    }
  </style>
</head>
<body>
  <textarea id="poppedTextarea" readonly></textarea>
  <script>
    // Debug logging
    console.log('Pop-out window loaded');
    
    const ip = '${ip}';
    let ws = null;
    const textarea = document.getElementById('poppedTextarea');
    
    function log(msg) {
      console.log(msg);
      textarea.value += msg + '\\n';
      textarea.scrollTop = textarea.scrollHeight;
    }

    function setupWebSocket() {
      try {
        log('Attempting WebSocket connection...');
        
        ws = new WebSocket('ws://' + ip + ':8080/ws');
        
        ws.onopen = () => {
          log('Connected to WebSocket server');
        };
        
        ws.onmessage = (event) => {
          if (!event.data) return;
          
          try {
            const parsed = JSON.parse(event.data);
            const formatted = JSON.stringify(parsed, null, 2);
            log('Received: ' + formatted);
          } catch (e) {
            log('Received: ' + event.data);
          }
        };

        ws.onclose = () => {
          log('WebSocket connection closed');
          ws = null;
          // Try to reconnect after 3 seconds
          setTimeout(setupWebSocket, 3000);
        };

        ws.onerror = (error) => {
          log('WebSocket error: ' + error.message);
        };

      } catch (error) {
        log('Setup error: ' + error.message);
      }
    }

    // Start connection when window loads
    window.addEventListener('load', setupWebSocket);
  <\/script>
</body>
</html>`;

    // Write the HTML and explicitly close the document
    popOutWSWindow.document.open();
    popOutWSWindow.document.write(html);
    popOutWSWindow.document.close();

    // Log success in main window
    console.log('Pop-out window created successfully');
  } else {
    console.error('Failed to create pop-out window');
  }
});

// FPS Button Event Listeners
document.querySelectorAll('.fps-button').forEach(button => {
  button.addEventListener('click', function() {
    const fps = parseInt(this.dataset.fps);
    setFPS(fps);
  });
});

// Initialize default FPS state
setFPS(currentFPS);

// Session-specific fullscreen functionality
function setupSessionFullscreen(session) {
  const screenshotOverlay = session.content.querySelector('.screenshot-overlay');
  const screenshotContainer = session.screenshotContainer;
  const fullscreenBtn = session.fullscreenBtn;
  const maximizeBtn = screenshotOverlay.querySelector('.fullscreen-button:first-child');
  const llmChatInput = document.getElementById('llmChatInput');
  
  // Inactivity timer for overlay
  let overlayTimeout = null;
  const OVERLAY_TIMEOUT = 3000; // 3 seconds
  
  function resetOverlayTimer() {
    // Clear existing timeout
    if (overlayTimeout) {
      clearTimeout(overlayTimeout);
    }
    
    // Only set timeout if in fullscreen mode
    const isFullscreen = document.fullscreenElement || 
                        document.webkitFullscreenElement || 
                        document.mozFullScreenElement || 
                        document.msFullscreenElement;
    
    if (isFullscreen) {
      // Show overlay immediately when activity is detected
      screenshotOverlay.style.opacity = '1';
      screenshotOverlay.style.pointerEvents = 'auto';
      
      // Set new timeout to hide overlay after inactivity
      overlayTimeout = setTimeout(() => {
        screenshotOverlay.style.opacity = '0';
        screenshotOverlay.style.pointerEvents = 'none';
      }, OVERLAY_TIMEOUT);
    }
  }
  
  // Mouse event handling for overlay
  let isMouseOverOverlay = false;
  let isMouseOverContainer = false;
  
  // Handle mouse enter on overlay
  screenshotOverlay.addEventListener('mouseenter', () => {
    isMouseOverOverlay = true;
    if (overlayTimeout) {
      clearTimeout(overlayTimeout);
    }
  });
  
  // Handle mouse leave from overlay
  screenshotOverlay.addEventListener('mouseleave', () => {
    isMouseOverOverlay = false;
    // Only hide if mouse is not over container either
    if (!isMouseOverContainer) {
      const isFullscreen = document.fullscreenElement || 
                          document.webkitFullscreenElement || 
                          document.mozFullScreenElement || 
                          document.msFullscreenElement;
      
      if (!isFullscreen) {
        // Normal mode - hide overlay immediately when mouse leaves
        screenshotOverlay.style.opacity = '0';
        screenshotOverlay.style.pointerEvents = 'none';
      } else {
        // Fullscreen mode - start timer
        resetOverlayTimer();
      }
    }
  });
  
  // Handle mouse enter on container
  screenshotContainer.addEventListener('mouseenter', () => {
    isMouseOverContainer = true;
    const isFullscreen = document.fullscreenElement || 
                        document.webkitFullscreenElement || 
                        document.mozFullScreenElement || 
                        document.msFullscreenElement;
    
    if (isFullscreen) {
      // Fullscreen mode - show overlay and start timer
      screenshotOverlay.style.opacity = '1';
      screenshotOverlay.style.pointerEvents = 'auto';
      resetOverlayTimer();
    } else {
      // Normal mode - show overlay immediately
      screenshotOverlay.style.opacity = '1';
      screenshotOverlay.style.pointerEvents = 'auto';
    }
  });
  
  // Handle mouse leave from container
  screenshotContainer.addEventListener('mouseleave', () => {
    isMouseOverContainer = false;
    // Only hide if mouse is not over overlay either
    if (!isMouseOverOverlay) {
      const isFullscreen = document.fullscreenElement || 
                          document.webkitFullscreenElement || 
                          document.mozFullScreenElement || 
                          document.msFullscreenElement;
      
      if (!isFullscreen) {
        // Normal mode - hide overlay immediately when mouse leaves container
        screenshotOverlay.style.opacity = '0';
        screenshotOverlay.style.pointerEvents = 'none';
      } else {
        // Fullscreen mode - start timer
        resetOverlayTimer();
      }
    }
  });
  
  // Mouse move detection for both container and overlay
  screenshotContainer.addEventListener('mousemove', resetOverlayTimer);
  screenshotOverlay.addEventListener('mousemove', resetOverlayTimer);
  
  // Reset timer on key press
  document.addEventListener('keydown', resetOverlayTimer);
  
  // Reset timer on fullscreen button click
  fullscreenBtn.addEventListener('click', resetOverlayTimer);
  
  // Fullscreen button click handler
  fullscreenBtn.addEventListener('click', () => {
    toggleSessionFullscreen(session);
  });
  
  // Maximize button click handler
  maximizeBtn.addEventListener('click', () => {
    toggleSessionMaximize(session);
  });
  
  // Make screenshot container focusable and handle focus for keyboard navigation
  screenshotContainer.setAttribute('tabindex', '0');
  screenshotContainer.addEventListener('focus', () => {
    screenshotContainer.classList.add('focused');
  });
  
  screenshotContainer.addEventListener('blur', () => {
    screenshotContainer.classList.remove('focused');
  });
  
  // Also allow fullscreen when container is focused and Enter key is pressed
  screenshotContainer.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      toggleSessionFullscreen(session);
    }
  });
  
  // Focus screenshot container when clicked with left mouse button
  screenshotContainer.addEventListener('mousedown', (event) => {
    // Only handle left mouse button clicks (button 0)
    if (event.button === 0) {
      event.preventDefault();
      screenshotContainer.focus();
    }
  });
  
  // Also focus when the image itself is clicked (in case click doesn't bubble properly)
  session.img.addEventListener('mousedown', (event) => {
    if (event.button === 0) {
      event.preventDefault();
      screenshotContainer.focus();
    }
  });
}

function toggleSessionFullscreen(session) {
  const screenshotContainer = session.screenshotContainer;
  
  if (!document.fullscreenElement) {
    // Enter fullscreen
    if (screenshotContainer.requestFullscreen) {
      screenshotContainer.requestFullscreen().catch(err => {
        console.error('Error attempting to enable fullscreen:', err);
      });
    } else if (screenshotContainer.webkitRequestFullscreen) { /* Safari */
      screenshotContainer.webkitRequestFullscreen();
    } else if (screenshotContainer.msRequestFullscreen) { /* IE11 */
      screenshotContainer.msRequestFullscreen();
    }
  } else {
    // Exit fullscreen
    if (document.exitFullscreen) {
      document.exitFullscreen();
    } else if (document.webkitExitFullscreen) { /* Safari */
      document.webkitExitFullscreen();
    } else if (document.msExitFullscreen) { /* IE11 */
      document.msExitFullscreen();
    }
  }
}

// Toggle session maximize functionality
function toggleSessionMaximize(session) {
  const sessionContainer = session.container;
  const mainContent = document.getElementById('mainContent');
  const imageSizeContainer = session.content.querySelector('.image-size-container');
  const screenshotOverlay = session.content.querySelector('.screenshot-overlay');
  const screenshotContainer = session.screenshotContainer;
  const mainWrapper = document.querySelector('.main-wrapper');
  
  // Check if session is already maximized
  const isMaximized = sessionContainer.classList.contains('maximized');
  
  // Check if settings sidebar is open
  const isSettingsOpen = mainWrapper.classList.contains('settings-open');
  
  // Clean up existing maximized mode event listeners if restoring
  if (isMaximized && screenshotContainer._maximizedHandlers) {
    const handlers = screenshotContainer._maximizedHandlers;
    
    // Remove event listeners
    document.removeEventListener('mousemove', handlers.mouseMove);
    screenshotContainer.removeEventListener('mouseenter', handlers.mouseEnter);
    screenshotContainer.removeEventListener('mouseleave', handlers.mouseLeave);
    
    // Clear timeout
    if (handlers.timeout) {
      clearTimeout(handlers.timeout);
    }
    
    // Remove handlers reference
    delete screenshotContainer._maximizedHandlers;
  }
  
  // Completely remove overlay from DOM during transition to prevent any artifacts
  let overlayParent = null;
  let overlayNextSibling = null;
  if (screenshotOverlay) {
    overlayParent = screenshotOverlay.parentNode;
    overlayNextSibling = screenshotOverlay.nextSibling;
    overlayParent.removeChild(screenshotOverlay);
  }
  
  if (isMaximized) {
    // Restore normal layout - show all sessions
    sessionContainer.classList.remove('maximized');
    sessions.forEach((otherSession, sessionId) => {
      otherSession.container.style.display = 'flex';
    });
    mainContent.setAttribute('data-session-count', sessions.size);
  } else {
    // Maximize
    sessionContainer.classList.add('maximized');
    sessions.forEach((otherSession, sessionId) => {
      if (sessionId !== session.id) {
        otherSession.container.style.display = 'none';
      }
    });
    mainContent.setAttribute('data-session-count', '1');
  }
  
  // Force immediate recalculation of screenshot overlay positioning
  setTimeout(() => {
    if (imageSizeContainer && session.img.width && session.img.height) {
      // Force recalculation of the image size container positioning
      const isFullscreen = document.fullscreenElement || 
                          document.webkitFullscreenElement || 
                          document.mozFullScreenElement || 
                          document.msFullscreenElement;
      
      if (!isFullscreen) {
        // Temporarily clear and reset positioning to force recalculation
        imageSizeContainer.style.width = '';
        imageSizeContainer.style.height = '';
        
        // Force a reflow
        imageSizeContainer.offsetHeight;
        
        // Restore proper positioning for maximized session
        imageSizeContainer.style.width = session.img.width + 'px';
        imageSizeContainer.style.height = session.img.height + 'px';
        imageSizeContainer.style.position = 'absolute';
        imageSizeContainer.style.top = '50%';
        imageSizeContainer.style.left = '50%';
        imageSizeContainer.style.transform = 'translate(-50%, -50%)';
        
        // Force another reflow to ensure that container is properly sized
        imageSizeContainer.offsetHeight;
      }
    }
    
    // Redraw connection line when exiting maximized mode to fix the path redrawing issue
    updateConnectionLine();
    
    // Recreate and reinsert overlay after everything is properly resized
    if (overlayParent && screenshotOverlay) {
      // Reinsert overlay in its original position
      if (overlayNextSibling) {
        overlayParent.insertBefore(screenshotOverlay, overlayNextSibling);
      } else {
        overlayParent.appendChild(screenshotOverlay);
      }
      
      // NOW set up overlay timer and event listeners AFTER overlay is back in the DOM
      const screenshotContainer = session.screenshotContainer;
      const isFullscreen = document.fullscreenElement || 
                          document.webkitFullscreenElement || 
                          document.mozFullScreenElement || 
                          document.msFullscreenElement;
      
      if (!isFullscreen && screenshotContainer) {
        // Copy EXACTLY from fullscreen implementation
        let overlayTimeout = null;
        const OVERLAY_TIMEOUT = 3000; // 3 seconds
        
        function resetMaximizedOverlayTimer() {
          // Clear existing timeout
          if (overlayTimeout) {
            clearTimeout(overlayTimeout);
          }
          
          // Show overlay immediately when activity is detected
          screenshotOverlay.style.opacity = '1';
          screenshotOverlay.style.pointerEvents = 'auto';
          
          // Set new timeout to hide overlay after inactivity
          overlayTimeout = setTimeout(() => {
            screenshotOverlay.style.opacity = '0';
            screenshotOverlay.style.pointerEvents = 'none';
          }, OVERLAY_TIMEOUT);
        }
        
        // Mouse event handling for overlay - EXACTLY like fullscreen
        let isMouseOverOverlay = false;
        let isMouseOverContainer = false;
        
        // Handle mouse enter on overlay
        screenshotOverlay.addEventListener('mouseenter', () => {
          isMouseOverOverlay = true;
          if (overlayTimeout) {
            clearTimeout(overlayTimeout);
          }
        });
        
        // Handle mouse leave from overlay
        screenshotOverlay.addEventListener('mouseleave', () => {
          isMouseOverOverlay = false;
          // Only hide if mouse is not over container either
          if (!isMouseOverContainer) {
            const isFullscreen = document.fullscreenElement || 
                                document.webkitFullscreenElement || 
                                document.mozFullScreenElement || 
                                document.msFullscreenElement;
            
            if (!isFullscreen) {
              // Normal mode - hide overlay immediately when mouse leaves
              screenshotOverlay.style.opacity = '0';
              screenshotOverlay.style.pointerEvents = 'none';
            } else {
              // Fullscreen mode - start timer
              resetMaximizedOverlayTimer();
            }
          }
        });
        
        // Handle mouse enter on container
        screenshotContainer.addEventListener('mouseenter', () => {
          isMouseOverContainer = true;
          const isFullscreen = document.fullscreenElement || 
                              document.webkitFullscreenElement || 
                              document.mozFullScreenElement || 
                              document.msFullscreenElement;
          
          if (isFullscreen) {
            // Fullscreen mode - show overlay and start timer
            screenshotOverlay.style.opacity = '1';
            screenshotOverlay.style.pointerEvents = 'auto';
            resetMaximizedOverlayTimer();
          } else {
            // Normal mode - show overlay immediately
            screenshotOverlay.style.opacity = '1';
            screenshotOverlay.style.pointerEvents = 'auto';
          }
        });
        
        // Handle mouse leave from container
        screenshotContainer.addEventListener('mouseleave', () => {
          isMouseOverContainer = false;
          // Only hide if mouse is not over overlay either
          if (!isMouseOverOverlay) {
            const isFullscreen = document.fullscreenElement || 
                                document.webkitFullscreenElement || 
                                document.mozFullScreenElement || 
                                document.msFullscreenElement;
            
            if (!isFullscreen) {
              // Normal mode - hide overlay immediately when mouse leaves container
              screenshotOverlay.style.opacity = '0';
              screenshotOverlay.style.pointerEvents = 'none';
            } else {
              // Fullscreen mode - start timer
              resetMaximizedOverlayTimer();
            }
          }
        });
        
        // Mouse move detection for both container and overlay - EXACTLY like fullscreen
        screenshotContainer.addEventListener('mousemove', resetMaximizedOverlayTimer);
        screenshotOverlay.addEventListener('mousemove', resetMaximizedOverlayTimer);
        
        // Store handlers for cleanup
        screenshotContainer._maximizedHandlers = {
          timeout: overlayTimeout
        };
        
        // Start to timer - EXACTLY like fullscreen
        resetMaximizedOverlayTimer();
        
        console.log('Maximized mode overlay timer setup completed - copied from fullscreen');
      }
    }
  }, 0);
}

// Global keyboard shortcut for fullscreen (F key) and maximize (M key) - applies to selected session
document.addEventListener('keydown', (event) => {
  // Check if chat input or IP input is focused
  const isChatInputFocused = document.activeElement === document.getElementById('llmChatInput');
  const isIpInputFocused = document.activeElement === document.getElementById('ipv4');
  
  if (!isChatInputFocused && !isIpInputFocused) {
    // Fullscreen shortcut (F key)
    if (event.key === 'f' || event.key === 'F') {
      event.preventDefault(); // Prevent default browser behavior
      
      // Check if we're already in fullscreen mode
      const isFullscreen = document.fullscreenElement || 
                          document.webkitFullscreenElement || 
                          document.mozFullScreenElement || 
                          document.msFullscreenElement;
      
      if (isFullscreen) {
        // Exit fullscreen mode
        if (document.exitFullscreen) {
          document.exitFullscreen();
        } else if (document.webkitExitFullscreen) { /* Safari */
          document.webkitExitFullscreen();
        } else if (document.msExitFullscreen) { /* IE11 */
          document.msExitFullscreen();
        }
      } else {
        // Enter fullscreen mode for selected session
        const selectedSession = getSelectedSession();
        if (selectedSession) {
          toggleSessionFullscreen(selectedSession);
        }
      }
    }
    
    // Maximize shortcut (M key)
    if (event.key === 'm' || event.key === 'M') {
      event.preventDefault(); // Prevent default browser behavior
      
      // Blur any potentially focused elements to prevent outline on settings button
      if (document.activeElement) {
        document.activeElement.blur();
      }
      
      // Maximize selected session
      const selectedSession = getSelectedSession();
        if (selectedSession) {
          toggleSessionMaximize(selectedSession);
        }
    }
    
    // Settings shortcut (S key)
    if (event.key === 's' || event.key === 'S') {
      event.preventDefault(); // Prevent default browser behavior
      
      // Toggle settings sidebar with session-aware tab selection
      if (settingsOpen) {
        // Close settings sidebar if it's open
        toggleSettingsSidebar();
      } else {
        // Open settings sidebar only if it's closed
        toggleSettingsSidebar();
      }
    }
    
    // Chat input focus shortcut (C key)
    if (event.key === 'c' || event.key === 'C') {
      event.preventDefault(); // Prevent default browser behavior
      
      // Focus on the chat input textarea
      const chatInput = document.getElementById('llmChatInput');
      if (chatInput) {
        chatInput.focus();
      }
    }
  }
});

// Listen for fullscreen change events to handle container size properly
document.addEventListener('fullscreenchange', handleFullscreenChange);
document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
document.addEventListener('msfullscreenchange', handleFullscreenChange);

function handleFullscreenChange() {
  // Update all sessions when fullscreen changes
  const isFullscreen = document.fullscreenElement || 
                      document.webkitFullscreenElement || 
                      document.mozFullScreenElement || 
                      document.msFullscreenElement;
  
  sessions.forEach(session => {
    const imageSizeContainer = session.content.querySelector('.image-size-container');
    const screenshotOverlay = session.content.querySelector('.screenshot-overlay');
    
    if (isFullscreen) {
      // Remove inline styles when entering fullscreen to allow CSS to take over
      if (imageSizeContainer) {
        imageSizeContainer.style.width = '';
        imageSizeContainer.style.height = '';
      }
    } else {
      // Restore inline styles when exiting fullscreen
      if (imageSizeContainer && session.img.width && session.img.height) {
        imageSizeContainer.style.width = session.img.width + 'px';
        imageSizeContainer.style.height = session.img.height + 'px';
      }
      
      // Hide overlay toolbars for all sessions when exiting fullscreen
      if (screenshotOverlay) {
        screenshotOverlay.style.opacity = '0';
        screenshotOverlay.style.pointerEvents = 'none';
      }
    }
  });
}

// Test function to verify task creation works
function testTaskCreation() {
  console.log('Testing task creation...');
  handleTaskCreation('Test task message for demonstration purposes');
}

// Uncomment line below to test task creation functionality
// testTaskCreation();

// Connection line functionality
const connectionSvg = document.getElementById('connectionSvg');
let currentConnectionPath = null;
let currentPulseDots = [];

// Global animation synchronization
let animationStartTime = null;
let animationPhase = 0; // 0 to 100 for animation progress
let animationFrameId = null;
const ANIMATION_DURATION = 4000; // 4 seconds for one complete cycle
const ANIMATION_UPDATE_INTERVAL = 16; // ~60fps update rate

// Function to calculate connection path around session perimeter
function calculateConnectionPath(chatElement, sessionElement) {
  const chatRect = chatElement.getBoundingClientRect();
  const sessionRect = sessionElement.getBoundingClientRect();
  const mainContentRect = mainContent.getBoundingClientRect();
  
  // Get textbox element inside chat fieldset
  const chatInput = document.getElementById('llmChatInput');
  const inputRect = chatInput.getBoundingClientRect();
  
  // Check if we're in mobile responsive mode (width < 1100px)
  const isMobileMode = window.innerWidth < 1100;
  
  if (isMobileMode) {
    // Mobile mode: sessions are stacked vertically, route to center of left edge with same principle as desktop
    const chatStartX = inputRect.right;
    const chatStartY = inputRect.top + inputRect.height / 2;
    const sessionLeftX = sessionRect.left;
    const sessionCenterY = sessionRect.top + sessionRect.height / 2;
    
    // Calculate midpoint between chat and main content (same as desktop logic)
    const chatVisibleRight = chatRect.right;
    const mainContentVisibleLeft = mainContentRect.left + 10;
    const midPointX = chatVisibleRight + (mainContentVisibleLeft - chatVisibleRight) / 2;
    
    // Same routing principle as desktop: go to midpoint, then to session Y level, then to left edge center
    let mobilePath = `M ${chatStartX} ${chatStartY}`;
    mobilePath += ` L ${midPointX} ${chatStartY}`;                    // Go to middle point between chatbox and main-content
    mobilePath += ` L ${midPointX} ${sessionCenterY}`;                // Go to session's Y level at middle X
    mobilePath += ` L ${sessionLeftX} ${sessionCenterY}`;              // Go to center of left edge
    
    return mobilePath;
  }
  
  // Desktop mode: use original logic
  // Start from right side of textbox inside chat fieldset
  const chatStartX = inputRect.right;
  const chatStartY = inputRect.top + inputRect.height / 2;
  
  // Calculate paths for both LEFT and RIGHT edges
  const sessionCenterY = sessionRect.top + sessionRect.height / 2;
  const sessionToRightScreenPadding = (window.innerWidth - sessionRect.right) / 2;
  const sessionRightEndX = sessionRect.right + sessionToRightScreenPadding;
  
  // Calculate midpoint between chatbox and main content (excluding invisible padding)
  const chatVisibleRight = chatRect.right;
  const mainContentVisibleLeft = mainContentRect.left + 10; // Add back 10px padding to get actual visible left edge
  const midPointX = chatVisibleRight + (mainContentVisibleLeft - chatVisibleRight) / 2;
  
  // Calculate LEFT path: go to middle point, then to session's Y level, then to middle of left edge
  let leftPath = `M ${chatStartX} ${chatStartY}`;
  leftPath += ` L ${midPointX} ${chatStartY}`;                  // Go to middle point between chatbox and main-content
  leftPath += ` L ${midPointX} ${sessionCenterY}`;              // Go to session's Y level at middle X
  leftPath += ` L ${sessionRect.left} ${sessionCenterY}`;        // Go to middle of left edge
  
  // Calculate RIGHT path: go to middle point, then to TOP, then to RIGHT edge with padding, then down, then left to session edge
  let rightPath = `M ${chatStartX} ${chatStartY}`;
  rightPath += ` L ${midPointX} ${chatStartY}`;                  // Go to middle point between chatbox and main-content
  rightPath += ` L ${midPointX} ${mainContentRect.top}`;          // Go up to top edge of main-content
  rightPath += ` L ${window.innerWidth - sessionToRightScreenPadding} ${mainContentRect.top}`;  // Go to right edge at top with padding
  rightPath += ` L ${sessionRightEndX} ${sessionCenterY}`;          // Go down to session center at right edge with padding
  rightPath += ` L ${sessionRect.right} ${sessionCenterY}`;        // FINAL TURN: Go left to actual session edge
  
  // SIMPLE LOGIC: Check if session's left edge is accessible (not blocked by other sessions)
  const leftEdgeOffset = sessionRect.left - mainContentRect.left;
  const isLeftEdgeAccessible = leftEdgeOffset <= 10; // 10px or less from main content left edge means left edge is accessible
  
  // Choose path based on simple accessibility logic
  if (isLeftEdgeAccessible) {
    return leftPath;
  } else {
    return rightPath;
  }
}

// Helper function to check if a straight line path crosses main content container
function doesPathCrossMainContent(x1, y1, x2, y2, mainContentRect) {
  // Adjust main content rect to exclude padding (padding is 10px on all sides)
  const paddedRect = {
    left: mainContentRect.left + 10,
    top: mainContentRect.top + 10,
    right: mainContentRect.right - 10,
    bottom: mainContentRect.bottom - 10
  };
  
  // Check if line segment from (x1,y1) to (x2,y2) intersects with padded main content area
  return lineIntersectsRect(x1, y1, x2, y2, paddedRect.left, paddedRect.top, paddedRect.right, paddedRect.bottom);
}

// Helper function to check if a line segment intersects with a rectangle
function lineIntersectsRect(x1, y1, x2, y2, rectLeft, rectTop, rectRight, rectBottom) {
  // Check if either endpoint is inside rectangle
  if ((x1 >= rectLeft && x1 <= rectRight && y1 >= rectTop && y1 <= rectBottom) ||
      (x2 >= rectLeft && x2 <= rectRight && y2 >= rectTop && y2 <= rectBottom)) {
    return true;
  }
  
  // Check if line intersects with any of rectangle edges
  return lineIntersectsLine(x1, y1, x2, y2, rectLeft, rectTop, rectRight, rectTop) || // Top edge
         lineIntersectsLine(x1, y1, x2, y2, rectLeft, rectBottom, rectRight, rectBottom) || // Bottom edge
         lineIntersectsLine(x1, y1, x2, y2, rectLeft, rectTop, rectLeft, rectBottom) || // Left edge
         lineIntersectsLine(x1, y1, x2, y2, rectRight, rectTop, rectRight, rectBottom); // Right edge
}

// Helper function to check if two line segments intersect
function lineIntersectsLine(x1, y1, x2, y2, x3, y3, x4, y4) {
  const denom = (x1 - x2) * (y3 - y4) - (y1 - y2) * (x3 - x4);
  if (Math.abs(denom) < 0.0001) return false; // Lines are parallel
  
  const t = ((x1 - x3) * (y3 - y4) - (y1 - y3) * (x3 - x4)) / denom;
  const u = -((x1 - x2) * (y1 - y3) - (y1 - y2) * (x1 - x3)) / denom;
  
  return t >= 0 && t <= 1 && u >= 0 && u <= 1;
}

// Helper function to calculate actual distance of a path string
function calculatePathDistance(pathString) {
  const commands = pathString.match(/[ML]\s*[-+]?\d*\.?\d+\s*[-+]?\d*\.?\d+/g);
  if (!commands || commands.length < 2) return 0;
  
  let totalDistance = 0;
  let currentX = 0, currentY = 0;
  
  for (let i = 0; i < commands.length; i++) {
    const match = commands[i].match(/([ML])\s*([-+]?\d*\.?\d+)\s*([-+]?\d*\.?\d+)/);
    if (match) {
      const [, command, x, y] = match;
      const newX = parseFloat(x);
      const newY = parseFloat(y);
      
      if (i > 0) {
        const dx = newX - currentX;
        const dy = newY - currentY;
        totalDistance += Math.sqrt(dx * dx + dy * dy);
      }
      
      currentX = newX;
      currentY = newY;
    }
  }
  
  return totalDistance;
}

// Function to draw connection line
function drawConnectionLine(chatElement, sessionElement) {
  // Clear any existing connection
  clearConnectionLine();
  
  
  // Check if connectionSvg exists
  if (!connectionSvg) {
    console.error('Connection SVG element not found!');
    return;
  }
  
  const pathData = calculateConnectionPath(chatElement, sessionElement);
  
  if (!pathData || pathData.trim() === '') {
    console.error('Path data is empty, cannot draw connection line');
    return;
  }
  
  // Create SVG path
  currentConnectionPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  currentConnectionPath.setAttribute('d', pathData);
  currentConnectionPath.setAttribute('class', 'connection-path');
  
  // Use golden yellow color for user-assist mode, green for normal mode
  if (userAssistActive && userAssistTaskCard) {
    const sessionId = sessionElement.dataset.sessionId;
    const taskSessionId = userAssistTaskCard.dataset.sessionId;
    if (sessionId === taskSessionId) {
      currentConnectionPath.setAttribute('stroke', '#FFC107');
      currentConnectionPath.classList.add('user-assist');
    } else {
      currentConnectionPath.setAttribute('stroke', '#4CAF50');
    }
  } else {
    currentConnectionPath.setAttribute('stroke', '#4CAF50');
  }
  
  connectionSvg.appendChild(currentConnectionPath);
  
  // Create traveling pulse dots
  createPulseDots(pathData);
}

// Function to start synchronized animation
function startSynchronizedAnimation() {
  if (!animationStartTime) {
    animationStartTime = Date.now();
    animationPhase = 0;
  }
  
  if (!animationFrameId) {
    animationFrameId = requestAnimationFrame(updateSynchronizedAnimation);
  }
}

// Function to update synchronized animation
function updateSynchronizedAnimation() {
  const currentTime = Date.now();
  const elapsed = currentTime - animationStartTime;
  animationPhase = (elapsed % ANIMATION_DURATION) / ANIMATION_DURATION * 100;
  
  // Update all connection lines with synchronized phase
  updateAllConnectionLinesWithPhase(animationPhase);
  
  // Continue animation
  animationFrameId = requestAnimationFrame(updateSynchronizedAnimation);
}

// Function to stop synchronized animation
function stopSynchronizedAnimation() {
  if (animationFrameId) {
    cancelAnimationFrame(animationFrameId);
    animationFrameId = null;
  }
}

// Function to calculate animation values based on phase
function calculateAnimationValues(phase, isUserAssist = false) {
  // No brightness animation - always use consistent brightness across all segments
  const opacity = 0.84; // Fixed consistent brightness (matches CSS opacity)
  
  // No stroke width animation - always use fixed width
  const strokeWidth = 1;
  
  // Fixed colors without animation - use green for all lines (including user-assist)
  const strokeColor = 'rgb(76, 175, 80)';
  
  return { opacity, strokeWidth, strokeColor };
}

// Function to update all connection lines with synchronized phase (NO ANIMATION)
function updateAllConnectionLinesWithPhase(phase) {
  // NO ANIMATION - do nothing
}

// Function to update pulse dots with synchronized phase (simplified - no dots)
function updatePulseDotsWithPhase(phase) {
  // No traveling dots - just update line brightness
  // All animation is handled by updateAllConnectionLinesWithPhase
}

// Function to create line (no traveling dots)
function createPulseDots(pathData) {
  // No animation - static lines only
}

// Function to clear connection line
function clearConnectionLine() {
  if (currentConnectionPath) {
    connectionSvg.removeChild(currentConnectionPath);
    currentConnectionPath = null;
  }
  
  // Clear pulse dots
  currentPulseDots.forEach(dot => {
    if (dot.parentNode === connectionSvg) {
      connectionSvg.removeChild(dot);
    }
  });
  currentPulseDots = [];
}

// Function to update connection line when layout changes
function updateConnectionLine() {
  if (selectedSessionId) {
    const session = sessions.get(selectedSessionId);
    const chatFieldset = document.getElementById('chatFieldset');
    
    if (session && chatFieldset) {
      drawConnectionLine(chatFieldset, session.container);
    }
  } else {
    clearConnectionLine();
  }
}

// Override selectSession function to handle connection lines
const originalSelectSession = selectSession;
selectSession = function(sessionId) {
  originalSelectSession(sessionId);
  updateConnectionLine();
};

// Override updateSessionLayout to handle connection lines on layout changes
const originalUpdateSessionLayout = updateSessionLayout;
updateSessionLayout = function() {
  originalUpdateSessionLayout();
  updateConnectionLine();
};

// Override closeSession to handle connection lines
const originalCloseSession = closeSession;
closeSession = function(sessionId) {
  originalCloseSession(sessionId);
  if (selectedSessionId === sessionId) {
    selectedSessionId = null;
    clearConnectionLine();
  }
};

// Initial connection line setup
setTimeout(() => {
  if (selectedSessionId) {
    updateConnectionLine();
  }
}, 100);

// Handle window resize to update connection lines
window.addEventListener('resize', () => {
  updateConnectionLine();
});

// Function to get scrollbar width
function getScrollbarWidth() {
  // Create a temporary div to measure scrollbar width
  const outer = document.createElement('div');
  outer.style.visibility = 'hidden';
  outer.style.overflow = 'scroll';
  outer.style.width = '100px';
  outer.style.position = 'absolute';
  document.body.appendChild(outer);
  
  const inner = document.createElement('div');
  inner.style.width = '100%';
  outer.appendChild(inner);
  
  const scrollbarWidth = outer.offsetWidth - inner.offsetWidth;
  
  // Clean up
  outer.parentNode.removeChild(outer);
  
  return scrollbarWidth;
}

// Function to calculate user-assist connection path (straight line from task card to chat)
function calculateUserAssistPath(taskCardElement, chatElement) {
  const taskRect = taskCardElement.getBoundingClientRect();
  const chatRect = chatElement.getBoundingClientRect();
  const overlayRect = connectionSvg.getBoundingClientRect();
  
  // Calculate center of chat fieldset
  const chatCenterX = chatRect.left + chatRect.width / 2 - overlayRect.left;
  const chatTopY = chatRect.top - overlayRect.top;
  
  // Draw straight line from chat center to bottom edge of task card
  const taskBottomY = taskRect.bottom - overlayRect.top;
  
  // Straight line from bottom edge of task card to top center of chat
  return `M ${chatCenterX} ${chatTopY} L ${chatCenterX} ${taskBottomY}`;
}

// Function to draw user-assist connection line
function drawUserAssistConnectionLine(taskCardElement, chatElement) {
  // Clear any existing user-assist connection
  clearUserAssistConnectionLine();
  
  const pathData = calculateUserAssistPath(taskCardElement, chatElement);
  
  // Create SVG path
  userAssistConnectionPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  userAssistConnectionPath.setAttribute('d', pathData);
  userAssistConnectionPath.setAttribute('class', 'connection-path user-assist');
  userAssistConnectionPath.setAttribute('stroke', '#FFC107');
  
  connectionSvg.appendChild(userAssistConnectionPath);
  
  // Create traveling pulse dots
  createUserAssistPulseDots(pathData);
  
}

// Function to check if task card is visible in tasks container
function isTaskCardVisible(taskCard) {
  if (!taskCard) return false;
  
  const tasksContainer = document.getElementById('tasksContainer');
  if (!tasksContainer) return false;
  
  const containerRect = tasksContainer.getBoundingClientRect();
  const taskRect = taskCard.getBoundingClientRect();
  
  // Check if BOTTOM of task card is vertically visible within container
  // The connection line connects to bottom of task card, so we need bottom to be visible
  const taskTop = taskRect.top - containerRect.top;
  const taskBottom = taskRect.bottom - containerRect.top;
  const containerHeight = containerRect.height;
  
  // Task card bottom is visible if it's within visible area of container
  // We need some tolerance for bottom edge to be visible
  const bottomVisible = (taskBottom > 0 && taskBottom <= containerHeight);
  
  
  return bottomVisible;
}

// Function to update user-assist connection line
function updateUserAssistConnectionLine() {
  if (userAssistActive && userAssistTaskCard) {
    const chatFieldset = document.getElementById('chatFieldset');
    
    // Check if task card is visible
    if (isTaskCardVisible(userAssistTaskCard)) {
      drawUserAssistConnectionLine(userAssistTaskCard, chatFieldset);
    } else {
      // Task card is not visible, hide connection line
      clearUserAssistConnectionLine();
    }
  }
}

// Function to update connection line when layout changes
function updateConnectionLine() {
  if (selectedSessionId) {
    const session = sessions.get(selectedSessionId);
    const chatFieldset = document.getElementById('chatFieldset');
    
    if (session && chatFieldset) {
      drawConnectionLine(chatFieldset, session.container);
    }
  } else {
    clearConnectionLine();
  }
  
  // Also update user-assist connection line if active
  updateUserAssistConnectionLine();
}

// Handle scroll to update connection lines
window.addEventListener('scroll', () => {
  updateConnectionLine();
}, { passive: true });

// Handle main content scroll to update connection lines for dynamic following
const mainContentScrollHandler = () => {
  updateConnectionLine();
};

if (mainContent) {
  mainContent.addEventListener('scroll', mainContentScrollHandler, { passive: true });
}

// Handle tasks container scroll to update user-assist connection line
const tasksContainer = document.getElementById('tasksContainer');
if (tasksContainer) {
  tasksContainer.addEventListener('scroll', () => {
    updateUserAssistConnectionLine();
  }, { passive: true });
}

// Handle resize to update all connection lines
window.addEventListener('resize', () => {
  updateConnectionLine();
});

// Handle window resize to detect responsive mode changes
let previousWidth = window.innerWidth;
window.addEventListener('resize', () => {
  const currentWidth = window.innerWidth;
  
  // Check if we crossed the 1100px threshold
  const wasDesktop = previousWidth >= 1100;
  const isDesktop = currentWidth >= 1100;
  
  if (wasDesktop !== isDesktop) {
    // Force connection line update when switching between responsive modes
    setTimeout(() => {
      updateConnectionLine();
      updateUserAssistConnectionLine();
    }, 100); // Small delay to ensure layout has settled
  }
  
  previousWidth = currentWidth;
});

// Settings Sidebar Functionality
const settingsBtn = document.getElementById('settingsBtn');
const settingsSidebar = document.getElementById('settingsSidebar');
const settingsContainer = document.getElementById('settingsContainer');
const settingsCloseBtn = document.getElementById('settingsCloseBtn');
let settingsOpen = false;

// Toggle settings sidebar
function toggleSettingsSidebar() {
  settingsOpen = !settingsOpen;
  const mainWrapper = document.querySelector('.main-wrapper');
  
  if (settingsOpen) {
    // Open sidebar
    settingsContainer.classList.add('open');
    // Add mobile mode to main content
    mainContent.classList.add('mobile-mode');
    // Add settings-open class to main wrapper
    mainWrapper.classList.add('settings-open');
    
    // Ensure no sessions are automatically maximized when settings open
    sessions.forEach((session) => {
      if (session.container.classList.contains('maximized')) {
        // If any session is maximized, restore normal layout
        toggleSessionMaximize(session);
      }
    });
  } else {
    // Close sidebar
    settingsContainer.classList.remove('open');
    // Remove mobile mode from main content
    mainContent.classList.remove('mobile-mode');
    // Remove settings-open class from main wrapper
    mainWrapper.classList.remove('settings-open');
  }
}

// Listen for transition end on settings container
settingsContainer.addEventListener('transitionend', function(event) {
  if (event.propertyName === 'transform') {
    // Final update connection lines when transition completes
    updateConnectionLine();
    updateUserAssistConnectionLine();
  }
});

// Continuous update during transition
function updateConnectionLinesDuringTransition() {
  if (settingsContainer.classList.contains('open') || settingsContainer.classList.contains('open') === false) {
    updateConnectionLine();
    updateUserAssistConnectionLine();
    requestAnimationFrame(updateConnectionLinesDuringTransition);
  }
}

// Start continuous updates when toggling
const originalToggleSettingsSidebar = toggleSettingsSidebar;
toggleSettingsSidebar = function() {
  originalToggleSettingsSidebar();
  requestAnimationFrame(updateConnectionLinesDuringTransition);
};

// Settings button click event
if (settingsBtn) {
  settingsBtn.addEventListener('click', () => {
    toggleSettingsSidebar();
    // Remove focus from button to prevent outline
    settingsBtn.blur();
  });
}

// Settings close button click event
if (settingsCloseBtn) {
  settingsCloseBtn.addEventListener('click', () => {
    toggleSettingsSidebar();
    // Remove focus from close button to prevent outline
    settingsCloseBtn.blur();
  });
}

// Settings tabs functionality with session-specific state
function initSettingsTabs() {
  const tabButtons = document.querySelectorAll('.settings-tab');
  const tabContents = document.querySelectorAll('.settings-tab-content');
  
  tabButtons.forEach(button => {
    button.addEventListener('click', () => {
      const targetTab = button.getAttribute('data-tab');
      
      // Store selected tab for current session
      if (selectedSessionId) {
        setLastUsedTab(selectedSessionId, targetTab);
      }
      
      // Remove active class from all tabs and contents
      tabButtons.forEach(btn => btn.classList.remove('active'));
      tabContents.forEach(content => content.classList.remove('active'));
      
      // Add active class to clicked tab and corresponding content
      button.classList.add('active');
      document.getElementById(`${targetTab}-tab`).classList.add('active');
      
      // Initialize tab content
      initializeTabContent(targetTab);
    });
  });
}

// Initialize settings tabs when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  initSettingsTabs();
  
  // Add event listener for logs tab
  const logsTab = document.querySelector('[data-tab="logs"]');
  if (logsTab) {
    logsTab.addEventListener('click', () => {
      // Start log streaming when logs tab is selected
      startLogStreaming();
    });
  }
  
  // Add event listener for execution tab to stop log streaming
  const executionTab = document.querySelector('[data-tab="execution"]');
  if (executionTab) {
    executionTab.addEventListener('click', () => {
      // Stop log streaming for current session when switching away from logs tab
      const currentSessionId = selectedSessionId;
      if (currentSessionId) {
        const sessionState = getSessionState(currentSessionId);
        if (sessionState.isStreaming) {
          console.log(`Stopping log streaming for session ${currentSessionId} when switching to execution tab`);
          updateSessionState(currentSessionId, { isStreaming: false });
        }
      }
    });
  }
  
  // Add event listener for notifications tab to stop log streaming
  const notificationsTab = document.querySelector('[data-tab="notifications"]');
  if (notificationsTab) {
    notificationsTab.addEventListener('click', () => {
      // Stop log streaming for current session when switching to notifications tab
      const currentSessionId = selectedSessionId;
      if (currentSessionId) {
        const sessionState = getSessionState(currentSessionId);
        if (sessionState.isStreaming) {
          console.log(`Stopping log streaming for session ${currentSessionId} when switching to notifications tab`);
          updateSessionState(currentSessionId, { isStreaming: false });
        }
      }
    });
  }
  
  // Settings button click event will be set up in DOMContentLoaded
  // Settings close button click event will be set up in DOMContentLoaded
});

// Close settings sidebar when Escape key is pressed
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && settingsOpen) {
    toggleSettingsSidebar();
  }
});

// Settings tabs hotkey navigation with Spacebar + h/l
let spacePressed = false;
document.addEventListener('keydown', (event) => {
  // Check if chat input or IP input is focused
  const isChatInputFocused = document.activeElement === document.getElementById('llmChatInput');
  const isIpInputFocused = document.activeElement === document.getElementById('ipv4');
  
  // Track spacebar press state (only when not in input fields)
  if ((event.key === ' ' || event.code === 'Space') && !isChatInputFocused && !isIpInputFocused) {
    // Prevent default spacebar behavior (scrolling)
    event.preventDefault();
    spacePressed = true;
    return;
  }
  
  // Only process h/l keys when spacebar is pressed, settings sidebar is open, and not in input fields
  if (spacePressed && settingsOpen && !isChatInputFocused && !isIpInputFocused) {
    const activeTab = document.querySelector('.settings-tab.active');
    if (!activeTab) return;
    
    const allTabs = Array.from(document.querySelectorAll('.settings-tab'));
    const currentIndex = allTabs.indexOf(activeTab);
    
    let targetIndex = currentIndex;
    
    // Handle vim-like navigation with no wraparound
    if (event.key === 'h' || event.key === 'H') {
      // Move left (no wraparound)
      if (currentIndex > 0) {
        targetIndex = currentIndex - 1;
      }
      event.preventDefault();
    } else if (event.key === 'l' || event.key === 'L') {
      // Move right (no wraparound)
      if (currentIndex < allTabs.length - 1) {
        targetIndex = currentIndex + 1;
      }
      event.preventDefault();
    }
    
    // Only switch if we actually changed tabs
    if (targetIndex !== currentIndex) {
      const targetTab = allTabs[targetIndex];
      const targetTabName = targetTab.getAttribute('data-tab');
      
      // Store selected tab for current session
      if (selectedSessionId) {
        setLastUsedTab(selectedSessionId, targetTabName);
      }
      
      // Remove active class from all tabs and contents
      allTabs.forEach(btn => btn.classList.remove('active'));
      document.querySelectorAll('.settings-tab-content').forEach(content => content.classList.remove('active'));
      
      // Add active class to target tab and corresponding content
      targetTab.classList.add('active');
      document.getElementById(`${targetTabName}-tab`).classList.add('active');
      
      // Initialize tab content
      initializeTabContent(targetTabName);
    }
  }
});

// Reset spacebar state when key is released
document.addEventListener('keyup', (event) => {
  if (event.key === ' ' || event.code === 'Space') {
    spacePressed = false;
  }
});

// Vim-style session navigation with Ctrl+hjkl
document.addEventListener('keydown', (event) => {
  // Check if chat input or IP input is focused
  const isChatInputFocused = document.activeElement === document.getElementById('llmChatInput');
  const isIpInputFocused = document.activeElement === document.getElementById('ipv4');
  
  // Only process vim keys when Ctrl is pressed and not in input fields
  if (event.ctrlKey && !isChatInputFocused && !isIpInputFocused) {
    let direction = null;
    
    // Map vim keys to directions
    switch(event.key.toLowerCase()) {
      case 'h':
        direction = 'left';
        event.preventDefault();
        break;
      case 'j':
        direction = 'down';
        event.preventDefault();
        break;
      case 'k':
        direction = 'up';
        event.preventDefault();
        break;
      case 'l':
        direction = 'right';
        event.preventDefault();
        break;
    }
    
    if (direction) {
      navigateSession(direction);
    }
  }
});

// Function to get session grid layout information
function getSessionGridLayout() {
  const sessionContainers = Array.from(document.querySelectorAll('.session-container:not([style*="display: none"])'));
  const mainContent = document.getElementById('mainContent');
  
  if (sessionContainers.length === 0) return null;
  
  // Check if we're in mobile mode (sessions stacked vertically)
  const isMobileMode = window.innerWidth < 1100;
  
  if (isMobileMode) {
    // Mobile mode: sessions are stacked vertically
    return {
      type: 'vertical',
      rows: sessionContainers.length,
      cols: 1,
      grid: sessionContainers.map((container, index) => ({
        element: container,
        row: index,
        col: 0,
        sessionId: container.dataset.sessionId
      }))
    };
  }
  
  // Desktop mode: determine grid layout based on session count
  const sessionCount = sessionContainers.length;
  let cols, rows;
  
  // Use same logic as CSS grid layout
  if (sessionCount === 1) {
    cols = 1; rows = 1;
  } else if (sessionCount === 2) {
    cols = 2; rows = 1;
  } else if (sessionCount === 3) {
    cols = 2; rows = 2;
  } else if (sessionCount === 4) {
    cols = 2; rows = 2;
  } else if (sessionCount === 5) {
    cols = 3; rows = 2;
  } else if (sessionCount === 6) {
    cols = 3; rows = 2;
  } else {
    // Fallback for more sessions
    cols = Math.ceil(Math.sqrt(sessionCount));
    rows = Math.ceil(sessionCount / cols);
  }
  
  // Get actual positions from DOM to determine grid positions
  const grid = [];
  const positions = [];
  
  sessionContainers.forEach((container, index) => {
    const rect = container.getBoundingClientRect();
    const mainRect = mainContent.getBoundingClientRect();
    
    // Calculate relative position
    const relativeTop = rect.top - mainRect.top;
    const relativeLeft = rect.left - mainRect.left;
    
    positions.push({
      element: container,
      sessionId: container.dataset.sessionId,
      index: index,
      top: relativeTop,
      left: relativeLeft,
      width: rect.width,
      height: rect.height
    });
  });
  
  // Sort by top, then left to determine grid positions
  positions.sort((a, b) => {
    if (Math.abs(a.top - b.top) < 50) { // Same row (within 50px tolerance)
      return a.left - b.left;
    }
    return a.top - b.top;
  });
  
  // Determine grid layout based on actual positions
  let currentRow = 0;
  let currentCol = 0;
  let lastTop = -1;
  
  positions.forEach((pos, index) => {
    if (lastTop !== -1 && Math.abs(pos.top - lastTop) > 50) {
      // New row
      currentRow++;
      currentCol = 0;
    }
    
    grid.push({
      element: pos.element,
      row: currentRow,
      col: currentCol,
      sessionId: pos.sessionId,
      index: pos.index
    });
    
    currentCol++;
    lastTop = pos.top;
  });
  
  // Determine actual grid dimensions
  const actualRows = Math.max(...grid.map(item => item.row)) + 1;
  const actualCols = Math.max(...grid.map(item => item.col)) + 1;
  
  return {
    type: 'grid',
    rows: actualRows,
    cols: actualCols,
    grid: grid
  };
}

// Function to find session in a specific direction
function findSessionInDirection(direction) {
  if (!selectedSessionId || sessions.size === 0) return null;
  
  const layout = getSessionGridLayout();
  if (!layout) return null;
  
  // Find current session in grid
  const currentSession = layout.grid.find(item => item.sessionId === selectedSessionId);
  if (!currentSession) return null;
  
  let targetRow = currentSession.row;
  let targetCol = currentSession.col;
  
  // Calculate target position based on direction
  switch(direction) {
    case 'up':
      targetRow--;
      break;
    case 'down':
      targetRow++;
      break;
    case 'left':
      targetCol--;
      break;
    case 'right':
      targetCol++;
      break;
  }
  
  // Find session at target position
  const targetSession = layout.grid.find(item =>
    item.row === targetRow && item.col === targetCol
  );
  
  if (targetSession) {
    return targetSession.sessionId;
  }
  
  // Smart navigation: if exact position not found, find closest session in direction
  return findClosestSessionInDirection(layout, currentSession, direction);
}

// Function to find closest session in a direction when exact position doesn't exist
function findClosestSessionInDirection(layout, currentSession, direction) {
  const candidates = layout.grid.filter(item => item.sessionId !== currentSession.sessionId);
  
  if (candidates.length === 0) return null;
  
  let bestCandidate = null;
  let bestScore = Infinity;
  
  candidates.forEach(candidate => {
    let score = Infinity;
    
    switch(direction) {
      case 'up':
        if (candidate.row < currentSession.row) {
          const rowDiff = currentSession.row - candidate.row;
          const colDiff = Math.abs(candidate.col - currentSession.col);
          score = rowDiff * 100 + colDiff; // Prioritize rows over columns
        }
        break;
      case 'down':
        if (candidate.row > currentSession.row) {
          const rowDiff = candidate.row - currentSession.row;
          const colDiff = Math.abs(candidate.col - currentSession.col);
          score = rowDiff * 100 + colDiff;
        }
        break;
      case 'left':
        if (candidate.col < currentSession.col) {
          const colDiff = currentSession.col - candidate.col;
          const rowDiff = Math.abs(candidate.row - currentSession.row);
          score = colDiff * 100 + rowDiff;
        }
        break;
      case 'right':
        if (candidate.col > currentSession.col) {
          const colDiff = candidate.col - currentSession.col;
          const rowDiff = Math.abs(candidate.row - currentSession.row);
          score = colDiff * 100 + rowDiff;
        }
        break;
    }
    
    if (score < bestScore) {
      bestScore = score;
      bestCandidate = candidate;
    }
  });
  
  return bestCandidate ? bestCandidate.sessionId : null;
}

// Main navigation function
function navigateSession(direction) {
  // If no session is currently selected, select first one
  if (!selectedSessionId && sessions.size > 0) {
    const firstSessionId = Array.from(sessions.keys())[0];
    selectSession(firstSessionId);
    return;
  }
  
  const targetSessionId = findSessionInDirection(direction);
  
  if (targetSessionId) {
    selectSession(targetSessionId);
    
    // Scroll selected session into view if needed
    const targetSession = sessions.get(targetSessionId);
    if (targetSession && targetSession.container) {
      targetSession.container.scrollIntoView({
        behavior: 'smooth',
        block: 'nearest',
        inline: 'nearest'
      });
    }
  }
}

// COMPLETELY REWRITTEN LOG HANDLING SYSTEM FOR PROPER SESSION ISOLATION

// Function to handle log messages from WebSocket - COMPLETELY REWRITTEN
function handleLogMessage(logData, sessionId) {
  // Commented out to improve performance - log messages were slowing down the system
  // console.log(`Log message received for session ${sessionId}:`, logData);
  
  // CRITICAL FIX: Only process logs for valid sessions
  if (!sessionId) {
    console.warn('Log message received without session ID, ignoring');
    return;
  }
  
  // CRITICAL FIX: Always add logs to their respective session state FIRST
  // This ensures we don't lose any logs even if session is not selected
  const sessionState = getSessionState(sessionId);
  const currentLogs = sessionState.logs || [];
  currentLogs.push(logData);
  updateSessionState(sessionId, { logs: currentLogs });
  
  // CRITICAL FIX: Only update display if this is the currently selected session AND logs tab is active
  // This is the key to preventing log leakage between sessions
  if (sessionId === selectedSessionId) {
    const logsTab = document.getElementById('logs-tab');
    const logsTabIsActive = logsTab && logsTab.classList.contains('active');
    
    if (logsTabIsActive) {
      // CRITICAL FIX: Force complete refresh to prevent cross-contamination
      // Always rebuild from scratch to ensure no cross-session contamination
      updateLogsDisplayFromState();
    }
  }
}

// Function to update logs display - COMPLETELY REWRITTEN FOR ISOLATION
function updateLogsDisplayFromState() {
  const logsTab = document.getElementById('logs-tab');
  if (!logsTab) return;
  
  // Get current session state FIRST to ensure we have the right session
  const currentSessionId = selectedSessionId;
  if (!currentSessionId) {
    // CRITICAL FIX: If no session is selected, clear logs display
    logsTab.innerHTML = '<div class="logs-status">No session selected</div>';
    return;
  }
  
  // CRITICAL FIX: ALWAYS clear and rebuild logs display when updating
  // This prevents any possibility of cross-contamination between sessions
  logsTab.innerHTML = '';
  
  // Create fresh logs container and content
  const logsContainer = document.createElement('div');
  logsContainer.className = 'logs-container';
  
  const logsPre = document.createElement('pre');
  logsPre.className = 'logs-content';
  
  logsContainer.appendChild(logsPre);
  logsTab.appendChild(logsContainer);
  
  // Get session state and logs
  const sessionState = getSessionState(currentSessionId);
  const sessionLogs = sessionState.logs || [];
  
  // CRITICAL FIX: Only display logs for the CURRENT session
  // This is the absolute key to preventing log leakage
  if (sessionLogs.length > 0) {
    logsPre.textContent = sessionLogs.join('');
  } else {
    logsPre.textContent = '';
  }
  
  // CRITICAL FIX: Set current displaying session to ensure consistency
  logsTab.dataset.currentDisplayingSession = currentSessionId;
  
  // Auto-scroll to bottom
  logsPre.scrollTop = logsPre.scrollHeight;
  
  // Update read count to current position
  updateSessionState(currentSessionId, {
    lastReadLogCount: sessionLogs.length
  });
}

// Function to start log streaming - COMPLETELY REWRITTEN
function startLogStreaming() {
  const currentSessionId = selectedSessionId;
  if (!currentSessionId) {
    console.warn('Cannot start log streaming: no session selected');
    return;
  }
  
  // Set streaming state for current session only
  updateSessionState(currentSessionId, { isStreaming: true });
  
  const logsTab = document.getElementById('logs-tab');
  if (logsTab) {
    // CRITICAL FIX: Always update display to show all existing logs for current session
    // This ensures when opening logs tab for current session, all previous logs are displayed
    updateLogsDisplayFromState();
    
    // CRITICAL FIX: Set current displaying session to prevent race conditions
    // This ensures logs tab knows which session's logs to display
    logsTab.dataset.currentDisplayingSession = currentSessionId;
  }
}

// Function to stop log streaming - COMPLETELY REWRITTEN
function stopLogStreaming() {
  const currentSessionId = selectedSessionId;
  if (!currentSessionId) {
    console.warn('Cannot stop log streaming: no session selected');
    return;
  }
  
  // Stop streaming for current session only
  updateSessionState(currentSessionId, { isStreaming: false });
  
  const logsTab = document.getElementById('logs-tab');
  if (logsTab) {
    const statusDiv = logsTab.querySelector('.logs-status');
    if (statusDiv) {
      statusDiv.textContent = 'Log streaming stopped';
    }
  }
}

// Function to clear logs for current session - COMPLETELY REWRITTEN
function clearLogs() {
  const currentSessionId = selectedSessionId;
  if (!currentSessionId) {
    console.warn('Cannot clear logs: no session selected');
    return;
  }
  
  // Clear logs for current session only
  updateSessionState(currentSessionId, { logs: [] });
  
  // CRITICAL FIX: Clear display immediately
  updateLogsDisplayFromState();
}

// Function to completely reset logs for current session (for fresh start) - COMPLETELY REWRITTEN
function resetLogs() {
  const currentSessionId = selectedSessionId;
  if (!currentSessionId) {
    console.warn('Cannot reset logs: no session selected');
    return;
  }
  
  // Reset logs for current session only
  updateSessionState(currentSessionId, { logs: [] });
  
  const logsTab = document.getElementById('logs-tab');
  if (logsTab) {
    logsTab.innerHTML = '<div class="logs-status">Log streaming started...</div>';
  }
}

// Global Ctrl+U hotkey listener for user-assist toggle
document.addEventListener('keydown', (event) => {
  // Check if chat input is focused OR if settings sidebar is open
  const isChatInputFocused = document.activeElement === document.getElementById('llmChatInput');
  const isIpInputFocused = document.activeElement === document.getElementById('ipv4');
  
  // Debug logging for all Ctrl+U presses
  if (event.ctrlKey && event.key === "u") {
    console.log('Ctrl+U detected globally:', {
      isChatInputFocused,
      isIpInputFocused,
      activeElement: document.activeElement?.tagName + (document.activeElement?.id ? '#' + document.activeElement.id : '')
    });
  }
  
  // Process Ctrl+U when chat is focused and not in IP input
  if (event.ctrlKey && event.key === "u" && isChatInputFocused && !isIpInputFocused) {
    event.preventDefault();
    event.stopPropagation();
    console.log('✓ Global Ctrl+U conditions met, calling toggleUserAssistForCurrentSession');
    toggleUserAssistForCurrentSession();
  }
});
