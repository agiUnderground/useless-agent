import React, { useState, useEffect, useRef, useCallback } from 'react';
import { CONFIG } from '../config';
import { createSessionRecorder, formatRecordingTime } from '../recording';
import type { SessionRecorder } from '../recording';
import './SessionContainer.css';

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

interface SessionContainerProps {
  session: Session;
  isSelected: boolean;
  isUserAssistSelected: boolean;
  userAssistActive: boolean;
  onSelect: (sessionId: string) => void;
  onClose: (sessionId: string) => void;
  onToggleMaximize: (sessionId: string) => void;
  onActivateUserAssist: (taskId: string, event: React.MouseEvent) => void;
  onNetworkUpdate?: (bytes: number) => void;
  inProgressTask?: Task;
  selectedFps?: number;
  userAssistTaskId?: string;
  /** Width of this session in grid columns (>1 only for a widened last row). */
  gridSpan?: number;
  /** Persists a finished session recording on the backend. */
  onSaveRecording?: (payload: {
    sessionId: string;
    sessionIp: string;
    startedAt: number;
    blob: Blob;
  }) => Promise<unknown>;
}

const SessionContainer: React.FC<SessionContainerProps> = ({
  session,
  isSelected,
  isUserAssistSelected,
  userAssistActive,
  onSelect,
  onClose,
  onToggleMaximize,
  onActivateUserAssist,
  onNetworkUpdate,
  inProgressTask,
  selectedFps = 5, // Default to 5 FPS if not provided
  userAssistTaskId,
  gridSpan,
  onSaveRecording
}) => {
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [isFocused, setIsFocused] = useState(false);
  const [showImage, setShowImage] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const [recordingElapsedMs, setRecordingElapsedMs] = useState(0);
  const [recordingSaved, setRecordingSaved] = useState(false);
  const [recordingError, setRecordingError] = useState<string | null>(null);
  const sessionRef = useRef<HTMLDivElement>(null);
  const imgRef = useRef<HTMLImageElement>(null);
  const placeholderRef = useRef<HTMLDivElement>(null);
  const recorderRef = useRef<SessionRecorder | null>(null);
  const recordingStartedAtRef = useRef(0);
  const onSaveRecordingRef = useRef(onSaveRecording);

  useEffect(() => {
    onSaveRecordingRef.current = onSaveRecording;
  }, [onSaveRecording]);

  // Flush a stopped recording and upload it to the backend.
  const finalizeRecording = useCallback(
    async (recorder: SessionRecorder) => {
      let blob: Blob;
      try {
        blob = await recorder.stop();
      } catch (error) {
        console.error('Failed to finalize session recording:', error);
        return;
      }
      if (!blob.size) {
        return;
      }

      const save = onSaveRecordingRef.current;
      if (!save) {
        console.warn('Session recording discarded: no save handler available');
        return;
      }

      try {
        await save({
          sessionId: session.id,
          sessionIp: session.ip,
          startedAt: recorder.startedAt,
          blob
        });
        if (!recorderRef.current) {
          setRecordingSaved(true);
          window.setTimeout(() => setRecordingSaved(false), 2500);
        }
      } catch (error) {
        console.error('Failed to save session recording:', error);
        setRecordingError('Recording could not be saved');
      }
    },
    [session.id, session.ip]
  );

  // Keep a stable ref so unmount cleanup can finish an in-flight recording.
  const finalizeRecordingRef = useRef(finalizeRecording);
  useEffect(() => {
    finalizeRecordingRef.current = finalizeRecording;
  }, [finalizeRecording]);

  // Closing the session (or unmounting) while recording still saves the video.
  useEffect(() => {
    return () => {
      const recorder = recorderRef.current;
      if (recorder) {
        recorderRef.current = null;
        void finalizeRecordingRef.current(recorder);
      }
    };
  }, []);

  // Elapsed-time ticker for the recording indicator.
  useEffect(() => {
    if (!isRecording) {
      setRecordingElapsedMs(0);
      return;
    }
    const update = () => setRecordingElapsedMs(Date.now() - recordingStartedAtRef.current);
    update();
    const id = window.setInterval(update, 250);
    return () => window.clearInterval(id);
  }, [isRecording]);

  // Handle session click
  const handleSessionClick = () => {
    onSelect(session.id);
    
    // In mobile mode, check if session is partially visible and scroll to make it fully visible
    const isMobileMode = window.innerWidth < 1100;
    if (isMobileMode) {
      // Add a small delay to ensure DOM has updated after selection
      setTimeout(() => {
        const sessionElement = document.querySelector(`[data-session-id="${session.id}"]`) as HTMLElement;
        const mainContentElement = document.querySelector('.main-content') as HTMLElement;
        
        if (sessionElement && mainContentElement) {
          const sessionRect = sessionElement.getBoundingClientRect();
          const mainContentRect = mainContentElement.getBoundingClientRect();
          
          // Check if session is partially visible (top or bottom edge is outside viewport)
          const isPartiallyVisible = (
            sessionRect.top < mainContentRect.top ||
            sessionRect.bottom > mainContentRect.top + mainContentRect.height
          );
          
          // If partially visible, scroll to center it in viewport
          if (isPartiallyVisible) {
            const scrollTop = sessionRect.top - mainContentRect.top - (mainContentRect.height - sessionRect.height) / 2;
            mainContentElement.scrollTop = scrollTop;
            
            // Also try scrollIntoView as a fallback
            sessionElement.scrollIntoView({
              behavior: 'smooth',
              block: 'center',
              inline: 'nearest'
            });
          }
        }
      }, 100);
    }
  };

  // Handle close button click
  const handleCloseClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onClose(session.id);
  };

  // Handle maximize button click
  const handleMaximizeClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    console.log(`Maximize button clicked for session: ${session.id}`);
    onToggleMaximize(session.id);
  };

  // Handle user assist button click
  const handleUserAssistClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    // Find in-progress task for this session
    if (inProgressTask) {
      onActivateUserAssist(inProgressTask.id, e);
    }
  };

  // Start/stop recording the already-streamed session frames.
  const handleRecordClick = (e: React.MouseEvent) => {
    e.stopPropagation();

    if (recorderRef.current) {
      const recorder = recorderRef.current;
      recorderRef.current = null;
      setIsRecording(false);
      setRecordingError(null);
      void finalizeRecording(recorder);
      return;
    }

    const image = imgRef.current;
    if (!session.isConnected || !image || !image.naturalWidth) {
      setRecordingError('No frames to record yet');
      return;
    }

    const recorder = createSessionRecorder(image, selectedFps);
    if (!recorder) {
      setRecordingError('Recording is not supported by this browser');
      return;
    }

    setRecordingError(null);
    setRecordingSaved(false);
    recorderRef.current = recorder;
    recordingStartedAtRef.current = recorder.startedAt;
    setIsRecording(true);
  };

  // Every screenshot the stream delivers is pushed into the encoder.
  const handleImageLoad = () => {
    const recorder = recorderRef.current;
    if (recorder && imgRef.current) {
      recorder.drawFrame(imgRef.current);
    }
  };

  // Handle session fullscreen
  const handleFullscreen = () => {
    if (imgRef.current) {
      if (!isFullscreen) {
        if (imgRef.current.requestFullscreen) {
          imgRef.current.requestFullscreen();
        } else if ((imgRef.current as any).webkitRequestFullscreen) {
          (imgRef.current as any).webkitRequestFullscreen();
        } else if ((imgRef.current as any).mozRequestFullScreen) {
          (imgRef.current as any).mozRequestFullScreen();
        } else if ((imgRef.current as any).msRequestFullscreen) {
          (imgRef.current as any).msRequestFullscreen();
        }
        setIsFullscreen(true);
      } else {
        if (document.exitFullscreen) {
          document.exitFullscreen();
        } else if ((document as any).webkitExitFullscreen) {
          (document as any).webkitExitFullscreen();
        } else if ((document as any).mozCancelFullScreen) {
          (document as any).mozCancelFullScreen();
        } else if ((document as any).msExitFullscreen) {
          (document as any).msExitFullscreen();
        }
        setIsFullscreen(false);
      }
    }
  };

  // Fetch image for a specific session (matching original JavaScript)
  const fetchSessionImage = async (endpoint: string) => {
    try {
      const response = await fetch(CONFIG.getHttpUrl(session.ip, endpoint));
      if (!response.ok) throw new Error(`Failed to fetch ${endpoint} from ${session.ip}`);

      // Get blob first and track network usage
      const blob = await response.blob();
      const imageUrl = URL.createObjectURL(blob);
      
      // Track network usage
      if (onNetworkUpdate) {
        onNetworkUpdate(blob.size);
      }
      
      // Revoke previous URL to prevent memory leaks
      if (imgRef.current?.src) {
        URL.revokeObjectURL(imgRef.current.src);
      }
      
      if (imgRef.current) {
        imgRef.current.src = imageUrl;
        imgRef.current.classList.remove('hidden');
        // Hide placeholder when image loads successfully
        if (placeholderRef.current) {
          placeholderRef.current.classList.add('hidden');
        }
        setShowImage(true);
      }
    } catch (error) {
      console.error(`Error fetching image from ${session.ip}:`, error);
      // Reset to show placeholder on error
      if (imgRef.current) {
        imgRef.current.classList.add('hidden');
      }
      // Show placeholder on error
      if (placeholderRef.current) {
        placeholderRef.current.classList.remove('hidden');
      }
      setShowImage(false);
    }
  };

  // Start/stop video for a session (matching original JavaScript)
  const toggleSessionVideoLoop = (endpoint: string) => {
    if (session.videoInterval) {
      clearInterval(session.videoInterval);
      session.videoInterval = undefined;
      return false;
    } else {
      session.videoInterval = setInterval(() => {
        fetchSessionImage(endpoint);
      }, 1000 / selectedFps) as unknown as number; // Use selected FPS
      return true;
    }
  };

  // Start video streaming (matching original JavaScript)
  const startSessionVideo = (endpoint: string) => {
    if (!session.videoInterval) {
      session.videoInterval = setInterval(() => {
        fetchSessionImage(endpoint);
      }, 1000 / selectedFps) as unknown as number; // Use selected FPS
    }
  };

  // Auto-start video when session becomes connected
  useEffect(() => {
    if (session.isConnected) {
      // Start video streaming immediately when connected
      console.log(`Starting video stream for ${session.ip}`);
      startSessionVideo('screenshot');
      // Also fetch initial image
      fetchSessionImage('screenshot');
    } else {
      // Stop video when disconnected
      if (session.videoInterval) {
        clearInterval(session.videoInterval);
        session.videoInterval = undefined;
      }
      // Reset to show placeholder
      if (imgRef.current) {
        imgRef.current.classList.add('hidden');
      }
      if (placeholderRef.current) {
        placeholderRef.current.classList.remove('hidden');
      }
      setShowImage(false);
    }
    
    return () => {
      if (session.videoInterval) {
        clearInterval(session.videoInterval);
        session.videoInterval = undefined;
      }
    };
  }, [session.isConnected, session.ip, userAssistTaskId]);

  // Also start video when component mounts if session is already connected
  useEffect(() => {
    if (session.isConnected && !session.videoInterval) {
      console.log(`Session already connected, starting video stream for ${session.ip}`);
      startSessionVideo('screenshot');
      fetchSessionImage('screenshot');
    }
  }, []);

  // Add event listeners for focus/blur
  useEffect(() => {
    const handleFocus = () => setIsFocused(true);
    const handleBlur = () => setIsFocused(false);

    const container = sessionRef.current;
    if (container) {
      container.addEventListener('focus', handleFocus);
      container.addEventListener('blur', handleBlur);
    }

    return () => {
      if (container) {
        container.removeEventListener('focus', handleFocus);
        container.removeEventListener('blur', handleBlur);
      }
    };
  }, []);

  return (
    <div
      ref={sessionRef}
      className={`session-container ${isSelected ? 'selected' : ''} ${isUserAssistSelected ? 'user-assist-active' : ''} ${session.maximized ? 'maximized' : ''}`}
      style={{
        borderColor: isUserAssistSelected ? '#FFC107' : (isSelected ? '#4CAF50' : undefined),
        boxShadow: isUserAssistSelected ? '0 0 0 2px rgba(255, 193, 7, 0.5)' : undefined,
        // Set by the workspace grid; a widened session covers several columns.
        gridColumn: gridSpan && gridSpan > 1 ? `span ${gridSpan}` : undefined
      }}
      data-session-id={session.id}
      onClick={handleSessionClick}
      tabIndex={0}
    >
      {/* Session Header - matching original design */}
      <div className="session-header">
        <span className={`connection-status ${session.isConnected ? 'connected' : 'disconnected'}`}></span>
        <span>Session: {session.ip}</span>
        <button
          type="button"
          className={`rec-btn ${isRecording ? 'recording' : ''} ${recordingSaved ? 'saved' : ''}`}
          title={
            recordingError ||
            (isRecording
              ? `Stop recording (${formatRecordingTime(recordingElapsedMs)})`
              : 'Start recording this session')
          }
          onClick={handleRecordClick}
        >
          <span className="rec-label">Rec</span>
          <span className="rec-dot" aria-hidden="true" />
          {isRecording && (
            <span className="rec-time">{formatRecordingTime(recordingElapsedMs)}</span>
          )}
        </button>
        <button className="session-close" onClick={handleCloseClick}>×</button>
      </div>

      {/* Session Content - matching original structure exactly */}
      <div className="session-content">
        {/* Screenshot placeholder */}
        <div 
          ref={placeholderRef}
          className={`screenshot-placeholder ${showImage ? 'hidden' : ''}`}
        >
          Video stream will appear here
        </div>
        
        {/* Screenshot container with overlay */}
        <div className="screenshot-container">
          {/* Screenshot image */}
          <img
            ref={imgRef}
            className={`screenshot-img ${showImage ? '' : 'hidden'}`}
            alt={`Video stream from ${session.ip}`}
            draggable={false}
            onDoubleClick={handleFullscreen}
            onLoad={handleImageLoad}
          />
          
          {/* Image size container */}
          <div className="image-size-container">
            {/* Screenshot overlay with buttons */}
            <div className="screenshot-overlay">
              {/* Maximize button */}
              <button
                className="fullscreen-button"
                title="Maximize session (M)"
                onClick={handleMaximizeClick}
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M8 3v3a2 2 0 0 1-2 2H3m18 0h-3a2 2 0 0 1-2-2V3m0 18v-3a2 2 0 0 1 2-2h3M3 16h3a2 2 0 0 1 2 2v3"></path>
                </svg>
              </button>
              
              {/* Fullscreen button */}
              <button
                className="fullscreen-button"
                title="Toggle fullscreen (F)"
                onClick={handleFullscreen}
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"></path>
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default SessionContainer;
