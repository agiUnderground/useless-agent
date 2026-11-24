import React, { useState } from 'react';
import { InventoryServer } from '../api';
import './InventoryView.css';

interface InventoryViewProps {
  servers: InventoryServer[];
  onAddServer: (payload: { name: string; ip: string; description: string }) => Promise<void>;
  onUpdateServer: (id: string, payload: { name: string; ip: string; description: string }) => Promise<void>;
  onDeleteServer: (id: string) => Promise<void>;
  onConnectServer: (ip: string) => void;
}

const InventoryView: React.FC<InventoryViewProps> = ({
  servers,
  onAddServer,
  onUpdateServer,
  onDeleteServer,
  onConnectServer,
}) => {
  const [filter, setFilter] = useState('');
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
              <div key={srv.id} className="inventory-card">
                <div className="inventory-card-icon" aria-hidden="true">
                  <svg viewBox="0 0 16 16" fill="none" width="15" height="15">
                    <rect x="2" y="2.5" width="12" height="4.5" rx="1.1" stroke="currentColor" strokeWidth="1.35" />
                    <rect x="2" y="9" width="12" height="4.5" rx="1.1" stroke="currentColor" strokeWidth="1.35" />
                    <circle cx="4.5" cy="4.75" r="0.75" fill="currentColor" />
                    <circle cx="4.5" cy="11.25" r="0.75" fill="currentColor" />
                  </svg>
                </div>

                <div className="inventory-card-main">
                  <div className="inventory-card-title-row">
                    <span className="inventory-card-name">{srv.name}</span>
                    <span className="inventory-card-ip">{srv.ip}</span>
                  </div>
                  {srv.description && (
                    <div className="inventory-card-desc">{srv.description}</div>
                  )}
                </div>

                <div className="inventory-card-actions">
                  <button
                    type="button"
                    className="inventory-action-btn connect"
                    onClick={() => onConnectServer(srv.ip)}
                    title="Connect in active workspace"
                  >
                    Connect
                  </button>
                  <button
                    type="button"
                    className="inventory-action-btn"
                    onClick={() => startEdit(srv)}
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    className="inventory-action-btn danger"
                    onClick={() => onDeleteServer(srv.id)}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default InventoryView;
