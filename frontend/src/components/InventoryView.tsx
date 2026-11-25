import React, { useEffect, useState } from 'react';
import { InventoryServer } from '../api';
import { CONFIG } from '../config';
import './InventoryView.css';

interface InventoryViewProps {
  servers: InventoryServer[];
  /** True only when the backend can create disposable containers (docker compose). */
  disposableAvailable: boolean;
  onCreateDisposable: () => Promise<void>;
  onAddServer: (payload: { name: string; ip: string; description: string }) => Promise<void>;
  onUpdateServer: (id: string, payload: { name: string; ip: string; description: string }) => Promise<void>;
  onDeleteServer: (id: string) => Promise<void>;
  onConnectServer: (ip: string) => void;
}

const READY_PROBE_INTERVAL_MS = 1500;
const READY_PROBE_MAX_ATTEMPTS = 60;

/**
 * A freshly created disposable container needs a few seconds before its
 * backend answers. Poll /ping so the UI can show "Starting…" instead of a
 * connection error. After a while we stop waiting and let the user try.
 */
const useContainerReady = (ip: string | null): boolean => {
  const [ready, setReady] = useState(ip === null);

  useEffect(() => {
    if (ip === null) {
      setReady(true);
      return;
    }
    let cancelled = false;
    let timer: number | undefined;
    setReady(false);

    const probe = async (attempt: number) => {
      try {
        const res = await fetch(CONFIG.getHttpUrl(ip, '/ping'), { method: 'GET' });
        if (res.ok) {
          if (!cancelled) setReady(true);
          return;
        }
      } catch {
        // Not up yet.
      }
      if (cancelled) return;
      if (attempt >= READY_PROBE_MAX_ATTEMPTS) {
        setReady(true);
        return;
      }
      timer = window.setTimeout(() => probe(attempt + 1), READY_PROBE_INTERVAL_MS);
    };
    probe(0);

    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [ip]);

  return ready;
};

const InventoryView: React.FC<InventoryViewProps> = ({
  servers,
  disposableAvailable,
  onCreateDisposable,
  onAddServer,
  onUpdateServer,
  onDeleteServer,
  onConnectServer,
}) => {
  const [filter, setFilter] = useState('');
  const [creatingDisposable, setCreatingDisposable] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [ip, setIp] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const resetForm = () => {
    setName('');
    setIp('');
    setDescription('');
    setEditingId(null);
    setError(null);
    setShowForm(false);
  };

  const openCreateForm = () => {
    if (showForm && !editingId) {
      resetForm();
    } else {
      setEditingId(null);
      setName('');
      setIp('');
      setDescription('');
      setError(null);
      setShowForm(true);
    }
  };

  const startEdit = (srv: InventoryServer) => {
    setEditingId(srv.id);
    setName(srv.name);
    setIp(srv.ip);
    setDescription(srv.description || '');
    setError(null);
    setShowForm(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedIp = ip.trim();
    if (!trimmedIp) {
      setError('IP address is required.');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const payload = {
        name: name.trim() || trimmedIp,
        ip: trimmedIp,
        description: description.trim(),
      };
      if (editingId) {
        await onUpdateServer(editingId, payload);
      } else {
        await onAddServer(payload);
      }
      resetForm();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save server');
    } finally {
      setSubmitting(false);
    }
  };

  const handleCreateDisposable = async () => {
    if (!disposableAvailable || creatingDisposable) return;
    setCreatingDisposable(true);
    setError(null);
    try {
      await onCreateDisposable();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create container');
    } finally {
      setCreatingDisposable(false);
    }
  };

  const filtered = servers.filter((s) => {
    const q = filter.trim().toLowerCase();
    if (!q) return true;
    return (
      s.name.toLowerCase().includes(q) ||
      s.ip.toLowerCase().includes(q) ||
      (s.description || '').toLowerCase().includes(q)
    );
  });

  return (
    <div className="inventory-view">
      <div className="inventory-container">
        <header className="inventory-header">
          <div className="inventory-header-copy">
            <div className="inventory-title-row">
              <h1 className="inventory-title">Inventory</h1>
              <span className="inventory-count-pill">
                {servers.length} {servers.length === 1 ? 'server' : 'servers'}
              </span>
            </div>
            <p className="inventory-subtitle">
              Saved hosts available for one-click connection in any workspace
            </p>
          </div>

          <div className="inventory-header-actions">
            <button
              type="button"
              className="inventory-secondary-btn inventory-disposable-btn"
              onClick={handleCreateDisposable}
              disabled={!disposableAvailable || creatingDisposable}
              title={
                disposableAvailable
                  ? 'Start a disposable copy of the backend container and add it to the inventory'
                  : 'Only available when the app runs in docker compose with a configured container controller'
              }
            >
              {creatingDisposable ? (
                <span className="inventory-spinner" aria-hidden="true" />
              ) : (
                <svg viewBox="0 0 16 16" fill="none" width="14" height="14" aria-hidden="true">
                  <path d="M2.25 7.25h1.6M5.05 7.25h1.6M7.85 7.25h1.6" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
                  <path d="M3.1 8.6v1.9c0 .7.55 1.25 1.25 1.25h6.3c.95 0 1.8-.55 2.2-1.4l.85-1.7c.13-.26-.06-.57-.36-.57H4.2c-.6 0-1.1.25-1.1.52Z" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round" />
                  <path d="M5.6 5.2v1.5M7.4 4.6v2.1M9.2 5.5v1.2" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
                </svg>
              )}
              <span>{creatingDisposable ? 'Creating…' : '+ Disposable container'}</span>
            </button>

            <button
              type="button"
              className={`inventory-primary-btn ${showForm && !editingId ? 'is-cancel' : ''}`}
              onClick={openCreateForm}
            >
              {showForm && !editingId ? (
                <span>Cancel</span>
              ) : (
                <>
                  <svg viewBox="0 0 16 16" fill="none" width="13" height="13" aria-hidden="true">
                    <path d="M8 3.25v9.5M3.25 8h9.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
                  </svg>
                  <span>Add Server</span>
                </>
              )}
            </button>
          </div>
        </header>

        {showForm && (
          <form className="inventory-form" onSubmit={handleSubmit}>
            <div className="inventory-form-header">
              <span className="inventory-form-title">
                {editingId ? 'Edit Server' : 'New Server'}
              </span>
            </div>
            <div className="inventory-form-grid">
              <div className="inventory-field">
                <label htmlFor="inv-name">Label / Name</label>
                <input
                  id="inv-name"
                  type="text"
                  placeholder="e.g. Build VM 01"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoFocus
                />
              </div>
              <div className="inventory-field">
                <label htmlFor="inv-ip">IP Address *</label>
                <input
                  id="inv-ip"
                  type="text"
                  placeholder="e.g. 192.168.1.42 or ::1"
                  value={ip}
                  onChange={(e) => setIp(e.target.value)}
                  required
                />
              </div>
              <div className="inventory-field inventory-field-full">
                <label htmlFor="inv-desc">Description</label>
                <input
                  id="inv-desc"
                  type="text"
                  placeholder="Optional notes about this host"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </div>
            {error && <div className="inventory-error">{error}</div>}
            <div className="inventory-form-actions">
              <button type="button" className="inventory-secondary-btn" onClick={resetForm}>
                Cancel
              </button>
              <button type="submit" className="inventory-primary-btn" disabled={submitting}>
                {submitting ? 'Saving...' : editingId ? 'Save Changes' : 'Add Server'}
              </button>
            </div>
          </form>
        )}

        <div className="inventory-toolbar">
          <div className="inventory-search-wrapper">
            <svg className="inventory-search-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <circle cx="7.25" cy="7.25" r="4.5" stroke="currentColor" strokeWidth="1.4" />
              <path d="M10.75 10.75L13.75 13.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
            </svg>
            <input
              type="text"
              className="inventory-search-input"
              placeholder="Filter by name, IP address, or description..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
            {filter && (
              <button
                type="button"
                className="inventory-search-clear"
                onClick={() => setFilter('')}
                aria-label="Clear filter"
              >
                <svg viewBox="0 0 16 16" fill="none" width="12" height="12" aria-hidden="true">
                  <path d="M4.5 4.5l7 7M11.5 4.5l-7 7" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                </svg>
              </button>
            )}
          </div>
          {filter.trim() && (
            <span className="inventory-filter-meta">
              {filtered.length} of {servers.length}
            </span>
          )}
        </div>

        {filtered.length === 0 ? (
          <div className="inventory-empty">
            <div className="inventory-empty-icon" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none" width="22" height="22">
                <rect x="3" y="4" width="18" height="6.5" rx="1.75" stroke="currentColor" strokeWidth="1.5" />
                <rect x="3" y="13.5" width="18" height="6.5" rx="1.75" stroke="currentColor" strokeWidth="1.5" />
                <circle cx="7" cy="7.25" r="1" fill="currentColor" />
                <circle cx="7" cy="16.75" r="1" fill="currentColor" />
              </svg>
            </div>
            <div className="inventory-empty-title">
              {servers.length === 0 ? 'No servers in inventory' : 'No matching servers'}
            </div>
            <p className="inventory-empty-desc">
              {servers.length === 0
                ? 'Add a server host to quickly connect sessions across your workspaces.'
                : `No hosts match "${filter.trim()}".`}
            </p>
            {servers.length === 0 && !showForm && (
              <button type="button" className="inventory-secondary-btn" onClick={openCreateForm}>
                + Add your first server
              </button>
            )}
          </div>
        ) : (
          <div className="inventory-list">
            {filtered.map((srv) => (
              <DisposableAwareRow
                key={srv.id}
                srv={srv}
                onConnectServer={onConnectServer}
                onEdit={startEdit}
                onDeleteServer={onDeleteServer}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

interface RowProps {
  srv: InventoryServer;
  onConnectServer: (ip: string) => void;
  onEdit: (srv: InventoryServer) => void;
  onDeleteServer: (id: string) => Promise<void>;
}

const ServerIcon: React.FC = () => (
  <svg viewBox="0 0 16 16" fill="none" width="15" height="15" aria-hidden="true">
    <rect x="2" y="2.5" width="12" height="4.5" rx="1.1" stroke="currentColor" strokeWidth="1.35" />
    <rect x="2" y="9" width="12" height="4.5" rx="1.1" stroke="currentColor" strokeWidth="1.35" />
    <circle cx="4.5" cy="4.75" r="0.75" fill="currentColor" />
    <circle cx="4.5" cy="11.25" r="0.75" fill="currentColor" />
  </svg>
);

const ContainerIcon: React.FC = () => (
  <svg viewBox="0 0 16 16" fill="none" width="15" height="15" aria-hidden="true">
    <path d="M2.6 5.6v4.9c0 .75.6 1.35 1.35 1.35h6.1c1 0 1.9-.55 2.35-1.45l.9-1.8c.14-.28-.07-.6-.38-.6H3.75c-.63 0-1.15.52-1.15 1.15Z" stroke="currentColor" strokeWidth="1.25" strokeLinejoin="round" />
    <path d="M5.3 3.2v1.8M7.2 2.5v2.5M9.1 3.5v1.5" stroke="currentColor" strokeWidth="1.25" strokeLinecap="round" />
    <circle cx="6" cy="8.1" r="0.7" fill="currentColor" />
    <circle cx="9.2" cy="8.1" r="0.7" fill="currentColor" />
  </svg>
);

/** One inventory line. Disposable rows are tinted and cannot be edited. */
const DisposableAwareRow: React.FC<RowProps> = ({ srv, onConnectServer, onEdit, onDeleteServer }) => {
  const isDisposable = !!srv.isDisposable;
  const ready = useContainerReady(isDisposable ? srv.ip : null);

  return (
    <div className={`inventory-card${isDisposable ? ' is-disposable' : ''}`}>
      <div className="inventory-card-icon" aria-hidden="true">
        {isDisposable ? <ContainerIcon /> : <ServerIcon />}
      </div>

      <div className="inventory-card-main">
        <div className="inventory-card-title-row">
          <span className="inventory-card-name">{srv.name}</span>
          {isDisposable && (
            <span className="inventory-badge" title="Temporary Docker container, destroyed with this entry">
              TEMP
            </span>
          )}
          <span className="inventory-card-ip">{srv.ip}</span>
        </div>
        {isDisposable && !ready ? (
          <div className="inventory-card-desc">Starting container…</div>
        ) : (
          srv.description && <div className="inventory-card-desc">{srv.description}</div>
        )}
      </div>

      <div className="inventory-card-actions">
        <button
          type="button"
          className="inventory-action-btn connect"
          onClick={() => onConnectServer(srv.ip)}
          disabled={isDisposable && !ready}
          title="Connect in active workspace"
        >
          {isDisposable && !ready ? 'Starting…' : 'Connect'}
        </button>
        {!isDisposable && (
          <button type="button" className="inventory-action-btn" onClick={() => onEdit(srv)}>
            Edit
          </button>
        )}
        <button
          type="button"
          className="inventory-action-btn danger"
          onClick={() => onDeleteServer(srv.id)}
          title={isDisposable ? 'Destroy the container and remove this entry' : 'Remove from inventory'}
        >
          {isDisposable ? 'Destroy' : 'Delete'}
        </button>
      </div>
    </div>
  );
};

export default InventoryView;
