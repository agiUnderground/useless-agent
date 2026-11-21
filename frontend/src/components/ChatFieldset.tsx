import React, { useState, useEffect, useRef } from 'react';
import './ChatFieldset.css';

interface ModelOption {
  id: string;
  provider: string;
  hasCredentials: boolean;
  thinkingLevels?: string[];
  defaultThinkingLevel?: string;
}

interface Task {
  id: string;
  status: string;
  message: string;
  model?: string;
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
  availableModels?: ModelOption[];
  selectedModel?: string;
  onSelectModel?: (modelId: string, provider: string) => void;
  selectedThinkingLevel?: string;
  onSelectThinkingLevel?: (level: string) => void;
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
  onDeactivateUserAssist,
  availableModels = [],
  selectedModel = '',
  onSelectModel,
  selectedThinkingLevel = '',
  onSelectThinkingLevel
}) => {
  const [message, setMessage] = useState('');
  const [modelMenuOpen, setModelMenuOpen] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const modelSelectorRef = useRef<HTMLDivElement>(null);

  // Close model dropdown when clicking outside or pressing Escape
  useEffect(() => {
    if (!modelMenuOpen) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (modelSelectorRef.current && !modelSelectorRef.current.contains(e.target as Node)) {
        setModelMenuOpen(false);
      }
    };
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setModelMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    document.addEventListener('keydown', handleEsc);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.removeEventListener('keydown', handleEsc);
    };
  }, [modelMenuOpen]);

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

      {/* Model selector positioned below Settings and Session */}
      <div className="model-selector-wrapper" ref={modelSelectorRef}>
        {(() => {
          const currentModelObj = availableModels.find(m => m.id === selectedModel);
          const thinkingLevels = currentModelObj?.thinkingLevels || [];
          const activeThinkingLevel =
            selectedThinkingLevel ||
            currentModelObj?.defaultThinkingLevel ||
            thinkingLevels[0] ||
            '';

          return (
            <>
              <button
                type="button"
                className="model-selector-button"
                id="modelSelectorBtn"
                onClick={() => setModelMenuOpen(prev => !prev)}
                title={
                  selectedModel
                    ? `Selected model: ${selectedModel}${activeThinkingLevel ? ` (${activeThinkingLevel})` : ''}`
                    : 'Select model'
                }
              >
                <span className="model-selector-label">
                  {selectedModel
                    ? `${selectedModel}${activeThinkingLevel ? ` · ${activeThinkingLevel}` : ''}`
                    : 'Model'}
                </span>
                <span className="model-selector-caret">{modelMenuOpen ? '▲' : '▼'}</span>
              </button>

              {modelMenuOpen && (
                <div className="model-dropdown-menu" role="listbox" aria-label="Available models">
                  <div className="model-dropdown-list">
                    {availableModels.length === 0 ? (
                      <div className="model-dropdown-empty">No models available</div>
                    ) : (
                      [...availableModels]
                        .sort((a, b) => Number(b.hasCredentials) - Number(a.hasCredentials))
                        .map(m => {
                          const isSelected = m.id === selectedModel;
                          return (
                            <button
                              key={`${m.provider}:${m.id}`}
                              type="button"
                              role="option"
                              aria-selected={isSelected}
                              disabled={!m.hasCredentials}
                              className={`model-dropdown-item ${isSelected ? 'selected' : ''} ${!m.hasCredentials ? 'no-creds' : ''}`}
                              title={
                                m.hasCredentials
                                  ? `${m.provider} / ${m.id}`
                                  : `No API credentials configured for ${m.provider}`
                              }
                              onClick={() => {
                                if (!m.hasCredentials) return;
                                if (onSelectModel) {
                                  onSelectModel(m.id, m.provider);
                                }
                                if (!m.thinkingLevels || m.thinkingLevels.length === 0) {
                                  setModelMenuOpen(false);
                                }
                              }}
                            >
                              <span className="model-item-main">
                                <span className="model-item-name">{m.id}</span>
                                <span className="model-item-provider">{m.provider}</span>
                              </span>
                              {!m.hasCredentials && (
                                <span className="model-no-creds-badge">no creds</span>
                              )}
                            </button>
                          );
                        })
                    )}
                  </div>

                  {thinkingLevels.length > 0 && (
                    <div className="model-thinking-bar">
                      <span className="model-thinking-label">Thinking</span>
                      <div className="model-thinking-buttons">
                        {thinkingLevels.map(level => (
                          <button
                            key={level}
                            type="button"
                            className={`model-thinking-btn ${activeThinkingLevel === level ? 'active' : ''}`}
                            onClick={() => {
                              if (onSelectThinkingLevel) {
                                onSelectThinkingLevel(level);
                              }
                              setModelMenuOpen(false);
                            }}
                          >
                            {level}
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </>
          );
        })()}
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
