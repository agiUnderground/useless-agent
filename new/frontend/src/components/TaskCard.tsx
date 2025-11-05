import React, { useState, useEffect } from 'react';
import './TaskCard.css';

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

interface TaskCardProps {
  task: Task;
  onCancel: (taskId: string) => void;
  onActivateUserAssist: (taskId: string, event: React.MouseEvent) => void;
  onOpenSessionWindow?: (sessionId: string, sessionIp: string) => void;
  userAssistActive: boolean;
  isUserAssistTask: boolean;
  sessions?: Array<{ id: string; ip: string }>;
}

const TaskCard: React.FC<TaskCardProps> = ({
  task,
  onCancel,
  onActivateUserAssist,
  onOpenSessionWindow,
  userAssistActive,
  isUserAssistTask,
  sessions = []
}) => {
  const [elapsedTime, setElapsedTime] = useState<string>('00:00:00:00:000');
  const [finalElapsedTime, setFinalElapsedTime] = useState<string | null>(null);

  // Format elapsed time to match original format: days:hours:minutes:seconds:milliseconds
  useEffect(() => {
    // Always show elapsed time for all tasks with createdAt, regardless of status
    if (task.createdAt) {
      // For active tasks (in-progress, in-the-queue), keep timer running
      if (task.status === 'in-progress' || task.status === 'in-the-queue') {
        const interval = setInterval(() => {
          const now = Date.now();
          const elapsed = now - task.createdAt!;
          const days = Math.floor(elapsed / (1000 * 60 * 60 * 24));
          const hours = Math.floor((elapsed % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
          const minutes = Math.floor((elapsed % (1000 * 60 * 60)) / (1000 * 60));
          const seconds = Math.floor((elapsed % (1000 * 60)) / 1000);
          const milliseconds = elapsed % 1000;
          
          setElapsedTime(
            `${days.toString().padStart(2, '0')}:${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}:${milliseconds.toString().padStart(3, '0')}`
          );
        }, 10); // Update every 10ms for milliseconds

        return () => clearInterval(interval);
      }
      // For completed/canceled tasks, calculate final elapsed time once and store it
      else if (task.status === 'canceled' || task.status === 'completed' || task.status === 'broken') {
        // Use completedAt if available, otherwise use current time
        const endTime = task.completedAt || Date.now();
        const elapsed = endTime - task.createdAt!;
        const days = Math.floor(elapsed / (1000 * 60 * 60 * 24));
        const hours = Math.floor((elapsed % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
        const minutes = Math.floor((elapsed % (1000 * 60 * 60)) / (1000 * 60));
        const seconds = Math.floor((elapsed % (1000 * 60)) / 1000);
        const milliseconds = elapsed % 1000;
        
        const finalTime = `${days.toString().padStart(2, '0')}:${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}:${milliseconds.toString().padStart(3, '0')}`;
        
        setElapsedTime(finalTime);
        setFinalElapsedTime(finalTime);
      } else {
        setElapsedTime('00:00:00:00:000');
      }
    }
  }, [task.status, task.createdAt, task.completedAt]);

  const handleCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    onCancel(task.id);
  };

  const handleTaskCardClick = (e: React.MouseEvent) => {
    // Check if the click was on an actionable element (buttons, icons)
    const actionableElements = [
      '.task-session-window-icon',
      '.task-cancel-btn',
      '.task-info-icon'
    ];
    
    const clickedElement = e.target as HTMLElement;
    const isActionable = actionableElements.some(selector =>
      clickedElement.closest(selector)
    );
    
    // Only handle user-assist if not clicking on actionable elements
    if (!isActionable) {
      console.log('TaskCard clicked, task status:', task.status, 'userAssistActive:', userAssistActive);
      
      // Pass the taskId and event to parent component
      onActivateUserAssist(task.id, e);
    }
  };

  const getStatusDisplay = () => {
    switch (task.status) {
      case 'in-progress':
        return 'In Progress';
      case 'completed':
        return 'Completed';
      case 'broken':
        return 'Broken';
      case 'canceled':
        return 'Canceled';
      case 'in-the-queue':
        return 'In Queue';
      case 'created':
        return 'Created';
      default:
        return task.status;
    }
  };

  const canCancel = task.status === 'in-progress' || task.status === 'in-the-queue';
  const showTimer = task.status === 'in-progress' || task.status === 'in-the-queue' || task.status === 'canceled' || task.status === 'completed' || task.status === 'broken';
  const showUserAssist = task.status === 'in-progress' && !isUserAssistTask;
  
  // Function to get session IP from session ID or stored IP
  const getSessionIp = (sessionId?: string, sessionIp?: string) => {
    // First try to use the stored IP
    if (sessionIp) return sessionIp;
    
    // If no stored IP, try to find the session in the current sessions array
    if (!sessionId) return 'Unknown';
    const session = sessions.find(s => s.id === sessionId);
    return session ? session.ip : 'Unknown';
  };

  // Truncate message if too long
  const displayMessage = task.message.length > 100 ?
    task.message.substring(0, 100) + '...' : task.message;

  return (
    <div
      className={`task-card ${task.status} ${isUserAssistTask ? 'user-assist-task' : ''}`}
      onClick={handleTaskCardClick}
      data-task-id={task.id}
    >
      <div className="task-header">
        <div className="task-header-left">
          {/* Sequence number */}
          <span className="task-sequence-number">#{task.sequenceNumber || 0}</span>
          {/* Status */}
          <span className="task-status">{getStatusDisplay()}</span>
        </div>
        <div className="task-header-controls">
          {/* Session window icon for all tasks with session ID */}
          {task.sessionId && (
            <div
              className="task-session-window-icon"
              onClick={(e) => {
                e.stopPropagation();
                const ip = getSessionIp(task.sessionId, task.sessionIp);
                if (onOpenSessionWindow && task.sessionId && ip !== 'Unknown') {
                  onOpenSessionWindow(task.sessionId, ip);
                } else {
                  console.log(`Cannot open session window for ${task.sessionId} - session not available`);
                }
              }}
              title="Open session window"
            >
              ⛶
            </div>
          )}
          {/* Cancel button for in-progress and queued tasks */}
          {canCancel && (
            <button
              className="task-cancel-btn"
              onClick={handleCancel}
            >
              Cancel
            </button>
          )}
          {/* Info icon for non-in-progress tasks */}
          {task.status !== 'in-progress' && (
            <div
              className="task-info-icon"
              title={task.message}
            >
              ?
            </div>
          )}
        </div>
      </div>
      
      {/* Session info */}
      <div className="task-session-info">
        Session: {getSessionIp(task.sessionId, task.sessionIp) || 'Unknown'}
      </div>
      
      {/* Task message */}
      <div className="task-message">{displayMessage}</div>
      
      {/* Timer for in-progress and queued tasks */}
      {showTimer && (
        <div className="task-timer">
          <span className="timer-text">{elapsedTime}</span>
        </div>
      )}
    </div>
  );
};

export default TaskCard;
