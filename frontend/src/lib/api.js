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
  // Shows
  listShows: (year, month) => request(`/api/shows?year=${year}&month=${month}`),
  getShow: (id) => request(`/api/shows/${id}`),
  createShow: (data) => request('/api/shows', { method: 'POST', body: JSON.stringify(data) }),
  updateShow: (id, data) => request(`/api/shows/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteShow: (id) => request(`/api/shows/${id}`, { method: 'DELETE' }),
  searchShows: (q) => request(`/api/shows/search?q=${encodeURIComponent(q)}`),
  getUpcoming: (limit = 10) => request(`/api/shows/upcoming?limit=${limit}`),
  getRecent: (limit = 10) => request(`/api/shows/recent?limit=${limit}`),

  // Calendar
  getCalendar: (year, month) => request(`/api/calendar?year=${year}&month=${month}`),
  getICSUrl: () => `${API_BASE}/api/calendar.ics`,

  // Stats
  getStats: () => request('/api/stats'),

  // Categories
  listCategories: () => request('/api/categories'),
  createCategory: (data) => request('/api/categories', { method: 'POST', body: JSON.stringify(data) }),
  updateCategory: (id, data) => request(`/api/categories/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteCategory: (id) => request(`/api/categories/${id}`, { method: 'DELETE' })
};
