const API_BASE = '';

async function request(path, options = {}) {
  const url = `${API_BASE}${path}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);
  try {
    const res = await fetch(url, {
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
      ...options,
      signal: controller.signal
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Request failed' }));
      throw new Error(err.error || 'Request failed');
    }
    if (res.status === 204) return null;
    return res.json();
  } finally {
    clearTimeout(timer);
  }
}

// Stream a request whose body is newline-delimited JSON (NDJSON). Each line is
// parsed and handed to onLine as soon as it arrives, so callers can render
// live progress. Resolves with the object carrying "done": true, or — for a
// legacy server that returns a single JSON object — with that object. Unlike
// request(), this does NOT impose a fixed client-side timeout, because the
// operations that use it (e.g. batch AVIF conversion) can legitimately run for
// minutes; the server flushes progress lines to keep the connection alive.
export async function streamRequest(path, options = {}, onLine) {
  const url = `${API_BASE}${path}`;
  const res = await fetch(url, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(err.error || 'Request failed');
  }
  // No streaming support (very old browser): fall back to a single JSON body.
  if (!res.body || typeof res.body.getReader !== 'function') {
    return res.json();
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let lastObj = null;
  let finalResult = null;
  const handleLine = (raw) => {
    const line = raw.trim();
    if (!line) return;
    let obj;
    try {
      obj = JSON.parse(line);
    } catch {
      return;
    }
    lastObj = obj;
    if (onLine) onLine(obj);
    if (obj.done) finalResult = obj;
  };

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let nl;
    while ((nl = buffer.indexOf('\n')) >= 0) {
      handleLine(buffer.slice(0, nl));
      buffer = buffer.slice(nl + 1);
    }
  }
  if (buffer.trim()) handleLine(buffer);
  return finalResult || lastObj;
}

export const api = {
  listRecords: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== '' && v != null) q.set(k, v);
    }
    return request(`/api/records?${q.toString()}`);
  },
  getRecord: (id) => request(`/api/records/${id}`),
  createRecord: (data) => request('/api/records', { method: 'POST', body: JSON.stringify(data) }),
  updateRecord: (id, data) => request(`/api/records/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteRecord: (id) => request(`/api/records/${id}`, { method: 'DELETE' }),
  batchUpdate: (ids, data) => request('/api/records/batch', { method: 'POST', body: JSON.stringify({ ids, ...data }) }),
  batchDelete: (ids) => request('/api/records/batch/delete', { method: 'POST', body: JSON.stringify({ ids }) }),
  alignVenues: () => request('/api/records/align-venues', { method: 'POST' }),

  importRecords: async (file) => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${API_BASE}/api/records/import`, { method: 'POST', body: form });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Import failed' }));
      throw new Error(err.error || 'Import failed');
    }
    return res.json();
  },

  listCategories: () => request('/api/categories'),
  createCategory: (data) => request('/api/categories', { method: 'POST', body: JSON.stringify(data) }),
  updateCategory: (id, data) => request(`/api/categories/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteCategory: (id) => request(`/api/categories/${id}`, { method: 'DELETE' }),

  listDramas: () => request('/api/dramas'),
  getDramaTree: () => request('/api/dramas/tree'),
  createDrama: (data) => request('/api/dramas', { method: 'POST', body: JSON.stringify(data) }),
  getDrama: (id) => request(`/api/dramas/${id}`),
  updateDrama: (id, data) => request(`/api/dramas/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteDrama: (id) => request(`/api/dramas/${id}`, { method: 'DELETE' }),
  createZhezi: (dramaId, data) => request(`/api/dramas/${dramaId}/zhezis`, { method: 'POST', body: JSON.stringify(data) }),
  updateZhezi: (id, data) => request(`/api/zhezis/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteZhezi: (id) => request(`/api/zhezis/${id}`, { method: 'DELETE' }),
  reorderZhezis: (dramaId, ids) => request(`/api/dramas/${dramaId}/zhezis/reorder`, { method: 'POST', body: JSON.stringify({ ids }) }),

  getStats: () => request('/api/stats'),
  getDashboard: () => request('/api/dashboard'),
  getCalendar: (year, month) => request(`/api/calendar?year=${year}&month=${month}`),
  getICSUrl: () => `${API_BASE}/api/calendar.ics`,

  getAutocomplete: (field) => request(`/api/autocomplete/${field}`),
  getByField: (field, value) => request(`/api/field/${field}/${encodeURIComponent(value)}`),

  getSettings: () => request('/api/settings'),
  updateSettings: (data) => request('/api/settings', { method: 'PUT', body: JSON.stringify(data) }),

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

  getExportUrl: (format = '') => `${API_BASE}/api/export${format ? `?format=${format}` : ''}`,

  listCovers: (params = {}) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== '' && v != null) qs.set(k, v);
    }
    return request(`/api/covers?${qs.toString()}`);
  },
  getCoverDuplicates: () => request('/api/covers/duplicates'),
  mergeCovers: (hashes) => request('/api/covers/merge', { method: 'POST', body: JSON.stringify({ hashes }) }),
  getCoverOrphans: () => request('/api/covers/orphans'),
  cleanupCovers: (payload) => request('/api/covers/cleanup', { method: 'POST', body: JSON.stringify(payload) }),
  purgeTrash: () => request('/api/covers/trash/purge', { method: 'POST' }),
  regenerateThumbs: () => request('/api/covers/thumbs', { method: 'POST' }),
  convertCover: (key, format) => request('/api/covers/convert', { method: 'POST', body: JSON.stringify({ key, format }) }),
  convertBatchCovers: (format, onProgress) =>
    streamRequest('/api/covers/convert-batch', { method: 'POST', body: JSON.stringify({ format }) }, onProgress)
};

// ---- S3-aware cover URL resolution ----
let storageInfo = { storage_type: 'local', s3_public_url: '' };
let storageLoaded = false;

async function ensureStorageInfo() {
  if (storageLoaded) return;
  try {
    const s = await request('/api/settings');
    storageInfo = s || storageInfo;
  } catch (e) { /* keep local defaults */ }
  storageLoaded = true;
}

export async function initStorageInfo() {
  await ensureStorageInfo();
}

export function coverUrl(coverFile) {
  if (!coverFile) return '';
  if (coverFile.startsWith('http')) return coverFile;
  if (coverFile.startsWith('/uploads/')) return `${API_BASE}${coverFile}`;
  if (storageInfo.storage_type === 's3' && storageInfo.s3_public_url) {
    return `${storageInfo.s3_public_url}/${coverFile}`;
  }
  return `${API_BASE}/uploads/${coverFile}`;
}

export function formatCurrency(amount, currency) {
  const c = currency || 'CNY';
  const symbol = c === 'CNY' ? '¥' : c + ' ';
  return symbol + (Number(amount) || 0).toFixed(2);
}

export function formatDate(ts) {
  if (!ts) return '';
  const d = new Date(ts * 1000);
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
