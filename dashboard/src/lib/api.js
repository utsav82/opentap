const API_BASE = '';

let apiKey = localStorage.getItem('opentap_api_key') || '';
let adminToken = localStorage.getItem('opentap_admin_token') || '';

export function setApiKey(key) {
  apiKey = key;
  localStorage.setItem('opentap_api_key', key);
}

export function getApiKey() {
  return apiKey;
}

export function setAdminToken(token) {
  adminToken = token;
  localStorage.setItem('opentap_admin_token', token);
}

export function getAdminToken() {
  return adminToken;
}

async function send(path, options = {}, token = apiKey) {
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    if (res.status === 401 && !token) throw new Error('Set your credentials in Settings first.');
    const body = await res.json().catch(() => ({}));
    const msg = body?.error?.message || `Request failed: ${res.status}`;
    throw new Error(msg);
  }

  return res;
}

async function request(path, options, token) {
  const res = await send(path, options, token);
  if (res.status === 204) return null;
  return res.json();
}

const withQuery = (path, params = {}) => {
  const q = new URLSearchParams(params).toString();
  return q ? `${path}?${q}` : path;
};

export const api = {
  // Health
  health: () => request('/health'),

  // Projects
  createProject: (name) => request('/v1/projects', { method: 'POST', body: JSON.stringify({ name }) }, adminToken),
  listProjects: () => request('/v1/projects', {}, adminToken),
  getProject: (id) => request(`/v1/projects/${id}`, {}, adminToken),

  // API Keys
  createApiKey: (projectId, label, scope) =>
    request(`/v1/projects/${projectId}/keys`, { method: 'POST', body: JSON.stringify({ label, scope }) }, adminToken),
  listApiKeys: (projectId) => request(`/v1/projects/${projectId}/keys`, {}, adminToken),
  revokeApiKey: (projectId, keyId) =>
    request(`/v1/projects/${projectId}/keys/${keyId}`, { method: 'DELETE' }, adminToken),

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
  exportSegment: async (id) => (await send(`/v1/segments/${id}/export`)).blob(),

  // Schemas
  listSchemas: () => request('/v1/schemas'),
  getSchema: (eventName) => request(`/v1/schemas/${encodeURIComponent(eventName)}`),

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
  funnel: (params) => request(withQuery('/v1/analytics/funnel', params)),
  retention: (params) => request(withQuery('/v1/analytics/retention', params)),
};
