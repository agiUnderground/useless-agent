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
      <div className="inventory-header">
        <div>
          <h1 className="inventory-title">Inventory</h1>
          <p className="inventory-subtitle">
            Saved servers available for quick connection in any workspace
          </p>
        </div>
      </div>

      <div className="inventory-search-bar">
        <button
          className="inventory-primary-btn"
          onClick={() => {
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
          }}
        >
          {showForm && !editingId ? 'Cancel' : '+ Add Server'}
        </button>
        <input
          type="text"
          placeholder="Filter servers by name, IP, or description..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {showForm && (
        <form className="inventory-form" onSubmit={handleSubmit}>
          <div className="inventory-form-title">
            {editingId ? 'Edit Server' : 'New Server'}
          </div>
          <div className="inventory-form-grid">
            <div className="inventory-field">
              <label>Label / Name</label>
              <input
                type="text"
                placeholder="e.g. Build VM 01"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="inventory-field">
              <label>IP Address *</label>
              <input
                type="text"
                placeholder="e.g. 192.168.1.42 or ::1"
                value={ip}
                onChange={(e) => setIp(e.target.value)}
                required
              />
            </div>
            <div className="inventory-field inventory-field-full">
              <label>Description</label>
              <input
                type="text"
                placeholder="Optional notes"
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

      {filtered.length === 0 ? (
        <div className="inventory-empty">
          {servers.length === 0
            ? 'No servers in inventory yet. Click "+ Add Server" to add one.'
            : 'No servers match your filter.'}
        </div>
      ) : (
        <div className="inventory-list">
          {filtered.map((srv) => (
            <div key={srv.id} className="inventory-card">
              <div className="inventory-card-actions">
                <button
                  className="inventory-action-btn connect"
                  onClick={() => onConnectServer(srv.ip)}
                  title="Connect in active workspace"
                >
                  Connect
                </button>
                <button
                  className="inventory-action-btn"
                  onClick={() => startEdit(srv)}
                >
                  Edit
                </button>
                <button
                  className="inventory-action-btn danger"
                  onClick={() => onDeleteServer(srv.id)}
                >
                  Delete
                </button>
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
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default InventoryView;
