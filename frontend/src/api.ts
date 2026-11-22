import { CONFIG } from './config';

export interface AuthUser {
  id: string;
  username: string;
  activeWorkspaceId?: string;
  createdAt?: number;
}

export interface AuthData {
  token: string;
  expiresAt: number;
  user: AuthUser;
}

export interface PersistedSession {
  id: string;
  ip: string;
  name: string;
  selected: boolean;
  maximized: boolean;
}

export interface PersistedTask {
  id: string;
  sessionId?: string;
  sessionIp?: string;
  status: string;
  message: string;
  model?: string;
  thinkingLevel?: string;
  sequenceNumber?: number;
  createdAt?: number;
  completedAt?: number;
}

export interface WorkspaceData {
  id: string;
  name: string;
  isDefault: boolean;
  fps?: number;
  tokenUsage?: number;
  networkUsage?: number;
  createdAt: number;
  updatedAt: number;
  sessions: PersistedSession[];
  tasks: PersistedTask[];
}

export interface InventoryServer {
  id: string;
  name: string;
  ip: string;
  description: string;
  createdAt: number;
  updatedAt: number;
}

const AUTH_STORAGE_KEY = 'ua_auth';

export function loadStoredAuth(): AuthData | null {
  try {
    const raw = localStorage.getItem(AUTH_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as AuthData;
    if (!parsed?.token || !parsed?.expiresAt || !parsed?.user) return null;
    if (Date.now() >= parsed.expiresAt) {
      localStorage.removeItem(AUTH_STORAGE_KEY);
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function saveStoredAuth(auth: AuthData): void {
  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(auth));
}

export function clearStoredAuth(): void {
  localStorage.removeItem(AUTH_STORAGE_KEY);
}

async function apiFetch<T>(path: string, token: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init.headers as Record<string, string> | undefined),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${CONFIG.BACKEND_URL}${path}`, {
    ...init,
    credentials: 'include',
    headers,
  });

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    let msg = text;
    try {
      const parsed = JSON.parse(text);
      if (parsed?.error) msg = parsed.error;
    } catch {
      // plain text error
    }
    const err = new Error(msg || `HTTP ${res.status}`) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }

  return res.json() as Promise<T>;
}

export const api = {
  me(token: string) {
    return apiFetch<{ user: AuthUser; expiresAt: number }>('/api/auth/me', token, { method: 'GET' });
  },
  logout(token: string) {
    return apiFetch<{ result: string }>('/api/auth/logout', token, { method: 'POST' });
  },
  listWorkspaces(token: string) {
    return apiFetch<{ workspaces: WorkspaceData[] }>('/api/workspaces', token, { method: 'GET' });
  },
  createWorkspace(token: string, name: string) {
    return apiFetch<WorkspaceData>('/api/workspaces', token, {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  },
  renameWorkspace(token: string, id: string, name: string) {
    return apiFetch<{ result: string }>(`/api/workspaces/${encodeURIComponent(id)}`, token, {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    });
  },
  deleteWorkspace(token: string, id: string) {
    return apiFetch<{ result: string }>(`/api/workspaces/${encodeURIComponent(id)}`, token, {
      method: 'DELETE',
    });
  },
  setActiveWorkspace(token: string, activeWorkspaceId: string) {
    return apiFetch<{ result: string }>('/api/user/preferences', token, {
      method: 'PUT',
      body: JSON.stringify({ activeWorkspaceId }),
    });
  },
  saveWorkspaceState(
    token: string,
    id: string,
    payload: {
      sessions: PersistedSession[];
      tasks: PersistedTask[];
      fps: number;
      tokenUsage: number;
      networkUsage: number;
    }
  ) {
    return apiFetch<{ result: string }>(`/api/workspaces/${encodeURIComponent(id)}/state`, token, {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
  },
  listInventory(token: string) {
    return apiFetch<{ servers: InventoryServer[] }>('/api/inventory', token, { method: 'GET' });
  },
  createInventoryServer(token: string, payload: { name: string; ip: string; description: string }) {
    return apiFetch<InventoryServer>('/api/inventory', token, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },
  updateInventoryServer(token: string, id: string, payload: { name: string; ip: string; description: string }) {
    return apiFetch<InventoryServer>(`/api/inventory/${encodeURIComponent(id)}`, token, {
      method: 'PATCH',
      body: JSON.stringify(payload),
    });
  },
  deleteInventoryServer(token: string, id: string) {
    return apiFetch<{ result: string }>(`/api/inventory/${encodeURIComponent(id)}`, token, {
      method: 'DELETE',
    });
  },
};
