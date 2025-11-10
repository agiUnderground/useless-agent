// Configuration file for frontend
export const CONFIG = {
  // Backend API URL from environment variable or default to localhost:8081
  BACKEND_URL: import.meta.env.VITE_API_URL || 'http://localhost:8080',
  
  // Extract host and port from BACKEND_URL
  get BACKEND_HOST() {
    const url = new URL(this.BACKEND_URL);
    return url.hostname;
  },
  
  get BACKEND_PORT() {
    const url = new URL(this.BACKEND_URL);
    return url.port || '8080';
  },
  
  get BACKEND_WS_URL() {
    const url = new URL(this.BACKEND_URL);
    const protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${url.hostname}:${url.port || '8080'}`;
  }
};