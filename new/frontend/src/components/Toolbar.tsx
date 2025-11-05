import React, { useState, useEffect } from 'react';
import ChatFieldset from './ChatFieldset';
import TasksSection from './TasksSection';
import './Toolbar.css';

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

interface ToolbarProps {
  selectedSessionIp: string | null;
  userAssistActive: boolean;
  onSendMessage: (message: string, isUserAssist: boolean) => Promise<boolean>;
  onToggleSettings: () => void;
  onToggleUserAssist: (taskId: string, event: React.MouseEvent) => void;
  onOpenSessionWindow?: (sessionId: string, sessionIp: string) => void;
  serverOffline: boolean;
  tasks: Task[];
  onCancelTask: (taskId: string) => void;
  onActivateUserAssist: (taskId: string, event?: React.MouseEvent) => void;
  userAssistTaskCard: Task | null;
  onDeactivateUserAssist: () => void;
  networkUsage: number;
  tokenUsage: number;
  onFpsChange?: (fps: number) => void;
  sessions?: Array<{ id: string; ip: string }>;
  selectedSessionId?: string | null;
}

const Toolbar: React.FC<ToolbarProps> = ({
  selectedSessionIp,
  userAssistActive = false,
  onSendMessage,
  onToggleSettings,
  onToggleUserAssist,
  onOpenSessionWindow,
  serverOffline = false,
  tasks,
  onCancelTask,
  onActivateUserAssist,
  userAssistTaskCard,
  onDeactivateUserAssist,
  networkUsage,
  tokenUsage,
  onFpsChange,
  sessions = [],
  selectedSessionId
}) => {
  const [selectedFps, setSelectedFps] = useState(5); // Default to 5 FPS
  
  // Load saved FPS from localStorage on component mount
  useEffect(() => {
    const savedFps = localStorage.getItem('selectedFps');
    if (savedFps) {
      const fps = parseInt(savedFps, 10);
      setSelectedFps(fps);
      
      // Notify parent component about initial FPS
      if (onFpsChange) {
        onFpsChange(fps);
      }
      
      // Update the UI to show the selected FPS
      const fpsButtons = document.querySelectorAll('.fps-button');
      fpsButtons.forEach(button => {
        if (parseInt(button.getAttribute('data-fps') || '0', 10) === fps) {
          button.classList.add('active');
        } else {
          button.classList.remove('active');
        }
      });
    } else {
      // No saved FPS, use default 5 FPS
      if (onFpsChange) {
        onFpsChange(5);
      }
      
      // Update UI to show the default FPS (5)
      const fpsButtons = document.querySelectorAll('.fps-button');
      fpsButtons.forEach(button => {
        if (parseInt(button.getAttribute('data-fps') || '0', 10) === 5) {
          button.classList.add('active');
        } else {
          button.classList.remove('active');
        }
      });
    }
  }, []);
  
  // Handle FPS button click
  const handleFpsClick = (fps: number) => {
    setSelectedFps(fps);
    localStorage.setItem('selectedFps', fps.toString());
    
    // Notify parent component about FPS change
    if (onFpsChange) {
      onFpsChange(fps);
    }
    
    // Update active state on buttons
    const fpsButtons = document.querySelectorAll('.fps-button');
    fpsButtons.forEach(button => {
      if (parseInt(button.getAttribute('data-fps') || '0', 10) === fps) {
        button.classList.add('active');
      } else {
        button.classList.remove('active');
      }
    });
  };
  
  // Validate IP address
  const validateIpAddress = (ip: string) => {
    // IPv4 validation
    const ipv4Regex = /^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$/;
    if (ipv4Regex.test(ip)) {
      const parts = ip.split('.');
      return parts.every(part => {
        const num = parseInt(part, 10);
        return num >= 0 && num <= 255;
      });
    }
    
    // IPv6 validation (basic)
    const ipv6Regex = /^[\da-fA-F:]+$/;
    if (ipv6Regex.test(ip)) {
      // Basic validation - ensure it has at least one colon
      return ip.includes(':');
    }
    
    return false;
  };

  // Handle connect button click
  const handleConnectClick = () => {
    const ipInput = document.getElementById('ipv4') as HTMLInputElement;
    if (ipInput && ipInput.value.trim()) {
      const ip = ipInput.value.trim();
      
      if (!validateIpAddress(ip)) {
        // Show error message
        ipInput.style.border = '1px solid #ff4444';
        ipInput.title = 'Invalid IP address format';
        setTimeout(() => {
          ipInput.style.border = '';
          ipInput.title = '';
        }, 3000);
        return;
      }
      
      // Dispatch custom event to notify parent component
      const event = new CustomEvent('connectSession', { detail: ip });
      document.dispatchEvent(event);
      ipInput.value = ''; // Clear input after connection
    }
  };
  
  // Handle debug button click
  const handleDebugClick = () => {
    const mediaButtonsGroup = document.getElementById('mediaButtonsGroup');
    if (mediaButtonsGroup) {
      if (mediaButtonsGroup.classList.contains('hidden')) {
        mediaButtonsGroup.classList.remove('hidden');
      } else {
        mediaButtonsGroup.classList.add('hidden');
      }
    }
  };

  // Handle screenshot button click
  const handleScreenshotClick = () => {
    // Dispatch event to take screenshot
    const event = new CustomEvent('takeScreenshot', { detail: {} });
    document.dispatchEvent(event);
  };

  // Handle video button click
  const handleVideoClick = () => {
    // Dispatch event to start/stop video
    const event = new CustomEvent('toggleVideo', { detail: {} });
    document.dispatchEvent(event);
  };

  // Handle mouse move
  const handleMouseMove = () => {
    const xInput = document.getElementById('x-coord') as HTMLInputElement;
    const yInput = document.getElementById('y-coord') as HTMLInputElement;
    
    if (xInput && yInput && xInput.value && yInput.value) {
      const x = parseInt(xInput.value, 10);
      const y = parseInt(yInput.value, 10);
      
      // Dispatch mouse move event with immediate processing
      const event = new CustomEvent('mouseMove', { detail: { x, y } });
      document.dispatchEvent(event);
      
      // Also log for debugging
      console.log(`Mouse move event dispatched: x=${x}, y=${y}`);
    } else {
      console.error('Cannot send mouse move: Missing X or Y coordinates');
    }
  };

  // Handle mouse click
  const handleMouseClick = () => {
    // Dispatch mouse click event with immediate processing
    const event = new CustomEvent('mouseClick', { detail: {} });
    document.dispatchEvent(event);
    
    // Also log for debugging
    console.log('Mouse click event dispatched');
  };
  
  return (
    <div className="toolbar">
      {/* Media buttons group */}
      <div className="button-group hidden" id="mediaButtonsGroup">
        {/* Mouse control fieldset */}
        <fieldset>
          <legend>Mouse Control</legend>
          <div className="coord-inputs">
            <input
              type="text"
              id="x-coord"
              className="coord-input"
              placeholder="X"
              pattern="-?[0-9]+"
              title="Please enter a valid coordinate value."
              required
              onInput={(e) => {
                const input = e.target as HTMLInputElement;
                // Only allow digits and minus sign
                input.value = input.value.replace(/[^-0-9]/g, '');
              }}
            />
            <input
              type="text"
              id="y-coord"
              className="coord-input"
              placeholder="Y"
              pattern="-?[0-9]+"
              title="Please enter a valid coordinate value."
              required
              onInput={(e) => {
                const input = e.target as HTMLInputElement;
                // Only allow digits and minus sign
                input.value = input.value.replace(/[^-0-9]/g, '');
              }}
            />
            <button className="control-button" id="sendMouseInput" onClick={handleMouseMove}>Move</button>
            <button className="control-button" id="sendMouseClick" onClick={handleMouseClick}>Click</button>
          </div>
        </fieldset>
      </div>

      {/* Connection fieldset */}
      <fieldset className="connection-fieldset">
        <legend>Connection</legend>
        
        {/* Debug Button positioned at top right */}
        <button className="debug-button" id="toggleButtonsBtn" title="Debug" onClick={handleDebugClick}></button>
        
        {/* FPS Selection */}
        <div className="fps-selector" style={{marginBottom: '10px'}}>
          <label style={{display: 'block', marginBottom: '4px', fontSize: '12px', color: '#6f6f6f'}}>FPS</label>
          <div className="fps-buttons" style={{display: 'flex', gap: '4px', height: '20px'}}>
            <button className="fps-button" data-fps="1" onClick={() => handleFpsClick(1)}>1</button>
            <button className="fps-button active" data-fps="5" onClick={() => handleFpsClick(5)}>5</button>
            <button className="fps-button" data-fps="10" onClick={() => handleFpsClick(10)}>10</button>
            <button className="fps-button" data-fps="15" onClick={() => handleFpsClick(15)}>15</button>
            <button className="fps-button" data-fps="30" onClick={() => handleFpsClick(30)}>30</button>
          </div>
        </div>

        <div className="input-group">
          <input
            type="text"
            id="ipv4"
            className="dark-textarea"
            placeholder="IP Address"
            pattern="^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$|^[\da-fA-F:]+$"
            style={{ userSelect: 'none' }}
            required
            onInput={(e) => {
              const input = e.target as HTMLInputElement;
              // Only allow valid IP address characters
              const ipv4Regex = /^[\d.]*$/;
              const ipv6Regex = /^[\da-fA-F:]*$/;
              
              if (!ipv4Regex.test(input.value) && !ipv6Regex.test(input.value)) {
                // Remove invalid characters
                input.value = input.value.replace(/[^\d.a-fA-F:]/g, '');
              }
              
              // Validate IPv4 format as user types
              if (ipv4Regex.test(input.value)) {
                const parts = input.value.split('.');
                // Ensure each part is valid (0-255) and there are max 4 parts
                const validParts = parts.map(part => {
                  if (part === '') return part;
                  const num = parseInt(part, 10);
                  if (isNaN(num) || num > 255) {
                    return part.substring(0, part.length - 1);
                  }
                  return part;
                });
                
                if (validParts.join('.') !== input.value) {
                  input.value = validParts.join('.');
                }
                
                // Limit to 4 parts
                if (parts.length > 4) {
                  input.value = parts.slice(0, 4).join('.');
                }
              }
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                handleConnectClick();
              }
            }}
          />
          <button className="control-button" id="setTargetIP" onClick={handleConnectClick}>Connect</button>
        </div>
      </fieldset>

      {/* Status fieldset */}
      <fieldset className="status-fieldset">
        <legend>Status</legend>
        <p style={{margin: 'auto', fontSize: '12px', padding: '5px', width: 'fit-content'}}>
          Total tokens used: <span id="tokenCounter">{tokenUsage}</span>
        </p>
        <p style={{margin: 'auto', fontSize: '12px', padding: '5px', width: 'fit-content'}}>
          Total network data usage: <span id="networkCounter">{(networkUsage / 1024 / 1024).toFixed(2)} MB</span>
        </p>
      </fieldset>

      {/* Tasks section */}
      <TasksSection
        tasks={tasks}
        onCancelTask={onCancelTask}
        onActivateUserAssist={onActivateUserAssist}
        onOpenSessionWindow={onOpenSessionWindow}
        userAssistActive={userAssistActive}
        userAssistTaskCard={userAssistTaskCard}
        onDeactivateUserAssist={onDeactivateUserAssist}
        sessions={sessions}
      />

      {/* Chat fieldset */}
      <ChatFieldset
        selectedSessionIp={selectedSessionIp}
        userAssistActive={userAssistActive}
        onSendMessage={onSendMessage}
        onToggleSettings={onToggleSettings}
        onToggleUserAssist={(taskId?: string, event?: React.MouseEvent<Element, MouseEvent>) => {
          if (taskId && event) {
            onToggleUserAssist(taskId, event);
          }
        }}
        serverOffline={serverOffline}
        selectedSessionId={selectedSessionId}
        tasks={tasks}
        onActivateUserAssist={onActivateUserAssist}
        onDeactivateUserAssist={onDeactivateUserAssist}
      />
    </div>
  );
};

export default Toolbar;
