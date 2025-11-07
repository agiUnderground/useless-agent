import React, { useState, useEffect, useRef } from 'react';
import { CONFIG } from '../config';
import './SettingsPanel.css';

interface SettingsPanelProps {
  isOpen: boolean;
  onClose: () => void;
  logs: string[];
  tokenUsage: number;
  activeTab?: string;
  onTabChange?: (tab: string) => void;
  selectedSessionId?: string | null;
  tasks?: Array<{ id: string; status: string; message: string; sessionId?: string; sequenceNumber?: number }>;
}

interface ExecutionEngineState {
  tasks: Array<{
    id: string;
    originalId?: string;
    status: string;
    message: string;
    sequenceNumber?: number;
    subtasks?: Array<{
      id: string;
      description: string;
      isActive: boolean;
      actions?: Array<{
        id: string;
        action: string;
        description?: string;
        coordinates?: { x: number; y: number };
        inputString?: string;
        keyTapString?: string;
        duration?: number;
      }>;
    }>;
  }>;
  selectedTask: string | null;
  queuedTasks: Array<any>;
}

const SettingsPanel: React.FC<SettingsPanelProps> = ({
  isOpen,
  onClose,
  logs,
  tokenUsage,
  activeTab: externalActiveTab,
  onTabChange: externalOnTabChange,
  selectedSessionId,
  tasks = []
}) => {
  const [internalActiveTab, setInternalActiveTab] = useState('execution');
  
  // Use external activeTab if provided, otherwise use internal state
  const activeTab = externalActiveTab || internalActiveTab;
  
  // Sync internal state when external active tab changes
  useEffect(() => {
    if (externalActiveTab) {
      setInternalActiveTab(externalActiveTab);
    }
  }, [externalActiveTab]);
  
  const handleTabChange = (tab: string) => {
    if (externalOnTabChange) {
      externalOnTabChange(tab);
    } else {
      setInternalActiveTab(tab);
    }
  };

  // Execution engine state
  const [executionEngineState, setExecutionEngineState] = useState<ExecutionEngineState>({
    tasks: [],
    selectedTask: null,
    queuedTasks: []
  });

  const executionTasksRef = useRef<HTMLDivElement>(null);
  const taskPointerRef = useRef<HTMLDivElement>(null);
  const promptContentRef = useRef<HTMLDivElement>(null);
  const timelineContentRef = useRef<HTMLDivElement>(null);
  const logsContentRef = useRef<HTMLDivElement>(null);

  // Handle tab switching
  const handleTabClick = (tab: string) => {
    handleTabChange(tab);
    
    // Initialize execution engine when switching to execution tab
    if (tab === 'execution' && selectedSessionId) {
      initializeExecutionEngine();
    }
  };

  // Handle close button click
  const handleCloseClick = () => {
    onClose();
  };

  // Initialize execution engine
  const initializeExecutionEngine = async () => {
    if (!selectedSessionId) return;
    
    try {
      // Fetch execution state from backend
      const response = await fetch(`http://${selectedSessionId}:${CONFIG.BACKEND_PORT}/execution-state`);
      if (!response.ok) {
        throw new Error(`Failed to fetch execution state: ${response.status}`);
      }
      
      const data = await response.json();
      console.log('Fetched execution state from backend:', data);
      
      // Sort tasks by sequenceNumber to maintain order and add originalId for consistency
      const sortedTasks = (data.tasks || []).map((task: any) => ({
        ...task,
        originalId: task.originalId || task.id
      })).sort((a: any, b: any) =>
        (a.sequenceNumber || 0) - (b.sequenceNumber || 0)
      );
      
      // Auto-select first in-progress task, or first task if none are in progress
      let selectedTaskId = null;
      if (sortedTasks.length > 0) {
        const inProgressTask = sortedTasks.find((t: any) => t.status === 'in-progress');
        const targetTask = inProgressTask || sortedTasks[0];
        selectedTaskId = targetTask.id;
      }
      
      // Update execution engine state with backend data
      setExecutionEngineState({
        tasks: sortedTasks,
        selectedTask: selectedTaskId,
        queuedTasks: data.queuedTasks || []
      });
      
    } catch (error) {
      console.error('Error fetching execution state:', error);
    }
  };

  // Handle task square click
  const handleTaskSquareClick = (taskId: string) => {
    setExecutionEngineState(prev => ({
      ...prev,
      selectedTask: taskId
    }));
  };

  // Handle task selector click (cycle through tasks)
  const handleTaskSelectorClick = () => {
    if (executionEngineState.tasks.length === 0) return;
    
    const currentIndex = executionEngineState.selectedTask
      ? executionEngineState.tasks.findIndex(t => t.id === executionEngineState.selectedTask)
      : -1;
    
    const nextIndex = (currentIndex + 1) % executionEngineState.tasks.length;
    const nextTaskId = executionEngineState.tasks[nextIndex].id;
    
    setExecutionEngineState(prev => ({
      ...prev,
      selectedTask: nextTaskId
    }));
  };

  // Handle execution engine WebSocket updates
  const handleExecutionEngineUpdate = (data: any) => {
    console.log('[DEBUG] handleExecutionEngineUpdate called with:', data);
    
    if (data.updateType === 'taskUpdate') {
      // Update task in our state
      const taskData = data.data;
      console.log('[DEBUG] Processing taskUpdate:', taskData);
      
      // Check if this is a new task or an update to an existing one
      // A new task should have a different ID than any existing task
      const existingTaskIndex = executionEngineState.tasks.findIndex(t =>
        t.originalId === taskData.taskId
      );
      
      let updatedTasks = [...executionEngineState.tasks];
      let updatedSelectedTask = executionEngineState.selectedTask;
      
      if (existingTaskIndex !== -1) {
        // Update existing task - just change the status and message
        updatedTasks[existingTaskIndex] = {
          ...updatedTasks[existingTaskIndex],
          status: taskData.status,
          message: taskData.message
        };
        console.log('[DEBUG] Updated existing task:', updatedTasks[existingTaskIndex]);
      } else {
        // This is a completely new task - create a new square for it
        const timestamp = Date.now();
        const newTask = {
          id: `${taskData.taskId}_${timestamp}`,
          originalId: taskData.taskId,
          status: taskData.status,
          message: taskData.message,
          sequenceNumber: taskData.sequenceNumber || updatedTasks.length + 1,
          subtasks: []
        };
        updatedTasks.push(newTask);
        console.log('[DEBUG] Added completely new task with unique ID:', newTask);
        
        // Auto-select the new task if it's the first one
        if (updatedTasks.length === 1) {
          updatedSelectedTask = newTask.id;
          console.log('[DEBUG] Auto-selecting first created task:', newTask.id);
        }
      }
      
      // Sort tasks by sequenceNumber to maintain order
      updatedTasks.sort((a, b) => (a.sequenceNumber || 0) - (b.sequenceNumber || 0));
      
      // If the updated task was just completed, auto-select the next available task
      if (existingTaskIndex !== -1 && taskData.status === 'completed' &&
          (executionEngineState.selectedTask === taskData.taskId ||
           executionEngineState.tasks.find(t => t.id === executionEngineState.selectedTask)?.originalId === taskData.taskId)) {
        const nextTask = updatedTasks.find(t => t.status !== 'completed' &&
          t.id !== executionEngineState.selectedTask &&
          t.originalId !== taskData.taskId);
        if (nextTask) {
          updatedSelectedTask = nextTask.id;
          console.log('[DEBUG] Auto-selecting next task after completion:', nextTask.id);
        }
      }
      
      // Auto-select in-progress task if no task is selected
      const inProgressTask = updatedTasks.find(t => t.status === 'in-progress');
      if (inProgressTask && !updatedSelectedTask) {
        updatedSelectedTask = inProgressTask.id;
        console.log('[DEBUG] Auto-selecting in-progress task:', inProgressTask.id);
      }
      
      setExecutionEngineState(prev => ({
        ...prev,
        tasks: updatedTasks,
        selectedTask: updatedSelectedTask
      }));
      
    } else if (data.updateType === 'subtaskUpdate') {
      // Handle subtask update
      const subtaskData = data.data;
      console.log('[DEBUG] Processing subtaskUpdate:', subtaskData);
      const taskIndex = executionEngineState.tasks.findIndex(t =>
        t.id === subtaskData.taskId || t.originalId === subtaskData.taskId
      );
      
      if (taskIndex !== -1) {
        // Ensure subtasks array exists
        setExecutionEngineState(prev => {
          const updatedTasks = [...prev.tasks];
          if (!updatedTasks[taskIndex].subtasks) {
            updatedTasks[taskIndex].subtasks = [];
          }
          
          // Find existing subtask
          const subtaskIndex = updatedTasks[taskIndex].subtasks?.findIndex(st => st.id === subtaskData.subtaskId) ?? -1;
          
          if (subtaskIndex !== -1) {
            // Update existing subtask
            const existingActions = updatedTasks[taskIndex].subtasks?.[subtaskIndex]?.actions || [];
            updatedTasks[taskIndex].subtasks![subtaskIndex] = {
              id: subtaskData.subtaskId,
              description: subtaskData.description,
              isActive: subtaskData.isActive,
              actions: existingActions
            };
            console.log('[DEBUG] Updated existing subtask:', updatedTasks[taskIndex].subtasks![subtaskIndex]);
          } else {
            // Add new subtask
            const newSubtask = {
              id: subtaskData.subtaskId,
              description: subtaskData.description,
              isActive: subtaskData.isActive,
              actions: []
            };
            updatedTasks[taskIndex].subtasks?.push(newSubtask);
            console.log('[DEBUG] Added new subtask:', newSubtask);
          }
          
          return {
            ...prev,
            tasks: updatedTasks
          };
        });
      }
      
    } else if (data.updateType === 'actionUpdate') {
      // Handle action update
      const actionData = data.data;
      console.log('[DEBUG] Processing actionUpdate:', actionData);
      const taskIndex = executionEngineState.tasks.findIndex(t =>
        t.id === actionData.taskId || t.originalId === actionData.taskId
      );
      
      if (taskIndex !== -1) {
        setExecutionEngineState(prev => {
          const updatedTasks = [...prev.tasks];
          const task = updatedTasks[taskIndex];
          
          if (!task.subtasks) {
            task.subtasks = [];
          }
          
          // Find subtask
          const subtaskIndex = task.subtasks?.findIndex(st => st.id === actionData.subtaskId) ?? -1;
          
          if (subtaskIndex !== -1) {
            // Ensure actions array exists
            if (!task.subtasks![subtaskIndex].actions) {
              task.subtasks![subtaskIndex].actions = [];
            }
            
            // Find existing action
            const actionIndex = task.subtasks![subtaskIndex].actions?.findIndex(a => a.id === actionData.actionId) ?? -1;
            
            if (actionIndex !== -1) {
              // Update existing action
              task.subtasks![subtaskIndex].actions![actionIndex] = {
                id: actionData.actionId,
                action: actionData.action.action,
                description: actionData.action.description,
                coordinates: actionData.action.coordinates,
                inputString: actionData.action.inputString,
                keyTapString: actionData.action.keyTapString,
                duration: actionData.action.duration
              };
              console.log('[DEBUG] Updated existing action:', task.subtasks![subtaskIndex].actions![actionIndex]);
            } else {
              // Add new action
              const newAction = {
                id: actionData.actionId,
                action: actionData.action.action,
                description: actionData.action.description,
                coordinates: actionData.action.coordinates,
                inputString: actionData.action.inputString,
                keyTapString: actionData.action.keyTapString,
                duration: actionData.action.duration
              };
              task.subtasks![subtaskIndex].actions?.push(newAction);
              console.log('[DEBUG] Added new action:', newAction);
            }
          }
          
          return {
            ...prev,
            tasks: updatedTasks
          };
        });
      }
      
    } else if (data.updateType === 'completionEvent') {
      // Handle task completion events
      const completionData = data.data;
      console.log('[DEBUG] Processing completionEvent:', completionData);
      
      if (completionData.event === 'completed') {
        // Update pointer position when task is completed
        setTimeout(() => {
          updateTaskPointer();
        }, 100);
      }
    }
  };

  // Expose handler for external WebSocket updates
  useEffect(() => {
    // This would be called from parent component when WebSocket messages are received
    (window as any).handleExecutionEngineUpdate = handleExecutionEngineUpdate;
    
    return () => {
      delete (window as any).handleExecutionEngineUpdate;
    };
  }, []);
  
  // Debug log to verify handler is properly set
  useEffect(() => {
    console.log('[DEBUG] Execution engine handler set:', typeof (window as any).handleExecutionEngineUpdate);
  }, []);

  // Update task pointer position
  const updateTaskPointer = () => {
    if (!taskPointerRef.current || !executionTasksRef.current) return;
    
    // Find in-progress task
    const inProgressTask = executionEngineState.tasks.find(task => task.status === 'in-progress');
    
    if (inProgressTask) {
      // Find index of in-progress task
      const taskIndex = executionEngineState.tasks.findIndex(task => task.status === 'in-progress');
      
      // Get corresponding task square
      const taskSquares = executionTasksRef.current.querySelectorAll('.task-square');
      const targetSquare = taskSquares[taskIndex];
      
      if (targetSquare) {
        // Get position of target task square
        const squareRect = targetSquare.getBoundingClientRect();
        const containerRect = executionTasksRef.current.getBoundingClientRect();
        
        // Calculate position relative to container, accounting for 6px padding
        const relativeLeft = squareRect.left - containerRect.left + (squareRect.width / 2) + 6; // Add 6px padding
        const relativeTop = squareRect.top - containerRect.top - 6; // Subtract 6px padding
        
        // Position pointer above task square
        taskPointerRef.current.style.position = 'relative';
        taskPointerRef.current.style.left = `${relativeLeft}px`;
        taskPointerRef.current.style.top = `${relativeTop - 10}px`; // 10px above task
        taskPointerRef.current.style.display = 'block';
        taskPointerRef.current.style.transform = 'translateX(-50%)'; // Center pointer
      }
    } else {
      // Hide pointer if no in-progress task
      taskPointerRef.current.style.display = 'none';
    }
  };

  useEffect(() => {
    updateTaskPointer();
  }, [executionEngineState.tasks, executionEngineState.selectedTask]);

  // Scroll logs to bottom when new logs are added
  useEffect(() => {
    if (logsContentRef.current) {
      logsContentRef.current.scrollTop = logsContentRef.current.scrollHeight;
    }
  }, [logs]);

  // Handle keyboard events for Q key to scroll to bottom of logs
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // Only handle Q key when settings panel is open and logs tab is active
      if (isOpen && activeTab === 'logs' && (event.key === 'q' || event.key === 'Q')) {
        // Check if any input field is focused
        const activeElement = document.activeElement;
        const isInputFocused = activeElement && (
          activeElement.tagName === 'INPUT' ||
          activeElement.tagName === 'TEXTAREA' ||
          (activeElement as HTMLElement).contentEditable === 'true'
        );
        
        // Only scroll to bottom if no input is focused
        if (!isInputFocused && logsContentRef.current) {
          logsContentRef.current.scrollTop = logsContentRef.current.scrollHeight;
        }
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, activeTab]);

  // Generate task squares for execution tab
  // No tasks available for this session.
  const generateTaskSquares = () => {
    if (executionEngineState.tasks.length === 0) {
      return (
        <div style={{ fontSize: '12px', color: '#888', padding: '20px', textAlign: 'center' }}>
        </div>
      );
    }

    // Sort tasks by sequenceNumber to ensure consistent order
    const sortedTasks = [...executionEngineState.tasks].sort((a, b) =>
      (a.sequenceNumber || 0) - (b.sequenceNumber || 0)
    );
    
    return sortedTasks.map(task => (
      <div
        key={task.id}
        className={`task-square ${task.status} ${executionEngineState.selectedTask === task.id ? 'selected' : ''}`}
        onClick={() => handleTaskSquareClick(task.id)}
        title={`${task.status}: ${task.message}`}
        data-task-id={task.id}
        data-original-id={task.originalId}
      >
        {/* Remove task numbers from squares as per original design */}
      </div>
    ));
  };

  // Direct DOM manipulation to append new task squares
  const appendTaskSquare = (task: any) => {
    if (!executionTasksRef.current) return;
    
    const newSquare = document.createElement('div');
    newSquare.className = `task-square ${task.status}`;
    newSquare.title = `${task.status}: ${task.message}`;
    newSquare.setAttribute('data-task-id', task.id);
    newSquare.setAttribute('data-original-id', task.originalId || task.id);
    
    newSquare.addEventListener('click', () => handleTaskSquareClick(task.id));
    
    executionTasksRef.current.appendChild(newSquare);
  };

  // Update existing task square
  const updateTaskSquare = (task: any) => {
    if (!executionTasksRef.current) return;
    
    const existingSquare = executionTasksRef.current.querySelector(
      `[data-original-id="${task.originalId || task.id}"]`
    );
    
    if (existingSquare) {
      const htmlElement = existingSquare as HTMLElement;
      htmlElement.className = `task-square ${task.status} ${executionEngineState.selectedTask === task.id ? 'selected' : ''}`;
      htmlElement.title = `${task.status}: ${task.message}`;
    }
  };

  // Use direct DOM manipulation for task squares - only append new squares
  useEffect(() => {
    if (!executionTasksRef.current) return;
    
    // Get existing task IDs to determine which ones are new
    const existingTaskIds = Array.from(executionTasksRef.current.children).map(
      child => child.getAttribute('data-original-id')
    );
    
    // Sort tasks by sequenceNumber
    const sortedTasks = [...executionEngineState.tasks].sort((a, b) =>
      (a.sequenceNumber || 0) - (b.sequenceNumber || 0)
    );
    
    // Only append new tasks that don't already exist in the DOM
    sortedTasks.forEach(task => {
      const originalId = task.originalId || task.id;
      if (!existingTaskIds.includes(originalId)) {
        appendTaskSquare(task);
      } else {
        // Update existing task square if needed
        updateTaskSquare(task);
      }
    });
    
    // Update pointer position after DOM changes
    setTimeout(() => {
      updateTaskPointer();
    }, 10);
  }, [executionEngineState.tasks]);

  // Generate timeline items for execution tab
  const generateTimelineItems = () => {
    const selectedTask = executionEngineState.tasks.find(t => t.id === executionEngineState.selectedTask);
    
    //No task selected.
    if (!selectedTask) {
      return (
        <div style={{ fontSize: '12px', color: '#888', padding: '20px', textAlign: 'center' }}>
        </div>
      );
    }

    if (!selectedTask.subtasks || selectedTask.subtasks.length === 0) {
      return (
        <div style={{ fontSize: '12px', color: '#888', padding: '20px', textAlign: 'center' }}>
          No subtasks available for this task.
        </div>
      );
    }

    // Sort subtasks by ID to ensure proper order
    const sortedSubtasks = [...selectedTask.subtasks].sort((a, b) => {
      const aId = parseInt(a.id) || 0;
      const bId = parseInt(b.id) || 0;
      return aId - bId;
    });

    return sortedSubtasks.map((subtask, index) => {
      // Check if this is a nested subtask (iteration > 1)
      const isNested = index > 0;
      
      return (
        <div key={subtask.id} className={`timeline-item ${isNested ? 'nested' : ''} ${subtask.isActive ? 'active' : ''}`}>
          <div className="timeline-item-container">
            <div className={`timeline-dot ${subtask.isActive ? 'active' : ''}`}></div>
            <div className="timeline-content-wrapper">
              <div className="timeline-subtask">{subtask.description}</div>
              {subtask.actions && subtask.actions.length > 0 && (
                <div className="timeline-actions">
                  {subtask.actions.map(action => (
                    <div key={action.id} className="action-item">
                      <span className="action-type">{action.action}</span>
                      <span className="action-params">
                        {action.coordinates && `(${action.coordinates.x}, ${action.coordinates.y})`}
                        {action.inputString && `"${action.inputString}"`}
                        {action.keyTapString && action.keyTapString}
                        {action.duration && `${action.duration}ms`}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      );
    });
  };

  return (
    <div className={`settings-container ${isOpen ? 'open' : ''}`}>
      <div className="settings-sidebar">
        <div className="settings-header">
          <span style={{ fontSize: '12px !important', fontWeight: 'normal !important', letterSpacing: '1px', cursor: 'default', userSelect: 'none' }}>Settings</span>
          <button className="settings-close-btn" onClick={handleCloseClick}>×</button>
        </div>
        <div className="settings-tabs">
          <button
            className={`settings-tab ${activeTab === 'execution' ? 'active' : ''}`}
            onClick={() => handleTabClick('execution')}
            data-tab="execution"
          >
            Execution engine
          </button>
          <button
            className={`settings-tab ${activeTab === 'logs' ? 'active' : ''}`}
            onClick={() => handleTabClick('logs')}
            data-tab="logs"
          >
            Logs
          </button>
          <button
            className={`settings-tab ${activeTab === 'notifications' ? 'active' : ''}`}
            onClick={() => handleTabClick('notifications')}
            data-tab="notifications"
          >
            Notifications
          </button>
        </div>
        <div className="settings-content">
          <div className={`settings-tab-content ${activeTab === 'execution' ? 'active' : ''}`} id="execution-tab">
            {/* Task visualization rectangle - only show when there are tasks */}
            {executionEngineState.tasks.length > 0 && (
              <>
                <div className="tasks-horizontal-line"></div>
                <div ref={taskPointerRef} className="task-pointer"></div>
                <div className="tasks-rectangle" id="tasksRectangle">
                  <div ref={executionTasksRef} className="tasks-container" id="executionTasksContainer">
                    {/* Task squares will be added via direct DOM manipulation */}
                  </div>
                </div>
              </>
            )}
            
            {/* Initial user prompt */}
            <div className="user-prompt-section" id="userPromptSection">
              <div className="prompt-label">TASK DESCRIPTION:</div>
              <div ref={promptContentRef} className="prompt-content" id="promptContent">
                {executionEngineState.selectedTask
                  ? executionEngineState.tasks.find(t => t.id === executionEngineState.selectedTask)?.message || 'Task details not available'
                  : 'No task selected'
                }
              </div>
            </div>
            
            {/* Vertical timeline - only show when there's a selected task with subtasks */}
            {executionEngineState.selectedTask && executionEngineState.tasks.find(t => t.id === executionEngineState.selectedTask)?.subtasks &&
             executionEngineState.tasks.find(t => t.id === executionEngineState.selectedTask)!.subtasks!.length > 0 && (
              <div className="timeline-container" id="timelineContainer">
                <div ref={timelineContentRef} className="timeline-content" id="timelineContent">
                  <div className="timeline-line"></div>
                  {generateTimelineItems()}
                </div>
              </div>
            )}
          </div>
          <div className={`settings-tab-content ${activeTab === 'logs' ? 'active' : ''}`} id="logs-tab">
            <div className="logs-container">
              <div className="logs-content" ref={logsContentRef}>
                {logs.length === 0 ? (
                  <p style={{ fontSize: '12px', color: '#888' }}>No logs available.</p>
                ) : (
                  logs.map((log, index) => (
                    <div key={index} className="log-entry">
                      {log}
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
          <div className={`settings-tab-content ${activeTab === 'notifications' ? 'active' : ''}`} id="notifications-tab">
            <p>Receive task updates and daily reports in Slack and Telegram.</p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default SettingsPanel;