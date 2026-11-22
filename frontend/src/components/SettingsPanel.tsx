import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
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
  selectedSessionIp?: string | null;
  tasks?: Array<{ id: string; status: string; message: string; sessionId?: string; sequenceNumber?: number }>;
}

// ---------------------------------------------------------------------------
// Execution engine data model
// ---------------------------------------------------------------------------

interface ExecutionAction {
  id: string;
  action: string;
  actionSequenceId?: number;
  coordinates?: { x: number; y: number };
  inputString?: string;
  keyTapString?: string;
  keyString?: string;
  duration?: number;
  repeatTimes?: number;
}

interface ExecutionSubtask {
  id: number;
  description: string;
  isActive: boolean;
  isCompleted: boolean;
  actions: ExecutionAction[];
}

interface ExecutionTask {
  id: string;
  status: string;
  message: string;
  model?: string;
  createdAt?: string;
  subtasks: ExecutionSubtask[];
}

interface ExecutionEngineState {
  tasks: ExecutionTask[];
  selectedTask: string | null;
  queuedTasks: string[];
}

const EMPTY_EXECUTION_STATE: ExecutionEngineState = {
  tasks: [],
  selectedTask: null,
  queuedTasks: []
};

type RowState = 'is-planned' | 'is-active' | 'is-done' | 'is-failed' | 'is-canceled';

// ---------------------------------------------------------------------------
// Normalization helpers
// ---------------------------------------------------------------------------

const TERMINAL_STATUSES = ['completed', 'broken', 'canceled'];

const isTerminal = (status: string) => TERMINAL_STATUSES.includes(status);

const statusLabel = (status: string): string => {
  switch (status) {
    case 'in-progress': return 'RUNNING';
    case 'completed': return 'DONE';
    case 'broken': return 'FAILED';
    case 'canceled': return 'CANCELED';
    case 'in-the-queue': return 'QUEUED';
    default: return status.toUpperCase();
  }
};

const taskSortTime = (task: ExecutionTask): number => {
  const parsed = Date.parse(task.createdAt || '');
  return Number.isFinite(parsed) ? parsed : 0;
};

const sortTasks = (tasks: ExecutionTask[]): ExecutionTask[] =>
  [...tasks].sort(
    (a, b) =>
      taskSortTime(a) - taskSortTime(b) ||
      a.id.localeCompare(b.id, undefined, { numeric: true })
  );

const normalizeAction = (raw: any, fallbackIndex: number): ExecutionAction => ({
  id: String(raw?.actionId ?? raw?.id ?? fallbackIndex),
  action: String(raw?.action ?? ''),
  actionSequenceId: raw?.actionSequenceId,
  coordinates: raw?.coordinates,
  inputString: raw?.inputString,
  keyTapString: raw?.keyTapString,
  keyString: raw?.keyString,
  duration: raw?.duration,
  repeatTimes: raw?.repeatTimes
});

const normalizeSubtask = (raw: any): ExecutionSubtask => ({
  id: Number(raw?.id ?? 0),
  description: String(raw?.description ?? ''),
  isActive: Boolean(raw?.isActive),
  isCompleted: Boolean(raw?.isCompleted),
  actions: Array.isArray(raw?.actions) ? raw.actions.map((a: any, i: number) => normalizeAction(a, i)) : []
});

const normalizeFetchedState = (data: any): ExecutionEngineState => {
  const subtasksByTask: Record<string, any[]> = data?.subtasks ?? {};
  const tasks: ExecutionTask[] = (Array.isArray(data?.tasks) ? data.tasks : []).map((raw: any) => {
    const id = String(raw?.id ?? raw?.ID ?? '');
    return {
      id,
      status: String(raw?.status ?? raw?.Status ?? 'in-the-queue'),
      message: String(raw?.message ?? raw?.Message ?? ''),
      model: raw?.model ?? raw?.Model ?? undefined,
      createdAt: raw?.createdAt ?? raw?.CreatedAt ?? undefined,
      subtasks: (subtasksByTask[id] ?? []).map((st: any) => normalizeSubtask(st))
    };
  });

  const sorted = sortTasks(tasks);
  const running = sorted.find(t => t.status === 'in-progress');
  const queued = sorted.find(t => t.status === 'in-the-queue');
  const selectedTask = running?.id ?? queued?.id ?? sorted[0]?.id ?? null;

  return {
    tasks: sorted,
    selectedTask,
    queuedTasks: Array.isArray(data?.queuedTasks) ? data.queuedTasks : []
  };
};

// Merge a freshly fetched snapshot into state that may already contain newer
// live WebSocket updates (WS status wins; the fetch fills in subtask structure).
const mergeFetchedState = (prev: ExecutionEngineState, fetched: ExecutionEngineState): ExecutionEngineState => {
  if (prev.tasks.length === 0) return fetched;

  const merged = fetched.tasks.map(fetchTask => {
    const liveTask = prev.tasks.find(t => t.id === fetchTask.id);
    if (!liveTask) return fetchTask;
    return {
      ...fetchTask,
      status: liveTask.status,
      message: liveTask.message || fetchTask.message,
      model: liveTask.model || fetchTask.model,
      subtasks: fetchTask.subtasks.map(fetchSub => {
        const liveSub = liveTask.subtasks.find(s => s.id === fetchSub.id);
        if (!liveSub) return fetchSub;
        return {
          ...fetchSub,
          isActive: liveSub.isActive,
          isCompleted: liveSub.isCompleted || fetchSub.isCompleted,
          actions: liveSub.actions.length > 0 ? liveSub.actions : fetchSub.actions
        };
      })
    };
  });

  const extras = prev.tasks.filter(t => !merged.some(m => m.id === t.id));
  const tasks = sortTasks([...merged, ...extras]);
  const selectedTask =
    prev.selectedTask && tasks.some(t => t.id === prev.selectedTask)
      ? prev.selectedTask
      : fetched.selectedTask;

  return { tasks, selectedTask, queuedTasks: fetched.queuedTasks };
};

// ---------------------------------------------------------------------------
// Pure WebSocket event reducers (always built from prev, never stale state)
// ---------------------------------------------------------------------------

const applyTaskUpdate = (
  prev: ExecutionEngineState,
  payload: any,
  followLive: boolean
): ExecutionEngineState => {
  const taskId = String(payload?.taskId ?? '');
  if (!taskId) return prev;

  const status = payload?.status != null ? String(payload.status) : '';
  let tasks: ExecutionTask[];
  const index = prev.tasks.findIndex(t => t.id === taskId);

  if (index !== -1) {
    tasks = prev.tasks.slice();
    const existing = tasks[index];
    tasks[index] = {
      ...existing,
      status: status || existing.status,
      message: payload?.message || existing.message,
      model: payload?.model || existing.model
    };
  } else {
    tasks = prev.tasks.concat({
      id: taskId,
      status: status || 'in-the-queue',
      message: String(payload?.message ?? ''),
      model: payload?.model,
      // Live events carry no timestamp; stamp at arrival so brand-new tasks
      // sort after already-known ones instead of jumping to the top.
      createdAt: payload?.createdAt ?? new Date().toISOString(),
      subtasks: []
    });
  }

  tasks = sortTasks(tasks);

  let selectedTask = prev.selectedTask;
  if (followLive) {
    const running = tasks.find(t => t.status === 'in-progress');
    if (running) {
      selectedTask = running.id;
    } else if (!selectedTask || !tasks.some(t => t.id === selectedTask)) {
      selectedTask = tasks.find(t => t.status === 'in-the-queue')?.id ?? tasks[0]?.id ?? null;
    }
  } else if (selectedTask && !tasks.some(t => t.id === selectedTask)) {
    selectedTask = tasks[0]?.id ?? null;
  }

  return { ...prev, tasks, selectedTask };
};

const applySubtaskUpdate = (prev: ExecutionEngineState, payload: any): ExecutionEngineState => {
  const taskId = String(payload?.taskId ?? '');
  const flatSubtask = payload?.subtask;
  const subtaskId = Number(payload?.subtaskId ?? flatSubtask?.id ?? NaN);
  if (!taskId || !Number.isFinite(subtaskId)) return prev;

  const description = String(payload?.description ?? flatSubtask?.description ?? '');
  const isActive = Boolean(payload?.isActive ?? flatSubtask?.isActive ?? false);
  const rawActions = payload?.actions ?? flatSubtask?.actions ?? null;

  let touched = false;
  const tasks = prev.tasks.map(task => {
    if (task.id !== taskId) return task;
    touched = true;

    const subtasks = task.subtasks.slice();
    const subIndex = subtasks.findIndex(s => s.id === subtaskId);

    if (subIndex !== -1) {
      const existing = subtasks[subIndex];
      subtasks[subIndex] = {
        ...existing,
        description: description || existing.description,
        isActive,
        isCompleted: existing.isCompleted || (!isActive && existing.isActive),
        actions: Array.isArray(rawActions)
          ? rawActions.map((a: any, i: number) => normalizeAction(a, i))
          : existing.actions
      };
    } else {
      subtasks.push({
        id: subtaskId,
        description,
        isActive,
        isCompleted: false,
        actions: Array.isArray(rawActions)
          ? rawActions.map((a: any, i: number) => normalizeAction(a, i))
          : []
      });
      subtasks.sort((a, b) => a.id - b.id);
    }

    return { ...task, subtasks };
  });

  return touched ? { ...prev, tasks } : prev;
};

const applyActionUpdate = (prev: ExecutionEngineState, payload: any): ExecutionEngineState => {
  const taskId = String(payload?.taskId ?? '');
  const subtaskId = Number(payload?.subtaskId ?? NaN);
  const actionIndex = Number(payload?.actionIndex ?? NaN);
  const rawAction = payload?.action;
  if (!taskId || !Number.isFinite(subtaskId) || !Number.isFinite(actionIndex) || !rawAction) return prev;

  let touched = false;
  const tasks = prev.tasks.map(task => {
    if (task.id !== taskId) return task;
    touched = true;

    const subtasks = task.subtasks.slice();
    let subIndex = subtasks.findIndex(s => s.id === subtaskId);
    if (subIndex === -1) {
      subtasks.push({ id: subtaskId, description: '', isActive: true, isCompleted: false, actions: [] });
      subtasks.sort((a, b) => a.id - b.id);
      subIndex = subtasks.findIndex(s => s.id === subtaskId);
    }

    const subtask = subtasks[subIndex];
    const actions = subtask.actions.slice();
    while (actions.length < actionIndex) {
      actions.push({ id: `gap-${actions.length}`, action: '' });
    }
    actions[actionIndex] = normalizeAction({ ...rawAction, actionId: rawAction.actionId ?? actionIndex }, actionIndex);

    subtasks[subIndex] = { ...subtask, actions };
    return { ...task, subtasks };
  });

  return touched ? { ...prev, tasks } : prev;
};

const applyCompletionEvent = (
  prev: ExecutionEngineState,
  payload: any,
  followLive: boolean
): ExecutionEngineState => {
  const event = payload?.event;
  if (event !== 'completed' && event !== 'broken' && event !== 'canceled') return prev;
  const existing = prev.tasks.find(t => t.id === payload?.taskId);
  if (!existing || isTerminal(existing.status)) return prev; // taskUpdate already handled it
  return applyTaskUpdate(prev, { taskId: payload.taskId, status: event }, followLive);
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const SettingsPanel: React.FC<SettingsPanelProps> = ({
  isOpen,
  onClose,
  logs,
  tokenUsage,
  activeTab: externalActiveTab,
  onTabChange: externalOnTabChange,
  selectedSessionId,
  selectedSessionIp,
  tasks: _tasks = []
}) => {
  const [internalActiveTab, setInternalActiveTab] = useState('execution');
  const activeTab = externalActiveTab || internalActiveTab;

  const [executionEngineState, setExecutionEngineState] = useState<ExecutionEngineState>(EMPTY_EXECUTION_STATE);
  const [isLoading, setIsLoading] = useState(false);
  const [followLive, setFollowLive] = useState(true);
  const [viewportHalf, setViewportHalf] = useState(180);
  const [loadedSessionKey, setLoadedSessionKey] = useState<string | null>(selectedSessionId ?? null);

  const timelineViewportRef = useRef<HTMLDivElement>(null);
  const logsContentRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<Map<string, HTMLElement>>(new Map());
  const isAutoScrollingRef = useRef(false);
  const autoScrollTimerRef = useRef<number | null>(null);

  // Always-fresh mirrors so callbacks/updaters never capture stale values.
  const selectedSessionIdRef = useRef(selectedSessionId);
  selectedSessionIdRef.current = selectedSessionId;
  const followLiveRef = useRef(followLive);
  followLiveRef.current = followLive;

  const sessionKey = selectedSessionId ?? selectedSessionIp ?? null;

  // Instantly reset when the selected session changes (before paint) so the
  // tab never shows the previous session's tasks, even for a single frame.
  if (sessionKey !== loadedSessionKey) {
    setLoadedSessionKey(sessionKey);
    setExecutionEngineState(EMPTY_EXECUTION_STATE);
    setFollowLive(true);
    setIsLoading(false);
  }

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

  // -------------------------------------------------------------------------
  // Fetch /execution-state (AbortController guards against session races)
  // -------------------------------------------------------------------------

  useEffect(() => {
    if (!isOpen || activeTab !== 'execution') return;

    const targetHost = selectedSessionIp || selectedSessionId;
    if (!targetHost) return;

    const controller = new AbortController();
    setIsLoading(true);

    fetch(CONFIG.getHttpUrl(targetHost, '/execution-state'), { signal: controller.signal })
      .then(response => {
        if (!response.ok) throw new Error(`Failed to fetch execution state: ${response.status}`);
        return response.json();
      })
      .then((data: any) => {
        if (controller.signal.aborted) return;
        const fetched = normalizeFetchedState(data);
        setExecutionEngineState(prev => mergeFetchedState(prev, fetched));
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted || (error as Error)?.name === 'AbortError') return;
        console.error('Error fetching execution state:', error);
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });

    return () => controller.abort();
  }, [isOpen, activeTab, selectedSessionId, selectedSessionIp]);

  // -------------------------------------------------------------------------
  // Real-time WebSocket updates (session-scoped, functional setState only)
  // -------------------------------------------------------------------------

  const applyExecutionEngineEvent = useCallback((raw: any, sessionId?: string) => {
    if (!raw || typeof raw !== 'object') return;

    // Scope events to the currently viewed session so other sessions never
    // bleed into the Execution Engine tab.
    if (sessionId && selectedSessionIdRef.current && sessionId !== selectedSessionIdRef.current) return;

    // Normalize both wire formats:
    //   wrapper: { type: 'executionEngineUpdate', updateType, data }
    //   flat:    { type: 'taskUpdate' | 'subtaskUpdate' | 'actionUpdate', ... }
    let eventType = String(raw.type ?? '');
    let payload = raw;
    if (eventType === 'executionEngineUpdate') {
      eventType = String(raw.updateType ?? '');
      payload = raw.data ?? {};
    }

    setExecutionEngineState(prev => {
      switch (eventType) {
        case 'taskUpdate':
          return applyTaskUpdate(prev, payload, followLiveRef.current);
        case 'subtaskUpdate':
          return applySubtaskUpdate(prev, payload);
        case 'actionUpdate':
          return applyActionUpdate(prev, payload);
        case 'completionEvent':
          return applyCompletionEvent(prev, payload, followLiveRef.current);
        default:
          return prev;
      }
    });
  }, []);

  // Re-register the window bridge through a ref so the handler never goes
  // stale (fixes the old empty-dependency closure bug).
  const eventHandlerRef = useRef(applyExecutionEngineEvent);
  useEffect(() => {
    eventHandlerRef.current = applyExecutionEngineEvent;
  }, [applyExecutionEngineEvent]);

  useEffect(() => {
    const bridge = (data: any, sessionId?: string) => eventHandlerRef.current(data, sessionId);
    (window as any).handleExecutionEngineUpdate = bridge;
    return () => {
      if ((window as any).handleExecutionEngineUpdate === bridge) {
        delete (window as any).handleExecutionEngineUpdate;
      }
    };
  }, []);

  // -------------------------------------------------------------------------
  // Centered auto-scrolling timeline
  // -------------------------------------------------------------------------

  const sortedTasks = executionEngineState.tasks;
  const selectedTask = useMemo(
    () => sortedTasks.find(t => t.id === executionEngineState.selectedTask) ?? null,
    [sortedTasks, executionEngineState.selectedTask]
  );

  // The row that the center pointer should track.
  const focusKey = useMemo(() => {
    if (!selectedTask) return null;
    const activeSubtask = selectedTask.subtasks.find(st => st.isActive);
    if (selectedTask.status === 'in-progress' && activeSubtask) {
      const lastAction = activeSubtask.actions[activeSubtask.actions.length - 1];
      if (lastAction) return `action-${selectedTask.id}-${activeSubtask.id}-${lastAction.id}`;
      return `subtask-${selectedTask.id}-${activeSubtask.id}`;
    }
    return `task-${selectedTask.id}`;
  }, [selectedTask]);

  const focusKeyRef = useRef(focusKey);
  focusKeyRef.current = focusKey;

  const registerItem = useCallback((key: string, element: HTMLElement | null) => {
    if (element) {
      itemRefs.current.set(key, element);
    } else {
      itemRefs.current.delete(key);
    }
  }, []);

  const scrollItemToCenter = useCallback((key: string | null, smooth = true) => {
    const viewport = timelineViewportRef.current;
    const element = key ? itemRefs.current.get(key) : null;
    if (!viewport || !element) return;

    const viewportRect = viewport.getBoundingClientRect();
    const elementRect = element.getBoundingClientRect();
    const delta =
      elementRect.top + elementRect.height / 2 -
      (viewportRect.top + viewportRect.height / 2);

    if (Math.abs(delta) < 2) return;

    if (autoScrollTimerRef.current !== null) {
      window.clearTimeout(autoScrollTimerRef.current);
    }
    isAutoScrollingRef.current = true;
    viewport.scrollBy({ top: delta, behavior: smooth ? 'smooth' : 'auto' });
    autoScrollTimerRef.current = window.setTimeout(() => {
      isAutoScrollingRef.current = false;
      autoScrollTimerRef.current = null;
    }, 750);
  }, []);

  // Auto-follow: glide the active task/subtask/action into the center pointer.
  useEffect(() => {
    if (!followLive || !isOpen || activeTab !== 'execution' || !focusKey) return;
    const raf = requestAnimationFrame(() => scrollItemToCenter(focusKey));
    return () => cancelAnimationFrame(raf);
  }, [followLive, isOpen, activeTab, focusKey, executionEngineState, scrollItemToCenter]);

  // Keep spacer heights in sync with half the viewport so any row can reach
  // the vertical center, including the first and last ones.
  useEffect(() => {
    const viewport = timelineViewportRef.current;
    if (!viewport) return;
    const update = () => setViewportHalf(Math.max(32, Math.floor(viewport.clientHeight / 2) - 6));
    update();
    const observer = new ResizeObserver(update);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, [isOpen, activeTab, sortedTasks.length]);

  const handleTimelineScroll = () => {
    if (isAutoScrollingRef.current) return;
    const viewport = timelineViewportRef.current;
    const activeElement = focusKeyRef.current ? itemRefs.current.get(focusKeyRef.current) : null;
    if (!viewport || !activeElement) return;

    const viewportRect = viewport.getBoundingClientRect();
    const elementRect = activeElement.getBoundingClientRect();
    const delta = Math.abs(
      elementRect.top + elementRect.height / 2 -
      (viewportRect.top + viewportRect.height / 2)
    );
    if (delta > 56) {
      setFollowLive(false); // user manually navigated away -> pause auto-follow
    }
  };

  const handleTaskSquareClick = (taskId: string) => {
    setExecutionEngineState(prev => ({ ...prev, selectedTask: taskId }));
    const running = sortedTasks.find(t => t.status === 'in-progress');
    setFollowLive(taskId === running?.id);
    requestAnimationFrame(() => scrollItemToCenter(`task-${taskId}`));
  };

  const handleResumeLive = () => {
    setFollowLive(true);
    setExecutionEngineState(prev => {
      const running = prev.tasks.find(t => t.status === 'in-progress');
      if (running) return { ...prev, selectedTask: running.id };
      const next = prev.tasks.find(t => t.status === 'in-the-queue') ?? prev.tasks[0];
      return { ...prev, selectedTask: next?.id ?? null };
    });
  };

  // -------------------------------------------------------------------------
  // Logs helpers (unchanged behavior)
  // -------------------------------------------------------------------------

  useEffect(() => {
    if (logsContentRef.current) {
      logsContentRef.current.scrollTop = logsContentRef.current.scrollHeight;
    }
  }, [logs]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isOpen && activeTab === 'logs' && (event.key === 'q' || event.key === 'Q')) {
        const activeElement = document.activeElement;
        const isInputFocused = activeElement && (
          activeElement.tagName === 'INPUT' ||
          activeElement.tagName === 'TEXTAREA' ||
          (activeElement as HTMLElement).contentEditable === 'true'
        );
        if (!isInputFocused && logsContentRef.current) {
          logsContentRef.current.scrollTop = logsContentRef.current.scrollHeight;
        }
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, activeTab]);

  // -------------------------------------------------------------------------
  // Rendering helpers
  // -------------------------------------------------------------------------

  const taskRowState = (task: ExecutionTask): RowState => {
    switch (task.status) {
      case 'in-progress': return 'is-active';
      case 'completed': return 'is-done';
      case 'broken': return 'is-failed';
      case 'canceled': return 'is-canceled';
      default: return 'is-planned';
    }
  };

  const subtaskRowState = (subtask: ExecutionSubtask): RowState => {
    if (subtask.isActive) return 'is-active';
    if (subtask.isCompleted) return 'is-done';
    return 'is-planned';
  };

  const actionRowState = (subtask: ExecutionSubtask, actionIndex: number): RowState => {
    if (subtask.isActive) {
      return actionIndex === subtask.actions.length - 1 ? 'is-active' : 'is-done';
    }
    return subtask.isCompleted ? 'is-done' : 'is-planned';
  };

  const renderActionParams = (action: ExecutionAction): string => {
    const parts: string[] = [];
    if (action.coordinates) parts.push(`(${action.coordinates.x}, ${action.coordinates.y})`);
    if (action.inputString) parts.push(`"${action.inputString}"`);
    if (action.keyTapString) parts.push(action.keyTapString);
    if (action.keyString) parts.push(`key: ${action.keyString}`);
    if (action.duration) parts.push(`${action.duration}s`);
    if (action.repeatTimes) parts.push(`x${action.repeatTimes}`);
    return parts.join(' ');
  };

  const renderTimeline = () => {
    if (!selectedSessionId && !selectedSessionIp) {
      return <div className="execution-empty">NO SESSION SELECTED</div>;
    }
    if (isLoading && sortedTasks.length === 0) {
      return <div className="execution-empty execution-loading">LOADING EXECUTION STATE…</div>;
    }
    if (sortedTasks.length === 0) {
      return <div className="execution-empty">NO TASKS YET FOR THIS SESSION</div>;
    }

    return (
      <div className="timeline-shell">
        <div
          ref={timelineViewportRef}
          className="timeline-viewport"
          onScroll={handleTimelineScroll}
        >
          <div
            className="timeline-scroll-content"
            style={{ paddingTop: viewportHalf, paddingBottom: viewportHalf }}
          >
            <div className="timeline-rail" aria-hidden="true" />
            {sortedTasks.map((task, taskIndex) => {
              const expanded = task.id === executionEngineState.selectedTask;
              const taskKey = `task-${task.id}`;
              return (
                <div key={task.id} className="tl-task-group">
                  <div
                    ref={el => registerItem(taskKey, el)}
                    className={`tl-row tl-task ${taskRowState(task)} ${focusKey === taskKey ? 'is-current' : ''}`}
                  >
                    <span className="tl-dot" aria-hidden="true" />
                    <span className="tl-tag">TASK {String(taskIndex + 1).padStart(2, '0')}</span>
                    <span className={`tl-status tl-status-${task.status}`}>{statusLabel(task.status)}</span>
                    <span className="tl-text" title={task.message}>{task.message}</span>
                  </div>

                  {expanded && task.subtasks.map(subtask => {
                    const subtaskKey = `subtask-${task.id}-${subtask.id}`;
                    return (
                      <div key={subtask.id} className="tl-subtask-group">
                        <div
                          ref={el => registerItem(subtaskKey, el)}
                          className={`tl-row tl-subtask ${subtaskRowState(subtask)} ${focusKey === subtaskKey ? 'is-current' : ''}`}
                        >
                          <span className="tl-dot" aria-hidden="true" />
                          <span className="tl-tag">SUBTASK {String(subtask.id).padStart(2, '0')}</span>
                          <span className="tl-text" title={subtask.description}>{subtask.description}</span>
                        </div>

                        {subtask.actions.map((action, actionIndex) => {
                          const actionKey = `action-${task.id}-${subtask.id}-${action.id}`;
                          return (
                            <div
                              key={action.id}
                              ref={el => registerItem(actionKey, el)}
                              className={`tl-row tl-action ${actionRowState(subtask, actionIndex)} ${focusKey === actionKey ? 'is-current' : ''}`}
                            >
                              <span className="tl-dot" aria-hidden="true" />
                              <span className="tl-tag">ACTION</span>
                              <span className="tl-action-name">{action.action || '—'}</span>
                              <span className="tl-params">{renderActionParams(action)}</span>
                            </div>
                          );
                        })}
                      </div>
                    );
                  })}
                </div>
              );
            })}
          </div>
        </div>

        {/* Centered focus pointer */}
        <div className="timeline-focus-band" aria-hidden="true">
          <span className="focus-chevron focus-chevron-top" />
          <span className="focus-line" />
          <span className="focus-chevron focus-chevron-bottom" />
        </div>

        {!followLive && (
          <button className="timeline-live-btn" onClick={handleResumeLive}>
            ● FOLLOW LIVE
          </button>
        )}
      </div>
    );
  };

  const queuedCount = executionEngineState.queuedTasks.length;

  return (
    <div className={`settings-container ${isOpen ? 'open' : ''}`}>
      <div className="settings-sidebar">
        <div className="settings-header">
          <span style={{ fontSize: '12px !important', fontWeight: 'normal !important', letterSpacing: '1px', cursor: 'default', userSelect: 'none' }}>Settings</span>
          <button className="settings-close-btn" onClick={onClose}>×</button>
        </div>

        <div className="settings-tabs">
          <button
            className={`settings-tab ${activeTab === 'execution' ? 'active' : ''}`}
            onClick={() => handleTabChange('execution')}
            data-tab="execution"
          >
            Execution engine
          </button>
          <button
            className={`settings-tab ${activeTab === 'logs' ? 'active' : ''}`}
            onClick={() => handleTabChange('logs')}
            data-tab="logs"
          >
            Logs
          </button>
          <button
            className={`settings-tab ${activeTab === 'notifications' ? 'active' : ''}`}
            onClick={() => handleTabChange('notifications')}
            data-tab="notifications"
          >
            Notifications
          </button>
        </div>

        <div className="settings-content">
          <div className={`settings-tab-content ${activeTab === 'execution' ? 'active' : ''}`} id="execution-tab">
            <div className="execution-engine">
              <div className="execution-meta-row">
                <span className="execution-meta-label">EXECUTION ENGINE</span>
                {followLive && sortedTasks.length > 0 && <span className="execution-live-pill">● LIVE</span>}
                {queuedCount > 0 && (
                  <span className="execution-queued-badge">{queuedCount} QUEUED</span>
                )}
              </div>

              {sortedTasks.length > 0 && (
                <div className="tasks-strip">
                  <div className="tasks-strip-line" />
                  <div className="tasks-strip-row">
                    {sortedTasks.map((task, index) => (
                      <button
                        key={task.id}
                        type="button"
                        className={[
                          'task-square',
                          `status-${task.status}`,
                          task.id === executionEngineState.selectedTask ? 'selected' : '',
                          task.status === 'in-progress' ? 'running' : ''
                        ].join(' ')}
                        onClick={() => handleTaskSquareClick(task.id)}
                        title={`#${index + 1} · ${statusLabel(task.status)}${task.message ? `\n${task.message}` : ''}`}
                        aria-label={`Task ${index + 1}: ${statusLabel(task.status)}`}
                        aria-pressed={task.id === executionEngineState.selectedTask}
                      />
                    ))}
                  </div>
                </div>
              )}

              {selectedTask && (
                <div className="task-header">
                  <div className="prompt-label">TASK DESCRIPTION:</div>
                  <div className="task-header-row">
                    <span className={`task-status-chip status-${selectedTask.status}`}>
                      {statusLabel(selectedTask.status)}
                    </span>
                    <span className="task-header-text">{selectedTask.message || 'Task details not available'}</span>
                  </div>
                </div>
              )}

              {renderTimeline()}
            </div>
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
