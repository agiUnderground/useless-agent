import React, { useState, useEffect, useRef } from 'react';
import './ChatFieldset.css';

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

interface ChatFieldsetProps {
  selectedSessionIp: string | null;
  userAssistActive: boolean;
  onSendMessage: (message: string, isUserAssist: boolean) => Promise<boolean>;
  onToggleSettings: () => void;
  onToggleUserAssist: (taskId?: string, event?: React.MouseEvent) => void;
  serverOffline: boolean;
  selectedSessionId?: string | null;
  tasks?: Task[];
  onActivateUserAssist?: (taskId: string, event?: React.MouseEvent) => void;
  onDeactivateUserAssist?: () => void;
}

const ChatFieldset: React.FC<ChatFieldsetProps> = ({
  selectedSessionIp,
  userAssistActive,
  onSendMessage,
  onToggleSettings,
  onToggleUserAssist,
  serverOffline,
  selectedSessionId,
  tasks = [],
  onActivateUserAssist,
  onDeactivateUserAssist
}) => {
  const [message, setMessage] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Handle message submission
  const handleSubmit = async (e?: React.FormEvent) => {
    if (e) {
      e.preventDefault();
    }
    
    if (message.trim()) {
      // Only clear the message if sending was successful
      const wasSuccessful = await onSendMessage(message, userAssistActive);
      if (wasSuccessful) {
        setMessage('');
      }
    }
  };

  // Function for smart scrolling based on task position
  const smartScrollToTask = (taskCardElement: HTMLElement, tasksContainerElement: HTMLElement) => {
    const taskCards = Array.from(tasksContainerElement.querySelectorAll('.task-card'));
    const taskIndex = taskCards.indexOf(taskCardElement);
    const totalTasks = taskCards.length;
    
    if (taskIndex === 0) {
      // First task - scroll to top
      tasksContainerElement.scrollTo({ top: 0, behavior: 'smooth' });
    } else if (taskIndex === totalTasks - 1) {
      // Last task - scroll to bottom
      tasksContainerElement.scrollTo({ top: tasksContainerElement.scrollHeight, behavior: 'smooth' });
    } else {
      // Middle task - scroll to center
      const taskRect = taskCardElement.getBoundingClientRect();
      const containerRect = tasksContainerElement.getBoundingClientRect();
      const containerCenter = containerRect.height / 2;
      const taskCenter = taskRect.top - containerRect.top + taskRect.height / 2;
      const scrollOffset = taskCenter - containerCenter;
      
      tasksContainerElement.scrollBy({ top: scrollOffset, behavior: 'smooth' });
    }
  };

  // Handle keyboard shortcuts
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    } else if (e.key === 'Escape' && userAssistActive) {
      e.preventDefault();
      onToggleUserAssist();
    } else if (e.ctrlKey && e.key === 'u') {
      e.preventDefault();
      
      if (userAssistActive) {
        // User-assist is active, deactivate it
        if (onDeactivateUserAssist) {
          onDeactivateUserAssist();
        }
      } else {
        // User-assist is not active, try to activate it for current session's in-progress task
        if (selectedSessionId && onActivateUserAssist) {
          const inProgressTask = tasks.find(task =>
            task.status === 'in-progress' && task.sessionId === selectedSessionId
          );
          
          if (inProgressTask) {
            // Scroll to the task before activating user-assist
            const taskCardElement = document.querySelector(`[data-task-id="${inProgressTask.id}"]`) as HTMLElement;
            const tasksContainerElement = document.getElementById('tasksContainer') as HTMLElement;
            
            if (tasksContainerElement && taskCardElement) {
              // Smart scroll based on task position
              smartScrollToTask(taskCardElement, tasksContainerElement);
              
              // Show tasks section if it's not visible
              const tasksSectionElement = document.getElementById('tasksSection');
              if (tasksSectionElement) {
                tasksSectionElement.classList.add('visible');
              }
              
              // Activate user-assist after a small delay to allow scrolling to complete
              setTimeout(() => {
                onActivateUserAssist(inProgressTask.id);
              }, 300);
            } else {
              // Fallback if elements not found
              onActivateUserAssist(inProgressTask.id);
            }
          }
        }
      }
    } else if (e.ctrlKey && (e.key === 'l' || e.key === 'L')) {
      e.preventDefault();
      setMessage('');
    }
  };

  // Focus textarea when user-assist is activated
  useEffect(() => {
    if (userAssistActive && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [userAssistActive]);

  return (
    <fieldset
      id="chatFieldset"
      className={`chat-fieldset ${serverOffline ? 'server-offline' : ''} ${userAssistActive ? 'user-assist-active' : ''}`}
    >
      <legend>Chat</legend>
      
      {/* Chat header with settings button and session IP */}
      <div className="chat-header">
        <button
          className="settings-button"
          id="settingsBtn"
          onClick={onToggleSettings}
        >
          Settings
        </button>
        
        <div
          id="selectedSessionIp"
          className="selected-session-ip"
          style={{ display: selectedSessionIp ? 'block' : 'none' }}
        >
          Session: <span id="selectedIpText">{selectedSessionIp}</span>
        </div>
      </div>
      
      
      {/* Message textarea */}
      <textarea
        ref={textareaRef}
        id="llmChatInput"
        className={`dark-textarea ${userAssistActive ? 'user-assist-active' : ''}`}
        placeholder="Type a message..."
        spellCheck="false"
        autoCorrect="off"
        autoCapitalize="off"
        autoComplete="off"
        style={{ userSelect: 'none' }}
        value={message}
        onChange={(e) => setMessage(e.target.value)}
        onKeyDown={handleKeyDown}
      />
      
      {/* Send button */}
      <button
        className="control-button"
        id="llmSendButton"
        style={{ marginTop: '10px' }}
        onClick={() => handleSubmit()}
      >
        Send
      </button>
    </fieldset>
  );
};

export default ChatFieldset;
