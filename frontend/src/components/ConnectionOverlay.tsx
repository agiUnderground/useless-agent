import React, { useEffect, useRef, useState } from 'react';
import './ConnectionOverlay.css';
import { routeConnection, routeToPath, type Rect } from '../connectionRouting';

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
  isExpanded?: boolean;
  previewScale?: number;
  /** Changes whenever the session grid changes, so lines are re-measured. */
  layoutVersion?: string;
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
  tasks = [],
  isExpanded = true,
  previewScale = 1,
  layoutVersion = ''
}) => {
  const svgRef = useRef<SVGSVGElement>(null);
  const [connectionLines, setConnectionLines] = useState<ConnectionLine[]>([]);

  // Convert screen-space DOMRect into SVG-local unscaled coordinates (works when zoomed out or expanded)
  const getLocalMetrics = () => {
    const svgEl = svgRef.current;
    if (!svgEl) return null;
    const svgRect = svgEl.getBoundingClientRect();
    const unscaledW = window.innerWidth || 1280;
    // Measure the scale actually applied to the SVG instead of trusting the React prop.
    // The zoomed-out preview frame adds its own hover transform, so the effective scale can
    // differ from (and animate independently of) previewScale.
    const measuredScale = svgRect.width > 0 && unscaledW > 0 ? svgRect.width / unscaledW : 0;
    const safeScale = isExpanded
      ? 1
      : measuredScale > 0.05
        ? measuredScale
        : previewScale > 0.05
          ? previewScale
          : 1;
    const toLocal = (r: DOMRect) => ({
      left: (r.left - svgRect.left) / safeScale,
      right: (r.right - svgRect.left) / safeScale,
      top: (r.top - svgRect.top) / safeScale,
      bottom: (r.bottom - svgRect.top) / safeScale,
      width: r.width / safeScale,
      height: r.height / safeScale,
    });
    return { svgRect, unscaledW, scale: safeScale, toLocal };
  };

  // Route the chat-to-session line through free space only (see connectionRouting.ts):
  // out of the chat input, along the channel beside the session grid, then
  // along the top margin or a gap between rows, and into the selected session.
  const routeToSession = (sessionElement: HTMLElement) => {
    const metrics = getLocalMetrics();
    if (!metrics) return null;
    const { toLocal } = metrics;

    const chatFieldsetElement = document.getElementById('chatFieldset');
    const chatInput = document.getElementById('llmChatInput') as HTMLTextAreaElement | null;
    const mainContentElement = document.querySelector('.main-content') as HTMLElement | null;
    if (!chatFieldsetElement || !chatInput || !mainContentElement) return null;

    const chatRect = toLocal(chatFieldsetElement.getBoundingClientRect());
    const inputRect = toLocal(chatInput.getBoundingClientRect());
    const stageRect = toLocal(mainContentElement.getBoundingClientRect());

    // Every visible session is an obstacle for the line, the selected one is the target.
    const sessionElements = Array.from(document.querySelectorAll<HTMLElement>('.session-container'));
    const sessions: Rect[] = [];
    let targetIndex = -1;
    for (const element of sessionElements) {
      const rect = toLocal(element.getBoundingClientRect());
      if (rect.width <= 0 || rect.height <= 0) continue; // hidden session
      if (element === sessionElement) targetIndex = sessions.length;
      sessions.push(rect);
    }
    if (targetIndex === -1) return null;

    const route = routeConnection({
      start: { x: inputRect.right, y: inputRect.top + inputRect.height / 2 },
      channelX: chatRect.right + (stageRect.left - chatRect.right) / 2,
      stage: stageRect,
      sessions,
      targetIndex,
    });
    const first = route.points[0];
    const last = route.points[route.points.length - 1];
    return {
      path: routeToPath(route.points),
      startX: first.x,
      startY: first.y,
      endX: last.x,
      endY: last.y,
    };
  };

  const linesEqual = (a: ConnectionLine[], b: ConnectionLine[]) => {
    if (a.length !== b.length) return false;
    return a.every((line, index) => {
      const other = b[index];
      return (
        line.id === other.id &&
        line.path === other.path &&
        line.userAssist === other.userAssist &&
        line.startX === other.startX &&
        line.startY === other.startY &&
        line.endX === other.endX &&
        line.endY === other.endY
      );
    });
  };

  // Consolidated function to recalculate session connection lines
  const recalculateConnectionLines = () => {
    if (!svgRef.current) return;

    const chatFieldsetElement = document.getElementById('chatFieldset');
    if (!chatFieldsetElement) return;

    setConnectionLines(prevLines => {
      // Preserve the user-assist connection; it is updated separately.
      const nextLines: ConnectionLine[] = prevLines.filter(line => line.id.startsWith('user-assist-'));

      // Determine which session to connect to
      let targetSessionId = selectedSessionId;

      // If user-assist is active, connect to the session associated with the user-assist task
      if (userAssistActive && userAssistTaskId && tasks) {
        const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
        if (userAssistTask && userAssistTask.sessionId) {
          targetSessionId = userAssistTask.sessionId;
        }
      }

      const targetSession = sessions.find(s => s.id === targetSessionId);

      if (targetSession) {
        const sessionElement = document.querySelector(`[data-session-id="${targetSession.id}"]`) as HTMLElement | null;

        if (sessionElement) {
          const routed = routeToSession(sessionElement);

          if (routed) {
            nextLines.push({
              id: `connection-${targetSession.id}`,
              path: routed.path,
              userAssist: userAssistActive,
              startX: routed.startX,
              startY: routed.startY,
              endX: routed.endX,
              endY: routed.endY
            });
          }
        }
      }

      return linesEqual(nextLines, prevLines) ? prevLines : nextLines;
    });
  };

  const updateUserAssistLine = () => {
    if (!svgRef.current) return;

    if (userAssistActive && userAssistTaskId) {
      const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
      if (!userAssistTask) {
        setConnectionLines(prevLines => prevLines.filter(line => line.id !== 'user-assist-connection'));
        return;
      }

      const taskElement = document.querySelector(`[data-task-id="${userAssistTaskId}"]`) as HTMLElement | null;
      if (!taskElement) {
        setConnectionLines(prevLines => prevLines.filter(line => line.id !== 'user-assist-connection'));
        return;
      }

      const chatFieldsetElement = document.getElementById('chatFieldset');
      if (!chatFieldsetElement) return;

      // Hide the line when the task card is scrolled out of the tasks list. The live
      // zoomed-out update loop must not resurrect a line pointing at a hidden card.
      const pathData = isTaskCardVisible(taskElement)
        ? calculateUserAssistPath(taskElement, chatFieldsetElement)
        : '';

      if (pathData) {
        setConnectionLines(prevLines => {
          const nextLines = prevLines
            .filter(line => line.id !== 'user-assist-connection')
            .map(line => ({ ...line }));
          nextLines.push({
            id: 'user-assist-connection',
            path: pathData,
            userAssist: true,
            startX: 0,
            startY: 0,
            endX: 0,
            endY: 0
          });
          return linesEqual(nextLines, prevLines) ? prevLines : nextLines;
        });
      } else {
        setConnectionLines(prevLines => prevLines.filter(line => line.id !== 'user-assist-connection'));
      }
    } else {
      setConnectionLines(prevLines => prevLines.filter(line => line.id !== 'user-assist-connection'));
    }
  };

  // Update connection lines when sessions, selection, or zoom state changes
  useEffect(() => {
    recalculateConnectionLines();
    const raf = requestAnimationFrame(() => recalculateConnectionLines());
    const timer = setTimeout(() => recalculateConnectionLines(), 280);
    return () => {
      cancelAnimationFrame(raf);
      clearTimeout(timer);
    };
  }, [selectedSessionId, userAssistActive, sessions, settingsOpen, isExpanded, previewScale, layoutVersion]);

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
    let resizeTimeout: ReturnType<typeof setTimeout>;
    const debouncedResize = () => {
      clearTimeout(resizeTimeout);
      resizeTimeout = setTimeout(() => {
        recalculateConnectionLines();
        updateUserAssistLine();
      }, 100);
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
    const metrics = getLocalMetrics();
    if (!metrics) return '';
    const { toLocal } = metrics;
    const taskRect = toLocal(taskCardElement.getBoundingClientRect());
    const chatRect = toLocal(chatElement.getBoundingClientRect());
    
    // Calculate center of chat fieldset
    const chatCenterX = chatRect.left + chatRect.width / 2;
    const chatTopY = chatRect.top;
    
    // Draw straight line from chat center to bottom edge of task card
    const taskBottomY = taskRect.bottom;
    
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
    const bottomVisible = (taskBottom > 0 && taskBottom <= containerHeight + 2);
    
    return bottomVisible;
  };

  // Update user-assist connection line (including when zooming in/out)
  useEffect(() => {
    updateUserAssistLine();
    const raf = requestAnimationFrame(updateUserAssistLine);
    const timer = setTimeout(updateUserAssistLine, 280);
    return () => {
      cancelAnimationFrame(raf);
      clearTimeout(timer);
    };
  }, [userAssistActive, userAssistTaskId, tasks, settingsOpen, isExpanded, previewScale]);

  // While zoomed out, the preview frame animates its own hover transform and geometry can
  // change without a React prop change. Update lines for a short window after mount and
  // restart that window on hover/transition/resize events. Equal results reuse the previous
  // state, so stable frames do not cause re-renders.
  useEffect(() => {
    if (isExpanded) return;

    let raf = 0;
    let framesLeft = 75;

    const tick = () => {
      recalculateConnectionLines();
      updateUserAssistLine();
      if (--framesLeft > 0) {
        raf = requestAnimationFrame(tick);
      } else {
        raf = 0;
      }
    };

    const restartUpdates = () => {
      framesLeft = 75;
      if (!raf) raf = requestAnimationFrame(tick);
    };

    raf = requestAnimationFrame(tick);

    const previewFrame = svgRef.current?.closest('.workspace-preview-frame') as HTMLElement | null;
    previewFrame?.addEventListener('pointerenter', restartUpdates);
    previewFrame?.addEventListener('pointerleave', restartUpdates);
    previewFrame?.addEventListener('transitionrun', restartUpdates);
    previewFrame?.addEventListener('transitionend', restartUpdates);
    window.addEventListener('resize', restartUpdates);

    return () => {
      cancelAnimationFrame(raf);
      previewFrame?.removeEventListener('pointerenter', restartUpdates);
      previewFrame?.removeEventListener('pointerleave', restartUpdates);
      previewFrame?.removeEventListener('transitionrun', restartUpdates);
      previewFrame?.removeEventListener('transitionend', restartUpdates);
      window.removeEventListener('resize', restartUpdates);
    };
  }, [
    isExpanded,
    previewScale,
    selectedSessionId,
    userAssistActive,
    userAssistTaskId,
    sessions,
    tasks,
    settingsOpen
  ]);

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
  }, [userAssistActive, userAssistTaskId, tasks, settingsOpen, isExpanded, previewScale]);

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
              strokeWidth={isExpanded ? 1 : 1.5}
              vectorEffect="non-scaling-stroke"
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
