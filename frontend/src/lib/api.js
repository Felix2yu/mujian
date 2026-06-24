const API_BASE = '';

async function request(path, options = {}) {
  const url = `${API_BASE}${path}`;
  const res = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers
    },
    ...options
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(err.error || 'Request failed');
  }

  return res.json();
}

export const api = {
  listShows: (year, month) => request(`/api/shows?year=${year}&month=${month}`),
  listAllShows: () => request('/api/shows/all'),
  listShowsByDateRange: (start, end) => {
    const params = new URLSearchParams();
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    return request(`/api/shows?${params.toString()}`);
  },
  getShow: (id) => request(`/api/shows/${id}`),
  createShow: (data) => request('/api/shows', { method: 'POST', body: JSON.stringify(data) }),
  updateShow: (id, data) => request(`/api/shows/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteShow: (id) => request(`/api/shows/${id}`, { method: 'DELETE' }),
  batchUpdate: (ids, data) => request('/api/shows/batch', { method: 'POST', body: JSON.stringify({ ids, ...data }) }),
  batchDelete: (ids) => request('/api/shows/batch/delete', { method: 'POST', body: JSON.stringify({ ids }) }),
  searchShows: (q) => request(`/api/shows/search?q=${encodeURIComponent(q)}`),
  getUpcoming: (limit = 10) => request(`/api/shows/upcoming?limit=${limit}`),
  getRecent: (limit = 10) => request(`/api/shows/recent?limit=${limit}`),

  getCalendar: (year, month) => request(`/api/calendar?year=${year}&month=${month}`),
  getICSUrl: () => `${API_BASE}/api/calendar.ics`,

  getStats: () => request('/api/stats'),
  getDashboard: () => request('/api/dashboard'),

  listCategories: () => request('/api/categories'),
  createCategory: (data) => request('/api/categories', { method: 'POST', body: JSON.stringify(data) }),
  updateCategory: (id, data) => request(`/api/categories/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteCategory: (id) => request(`/api/categories/${id}`, { method: 'DELETE' }),
  updateCategorySort: (ids) => request('/api/categories/sort', { method: 'PATCH', body: JSON.stringify({ ids }) }),

  getAutocomplete: (field) => request(`/api/autocomplete/${field}`),
  getByField: (field, value) => request(`/api/field/${field}/${encodeURIComponent(value)}`),

  getSettings: () => request('/api/settings'),
  updateSettings: (data) => request('/api/settings', { method: 'PUT', body: JSON.stringify(data) }),

  importShows: async (file) => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${API_BASE}/api/shows/import`, { method: 'POST', body: form });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Import failed' }));
      throw new Error(err.error || 'Import failed');
    }
    return res.json();
  },

  getImportTemplate: () => `${API_BASE}/api/import/template`,
  getExportUrl: () => `${API_BASE}/api/export`,

  getBackupUrl: () => `${API_BASE}/api/backup/download`,
  restoreBackup: async (file) => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${API_BASE}/api/backup/restore`, { method: 'POST', body: form });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Restore failed' }));
      throw new Error(err.error || 'Restore failed');
    }
    return res.json();
  },

  uploadFile: async (file) => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${API_BASE}/api/upload`, { method: 'POST', body: form });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Upload failed' }));
      throw new Error(err.error || 'Upload failed');
    }
    return res.json();
  },

  getSceneSorts: () => request('/api/scene-sorts'),
  updateSceneSort: (play, scenes) => request('/api/scene-sorts', { method: 'PUT', body: JSON.stringify({ play, scenes }) }),
  deleteSceneSort: (play) => request(`/api/scene-sorts/${encodeURIComponent(play)}`, { method: 'DELETE' }),
};
