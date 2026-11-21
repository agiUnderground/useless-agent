// Configuration file for frontend

/**
 * Strip surrounding IPv6 brackets and whitespace so hosts are stored consistently.
 */
export function normalizeHost(host: string): string {
  const trimmed = host.trim();
  if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

/**
 * Format a host/IP for use in a URL authority (wraps IPv6 literals in brackets).
 */
export function formatHostForUrl(host: string): string {
  const normalized = normalizeHost(host);
  if (normalized.includes(':')) {
    return `[${normalized}]`;
  }
  return normalized;
}

export const CONFIG = {
  // Backend API URL from environment variable or default to localhost:8080
  BACKEND_URL: import.meta.env.VITE_API_URL || 'http://localhost:8080',
  
  // Extract host and port from BACKEND_URL
  get BACKEND_HOST() {
    const url = new URL(this.BACKEND_URL);
    return normalizeHost(url.hostname);
  },
  
  get BACKEND_PORT() {
    const url = new URL(this.BACKEND_URL);
    return url.port || '8080';
  },

  get BACKEND_PROTOCOL() {
    const url = new URL(this.BACKEND_URL);
    return url.protocol;
  },
  
  get BACKEND_WS_URL() {
    const url = new URL(this.BACKEND_URL);
    const protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${formatHostForUrl(url.hostname)}:${url.port || '8080'}`;
  },

  getHttpUrl(host: string, path: string = '') {
    const normalizedPath = path.startsWith('/') ? path : `/${path}`;
    return `${this.BACKEND_PROTOCOL}//${formatHostForUrl(host)}:${this.BACKEND_PORT}${normalizedPath}`;
  },

  getWsUrl(host: string, path: string = '/ws') {
    const protocol = this.BACKEND_PROTOCOL === 'https:' ? 'wss:' : 'ws:';
    const normalizedPath = path.startsWith('/') ? path : `/${path}`;
    return `${protocol}//${formatHostForUrl(host)}:${this.BACKEND_PORT}${normalizedPath}`;
  }
};