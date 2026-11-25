import React, { useState, useEffect, useRef, useCallback } from 'react';
import './App.css';
import './Shell.css';
import { CONFIG } from './config';
import {
  api,
  AuthData,
  WorkspaceData,
  InventoryServer,
  PersistedSession,
  PersistedTask,
  loadStoredAuth,
  saveStoredAuth,
  clearStoredAuth,
} from './api';
import LoginPage from './components/LoginPage';
import WorkspaceApp from './components/WorkspaceApp';
import InventoryView from './components/InventoryView';

type NavTab = 'workspaces' | 'inventory';

const App: React.FC = () => {
  const [auth, setAuth] = useState<AuthData | null>(() => loadStoredAuth());
  const [authChecking, setAuthChecking] = useState<boolean>(true);
  const [activeNav, setActiveNav] = useState<NavTab>('workspaces');
  const [workspaces, setWorkspaces] = useState<WorkspaceData[]>([]);
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(null);
  const [inventory, setInventory] = useState<InventoryServer[]>([]);
  const [disposableAvailable, setDisposableAvailable] = useState<boolean>(false);
  const [isExpanded, setIsExpanded] = useState<boolean>(false);
  const [settingsOpenByWorkspace, setSettingsOpenByWorkspace] = useState<Record<string, boolean>>({});
  const [settingsTabsByWorkspace, setSettingsTabsByWorkspace] = useState<Record<string, Record<string, string>>>({});
  const [userAssistTaskIdByWorkspace, setUserAssistTaskIdByWorkspace] = useState<Record<string, string | null>>({});
  const [modelSelectionByWorkspace, setModelSelectionByWorkspace] = useState<Record<
    string,
    { modelId: string; provider: string; thinkingLevel: string } | null
  >>({});

  // Global Alt+W toggle for zooming in/out of the active workspace
  useEffect(() => {
    if (!auth || activeNav !== 'workspaces') return;
    const handleAltW = (e: KeyboardEvent) => {
      if (e.altKey && (e.key === 'w' || e.key === 'W')) {
        e.preventDefault();
        setIsExpanded((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleAltW);
    return () => window.removeEventListener('keydown', handleAltW);
  }, [auth, activeNav]);

  // New workspace inline form state
  const [creatingWorkspace, setCreatingWorkspace] = useState(false);
  const [newWorkspaceName, setNewWorkspaceName] = useState('');
  const [workspaceToDelete, setWorkspaceToDelete] = useState<WorkspaceData | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Dynamic scale calculation for the zoomed-out workspace preview
  const stageRef = useRef<HTMLDivElement>(null);
  const [previewSize, setPreviewSize] = useState<{ width: number; height: number; scale: number }>({
    width: 800,
    height: 500,
    scale: 0.6,
  });

  // Validate stored auth (or cookie) on mount for instant 7-day auto-login
  useEffect(() => {
    let cancelled = false;
    const stored = loadStoredAuth();
    const token = stored?.token || '';

    api
      .me(token)
      .then((res) => {
        if (cancelled) return;
        const nextAuth: AuthData = {
          token: token,
          expiresAt: res.expiresAt,
          user: res.user,
        };
        saveStoredAuth(nextAuth);
        setAuth(nextAuth);
      })
      .catch(() => {
        if (cancelled) return;
        clearStoredAuth();
        setAuth(null);
      })
      .finally(() => {
        if (!cancelled) setAuthChecking(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  // Load workspaces & inventory whenever user is authenticated, and sync live tasks across all workspaces
  useEffect(() => {
    if (!auth) {
      setWorkspaces([]);
      setActiveWorkspaceId(null);
      setInventory([]);
      setDisposableAvailable(false);
      setIsExpanded(false);
      return;
    }

    let cancelled = false;
    const token = auth.token;

    // The disposable-container button is only offered when the backend runs
    // inside docker compose with a working container controller.
    api
      .getCapabilities(token)
      .then((caps) => {
        if (!cancelled) setDisposableAvailable(!!caps.disposableContainers);
      })
      .catch(() => {
        if (!cancelled) setDisposableAvailable(false);
      });

    Promise.all([api.listWorkspaces(token), api.listInventory(token)])
      .then(async ([wsRes, invRes]) => {
        if (cancelled) return;
        const list = wsRes.workspaces || [];
        setWorkspaces(list);
        setInventory(invRes.servers || []);

        const preferredWs =
          (auth.user.activeWorkspaceId && list.find((w) => w.id === auth.user.activeWorkspaceId)) ||
          list.find((w) => w.isDefault) ||
          list[0];
        if (preferredWs) {
          setActiveWorkspaceId((prev) =>
            prev && list.some((w) => w.id === prev) ? prev : preferredWs.id
          );
        }

        // Background runaway-task check: query /execution-state for every saved session across all workspaces
        const uniqueChecks: Array<{ wsId: string; sessionId: string; ip: string }> = [];
        for (const ws of list) {
          for (const s of ws.sessions) {
            uniqueChecks.push({ wsId: ws.id, sessionId: s.id, ip: s.ip });
          }
        }

        if (uniqueChecks.length > 0) {
          const results = await Promise.allSettled(
            uniqueChecks.map(async (item) => {
              const res = await fetch(CONFIG.getHttpUrl(item.ip, '/execution-state'), {
                signal: AbortSignal.timeout(4000),
              });
              if (!res.ok) return null;
              const data = await res.json();
              return { ...item, serverTasks: Array.isArray(data?.tasks) ? data.tasks : [] };
            })
          );

          if (cancelled) return;

          setWorkspaces((prevList) =>
            prevList.map((ws) => {
              const wsUpdates = results
                .filter((r): r is PromiseFulfilledResult<any> => r.status === 'fulfilled' && r.value && r.value.wsId === ws.id)
                .map((r) => r.value);
              if (wsUpdates.length === 0) return ws;

              const mergedTasks = [...ws.tasks];
              let maxSeq = mergedTasks.reduce((m, t) => Math.max(m, t.sequenceNumber || 0), 0);
              let changed = false;

              for (const upd of wsUpdates) {
                for (const st of upd.serverTasks) {
                  if (!st?.id) continue;
                  const idx = mergedTasks.findIndex((t) => t.id === st.id);
                  const isTerminal = st.status === 'completed' || st.status === 'canceled' || st.status === 'broken';
                  if (idx >= 0) {
                    if (mergedTasks[idx].status !== st.status) {
                      mergedTasks[idx] = {
                        ...mergedTasks[idx],
                        status: st.status,
                        model: st.model || mergedTasks[idx].model,
                        completedAt: isTerminal ? mergedTasks[idx].completedAt || Date.now() : undefined,
                      };
                      changed = true;
                    }
                  } else {
                    maxSeq += 1;
                    mergedTasks.push({
                      id: st.id,
                      sessionId: upd.sessionId,
                      sessionIp: upd.ip,
                      status: st.status,
                      message: st.message || '(Running task)',
                      model: st.model,
                      thinkingLevel: st.thinkingLevel,
                      sequenceNumber: maxSeq,
                      createdAt: typeof st.createdAt === 'number' ? st.createdAt : Date.now(),
                      completedAt: isTerminal ? Date.now() : undefined,
                    });
                    changed = true;
                  }
                }
              }

              return changed ? { ...ws, tasks: mergedTasks } : ws;
            })
          );
        }
      })
      .catch((err) => {
        console.error('Failed to load workspaces/inventory:', err);
        if (err?.status === 401) {
          clearStoredAuth();
          setAuth(null);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [auth]);

  // Measure available stage area and compute exact scale so 100vw x 100vh fits cleanly
  useEffect(() => {
    const updateScale = () => {
      const el = stageRef.current;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const pad = 56; // 28px padding on each side
      const availW = Math.max(200, rect.width - pad);
      const availH = Math.max(150, rect.height - pad);
      const vpW = window.innerWidth || 1280;
      const vpH = window.innerHeight || 720;
      const scale = Math.min(availW / vpW, availH / vpH, 0.92);
      setPreviewSize({
        width: Math.floor(vpW * scale),
        height: Math.floor(vpH * scale),
        scale,
      });
    };

    updateScale();
    const observer = new ResizeObserver(updateScale);
    if (stageRef.current) observer.observe(stageRef.current);
    window.addEventListener('resize', updateScale);
    return () => {
      observer.disconnect();
      window.removeEventListener('resize', updateScale);
    };
  }, [activeNav, isExpanded, auth]);

  const handleAuthSuccess = (data: AuthData) => {
    saveStoredAuth(data);
    setAuth(data);
  };

  const handleLogout = async () => {
    if (auth) {
      try {
        await api.logout(auth.token);
      } catch {
        // ignore network errors on logout
      }
    }
    clearStoredAuth();
    setAuth(null);
  };

  const selectWorkspace = useCallback(
    (wsId: string) => {
      setActiveWorkspaceId(wsId);
      if (auth) {
        const updatedAuth: AuthData = {
          ...auth,
          user: { ...auth.user, activeWorkspaceId: wsId },
        };
        saveStoredAuth(updatedAuth);
        api.setActiveWorkspace(auth.token, wsId).catch((err) => {
          console.error('Failed to persist active workspace:', err);
        });
      }
    },
    [auth]
  );

  const handleCreateWorkspace = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!auth) return;
    const trimmed = newWorkspaceName.trim();
    if (!trimmed) return;
    try {
      const created = await api.createWorkspace(auth.token, trimmed);
      setWorkspaces((prev) => [...prev, created]);
      selectWorkspace(created.id);
      setNewWorkspaceName('');
      setCreatingWorkspace(false);
    } catch (err) {
      console.error('Failed to create workspace:', err);
    }
  };

  const cancelWorkspaceTasks = async (wsId: string): Promise<void> => {
    const ws = workspaces.find((w) => w.id === wsId);
    if (!ws) return;

    const nonTerminalStatuses = ['in-the-queue', 'in-progress', 'paused'];
    const activeTasks = ws.tasks.filter(
      (task) => nonTerminalStatuses.includes(task.status) && task.id && task.sessionIp
    );

    for (const task of activeTasks) {
      const response = await fetch(
        CONFIG.getHttpUrl(task.sessionIp as string, `/task-cancel?taskId=${encodeURIComponent(task.id)}`),
        { method: 'POST', headers: { 'Content-Type': 'application/json' } }
      );
      if (!response.ok) {
        throw new Error(
          `Failed to cancel task ${task.id} on server ${task.sessionIp}: ${response.statusText}`
        );
      }
    }
  };

  const handleDeleteWorkspace = async (wsId: string) => {
    if (!auth) return;
    try {
      // Cancel every queued/running/paused task of this workspace first.
      // The workspace is only deleted if all cancellations succeed.
      await cancelWorkspaceTasks(wsId);
      await api.deleteWorkspace(auth.token, wsId);
      setWorkspaces((prev) => {
        const remaining = prev.filter((w) => w.id !== wsId);
        if (activeWorkspaceId === wsId) {
          const fallback = remaining.find((w) => w.isDefault) || remaining[0];
          if (fallback) {
            selectWorkspace(fallback.id);
          } else {
            setActiveWorkspaceId(null);
          }
        }
        return remaining;
      });
      setWorkspaceToDelete(null);
      setDeleteError(null);
    } catch (err) {
      console.error('Failed to delete workspace:', err);
      setDeleteError(
        err instanceof Error
          ? `${err.message}. Workspace was not deleted.`
          : 'Failed to cancel workspace tasks. Workspace was not deleted.'
      );
    }
  };

  const handlePersistWorkspaceState = useCallback(
    (
      wsId: string,
      payload: {
        sessions: PersistedSession[];
        tasks: PersistedTask[];
        fps: number;
        tokenUsage: number;
        networkUsage: number;
      }
    ) => {
      if (!auth) return;
      setWorkspaces((prev) =>
        prev.map((w) =>
          w.id === wsId
            ? {
                ...w,
                sessions: payload.sessions,
                tasks: payload.tasks,
                fps: payload.fps,
                tokenUsage: payload.tokenUsage,
                networkUsage: payload.networkUsage,
                updatedAt: Date.now(),
              }
            : w
        )
      );
      api.saveWorkspaceState(auth.token, wsId, payload).catch((err) => {
        console.error('Failed to save workspace state:', err);
      });
    },
    [auth]
  );

  // Inventory CRUD
  const handleAddServer = async (payload: { name: string; ip: string; description: string }) => {
    if (!auth) return;
    const created = await api.createInventoryServer(auth.token, payload);
    setInventory((prev) => [created, ...prev]);
  };

  const handleCreateDisposable = async () => {
    if (!auth) return;
    const created = await api.createDisposableContainer(auth.token);
    setInventory((prev) => [created, ...prev]);
  };

  const handleUpdateServer = async (id: string, payload: { name: string; ip: string; description: string }) => {
    if (!auth) return;
    const updated = await api.updateInventoryServer(auth.token, id, payload);
    setInventory((prev) => prev.map((s) => (s.id === id ? updated : s)));
  };

  const handleDeleteServer = async (id: string) => {
    if (!auth) return;
    await api.deleteInventoryServer(auth.token, id);
    setInventory((prev) => prev.filter((s) => s.id !== id));
  };

  const handleConnectFromInventory = (ip: string) => {
    setActiveNav('workspaces');
    setIsExpanded(true);
    setTimeout(() => {
      document.dispatchEvent(new CustomEvent('connectSession', { detail: ip }));
    }, 60);
  };

  if (authChecking) {
    return <div className="shell-loading">Loading session...</div>;
  }

  if (!auth) {
    return <LoginPage onAuthSuccess={handleAuthSuccess} />;
  }

  const activeWorkspace =
    workspaces.find((w) => w.id === activeWorkspaceId) ||
    workspaces.find((w) => w.isDefault) ||
    workspaces[0] ||
    null;

  // Map of server IPs already connected in OTHER workspaces (ip -> workspace name)
  const occupiedSessionIps: Record<string, string> = {};
  if (activeWorkspace) {
    for (const ws of workspaces) {
      if (ws.id === activeWorkspace.id) continue;
      for (const s of ws.sessions) {
        if (s.ip && !occupiedSessionIps[s.ip]) {
          occupiedSessionIps[s.ip] = ws.name;
        }
      }
    }
  }

  return (
    <div className="app-shell">
      {/* Persistent Left Sidebar */}
      <aside className="shell-nav">
        <div className="shell-brand">
          <span className="shell-brand-mark" aria-hidden="true" />
          <span className="shell-brand-name">useless-agent</span>
        </div>

        <nav className="shell-nav-items" aria-label="Primary navigation">
          <div className="shell-nav-section-label">Navigation</div>
          <button
            type="button"
            className={`shell-nav-btn ${activeNav === 'workspaces' ? 'active' : ''}`}
            onClick={() => {
              setActiveNav('workspaces');
              setIsExpanded(false);
            }}
          >
            <svg className="shell-nav-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <rect x="2" y="2" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" />
              <rect x="9" y="2" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" />
              <rect x="2" y="9" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" />
              <rect x="9" y="9" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" />
            </svg>
            <span className="shell-nav-label">Workspaces</span>
            <span className="shell-nav-count">{workspaces.length}</span>
          </button>

          <button
            type="button"
            className={`shell-nav-btn ${activeNav === 'inventory' ? 'active' : ''}`}
            onClick={() => {
              setActiveNav('inventory');
              setIsExpanded(false);
            }}
          >
            <svg className="shell-nav-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <rect x="2" y="2.5" width="12" height="4.5" rx="1.2" stroke="currentColor" strokeWidth="1.4" />
              <rect x="2" y="9" width="12" height="4.5" rx="1.2" stroke="currentColor" strokeWidth="1.4" />
              <circle cx="4.5" cy="4.75" r="0.8" fill="currentColor" />
              <circle cx="4.5" cy="11.25" r="0.8" fill="currentColor" />
            </svg>
            <span className="shell-nav-label">Inventory</span>
            <span className="shell-nav-count">{inventory.length}</span>
          </button>
        </nav>

        <div className="shell-nav-footer">
          <div className="shell-user-info">
            <div className="shell-user-avatar">{auth.user.username.slice(0, 1)}</div>
            <span className="shell-username" title={auth.user.username}>
              {auth.user.username}
            </span>
          </div>
          <button type="button" className="shell-logout-btn" onClick={handleLogout} title="Sign out" aria-label="Sign out">
            <svg viewBox="0 0 16 16" fill="none" width="14" height="14" aria-hidden="true">
              <path d="M6 2.5H3.5A1.5 1.5 0 0 0 2 4v8a1.5 1.5 0 0 0 1.5 1.5H6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
              <path d="M10.5 11L13.5 8L10.5 5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
              <path d="M13.5 8H6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
            </svg>
          </button>
        </div>
      </aside>

      {/* Workspaces View */}
      {activeNav === 'workspaces' && (
        <>
          {/* Second Sidebar: Workspaces List */}
          <aside className="workspaces-sidebar">
            <div className="workspaces-sidebar-header">
              <div className="workspaces-sidebar-title-group">
                <span className="workspaces-sidebar-title">Workspaces</span>
                <span className="workspaces-sidebar-count">{workspaces.length}</span>
              </div>
              {!creatingWorkspace && (
                <button
                  type="button"
                  className="new-workspace-icon-btn"
                  onClick={() => setCreatingWorkspace(true)}
                  title="New workspace"
                  aria-label="New workspace"
                >
                  <svg viewBox="0 0 16 16" fill="none" width="14" height="14" aria-hidden="true">
                    <path d="M8 3.25v9.5M3.25 8h9.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                  </svg>
                </button>
              )}
            </div>

            {creatingWorkspace && (
              <div className="workspaces-create-panel">
                <form className="new-workspace-form" onSubmit={handleCreateWorkspace}>
                  <input
                    type="text"
                    className="new-workspace-input"
                    placeholder="Workspace name..."
                    value={newWorkspaceName}
                    onChange={(e) => setNewWorkspaceName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Escape') {
                        setCreatingWorkspace(false);
                        setNewWorkspaceName('');
                      }
                    }}
                    autoFocus
                  />
                  <div className="new-workspace-form-actions">
                    <button
                      type="button"
                      className="new-workspace-cancel"
                      onClick={() => {
                        setCreatingWorkspace(false);
                        setNewWorkspaceName('');
                      }}
                    >
                      Cancel
                    </button>
                    <button type="submit" className="new-workspace-confirm" disabled={!newWorkspaceName.trim()}>
                      Create
                    </button>
                  </div>
                </form>
              </div>
            )}

            <div className="workspaces-list">
              {workspaces.map((ws) => {
                const isSelected = activeWorkspace?.id === ws.id;
                const activeTaskCount = ws.tasks.filter(
                  (t) => t.status === 'in-progress' || t.status === 'in-the-queue'
                ).length;

                return (
                  <div
                    key={ws.id}
                    className={`workspace-item ${isSelected ? 'active' : ''}`}
                    onClick={() => selectWorkspace(ws.id)}
                  >
                    <div className="workspace-item-header">
                      <span className="workspace-item-name" title={ws.name}>{ws.name}</span>
                      {!ws.isDefault && (
                        <button
                          type="button"
                          className="workspace-delete-btn"
                          title="Delete workspace"
                          aria-label={`Delete ${ws.name}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteError(null);
                            setWorkspaceToDelete(ws);
                          }}
                        >
                          <svg viewBox="0 0 16 16" fill="none" width="12" height="12" aria-hidden="true">
                            <path d="M4.5 4.5l7 7M11.5 4.5l-7 7" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                          </svg>
                        </button>
                      )}
                    </div>
                    <div className="workspace-item-meta">
                      <span className="workspace-session-count">
                        {ws.sessions.length} {ws.sessions.length === 1 ? 'session' : 'sessions'}
                      </span>
                      {activeTaskCount > 0 && (
                        <span className="workspace-active-badge">
                          <span className="workspace-active-dot" />
                          {activeTaskCount} active
                        </span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </aside>

          {/* Workspace Stage (keeps WorkspaceApp mounted seamlessly across zoom in/out) */}
          <main className="workspace-stage" ref={stageRef}>
            {activeWorkspace && (
              <div
                className={isExpanded ? 'workspace-expanded-root' : 'workspace-preview-frame'}
                style={
                  isExpanded
                    ? undefined
                    : {
                        width: `${previewSize.width}px`,
                        height: `${previewSize.height}px`,
                      }
                }
                onClick={(e) => {
                  if (!isExpanded && e.button === 0) {
                    setIsExpanded(true);
                  }
                }}
              >
                <div
                  className={isExpanded ? undefined : 'workspace-scaled-inner'}
                  style={
                    isExpanded
                      ? { width: '100%', height: '100%' }
                      : { transform: `scale(${previewSize.scale})` }
                  }
                >
                  <WorkspaceApp
                    key={activeWorkspace.id}
                    workspaceId={activeWorkspace.id}
                    workspaceName={activeWorkspace.name}
                    initialSessions={activeWorkspace.sessions}
                    initialTasks={activeWorkspace.tasks}
                    initialFps={activeWorkspace.fps || 5}
                    initialTokenUsage={activeWorkspace.tokenUsage || 0}
                    initialNetworkUsage={activeWorkspace.networkUsage || 0}
                    inventoryServers={inventory}
                    occupiedSessionIps={occupiedSessionIps}
                    isExpanded={isExpanded}
                    previewScale={previewSize.scale}
                    settingsOpen={!!settingsOpenByWorkspace[activeWorkspace.id]}
                    onToggleSettings={() =>
                      setSettingsOpenByWorkspace((prev) => ({
                        ...prev,
                        [activeWorkspace.id]: !prev[activeWorkspace.id],
                      }))
                    }
                    sessionSettingsTabs={settingsTabsByWorkspace[activeWorkspace.id] || {}}
                    onChangeSessionSettingsTab={(sessionId, tab) =>
                      setSettingsTabsByWorkspace((prev) => ({
                        ...prev,
                        [activeWorkspace.id]: {
                          ...(prev[activeWorkspace.id] || {}),
                          [sessionId]: tab,
                        },
                      }))
                    }
                    initialUserAssistTaskId={userAssistTaskIdByWorkspace[activeWorkspace.id] ?? null}
                    onUserAssistTaskIdChange={(taskId) =>
                      setUserAssistTaskIdByWorkspace((prev) => ({
                        ...prev,
                        [activeWorkspace.id]: taskId,
                      }))
                    }
                    initialModelSelection={modelSelectionByWorkspace[activeWorkspace.id] ?? null}
                    onModelSelectionChange={(selection) =>
                      setModelSelectionByWorkspace((prev) => {
                        const current = prev[activeWorkspace.id];
                        if (
                          current?.modelId === selection.modelId &&
                          current?.provider === selection.provider &&
                          current?.thinkingLevel === selection.thinkingLevel
                        ) {
                          return prev;
                        }
                        return { ...prev, [activeWorkspace.id]: selection };
                      })
                    }
                    onZoomOut={() => setIsExpanded(false)}
                    onPersistState={handlePersistWorkspaceState}
                  />
                </div>
                {!isExpanded && (
                  <div className="workspace-preview-overlay">
                    <span className="workspace-preview-hint">
                      Click to enter {activeWorkspace.name}
                    </span>
                  </div>
                )}
              </div>
            )}
          </main>
        </>
      )}

      {/* Inventory View */}
      {activeNav === 'inventory' && (
        <InventoryView
          servers={inventory}
          disposableAvailable={disposableAvailable}
          onCreateDisposable={handleCreateDisposable}
          onAddServer={handleAddServer}
          onUpdateServer={handleUpdateServer}
          onDeleteServer={handleDeleteServer}
          onConnectServer={handleConnectFromInventory}
        />
      )}

      {/* Delete Workspace Confirmation Dialog */}
      {workspaceToDelete && (
        <div
          className="shell-modal-backdrop"
          onClick={() => {
            setWorkspaceToDelete(null);
            setDeleteError(null);
          }}
        >
          <div className="shell-modal" onClick={(e) => e.stopPropagation()}>
            <div className="shell-modal-title">Delete Workspace</div>
            <p className="shell-modal-body">
              Delete workspace <strong>{workspaceToDelete.name}</strong>? All saved sessions and tasks in this workspace will be permanently removed.
            </p>
            {deleteError && (
              <p className="shell-modal-body" style={{ color: '#f87171' }}>{deleteError}</p>
            )}
            <div className="shell-modal-actions">
              <button
                type="button"
                className="shell-modal-cancel"
                onClick={() => {
                  setWorkspaceToDelete(null);
                  setDeleteError(null);
                }}
              >
                Cancel
              </button>
              <button
                type="button"
                className="shell-modal-danger"
                onClick={() => handleDeleteWorkspace(workspaceToDelete.id)}
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default App;
