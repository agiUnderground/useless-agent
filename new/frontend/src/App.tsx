import React, { useState, useEffect, useRef } from 'react';
import './App.css';
import SessionContainer from './components/SessionContainer';
import Toolbar from './components/Toolbar';
import SettingsPanel from './components/SettingsPanel';
import ConnectionOverlay from './components/ConnectionOverlay';
import TasksSection from './components/TasksSection';

// Import smartScrollToTask function from TasksSection
declare global {
  function smartScrollToTask(taskCardElement: HTMLElement, tasksContainerElement: HTMLElement): void;
}

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
  sessionIp?: string; // Store the IP directly to preserve session info
  sequenceNumber?: number;
  createdAt?: number;
  completedAt?: number; // Store when task was completed or cancelled
  timerInterval?: number;
  isPlaceholder?: boolean; // Flag to identify placeholder tasks
  originalMessage?: string; // Store original message to match with backend response
}

const App: React.FC = () => {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [userAssistActive, setUserAssistActive] = useState(false);
  const [userAssistTaskId, setUserAssistTaskId] = useState<string | null>(null);
  const [userAssistTaskCard, setUserAssistTaskCard] = useState<Task | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [sessionSettingsTabs, setSessionSettingsTabs] = useState<Record<string, string>>({});
  const [sessionLogs, setSessionLogs] = useState<Record<string, string[]>>({});
  const [tokenUsage, setTokenUsage] = useState(0);
  const [networkUsage, setNetworkUsage] = useState(0);
  const [serverOffline, setServerOffline] = useState(false);
  const [tasksMaximized, setTasksMaximized] = useState(false);
  const [selectedFps, setSelectedFps] = useState(5); // Default to 5 FPS
  const [taskSequenceNumber, setTaskSequenceNumber] = useState(1); // Global task counter for sequence numbers

  const mainContentRef = useRef<HTMLDivElement>(null);
  const spaceKeyPressedRef = useRef<boolean>(false);
  
  // Monitor task status changes to deactivate user-assist
  useEffect(() => {
    if (userAssistActive && userAssistTaskId) {
      const activeTask = tasks.find(t => t.id === userAssistTaskId);
      if (activeTask && activeTask.status !== 'in-progress') {
        console.log('DEACTIVATING USER-ASSIST - active task status is no longer in-progress:', activeTask.status);
        handleDeactivateUserAssist();
      }
    }
  }, [tasks, userAssistActive, userAssistTaskId]);

  // Update session count attribute on main content
  useEffect(() => {
    if (mainContentRef.current) {
      mainContentRef.current.setAttribute('data-session-count', sessions.length.toString());
    }
  }, [sessions.length]);

  // Handle session selection
  const handleSessionSelect = (sessionId: string) => {
    // Only update if selecting a different session than the current one
    if (selectedSessionId !== sessionId) {
      setSelectedSessionId(sessionId);
      
      // Ensure this session has a settings tab (default to 'execution' if not set)
      if (!sessionSettingsTabs[sessionId]) {
        setSessionSettingsTabs(prev => ({
          ...prev,
          [sessionId]: 'execution'
        }));
      }
      
      // If user-assist is active and clicking on a different session, deactivate user-assist
      if (userAssistActive) {
        // Find the session associated with the current user-assist task
        const userAssistSessionId = userAssistTaskId ?
          tasks.find(t => t.id === userAssistTaskId)?.sessionId : null;
        
        // If clicking on a different session than the one associated with user-assist, deactivate user-assist
        if (userAssistSessionId && userAssistSessionId !== sessionId) {
          handleDeactivateUserAssist();
        }
      }
      
      // Update sessions
      setSessions(prevSessions =>
        prevSessions.map(session =>
          session.id === sessionId
            ? { ...session, selected: true }
            : { ...session, selected: false }
        )
      );
    }
    // If clicking on the same session that's already selected, do nothing to preserve user-assist mode
  };

  // Handle session close
  const handleSessionClose = (sessionId: string) => {
    const wasSelected = selectedSessionId === sessionId;
    const remainingSessions = sessions.filter(session => session.id !== sessionId);
    
    // Check if this session is associated with active user-assist
    if (userAssistActive && userAssistTaskId) {
      const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
      if (userAssistTask && userAssistTask.sessionId === sessionId) {
        // This session is associated with active user-assist, deactivate user-assist
        console.log('Deactivating user-assist because its session window was closed');
        handleDeactivateUserAssist();
      }
    }
    
    setSessions(remainingSessions);
    
    // If this was the selected session, select another one
    if (wasSelected) {
      if (remainingSessions.length > 0) {
        // Try to find the previously selected session (if any)
        const previousSelectedSession = remainingSessions.find(session => session.selected);
        
        // If found, select it; otherwise select the first available session
        const newSelectedSessionId = previousSelectedSession ?
          previousSelectedSession.id : remainingSessions[0].id;
        
        setSelectedSessionId(newSelectedSessionId);
        
        // Update sessions to reflect new selection
        setSessions(prevRemainingSessions =>
          prevRemainingSessions.map(session =>
            session.id === newSelectedSessionId
              ? { ...session, selected: true }
              : { ...session, selected: false }
          )
        );
      } else {
        // No sessions left
        setSelectedSessionId(null);
      }
    }
  };

  // Handle session maximize
  const handleSessionMaximize = (sessionId: string) => {
    setSessions(prevSessions =>
      prevSessions.map(session =>
        session.id === sessionId
          ? { ...session, maximized: !session.maximized }
          : { ...session, maximized: false }
      )
    );
  };

  // Handle user assist activation
  const handleActivateUserAssist = (taskId: string, event?: React.MouseEvent) => {
    // Find the task that was clicked
    const clickedTask = tasks.find(task => task.id === taskId);
    
    if (clickedTask) {
      // Smart scroll based on task position
      const tasksContainerElement = document.getElementById('tasksContainer') as HTMLElement;
      if (tasksContainerElement) {
        // Note: We can't use event here since it's not passed to this function
        // The scrolling will be handled by the TasksSection component instead
        console.log('User-assist activation requested for task:', clickedTask.id);
      }
      
      // Toggle user-assist only for in-progress tasks
      if (clickedTask.status === 'in-progress') {
        // Check if the session window is open
        if (clickedTask.sessionId) {
          const sessionExists = sessions.find(s => s.id === clickedTask.sessionId);
          
          if (!sessionExists && clickedTask.sessionIp) {
            // Session window is not open, try to open it first
            console.log('Session window not open, attempting to open session for task:', clickedTask.id);
            if (clickedTask.sessionId && clickedTask.sessionIp) {
              // The handleOpenSessionWindow will now handle activating user-assist when the session connects
              handleOpenSessionWindow(clickedTask.sessionId, clickedTask.sessionIp, true, clickedTask.id);
            }
          } else {
            // Session window is already open, proceed with user-assist activation
            setUserAssistActive(true);
            setUserAssistTaskId(clickedTask.id);
            setUserAssistTaskCard(clickedTask);
            
            // If task has a different session ID than currently selected session, update selection
            if (clickedTask.sessionId !== selectedSessionId) {
              if (clickedTask.sessionId) {
                setSelectedSessionId(clickedTask.sessionId);
              }
              
              // Update sessions to reflect new selection and deselect all others
              setSessions(prevSessions =>
                prevSessions.map(session =>
                  session.id === clickedTask.sessionId
                    ? { ...session, selected: true }
                    : { ...session, selected: false }
                )
              );
            }
            
            // Debug log to verify user-assist activation
            console.log('User-assist activated for task:', clickedTask.id, 'Task card:', clickedTask);
          }
        }
      } else if (userAssistActive) {
        // If user-assist is already active and clicking on a non-in-progress task, deactivate it
        handleDeactivateUserAssist();
      }
    }
  };

  // Handle user assist deactivation
  const handleDeactivateUserAssist = () => {
    setUserAssistActive(false);
    setUserAssistTaskId(null);
    setUserAssistTaskCard(null);
  };

  // Handle network update
  const handleNetworkUpdate = (bytes: number) => {
    setNetworkUsage(prev => prev + bytes);
  };

  // Handle FPS change
  const handleFpsChange = (fps: number) => {
    setSelectedFps(fps);
    
    // Restart video streams for all connected sessions with new FPS
    setSessions(prevSessions =>
      prevSessions.map(session => {
        if (session.isConnected && session.videoInterval) {
          // Clear existing interval
          clearInterval(session.videoInterval);
          
          // Start new interval with updated FPS and network tracking
          const newInterval = setInterval(async () => {
            try {
              const response = await fetch(`http://${session.ip}:8080/screenshot`);
              if (!response.ok) throw new Error(`Failed to fetch screenshot from ${session.ip}`);

              const blob = await response.blob();
              const imageUrl = URL.createObjectURL(blob);
              
              // Track network usage for this fetch
              handleNetworkUpdate(blob.size);
              
              const imgElement = document.querySelector(`[data-session-id="${session.id}"] .screenshot-img`) as HTMLImageElement;
              if (imgElement) {
                imgElement.src = imageUrl;
              }
            } catch (error) {
              console.error(`Error streaming from ${session.ip}:`, error);
            }
          }, 1000 / fps) as unknown as number;
          
          return { ...session, videoInterval: newInterval };
        }
        return session;
      })
    );
  };

  // Toast notification function
  const showToast = (message: string, type: 'error' | 'success' | 'info' = 'error') => {
    const toastContainer = document.getElementById('toastContainer');
    if (!toastContainer) return;
    
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    
    toastContainer.appendChild(toast);
    
    // Auto remove after 3 seconds
    setTimeout(() => {
      toast.classList.add('fade-out');
      setTimeout(() => {
        if (toast.parentNode) {
          toast.parentNode.removeChild(toast);
        }
      }, 300);
    }, 3000);
  };

  // Handle message send
  const handleSendMessage = async (message: string, isUserAssist: boolean): Promise<boolean> => {
    // This would be connected to the backend
    console.log('Sending message:', message, 'User assist:', isUserAssist);
    
    // Handle user-assist messages
    if (isUserAssist && message.trim()) {
      // Handle user-assist messages
      if (!userAssistTaskId) {
        console.error('No active user-assist task. Cannot send message.');
        showToast('No active user-assist task. Please activate user-assist for a task first.', 'error');
        return false;
      }
      
      const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
      if (!userAssistTask || !userAssistTask.sessionIp) {
        console.error('Cannot find session IP for user-assist task.');
        showToast('Cannot find session for user-assist task.', 'error');
        return false;
      }
      
      try {
        // Send user-assist message to backend
        const response = await fetch(`http://${userAssistTask.sessionIp}:8080/user-assist`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            taskId: userAssistTaskId,
            message: message
          })
        });
        
        if (!response.ok) {
          throw new Error(`Failed to send user-assist message: ${response.statusText}`);
        }
        
        const responseData = await response.json();
        console.log('User-assist message sent successfully:', responseData);
        
        return true; // Return true to indicate success
      } catch (error) {
        console.error('Error sending user-assist message to backend:', error);
        const errorMessage = error instanceof Error ? error.message : 'Unknown error';
        showToast(`Failed to send user-assist message: ${errorMessage}`, 'error');
        return false; // Return false to indicate failure
      }
    }
    
    // For non-user-assist messages, create a new task
    if (!isUserAssist && message.trim()) {
      // Check if a session is selected and connected
      if (!selectedSessionId) {
        console.error('No session selected. Cannot create task.');
        showToast('Please select a session before creating a task.', 'error');
        return false; // Return false to indicate failure
      }
      
      const selectedSession = sessions.find(s => s.id === selectedSessionId);
      if (!selectedSession || !selectedSession.isConnected) {
        console.error('Selected session is not connected. Cannot create task.');
        showToast('Selected session is not connected. Please connect to the session before creating a task.', 'error');
        return false; // Return false to indicate failure
      }
      
      // Create a placeholder task with temporary ID
      const tempId = `temp-${Date.now()}`;
      const placeholderTask: Task = {
        id: tempId,
        status: 'in-the-queue',
        message: message,
        sessionId: selectedSessionId,
        sessionIp: selectedSession?.ip, // Store the IP directly
        createdAt: Date.now(),
        sequenceNumber: taskSequenceNumber, // Assign current sequence number
        isPlaceholder: true, // Mark as placeholder
        originalMessage: message // Store original message for matching
      };
      
      // Increment task sequence counter
      setTaskSequenceNumber(prev => prev + 1);
      
      // Add placeholder task immediately
      setTasks(prevTasks => [...prevTasks, placeholderTask]);
      
      // Show tasks section
      const tasksSectionElement = document.getElementById('tasksSection');
      if (tasksSectionElement) {
        tasksSectionElement.classList.add('visible');
      }
      
      try {
        // Send the message to the backend to create a task
        const response = await fetch(`http://${selectedSession.ip}:8080/llm-input`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            text: message,
            sessionId: selectedSessionId
          })
        });
        
        if (!response.ok) {
          throw new Error(`Failed to send message to backend: ${response.statusText}`);
        }
        
        const responseData = await response.json();
        console.log('Task created successfully on backend:', responseData);
        
        // Update placeholder task with real task ID from backend response
        if (responseData.taskId) {
          setTasks(prevTasks =>
            prevTasks.map(task =>
              task.id === tempId
                ? { ...task, id: responseData.taskId, isPlaceholder: false }
                : task
            )
          );
        }
        
        // The backend will also send a WebSocket message with the task details
        // which will be handled by the WebSocket onmessage handler
        
        return true; // Return true to indicate success
      } catch (error) {
        console.error('Error sending message to backend:', error);
        const errorMessage = error instanceof Error ? error.message : 'Unknown error';
        showToast(`Failed to send message to backend: ${errorMessage}`, 'error');
        
        // Update placeholder task to show error state
        setTasks(prevTasks =>
          prevTasks.map(task => {
            if (task.id === tempId) {
              // Check if this is the active user-assist task and its status is changing from in-progress
              const wasInProgress = task.status === 'in-progress';
              const isNoLongerInProgress = true; // Since we're changing to 'broken' status
              
              // SIMPLIFIED CHECK: If this is the active user-assist task and new status is not in-progress, deactivate it
              if (userAssistActive && userAssistTaskId === tempId) {
                console.log('DEACTIVATING USER-ASSIST - active task status changed from in-progress to: broken');
                // Deactivate user-assist after state update
                setTimeout(() => handleDeactivateUserAssist(), 0);
              }
              
              return { ...task, status: 'broken', message: `Failed to send: ${errorMessage}`, isPlaceholder: false };
            }
            return task;
          })
        );
        
        return false; // Return false to indicate backend failure
      }
    }
    
    // For empty messages or other cases, return true
    return true;
  };

  // Handle task cancellation
  const handleCancelTask = async (taskId: string) => {
    // Find the task to get its session information
    const taskToCancel = tasks.find(task => task.id === taskId);
    
    if (taskToCancel && taskToCancel.sessionIp) {
      try {
        // Send cancellation request to backend
        const response = await fetch(`http://${taskToCancel.sessionIp}:8080/task-cancel?taskId=${taskId}`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          }
        });
        
        if (!response.ok) {
          throw new Error(`Failed to cancel task: ${response.statusText}`);
        }
        
        console.log('Task cancellation sent to backend successfully');
      } catch (error) {
        console.error('Error cancelling task on backend:', error);
        const errorMessage = error instanceof Error ? error.message : 'Unknown error';
        showToast(`Failed to cancel task on backend: ${errorMessage}`, 'error');
      }
    }
    
    // Update local task state with completion time
    setTasks(prevTasks =>
      prevTasks.map(task => {
        if (task.id === taskId) {
          // Check if this is the active user-assist task and its status is changing from in-progress
          const wasInProgress = task.status === 'in-progress';
          const isNoLongerInProgress = true; // Since we're changing to 'canceled' status
          
          // SIMPLIFIED CHECK: If this is the active user-assist task and new status is not in-progress, deactivate it
          if (userAssistActive && userAssistTaskId === taskId) {
            console.log('DEACTIVATING USER-ASSIST - active task status changed from in-progress to: canceled');
            // Deactivate user-assist after state update
            setTimeout(() => handleDeactivateUserAssist(), 0);
          }
          
          return { ...task, status: 'canceled', completedAt: Date.now() };
        }
        return task;
      })
    );
  };

  // Handle settings toggle
  const handleToggleSettings = () => {
    console.log('handleToggleSettings called, current state:', settingsOpen);
    setSettingsOpen(!settingsOpen);
    
    // When opening settings, ensure the selected session has a tab set
    if (!settingsOpen && selectedSessionId && !sessionSettingsTabs[selectedSessionId]) {
      setSessionSettingsTabs(prev => ({
        ...prev,
        [selectedSessionId]: 'execution' // Default tab
      }));
    }
  };

  // Handle settings tab navigation
  const handleSettingsTabNavigation = (direction: 'left' | 'right') => {
    if (!settingsOpen || !selectedSessionId) return;
    
    const tabs = ['execution', 'logs', 'notifications'];
    const currentTab = sessionSettingsTabs[selectedSessionId] || 'execution';
    const currentIndex = tabs.indexOf(currentTab);
    let newIndex: number;
    
    if (direction === 'left') {
      // Prevent rollover - stop at leftmost tab
      newIndex = currentIndex > 0 ? currentIndex - 1 : 0;
    } else {
      // Prevent rollover - stop at rightmost tab
      newIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : tabs.length - 1;
    }
    
    setSessionSettingsTabs(prev => ({
      ...prev,
      [selectedSessionId]: tabs[newIndex]
    }));
  };

  // Handle tasks maximize toggle
  const handleToggleTasksMaximize = () => {
    setTasksMaximized(!tasksMaximized);
    
    // Toggle classes on toolbar fieldsets
    const connectionFieldset = document.querySelector('.connection-fieldset') as HTMLElement;
    const statusFieldset = document.querySelector('.status-fieldset') as HTMLElement;
    const chatFieldset = document.getElementById('chatFieldset') as HTMLElement;
    const tasksSection = document.getElementById('tasksSection') as HTMLElement;
    
    if (connectionFieldset && statusFieldset && chatFieldset && tasksSection) {
      if (!tasksMaximized) {
        // Going to maximized state
        connectionFieldset.classList.add('connection-hidden');
        statusFieldset.classList.add('status-hidden');
        chatFieldset.classList.add('shrunk');
        tasksSection.classList.add('maximized');
      } else {
        // Going back to normal state
        connectionFieldset.classList.remove('connection-hidden');
        statusFieldset.classList.remove('status-hidden');
        chatFieldset.classList.remove('shrunk');
        tasksSection.classList.remove('maximized');
      }
    }
  };

  // Setup WebSocket connection for a session
  const setupSessionWebSocket = (session: Session) => {
    // Clear any existing ping interval
    if (session.pingInterval) {
      clearInterval(session.pingInterval);
    }
    
    // Close existing WebSocket if any
    if (session.ws) {
      session.ws.close();
    }
    
    // Create new WebSocket
    const ws = new WebSocket(`ws://${session.ip}:8080/ws`);
    
    ws.onopen = () => {
      console.log(`Connected to server ${session.ip}`);
      setSessions(prevSessions =>
        prevSessions.map(s =>
          s.id === session.id
            ? { ...s, isConnected: true, ws }
            : s
        )
      );
    };
    
    ws.onmessage = (event) => {
      const message = event.data.trim();
      
      // Check if message is a time string (format: "2006-01-02 15:04:05")
      if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(message)) {
        // This is a time string, ignore it or handle it as needed
        console.log('Received time message:', message);
        return;
      }
      
      // Try to parse as JSON
      try {
        const data = JSON.parse(message);
        
        if (data.type === 'taskUpdate') {
          // Handle task updates
          // Check if the task data is nested or flat
          const taskData = data.data || data;
          const taskId = taskData.taskId || data.taskId;
          const taskStatus = taskData.status || data.status;
          const taskMessage = taskData.message || data.message;
          
          if (!taskId || !taskStatus) {
            console.error('Invalid task update received:', data);
            return;
          }
          
          setTasks(prevTasks => {
            const existingTask = prevTasks.find(t => t.id === taskId);
            if (existingTask) {
              // Update existing task - preserve original message
              console.log(`Updating existing task ${taskId} to status: ${taskStatus}`);
              
              // If task is being marked as completed or canceled, store completion time
              const isCompleted = taskStatus === 'completed' || taskStatus === 'canceled' || taskStatus === 'broken';
              const completionTime = isCompleted ? Date.now() : existingTask.completedAt;
              
              // Check if this is the active user-assist task and its status is changing from in-progress
              const wasInProgress = existingTask.status === 'in-progress';
              const isNoLongerInProgress = taskStatus !== 'in-progress';
              
              console.log('Task update check:', {
                taskId,
                userAssistActive,
                userAssistTaskId,
                wasInProgress,
                isNoLongerInProgress,
                currentStatus: existingTask.status,
                newStatus: taskStatus
              });
              
              const updatedTasks = prevTasks.map(t =>
                t.id === taskId
                  ? { ...t, status: taskStatus, completedAt: completionTime }
                  : t
              );
              
              // SIMPLIFIED CHECK: If this is the active user-assist task and new status is not in-progress, deactivate it
              if (userAssistActive && userAssistTaskId === taskId && taskStatus !== 'in-progress') {
                console.log('DEACTIVATING USER-ASSIST - active task status changed from in-progress to:', taskStatus);
                // Deactivate user-assist immediately after state update
                handleDeactivateUserAssist();
              }
              
              return updatedTasks;
            } else {
              // Check if there's a placeholder task with the same message
              const placeholderTask = prevTasks.find(t =>
                t.isPlaceholder && t.originalMessage === taskMessage
              );
              
              if (placeholderTask) {
                // Update placeholder task with real ID and status
                console.log(`Updating placeholder task ${placeholderTask.id} to real ID ${taskId}`);
                return prevTasks.map(t =>
                  t.id === placeholderTask.id
                    ? { ...t, id: taskId, status: taskStatus, isPlaceholder: false }
                    : t
                );
              } else {
                // New task - assign next sequence number
                console.log(`Creating new task ${taskId} with status: ${taskStatus}`);
                const newTask: Task = {
                  id: taskId,
                  status: taskStatus,
                  message: taskMessage,
                  sessionId: session.id,
                  sessionIp: session.ip, // Store the IP directly
                  createdAt: Date.now(),
                  completedAt: (taskStatus === 'completed' || taskStatus === 'canceled' || taskStatus === 'broken') ? Date.now() : undefined,
                  sequenceNumber: taskSequenceNumber // Assign current sequence number
                };
                
                // Increment task sequence counter for next task
                setTaskSequenceNumber(prev => prev + 1);
                
                return [...prevTasks, newTask];
              }
            }
          });
        } else if (data.type === 'log') {
          // Handle log messages
          setSessionLogs(prevLogs => ({
            ...prevLogs,
            [session.id]: [...(prevLogs[session.id] || []), data.data]
          }));
        } else if (data.type === 'tokenUpdate') {
          // Handle token usage updates
          console.log('Updating token usage from WebSocket:', data.total);
          setTokenUsage(data.total);
        }
        
        // Also call execution engine update handler if it exists
        if ((window as any).handleExecutionEngineUpdate && typeof (window as any).handleExecutionEngineUpdate === 'function') {
          (window as any).handleExecutionEngineUpdate(data);
        }
      } catch (error) {
        console.error('Error parsing WebSocket message:', message, error);
      }
    };
    
    ws.onclose = () => {
      console.log(`Disconnected from server ${session.ip}`);
      setSessions(prevSessions =>
        prevSessions.map(s =>
          s.id === session.id
            ? { ...s, isConnected: false, ws: undefined }
            : s
        )
      );
      
      // Start ping mechanism to check when server comes back online
      startServerPing(session);
    };
    
    ws.onerror = (error) => {
      console.error(`WebSocket error for ${session.ip}:`, error);
      setServerOffline(true);
      setTimeout(() => setServerOffline(false), 1000);
    };
    
    return ws;
  };

  // Start server ping when disconnected
  const startServerPing = (session: Session) => {
    const pingInterval = setInterval(async () => {
      try {
        const response = await fetch(`http://${session.ip}:8080/ping`, {
          method: 'GET',
          signal: AbortSignal.timeout(3000)
        });
        
        if (response.ok) {
          console.log(`Server ${session.ip} is back online, attempting to reconnect...`);
          clearInterval(pingInterval);
          setupSessionWebSocket(session);
        }
      } catch (error) {
        // Server still offline
      }
    }, 1000) as unknown as number;
    
    setSessions(prevSessions =>
      prevSessions.map(s =>
        s.id === session.id
          ? { ...s, pingInterval }
          : s
      )
    );
  };

  // Handle IP connection
  const handleConnectSession = (ip: string) => {
    if (!ip.trim()) return;
    
    // Check if session with this IP already exists
    const existingSession = sessions.find(session => session.ip === ip);
    
    if (existingSession) {
      // Select existing session
      handleSessionSelect(existingSession.id);
    } else {
      // Create new session
      const newSession: Session = {
        id: Date.now().toString(),
        ip: ip,
        name: `Session: ${ip}`, // Use IP instead of random number
        selected: true,
        userAssistSelected: false,
        maximized: false,
        isConnected: false
      };
      
      setSessions(prevSessions => [...prevSessions, newSession]);
      setSelectedSessionId(newSession.id);
      
      // Setup WebSocket connection
      setupSessionWebSocket(newSession);
    }
  };

  // Handle opening session window from task card
  const handleOpenSessionWindow = (sessionId: string, sessionIp: string, activateUserAssist: boolean = false, userAssistTaskId?: string) => {
    // Check if session already exists
    const existingSession = sessions.find(session => session.id === sessionId);
    
    if (existingSession) {
      // Session exists, just select it
      handleSessionSelect(sessionId);
      
      // If user-assist should be activated, do it now
      if (activateUserAssist && userAssistTaskId) {
        setUserAssistActive(true);
        setUserAssistTaskId(userAssistTaskId);
        const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
        if (userAssistTask) {
          setUserAssistTaskCard(userAssistTask);
        }
        console.log('User-assist activated for task:', userAssistTaskId, 'after selecting existing session');
      }
    } else {
      // Session doesn't exist, create a new one
      const newSession: Session = {
        id: sessionId,
        ip: sessionIp,
        name: `Session: ${sessionIp}`,
        selected: true,
        userAssistSelected: false,
        maximized: false,
        isConnected: false
      };
      
      // Add the new session and select it
      setSessions(prevSessions => [...prevSessions, newSession]);
      setSelectedSessionId(sessionId);
      
      // Setup WebSocket connection with a callback to activate user-assist when connected
      const ws = new WebSocket(`ws://${sessionIp}:8080/ws`);
      
      ws.onopen = () => {
        console.log(`Connected to server ${sessionIp}`);
        
        // Update session state to connected
        setSessions(prevSessions =>
          prevSessions.map(s =>
            s.id === sessionId
              ? { ...s, isConnected: true, ws }
              : s
          )
        );
        
        // Now that the session is connected, activate user-assist if requested
        if (activateUserAssist && userAssistTaskId) {
          setUserAssistActive(true);
          setUserAssistTaskId(userAssistTaskId);
          const userAssistTask = tasks.find(t => t.id === userAssistTaskId);
          if (userAssistTask) {
            setUserAssistTaskCard(userAssistTask);
          }
          console.log('User-assist activated for task:', userAssistTaskId, 'after session connected');
        }
      };
      
      ws.onmessage = (event) => {
        const message = event.data.trim();
        
        // Check if message is a time string (format: "2006-01-02 15:04:05")
        if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(message)) {
          // This is a time string, ignore it or handle it as needed
          console.log('Received time message:', message);
          return;
        }
        
        // Try to parse as JSON
        try {
          const data = JSON.parse(message);
          
          if (data.type === 'taskUpdate') {
            // Handle task updates
            // Check if the task data is nested or flat
            const taskData = data.data || data;
            const taskId = taskData.taskId || data.taskId;
            const taskStatus = taskData.status || data.status;
            const taskMessage = taskData.message || data.message;
            
            if (!taskId || !taskStatus) {
              console.error('Invalid task update received:', data);
              return;
            }
            
            setTasks(prevTasks => {
              const existingTask = prevTasks.find(t => t.id === taskId);
              if (existingTask) {
                // Update existing task - preserve original message
                console.log(`Updating existing task ${taskId} to status: ${taskStatus}`);
                
                // If task is being marked as completed or canceled, store completion time
                const isCompleted = taskStatus === 'completed' || taskStatus === 'canceled' || taskStatus === 'broken';
                const completionTime = isCompleted ? Date.now() : existingTask.completedAt;
                
                // Check if this is the active user-assist task and its status is changing from in-progress
                const wasInProgress = existingTask.status === 'in-progress';
                const isNoLongerInProgress = taskStatus !== 'in-progress';
                
                console.log('Task update check (handleOpenSessionWindow):', {
                  taskId,
                  userAssistActive,
                  userAssistTaskId,
                  wasInProgress,
                  isNoLongerInProgress,
                  currentStatus: existingTask.status,
                  newStatus: taskStatus
                });
                
                const updatedTasks = prevTasks.map(t =>
                  t.id === taskId
                    ? { ...t, status: taskStatus, completedAt: completionTime }
                    : t
                );
                
                // SIMPLIFIED CHECK: If this is the active user-assist task and new status is not in-progress, deactivate it
                if (userAssistActive && userAssistTaskId === taskId && taskStatus !== 'in-progress') {
                  console.log('DEACTIVATING USER-ASSIST - active task status changed from in-progress to:', taskStatus);
                  // Deactivate user-assist immediately after state update
                  handleDeactivateUserAssist();
                }
                
                return updatedTasks;
              } else {
                // Check if there's a placeholder task with the same message
                const placeholderTask = prevTasks.find(t =>
                  t.isPlaceholder && t.originalMessage === taskMessage
                );
                
                if (placeholderTask) {
                  // Update placeholder task with real ID and status
                  console.log(`Updating placeholder task ${placeholderTask.id} to real ID ${taskId}`);
                  return prevTasks.map(t =>
                    t.id === placeholderTask.id
                      ? { ...t, id: taskId, status: taskStatus, isPlaceholder: false }
                      : t
                  );
                } else {
                  // New task - assign next sequence number
                  console.log(`Creating new task ${taskId} with status: ${taskStatus}`);
                  const newTask: Task = {
                    id: taskId,
                    status: taskStatus,
                    message: taskMessage,
                    sessionId: sessionId,
                    sessionIp: sessionIp, // Store the IP directly
                    createdAt: Date.now(),
                    completedAt: (taskStatus === 'completed' || taskStatus === 'canceled' || taskStatus === 'broken') ? Date.now() : undefined,
                    sequenceNumber: taskSequenceNumber // Assign current sequence number
                  };
                  
                  // Increment task sequence counter for next task
                  setTaskSequenceNumber(prev => prev + 1);
                  
                  return [...prevTasks, newTask];
                }
              }
            });
          } else if (data.type === 'log') {
            // Handle log messages
            setSessionLogs(prevLogs => ({
              ...prevLogs,
              [sessionId]: [...(prevLogs[sessionId] || []), data.data]
            }));
          } else if (data.type === 'tokenUpdate') {
            // Handle token usage updates
            setTokenUsage(data.total);
          }
        } catch (error) {
          console.error('Error parsing WebSocket message:', message, error);
        }
      };
      
      ws.onclose = () => {
        console.log(`Disconnected from server ${sessionIp}`);
        setSessions(prevSessions =>
          prevSessions.map(s =>
            s.id === sessionId
              ? { ...s, isConnected: false, ws: undefined }
              : s
          )
        );
        
        // Start ping mechanism to check when server comes back online
        startServerPing(newSession);
      };
      
      ws.onerror = (error) => {
        console.error(`WebSocket error for ${sessionIp}:`, error);
        setServerOffline(true);
        setTimeout(() => setServerOffline(false), 1000);
      };
      
      // Store the WebSocket in the session
      newSession.ws = ws;
      
      // Start server ping mechanism
      startServerPing(newSession);
    }
  };

  // Get selected session IP
  const getSelectedSessionIp = () => {
    const selectedSession = sessions.find(session => session.id === selectedSessionId);
    return selectedSession ? selectedSession.ip : null;
  };

  // Get in-progress task for selected session
  const getInProgressTaskForSelectedSession = () => {
    return tasks.find(task => 
      task.status === 'in-progress' && task.sessionId === selectedSessionId
    );
  };

  // Listen for events from Toolbar
  useEffect(() => {
    const handleConnectSessionEvent = (event: any) => {
      handleConnectSession(event.detail);
    };

    const handleTakeScreenshotEvent = (event: any) => {
      const selectedSession = sessions.find(s => s.id === selectedSessionId);
      if (selectedSession) {
        // Trigger screenshot for selected session
        const imgElement = document.querySelector(`[data-session-id="${selectedSession.id}"] .screenshot-img`) as HTMLImageElement;
        if (imgElement) {
          fetch(`http://${selectedSession.ip}:8080/screenshot`)
            .then(response => response.blob())
            .then(blob => {
              const imageUrl = URL.createObjectURL(blob);
              imgElement.src = imageUrl;
            })
            .catch(error => console.error('Error taking screenshot:', error));
        }
      }
    };

    const handleToggleVideoEvent = (event: any) => {
      const selectedSession = sessions.find(s => s.id === selectedSessionId);
      if (selectedSession) {
        if (selectedSession.videoInterval) {
          // Stop video
          clearInterval(selectedSession.videoInterval);
          selectedSession.videoInterval = undefined;
        } else {
          // Start video streaming
          const interval = setInterval(async () => {
            try {
              const response = await fetch(`http://${selectedSession.ip}:8080/screenshot`);
              if (!response.ok) throw new Error(`Failed to fetch screenshot from ${selectedSession.ip}`);

              const blob = await response.blob();
              const imageUrl = URL.createObjectURL(blob);
              
              const imgElement = document.querySelector(`[data-session-id="${selectedSession.id}"] .screenshot-img`) as HTMLImageElement;
              if (imgElement) {
                imgElement.src = imageUrl;
              }
            } catch (error) {
              console.error(`Error streaming from ${selectedSession.ip}:`, error);
            }
          }, 1000 / 10) as unknown as number; // 10 FPS default

          selectedSession.videoInterval = interval;
        }
      }
    };

    const handleMouseMoveEvent = (event: any) => {
      console.log('Mouse move event received:', event.detail);
      const selectedSession = sessions.find(s => s.id === selectedSessionId);
      
      if (!selectedSession) {
        console.error('Cannot send mouse move: No session selected');
        showToast('Please select a session before using mouse controls', 'error');
        return;
      }
      
      if (!selectedSession.isConnected) {
        console.error('Cannot send mouse move: Session is not connected');
        showToast('Selected session is not connected. Please wait for connection.', 'error');
        return;
      }
      
      // Use HTTP request instead of WebSocket (matching original JavaScript)
      fetch(`http://${selectedSession.ip}:8080/mouse-input?x=${event.detail.x}&y=${event.detail.y}`)
        .then(response => {
          if (!response.ok) {
            throw new Error(`Failed to send mouse move to ${selectedSession.ip}`);
          }
          console.log(`Successfully sent mouse move to ${selectedSession.ip}: x=${event.detail.x}, y=${event.detail.y}`);
        })
        .catch(error => {
          console.error(`Error sending mouse move to ${selectedSession.ip}:`, error);
          showToast(`Failed to send mouse move: ${error.message}`, 'error');
        });
    };

    const handleMouseClickEvent = (event: any) => {
      console.log('Mouse click event received:', event.detail);
      const selectedSession = sessions.find(s => s.id === selectedSessionId);
      
      if (!selectedSession) {
        console.error('Cannot send mouse click: No session selected');
        showToast('Please select a session before using mouse controls', 'error');
        return;
      }
      
      if (!selectedSession.isConnected) {
        console.error('Cannot send mouse click: Session is not connected');
        showToast('Selected session is not connected. Please wait for connection.', 'error');
        return;
      }
      
      // Use HTTP request instead of WebSocket (matching original JavaScript)
      fetch(`http://${selectedSession.ip}:8080/mouse-click`)
        .then(response => {
          if (!response.ok) {
            throw new Error(`Failed to send mouse click to ${selectedSession.ip}`);
          }
          console.log(`Successfully sent mouse click to ${selectedSession.ip}`);
        })
        .catch(error => {
          console.error(`Error sending mouse click to ${selectedSession.ip}:`, error);
          showToast(`Failed to send mouse click: ${error.message}`, 'error');
        });
    };

    document.addEventListener('connectSession', handleConnectSessionEvent);
    document.addEventListener('takeScreenshot', handleTakeScreenshotEvent);
    document.addEventListener('toggleVideo', handleToggleVideoEvent);
    document.addEventListener('mouseMove', handleMouseMoveEvent);
    document.addEventListener('mouseClick', handleMouseClickEvent);
    
    return () => {
      document.removeEventListener('connectSession', handleConnectSessionEvent);
      document.removeEventListener('takeScreenshot', handleTakeScreenshotEvent);
      document.removeEventListener('toggleVideo', handleToggleVideoEvent);
      document.removeEventListener('mouseMove', handleMouseMoveEvent);
      document.removeEventListener('mouseClick', handleMouseClickEvent);
    };
  }, [sessions, selectedSessionId]);

  // Function to navigate between sessions
  const navigateSession = (direction: string) => {
    // If no session is currently selected, select first one
    if (!selectedSessionId && sessions.length > 0) {
      const firstSessionId = sessions[0].id;
      handleSessionSelect(firstSessionId);
      return;
    }
    
    // Find currently selected session
    const currentSession = sessions.find((s: Session) => s.id === selectedSessionId);
    if (!currentSession) return;
    
    // Get all visible session elements
    const sessionElements = Array.from(document.querySelectorAll('.session-container:not([style*="display: none"])')) as HTMLElement[];
    if (sessionElements.length === 0) return;
    
    // Find index of current session in visible sessions
    const currentIndex = sessionElements.findIndex((el: HTMLElement) => el.dataset.sessionId === selectedSessionId);
    if (currentIndex === -1) return;
    
    // Calculate grid layout based on session count
    const sessionCount = sessionElements.length;
    let cols: number, rows: number;
    
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
    
    // Calculate target position based on direction
    let targetIndex = currentIndex;
    
    // Check if we're in mobile mode (sessions stacked vertically)
    const isMobileMode = window.innerWidth < 1100 || settingsOpen;
    
    // In mobile mode, simply move up/down through the stack
    if (isMobileMode) {
      if (direction === 'up') {
        targetIndex = Math.max(0, currentIndex - 1);
      } else if (direction === 'down') {
        targetIndex = Math.min(sessionElements.length - 1, currentIndex + 1);
      }
    } else {
      // Desktop mode - use grid navigation
      let targetRow = Math.floor(currentIndex / cols);
      let targetCol = currentIndex % cols;
      
      switch(direction) {
        case 'up':
          targetRow = Math.max(0, targetRow - 1);
          break;
        case 'down':
          targetRow = Math.min(rows - 1, targetRow + 1);
          break;
        case 'left':
          targetCol = Math.max(0, targetCol - 1);
          break;
        case 'right':
          targetCol = Math.min(cols - 1, targetCol + 1);
          break;
      }
      
      // Calculate new index
      targetIndex = targetRow * cols + targetCol;
    }
    
    // Make sure target index is valid
    if (targetIndex >= 0 && targetIndex < sessionElements.length) {
      const targetSessionId = sessionElements[targetIndex].dataset.sessionId;
      if (targetSessionId) {
        handleSessionSelect(targetSessionId);
        
        // Scroll selected session into view if needed
        const targetElement = sessionElements[targetIndex];
        targetElement.scrollIntoView({
          behavior: 'smooth',
          block: 'nearest',
          inline: 'nearest'
        });
        
        // In mobile mode, also ensure session is fully visible
        const isMobileMode = window.innerWidth < 1100 || settingsOpen;
        if (isMobileMode) {
          // Check if session is partially visible
          const targetRect = targetElement.getBoundingClientRect();
          const mainContentRect = mainContentRef.current?.getBoundingClientRect();
          
          if (mainContentRect && targetRect) {
            // If any part of the session is outside the visible viewport, scroll more
            const isVisible = (
              targetRect.top >= mainContentRect.top &&
              targetRect.bottom <= mainContentRect.top + mainContentRect.height
            );
            
            if (!isVisible) {
              // Add a small delay to ensure DOM has updated after selection
              setTimeout(() => {
                targetElement.scrollIntoView({
                  behavior: 'smooth',
                  block: 'center',
                  inline: 'nearest'
                });
              }, 100);
            }
          }
        }
      }
    }
  };

  // Global keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // Check if chat input or IP input is focused
      const isChatInputFocused = document.activeElement === document.getElementById('llmChatInput');
      const isIpInputFocused = document.activeElement === document.getElementById('ipv4');
      // Also check if mouse coordinate inputs are focused
      const isXCoordInputFocused = document.activeElement === document.getElementById('x-coord');
      const isYCoordInputFocused = document.activeElement === document.getElementById('y-coord');
      // Also check if any input field is focused to prevent hotkeys
      const isAnyInputFocused = isChatInputFocused || isIpInputFocused || isXCoordInputFocused || isYCoordInputFocused;
      
      // Handle ESC key to blur input fields
      if (event.key === 'Escape') {
        if (isChatInputFocused || isIpInputFocused || isXCoordInputFocused || isYCoordInputFocused) {
          event.preventDefault();
          (document.activeElement as HTMLElement).blur();
          return;
        }
      }
      
      // Track space key for Space+H/L combination
      if (event.code === 'Space') {
        if (!event.repeat) {
          spaceKeyPressedRef.current = true;
        }
        // Prevent default space bar behavior (scrolling) when settings is open and chat input is NOT focused
        if (settingsOpen && !isChatInputFocused) {
          event.preventDefault();
        }
        return;
      }
      
      // Handle Space+H/L for settings tab navigation (without releasing space)
      // Only allow these shortcuts when settings is open and chat input is NOT focused
      if (spaceKeyPressedRef.current && settingsOpen && !isChatInputFocused) {
        if (event.key === 'h' || event.key === 'H') {
          event.preventDefault();
          handleSettingsTabNavigation('left');
          return;
        } else if (event.key === 'l' || event.key === 'L') {
          event.preventDefault();
          handleSettingsTabNavigation('right');
          return;
        }
      }
      
      // Allow session navigation hotkeys even when settings is open (but not when input fields are focused)
      if (!isAnyInputFocused) {
        // Fullscreen shortcut (F key)
        if (event.key === 'f' || event.key === 'F') {
          event.preventDefault(); // Prevent default browser behavior
          
          // Check if we're already in fullscreen mode
          const isFullscreen = !!(document.fullscreenElement ||
                                (document as any).webkitFullscreenElement ||
                                (document as any).mozFullScreenElement ||
                                (document as any).msFullscreenElement);
          
          if (isFullscreen) {
            // Exit fullscreen mode
            if (document.exitFullscreen) {
              document.exitFullscreen();
            } else if ((document as any).webkitExitFullscreen) {
              (document as any).webkitExitFullscreen();
            } else if ((document as any).mozCancelFullScreen) {
              (document as any).mozCancelFullScreen();
            } else if ((document as any).msExitFullscreen) {
              (document as any).msExitFullscreen();
            }
          } else {
            // Enter fullscreen mode for selected session
            const selectedSession = sessions.find((s: Session) => s.id === selectedSessionId);
            if (selectedSession) {
              const sessionElement = document.querySelector(`[data-session-id="${selectedSession.id}"]`) as HTMLElement;
              if (sessionElement) {
                const screenshotContainer = sessionElement.querySelector('.screenshot-container') as HTMLElement;
                if (screenshotContainer) {
                  if (screenshotContainer.requestFullscreen) {
                    screenshotContainer.requestFullscreen();
                  } else if ((screenshotContainer as any).webkitRequestFullscreen) {
                    (screenshotContainer as any).webkitRequestFullscreen();
                  } else if ((screenshotContainer as any).mozRequestFullScreen) {
                    (screenshotContainer as any).mozRequestFullScreen();
                  } else if ((screenshotContainer as any).msRequestFullscreen) {
                    (screenshotContainer as any).msRequestFullscreen();
                  }
                }
              }
            }
          }
        }
        
        // Maximize shortcut (M key)
        if (event.key === 'm' || event.key === 'M') {
          event.preventDefault(); // Prevent default browser behavior
          
          // Blur any potentially focused elements to prevent outline on settings button
          if (document.activeElement) {
            (document.activeElement as HTMLElement).blur();
          }
          
          // Maximize selected session
          const selectedSession = sessions.find((s: Session) => s.id === selectedSessionId);
          if (selectedSession) {
            handleSessionMaximize(selectedSession.id);
            console.log(`Maximized session: ${selectedSession.id}`);
          } else {
            console.log('No session selected to maximize');
          }
        }
        
        // Settings shortcut (S key)
        if (event.key === 's' || event.key === 'S') {
          event.preventDefault(); // Prevent default browser behavior
          event.stopPropagation(); // Stop event propagation
          console.log('S key pressed, toggling settings'); // Add debug log
          handleToggleSettings();
        }
        
        // Chat input focus shortcut (C key)
        if (event.key === 'c' || event.key === 'C') {
          event.preventDefault(); // Prevent default browser behavior
          
          // Focus on the chat input textarea
          const chatInput = document.getElementById('llmChatInput') as HTMLTextAreaElement;
          if (chatInput) {
            chatInput.focus();
          }
        }
        
        // Tasks maximize toggle (T key)
        if (event.key === 't' || event.key === 'T') {
          event.preventDefault(); // Prevent default browser behavior
          handleToggleTasksMaximize();
        }
        
        // User-assist toggle (Ctrl+U)
        if (event.ctrlKey && (event.key === 'u' || event.key === 'U')) {
          event.preventDefault();
          event.stopPropagation();
          
          if (userAssistActive) {
            // User-assist is active, deactivate it
            console.log('✓ User-assist is active, deactivating...');
            handleDeactivateUserAssist();
          } else {
            // User-assist is not active, try to activate it for current session's in-progress task
            const selectedSession = sessions.find((s: Session) => s.id === selectedSessionId);
            if (selectedSession) {
              const inProgressTask = tasks.find((task: Task) =>
                task.status === 'in-progress' && task.sessionId === selectedSessionId
              );
              
              if (inProgressTask) {
                console.log('✓ Found in-progress task, activating user-assist:', inProgressTask.id);
                handleActivateUserAssist(inProgressTask.id);
              } else {
                console.log('✗ No in-progress task found for selected session');
              }
            } else {
              console.log('✗ No session selected');
            }
          }
        }
        
        // Clear chat with Ctrl+L
        if (event.ctrlKey && (event.key === 'l' || event.key === 'L')) {
          event.preventDefault();
          event.stopPropagation();
          
          // Clear the chat input
          const chatInput = document.getElementById('llmChatInput') as HTMLTextAreaElement;
          if (chatInput) {
            chatInput.value = '';
            // Trigger onChange event to update React state
            const changeEvent = new Event('input', { bubbles: true });
            chatInput.dispatchEvent(changeEvent);
          }
        }
        
        // Vim-style session navigation with Ctrl+hjkl
        if (event.ctrlKey && !isChatInputFocused && !isIpInputFocused && !isXCoordInputFocused && !isYCoordInputFocused) {
          let direction: string | null = null;
          
          // Check if we're in mobile mode (sessions stacked vertically)
          const isMobileMode = window.innerWidth < 1100 || settingsOpen;
          
          // Map vim keys to directions based on layout mode
          switch(event.key.toLowerCase()) {
            case 'h':
              direction = isMobileMode ? 'up' : 'left'; // In mobile mode, H goes up (previous session)
              break;
            case 'j':
              direction = 'down'; // J always goes down (next session)
              break;
            case 'k':
              direction = 'up'; // K always goes up (previous session)
              break;
            case 'l':
              direction = isMobileMode ? 'down' : 'right'; // In mobile mode, L goes down (next session)
              break;
          }
          
          // Allow vim navigation in mobile mode even when settings is open, but not in desktop mode
          if (direction && (isMobileMode || !settingsOpen)) {
            event.preventDefault();
            navigateSession(direction);
          }
        }
      }
    };
    
    const handleKeyUp = (event: KeyboardEvent) => {
      if (event.code === 'Space') {
        spaceKeyPressedRef.current = false;
      }
    };
    
    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('keyup', handleKeyUp);
    
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('keyup', handleKeyUp);
    };
  }, [sessions, selectedSessionId, userAssistActive, tasks, settingsOpen, sessionSettingsTabs]);

  return (
    <div className={`main-wrapper ${settingsOpen ? 'settings-open' : ''}`}>
      <div className="left-sidebar">
        <Toolbar
          selectedSessionIp={getSelectedSessionIp()}
          userAssistActive={userAssistActive}
          onSendMessage={handleSendMessage}
          onToggleSettings={handleToggleSettings}
          onToggleUserAssist={handleDeactivateUserAssist}
          serverOffline={serverOffline}
          tasks={tasks}
          onCancelTask={handleCancelTask}
          onActivateUserAssist={handleActivateUserAssist}
          onOpenSessionWindow={handleOpenSessionWindow}
          userAssistTaskCard={userAssistTaskCard}
          onDeactivateUserAssist={handleDeactivateUserAssist}
          networkUsage={networkUsage}
          tokenUsage={tokenUsage}
          onFpsChange={handleFpsChange}
          sessions={sessions}
          selectedSessionId={selectedSessionId}
        />
      </div>
      
      <div
        ref={mainContentRef}
        className={`main-content ${settingsOpen ? 'mobile-mode' : ''}`}
        id="mainContent"
        data-session-count={sessions.length.toString()}
      >
        {sessions.map(session => {
          // Check if this session should have golden border when user-assist is active
          // Find the task associated with user-assist
          const userAssistTask = userAssistTaskId ? tasks.find(t => t.id === userAssistTaskId) : null;
          // Find the session associated with that task
          const userAssistSessionId = userAssistTask?.sessionId;
          // Check if current session matches the user-assist session
          const shouldHaveGoldenBorder = userAssistActive && userAssistSessionId === session.id;
          
          return (
            <SessionContainer
              key={session.id}
              session={session}
              isSelected={session.id === selectedSessionId}
              isUserAssistSelected={shouldHaveGoldenBorder}
              userAssistActive={userAssistActive}
              onSelect={handleSessionSelect}
              onClose={handleSessionClose}
              onToggleMaximize={handleSessionMaximize}
              onActivateUserAssist={handleActivateUserAssist}
              onNetworkUpdate={handleNetworkUpdate}
              inProgressTask={getInProgressTaskForSelectedSession()}
              selectedFps={selectedFps}
            />
          );
        })}
      </div>
      
      <ConnectionOverlay
        selectedSessionId={selectedSessionId}
        userAssistActive={userAssistActive}
        userAssistTaskId={userAssistTaskId}
        sessions={sessions}
        settingsOpen={settingsOpen}
        tasks={tasks}
      />
      
      <SettingsPanel
        isOpen={settingsOpen}
        onClose={handleToggleSettings}
        logs={selectedSessionId ? (sessionLogs[selectedSessionId] || []) : []}
        tokenUsage={tokenUsage}
        activeTab={selectedSessionId ? (sessionSettingsTabs[selectedSessionId] || 'execution') : 'execution'}
        onTabChange={(tab) => {
          if (selectedSessionId) {
            setSessionSettingsTabs(prev => ({
              ...prev,
              [selectedSessionId]: tab
            }));
          }
        }}
        selectedSessionId={selectedSessionId}
        tasks={tasks}
      />
      
      {/* Toast notification container */}
      <div className="toast-container" id="toastContainer"></div>
    </div>
  );
};

export default App;
