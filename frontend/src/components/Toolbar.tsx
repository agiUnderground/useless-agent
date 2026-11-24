import React, { useState, useEffect, useRef } from 'react';
import ChatFieldset from './ChatFieldset';
import TasksSection from './TasksSection';
import { InventoryServer } from '../api';
import './Toolbar.css';

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
  selectedFps?: number;
  onFpsChange?: (fps: number) => void;
  sessions?: Array<{ id: string; ip: string }>;
  selectedSessionId?: string | null;
  availableModels?: ModelOption[];
  selectedModel?: string;
  onSelectModel?: (modelId: string, provider: string) => void;
  selectedThinkingLevel?: string;
  onSelectThinkingLevel?: (level: string) => void;
  inventoryServers?: InventoryServer[];
  workspaceId?: string;
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
  selectedFps = 5,
  onFpsChange,
  sessions = [],
  selectedSessionId,
  availableModels = [],
  selectedModel = '',
  onSelectModel,
  selectedThinkingLevel = '',
  onSelectThinkingLevel,
  inventoryServers = [],
  workspaceId
}) => {
  const [inventoryOpen, setInventoryOpen] = useState(false);
  const [inventoryFilter, setInventoryFilter] = useState('');
  const inventoryDropdownRef = useRef<HTMLDivElement>(null);

  // Close inventory dropdown when clicking outside
  useEffect(() => {
    if (!inventoryOpen) return;
    const handleOutsideClick = (e: MouseEvent) => {
      if (inventoryDropdownRef.current && !inventoryDropdownRef.current.contains(e.target as Node)) {
        setInventoryOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutsideClick);
    return () => document.removeEventListener('mousedown', handleOutsideClick);
  }, [inventoryOpen]);

  const filteredInventory = inventoryServers.filter((srv) => {
    const q = inventoryFilter.trim().toLowerCase();
    if (!q) return true;
    return (
      srv.name.toLowerCase().includes(q) ||
      srv.ip.toLowerCase().includes(q) ||
      (srv.description || '').toLowerCase().includes(q)
    );
  });

  const handleSelectInventoryServer = (ip: string) => {
    const event = new CustomEvent('connectSession', { detail: ip });
    document.dispatchEvent(event);
    setInventoryOpen(false);
    setInventoryFilter('');
  };
  
  // Handle FPS button click (state is managed per-workspace by WorkspaceApp)
  const handleFpsClick = (fps: number) => {
    if (onFpsChange) {
      onFpsChange(fps);
    }
  };
  
  // Validate IP address (IPv4 or IPv6, with optional brackets for IPv6)
  const validateIpAddress = (rawIp: string) => {
    const trimmed = rawIp.trim();
    const isBracketed = trimmed.startsWith('[') && trimmed.endsWith(']');
    const ip = isBracketed ? trimmed.slice(1, -1) : trimmed;

    // IPv4 validation (not allowed inside brackets)
    const ipv4Regex = /^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$/;
    if (!isBracketed && ipv4Regex.test(ip)) {
      const parts = ip.split('.');
      return parts.every(part => {
        const num = parseInt(part, 10);
        return num >= 0 && num <= 255;
      });
    }

    // IPv6 validation (supports full, compressed ::, and IPv4-mapped forms)
    if (ip.includes(':')) {
      try {
        new URL(`http://[${ip}]`);
        return true;
      } catch {
        return false;
      }
    }

    return false;
  };

  // Handle connect button click
  const handleConnectClick = () => {
    const ipInput = document.getElementById('ipv4') as HTMLInputElement;
    if (ipInput && ipInput.value.trim()) {
      const rawIp = ipInput.value.trim();
      
      if (!validateIpAddress(rawIp)) {
        // Show error message
        ipInput.style.border = '1px solid #ff4444';
        ipInput.title = 'Invalid IP address format';
        setTimeout(() => {
          ipInput.style.border = '';
          ipInput.title = '';
        }, 3000);
        return;
      }

      const ip = rawIp.startsWith('[') && rawIp.endsWith(']')
        ? rawIp.slice(1, -1)
        : rawIp;
      
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
            {[1, 5, 10, 15, 30].map((fps) => (
              <button
                key={fps}
                className={`fps-button ${selectedFps === fps ? 'active' : ''}`}
                data-fps={fps}
                onClick={() => handleFpsClick(fps)}
              >
                {fps}
              </button>
            ))}
          </div>
        </div>

        <div className="input-group">
          <input
            type="text"
            id="ipv4"
            className="dark-textarea"
            placeholder="IPv4 / IPv6 Address"
            style={{ userSelect: 'none', minWidth: '150px' }}
            required
            onInput={(e) => {
              const input = e.target as HTMLInputElement;
              // Allow IPv4, IPv6, IPv4-mapped IPv6, and optional surrounding brackets
              const allowedCharsRegex = /^[\da-fA-F:.[\]]*$/;
              if (!allowedCharsRegex.test(input.value)) {
                input.value = input.value.replace(/[^\da-fA-F:.[\]]/g, '');
              }
              
              // Validate IPv4 format as user types (only when it doesn't look like IPv6)
              const ipv4Regex = /^[\d.]*$/;
              if (!input.value.includes(':') && !input.value.includes('[') && ipv4Regex.test(input.value)) {
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
          <div className="inventory-picker-wrapper" ref={inventoryDropdownRef}>
            <button
              type="button"
              className={`inventory-picker-btn ${inventoryOpen ? 'open' : ''}`}
              title="Connect from Inventory"
              onClick={() => setInventoryOpen((prev) => !prev)}
            >
              +
            </button>
            {inventoryOpen && (
              <div className="inventory-dropdown">
                <input
                  type="text"
                  className="inventory-dropdown-filter"
                  placeholder="Filter inventory..."
                  value={inventoryFilter}
                  onChange={(e) => setInventoryFilter(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Escape') {
                      setInventoryOpen(false);
                    } else if (e.key === 'Enter' && filteredInventory.length > 0) {
                      handleSelectInventoryServer(filteredInventory[0].ip);
                    }
                  }}
                  autoFocus
                />
                <div className="inventory-dropdown-list">
                  {filteredInventory.length === 0 ? (
                    <div className="inventory-dropdown-empty">
                      {inventoryServers.length === 0 ? 'No servers in inventory' : 'No matches'}
                    </div>
                  ) : (
                    filteredInventory.map((srv) => (
                      <button
                        key={srv.id}
                        type="button"
                        className="inventory-dropdown-item"
                        onClick={() => handleSelectInventoryServer(srv.ip)}
                      >
                        <span className="inventory-dropdown-name">{srv.name}</span>
                        <span className="inventory-dropdown-ip">{srv.ip}</span>
                      </button>
                    ))
                  )}
                </div>
              </div>
            )}
          </div>
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
        workspaceId={workspaceId}
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
        availableModels={availableModels}
        selectedModel={selectedModel}
        onSelectModel={onSelectModel}
        selectedThinkingLevel={selectedThinkingLevel}
        onSelectThinkingLevel={onSelectThinkingLevel}
      />
    </div>
  );
};

export default Toolbar;
