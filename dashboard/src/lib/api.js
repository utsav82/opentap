const API_BASE = '';

let apiKey = localStorage.getItem('opentap_api_key') || '';

export function setApiKey(key) {
  apiKey = key;
  localStorage.setItem('opentap_api_key', key);
}

export function getApiKey() {
  return apiKey;
}

export function clearApiKey() {
  apiKey = '';
  localStorage.removeItem('opentap_api_key');
}

async function request(path, options = {}) {
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  if (apiKey) {
    headers['Authorization'] = `Bearer ${apiKey}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const msg = body?.error?.message || `Request failed: ${res.status}`;
    throw new Error(msg);
  }

  if (res.status === 204) return null;
  return res.json();
}

export const api = {
  // Health
  health: () => request('/health'),

  // Projects
  createProject: (name) => request('/v1/projects', { method: 'POST', body: JSON.stringify({ name }) }),
  listProjects: () => request('/v1/projects'),
  getProject: (id) => request(`/v1/projects/${id}`),

  // API Keys
  createApiKey: (projectId, label) =>
    request(`/v1/projects/${projectId}/keys`, { method: 'POST', body: JSON.stringify({ label }) }),
  revokeApiKey: (projectId, keyId) =>
    request(`/v1/projects/${projectId}/keys/${keyId}`, { method: 'DELETE' }),

  // Events
  trackEvent: (data) => request('/v1/events', { method: 'POST', body: JSON.stringify(data) }),
  trackBatch: (events) => request('/v1/events/batch', { method: 'POST', body: JSON.stringify({ events }) }),
  listEvents: (params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/events${q ? '?' + q : ''}`);
  },
  getEvent: (id) => request(`/v1/events/${id}`),

  // Users
  listUsers: (params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/users${q ? '?' + q : ''}`);
  },
  getUser: (id) => request(`/v1/users/${id}`),
  getUserEvents: (id, params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/users/${id}/events${q ? '?' + q : ''}`);
  },

  // Segments
  createSegment: (data) => request('/v1/segments', { method: 'POST', body: JSON.stringify(data) }),
  listSegments: (params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/segments${q ? '?' + q : ''}`);
  },
  getSegment: (id) => request(`/v1/segments/${id}`),
  updateSegment: (id, data) => request(`/v1/segments/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteSegment: (id) => request(`/v1/segments/${id}`, { method: 'DELETE' }),
  getSegmentUsers: (id, params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/segments/${id}/users${q ? '?' + q : ''}`);
  },

  // Schemas
  listSchemas: () => request('/v1/schemas'),
  getSchema: (eventName) => request(`/v1/schemas/${eventName}`),

  // Analytics
  eventCount: (params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/analytics/events/count${q ? '?' + q : ''}`);
  },
  topEvents: (params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/analytics/events/top${q ? '?' + q : ''}`);
  },
  activeUsers: (params = {}) => {
    const q = new URLSearchParams(params).toString();
    return request(`/v1/analytics/users/active${q ? '?' + q : ''}`);
  },
};
