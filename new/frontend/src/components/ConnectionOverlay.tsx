import React, { useEffect, useRef, useState } from 'react';
import './ConnectionOverlay.css';

interface Session {
  id: string;
  ip: string;
  name: string;
  selected: boolean;
  userAssistSelected: boolean;
  maximized: boolean;
  screenshot?: string;
  isConnected: boolean;
  ws?: WebSocket;
  videoInterval?: number;
  pingInterval?: number;
}

interface Task {
  id: string;
  status: string;
  message: string;
  sessionId?: string;
  sequenceNumber?: number;
  createdAt?: number;
  completedAt?: number; // Store when task was completed or cancelled
  timerInterval?: number;
  isPlaceholder?: boolean; // Flag to identify placeholder tasks
  originalMessage?: string; // Store original message to match with backend response
}

interface ConnectionOverlayProps {
  selectedSessionId: string | null;
  userAssistActive: boolean;
  userAssistTaskId?: string | null;
  sessions: Session[];
  settingsOpen?: boolean;
  tasks?: Task[];
}

interface ConnectionLine {
  id: string;
  path: string;
  userAssist: boolean;
  startX: number;
  startY: number;
  endX: number;
  endY: number;
}

const ConnectionOverlay: React.FC<ConnectionOverlayProps> = ({
  selectedSessionId,
  userAssistActive,
  userAssistTaskId,
  sessions,
  settingsOpen = false,
  tasks = []
}) => {
  const svgRef = useRef<SVGSVGElement>(null);
  const [connectionLines, setConnectionLines] = useState<ConnectionLine[]>([]);

  // Calculate connection path around session perimeter
  const calculateConnectionPath = (chatElement: HTMLElement, sessionElement: HTMLElement) => {
    const chatRect = chatElement.getBoundingClientRect();
    const sessionRect = sessionElement.getBoundingClientRect();
    const mainContentElement = document.querySelector('.main-content') as HTMLElement;
    
    if (!mainContentElement) return '';
    
    const mainContentRect = mainContentElement.getBoundingClientRect();
    
    // Get textbox element inside chat fieldset
    const chatInput = document.getElementById('llmChatInput') as HTMLTextAreaElement;
    if (!chatInput) return '';
    
    const inputRect = chatInput.getBoundingClientRect();
    
    // Check if we're in mobile responsive mode (width < 1100px)
    const isMobileMode = window.innerWidth < 1100;
    
    if (isMobileMode) {
      // Mobile mode: sessions are stacked vertically, route to center of left edge with same principle as desktop
      const chatStartX = inputRect.right;
      const chatStartY = inputRect.top + inputRect.height / 2;
      const sessionLeftX = sessionRect.left;
      const sessionCenterY = sessionRect.top + sessionRect.height / 2;
      
      // Calculate midpoint between chat and main content (exactly matching original logic)
      const chatVisibleRight = chatRect.right;
      const mainContentVisibleLeft = mainContentRect.left;
      const midPointX = chatVisibleRight + (mainContentVisibleLeft - chatVisibleRight) / 2;
      
      // Route to midpoint, then to session Y level, then to left edge center
      let mobilePath = `M ${chatStartX} ${chatStartY}`;
      mobilePath += ` L ${midPointX} ${chatStartY}`;
      mobilePath += ` L ${midPointX} ${sessionCenterY}`;
      mobilePath += ` L ${sessionLeftX} ${sessionCenterY}`;
      
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
    
    // Calculate midpoint between chatbox and main content (exactly matching original)
    const chatVisibleRight = chatRect.right;
    const mainContentVisibleLeft = mainContentRect.left;
    const midPointX = chatVisibleRight + (mainContentVisibleLeft - chatVisibleRight) / 2;
    
    // Calculate LEFT path
    let leftPath = `M ${chatStartX} ${chatStartY}`;
    leftPath += ` L ${midPointX} ${chatStartY}`;
    leftPath += ` L ${midPointX} ${sessionCenterY}`;
    leftPath += ` L ${sessionRect.left} ${sessionCenterY}`;
    
    // Calculate RIGHT path
    let rightPath = `M ${chatStartX} ${chatStartY}`;
    rightPath += ` L ${midPointX} ${chatStartY}`;
    rightPath += ` L ${midPointX} ${mainContentRect.top}`;
    rightPath += ` L ${window.innerWidth - sessionToRightScreenPadding} ${mainContentRect.top}`;
    rightPath += ` L ${sessionRightEndX} ${sessionCenterY}`;
    rightPath += ` L ${sessionRect.right} ${sessionCenterY}`;
    
    // Check if session's left edge is accessible
    const leftEdgeOffset = sessionRect.left - mainContentRect.left;
    const isLeftEdgeAccessible = leftEdgeOffset <= 10;
    
    // Choose path based on simple accessibility logic
    return isLeftEdgeAccessible ? leftPath : rightPath;
  };

  // Calculate coordinates for connection line elements
  const calculateConnectionCoordinates = (chatElement: HTMLElement, sessionElement: HTMLElement) => {
    const chatRect = chatElement.getBoundingClientRect();
    const sessionRect = sessionElement.getBoundingClientRect();
    const isMaximized = sessionElement?.classList.contains('maximized');
    
    // Get textbox element inside chat fieldset
    const chatInput = document.getElementById('llmChatInput') as HTMLTextAreaElement;
    if (!chatInput) return { startX: 0, startY: 0, endX: 0, endY: 0 };
    
    const inputRect = chatInput.getBoundingClientRect();
    const svgRect = svgRef.current?.getBoundingClientRect();
    
    if (!svgRect) return { startX: 0, startY: 0, endX: 0, endY: 0 };
    
    // Check if we're in mobile responsive mode (width < 1100px)
    const isMobileMode = window.innerWidth < 1100;
    
    // Calculate start position (from chat input)
    const startX = inputRect.right - svgRect.left;
    const startY = inputRect.top + inputRect.height / 2 - svgRect.top;
    
    // Calculate end position (to session)
    let endX, endY;
    
    if (isMaximized) {
      // For maximized sessions, use the fixed position
      endX = sessionRect.left + sessionRect.width / 2 - svgRect.left;
      endY = sessionRect.top + sessionRect.height / 2 - svgRect.top;
    } else if (isMobileMode) {
      // Mobile mode: route to center of left edge
      const sessionLeftX = sessionRect.left - svgRect.left;
      const sessionCenterY = sessionRect.top + sessionRect.height / 2 - svgRect.top;
      
      // Calculate midpoint between chat and main content
      const chatVisibleRight = chatRect.right;
      const mainContentElement = document.querySelector('.main-content') as HTMLElement;
      const mainContentRect = mainContentElement?.getBoundingClientRect();
      
      if (mainContentRect) {
        const mainContentVisibleLeft = mainContentRect.left;
        const midPointX = chatVisibleRight + (mainContentVisibleLeft - chatVisibleRight) / 2 - svgRect.left;
        
        // Route to midpoint, then to session Y level, then to left edge center
        endX = sessionLeftX;
        endY = sessionCenterY;
      } else {
        endX = sessionLeftX;
        endY = sessionCenterY;
      }
    } else {
      // Desktop mode: use original logic
      const sessionCenterY = sessionRect.top + sessionRect.height / 2 - svgRect.top;
      
      // Calculate midpoint between chatbox and main content
      const chatVisibleRight = chatRect.right;
      const mainContentElement = document.querySelector('.main-content') as HTMLElement;
      const mainContentRect = mainContentElement?.getBoundingClientRect();
      
      if (mainContentRect) {
        const mainContentVisibleLeft = mainContentRect.left;
        const midPointX = chatVisibleRight + (mainContentVisibleLeft - chatVisibleRight) / 2 - svgRect.left;
        
        // Check if session's left edge is accessible
        const leftEdgeOffset = sessionRect.left - mainContentRect.left;
        const isLeftEdgeAccessible = leftEdgeOffset <= 10;
        
        if (isLeftEdgeAccessible) {
          // Connect to left edge
          endX = sessionRect.left - svgRect.left;
          endY = sessionCenterY;
        } else {
          // Connect to right edge
          const sessionToRightScreenPadding = (window.innerWidth - sessionRect.right) / 2;
          endX = sessionRect.right + sessionToRightScreenPadding - svgRect.left;
          endY = sessionCenterY;
        }
      } else {
        endX = sessionRect.left - svgRect.left;
        endY = sessionCenterY;
      }
    }
    
    return { startX, startY, endX, endY };
  };

  // Consolidated function to recalculate connection lines
  const recalculateConnectionLines = () => {
    if (!svgRef.current) return;

    const chatFieldsetElement = document.getElementById('chatFieldset');
    if (!chatFieldsetElement) return;

    // Start with existing connection lines to preserve user-assist connection
    setConnectionLines((prevLines: ConnectionLine[]) => {
      // Filter out only session connections, keep user-assist connections
      const nonSessionConnections = prevLines.filter(line => line.id.startsWith('user-assist-'));
      const lines: ConnectionLine[] = [...nonSessionConnections];

      // Determine which session to connect to
      let targetSessionId = selectedSessionId;

      // If user-assist is active, connect to the session associated with the user-assist task
      if (userAssistActive && userAssistTaskId && tasks) {
        const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
        if (userAssistTask && userAssistTask.sessionId) {
          targetSessionId = userAssistTask.sessionId;
        }
      }

      // Find target session
      const targetSession = sessions.find(s => s.id === targetSessionId);

      if (targetSession) {
        // Find session container element
        const sessionElement = document.querySelector(`[data-session-id="${targetSession.id}"]`) as HTMLElement;
        const isMaximized = sessionElement?.classList.contains('maximized');

        if (sessionElement) {
          // Calculate path using same logic as original
          const pathData = calculateConnectionPath(chatFieldsetElement, sessionElement);
          const coords = calculateConnectionCoordinates(chatFieldsetElement, sessionElement);

          if (pathData) {
            lines.push({
              id: `connection-${targetSession.id}`,
              path: pathData,
              userAssist: userAssistActive, // Main connection line should be golden when user-assist is active
              startX: coords.startX,
              startY: coords.startY,
              endX: coords.endX,
              endY: coords.endY
            });
          }
        }
      }

      return lines;
    });
  };

  // Update connection lines when sessions or selection changes
  useEffect(() => {
    recalculateConnectionLines();
  }, [selectedSessionId, userAssistActive, sessions, settingsOpen]);

  // Update connection lines when maximize state changes in mobile mode
  useEffect(() => {
    // Check if any session is maximized
    const hasMaximizedSession = sessions.some(s => s.maximized);
    if (hasMaximizedSession) {
      // Recalculate connection lines with a small delay to ensure DOM has updated
      const timer = setTimeout(() => {
        recalculateConnectionLines();
      }, 100);
      return () => clearTimeout(timer);
    }
  }, [sessions.map(s => s.maximized).join(',')]); // Track changes to maximize state

  // Also update connection lines when user-assist state changes (to handle golden color for main connection)
  useEffect(() => {
    recalculateConnectionLines();
  }, [userAssistActive]);

  // Handle session switching in mobile mode with a delay
  useEffect(() => {
    // Only run this when we have a selected session and we're in mobile mode
    if (!selectedSessionId) return;
    
    const isMobileMode = window.innerWidth < 1100 || settingsOpen;
    if (!isMobileMode) return;
    
    // Small delay to ensure DOM has updated after session selection
    const timer = setTimeout(() => {
      recalculateConnectionLines();
    }, 100); // Small delay to ensure DOM has updated
    
    return () => clearTimeout(timer);
  }, [selectedSessionId, sessions, settingsOpen]); // Don't include userAssistActive to avoid unnecessary recalculations

  // Update on window resize and scroll
  useEffect(() => {
    // Add debouncing for resize but not for scroll to ensure smooth updates during scrolling
    let resizeTimeout: NodeJS.Timeout;
    const debouncedResize = () => {
      clearTimeout(resizeTimeout);
      resizeTimeout = setTimeout(recalculateConnectionLines, 100);
    };

    window.addEventListener('resize', debouncedResize);
    window.addEventListener('scroll', recalculateConnectionLines); // No debouncing for scroll
    
    return () => {
      window.removeEventListener('resize', debouncedResize);
      window.removeEventListener('scroll', recalculateConnectionLines);
      clearTimeout(resizeTimeout);
    };
  }, [selectedSessionId, userAssistActive, sessions, settingsOpen]);

  // Add scroll listener for main content container in mobile mode
  useEffect(() => {
    const mainContentElement = document.getElementById('mainContent');
    
    if (!mainContentElement) return;
    
    const handleMainContentScroll = () => {
      // Check if we're in mobile mode
      const isMobileMode = window.innerWidth < 1100 || settingsOpen;
      if (!isMobileMode) return;
      
      recalculateConnectionLines();
    };

    // No debouncing for scroll to ensure smooth updates during scrolling
    mainContentElement.addEventListener('scroll', handleMainContentScroll);
    
    return () => {
      mainContentElement.removeEventListener('scroll', handleMainContentScroll);
    };
  }, [selectedSessionId, userAssistActive, sessions, settingsOpen]);

  // Calculate user-assist connection path (straight line from task card to chat)
  const calculateUserAssistPath = (taskCardElement: HTMLElement, chatElement: HTMLElement) => {
    const taskRect = taskCardElement.getBoundingClientRect();
    const chatRect = chatElement.getBoundingClientRect();
    const overlayRect = svgRef.current?.getBoundingClientRect();
    
    if (!overlayRect) return '';
    
    // Calculate center of chat fieldset
    const chatCenterX = chatRect.left + chatRect.width / 2 - overlayRect.left;
    const chatTopY = chatRect.top - overlayRect.top;
    
    // Draw straight line from chat center to bottom edge of task card
    const taskBottomY = taskRect.bottom - overlayRect.top;
    
    // Straight line from bottom edge of task card to top center of chat
    return `M ${chatCenterX} ${chatTopY} L ${chatCenterX} ${taskBottomY}`;
  };

  // Function to check if task card is visible in tasks container
  const isTaskCardVisible = (taskCard: HTMLElement) => {
    if (!taskCard) return false;
    
    const tasksContainer = document.getElementById('tasksContainer');
    if (!tasksContainer) return false;
    
    const containerRect = tasksContainer.getBoundingClientRect();
    const taskRect = taskCard.getBoundingClientRect();
    
    // Check if BOTTOM of task card is vertically visible within container
    // The connection line connects to bottom of task card, so we need bottom to be visible
    const taskBottom = taskRect.bottom - containerRect.top;
    const containerHeight = containerRect.height;
    
    // Task card bottom is visible if it's within visible area of container
    const bottomVisible = (taskBottom > 0 && taskBottom <= containerHeight);
    
    return bottomVisible;
  };

  // Update user-assist connection line
  useEffect(() => {
    if (!svgRef.current) return;
    
    // Add user-assist connection line if active
    if (userAssistActive && userAssistTaskId) {
      // Find the user-assist task
      const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
      if (userAssistTask) {
        // Find task element
        const tasksContainer = document.getElementById('tasksContainer');
        if (tasksContainer) {
          const taskElement = document.querySelector(`[data-task-id="${userAssistTaskId}"]`) as HTMLElement;
          
          if (taskElement) {
            const chatFieldsetElement = document.getElementById('chatFieldset');
            if (chatFieldsetElement) {
              const pathData = calculateUserAssistPath(taskElement, chatFieldsetElement);
              
              // Always show user-assist connection in mobile mode (when settings sidebar is open)
              // regardless of task card visibility, since the user is actively in user-assist mode
              const isMobileMode = window.innerWidth < 1100 || settingsOpen;
              
              // Always show user-assist connection when user-assist is active
              if (pathData) {
                setConnectionLines((prevLines: ConnectionLine[]) => {
                  // Remove existing user-assist connection
                  const filtered = prevLines.filter(line => line.id !== 'user-assist-connection');
                  // Add new user-assist connection AFTER main connection line to ensure it's visible
                  const userAssistConnection = {
                    id: 'user-assist-connection',
                    path: pathData,
                    userAssist: true,
                    startX: 0,
                    startY: 0,
                    endX: 0,
                    endY: 0
                  };
                  return [...filtered, userAssistConnection];
                });
              }
            }
          } else {
            // Remove user-assist connection if task element not found
            setConnectionLines((prevLines: ConnectionLine[]) =>
              prevLines.filter(line => line.id !== 'user-assist-connection')
            );
          }
        }
      }
    } else {
      // Remove user-assist connection if not active
      setConnectionLines((prevLines: ConnectionLine[]) =>
        prevLines.filter(line => line.id !== 'user-assist-connection')
      );
    }
  }, [userAssistActive, userAssistTaskId, tasks, settingsOpen]);

  // Handle tasks container scroll to update user-assist connection line
  useEffect(() => {
    const tasksContainer = document.getElementById('tasksContainer');
    if (!tasksContainer) return;
    
    const handleTasksScroll = () => {
      if (userAssistActive && userAssistTaskId) {
        // Find the user-assist task
        const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
        if (userAssistTask) {
          // Find task element
          const taskElement = document.querySelector(`[data-task-id="${userAssistTaskId}"]`) as HTMLElement;
          
          if (taskElement) {
            const chatFieldsetElement = document.getElementById('chatFieldset');
            if (chatFieldsetElement) {
              const pathData = calculateUserAssistPath(taskElement, chatFieldsetElement);
              
              if (pathData && isTaskCardVisible(taskElement)) {
                setConnectionLines((prevLines: ConnectionLine[]) => {
                  // Remove existing user-assist connection
                  const filtered = prevLines.filter(line => line.id !== 'user-assist-connection');
                  // Add new user-assist connection AFTER main connection line to ensure it's visible
                  const userAssistConnection = {
                    id: 'user-assist-connection',
                    path: pathData,
                    userAssist: true,
                    startX: 0,
                    startY: 0,
                    endX: 0,
                    endY: 0
                  };
                  return [...filtered, userAssistConnection];
                });
              } else {
                // Remove user-assist connection if task not visible
                setConnectionLines((prevLines: ConnectionLine[]) =>
                  prevLines.filter(line => line.id !== 'user-assist-connection')
                );
              }
            }
          }
        }
      }
    };

    tasksContainer.addEventListener('scroll', handleTasksScroll);
    
    return () => {
      tasksContainer.removeEventListener('scroll', handleTasksScroll);
    };
  }, [userAssistActive, userAssistTaskId, tasks, settingsOpen]);

  return (
    <div className="connection-overlay" id="connectionOverlay">
      <svg width="100%" height="100%" id="connectionSvg" ref={svgRef}>
        {connectionLines.map((line) => (
          <g key={line.id}>
            <path
              className={`connection-path ${line.userAssist ? 'user-assist-active' : ''}`}
              d={line.path}
              fill="none"
              strokeDasharray="5,5"
              stroke={line.userAssist ? '#FFC107' : '#4CAF50'}
              opacity="0.84"
            />
          </g>
        ))}
      </svg>
    </div>
  );
};

export default ConnectionOverlay;