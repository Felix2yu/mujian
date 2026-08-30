const API_BASE = '';

// Optional bearer token (set on the settings page, stored locally). When the
// server enables MJ_AUTH_TOKEN, every API request must carry it; calendar
// clients get it appended to the ICS URL instead.
export function getAuthToken() {
  try {
    return localStorage.getItem('mujian:auth_token') || '';
  } catch (e) {
    return '';
  }
}

function authHeaders() {
  const t = getAuthToken();
  return t ? { 'X-Auth-Token': t } : {};
}

// 任意接口返回 401 时广播一次全局事件：布局层据此弹出全站令牌门，
// 覆盖「服务端启用/更换令牌后会话中途失效」的场景。
function markUnauthorized() {
  try {
    window.dispatchEvent(new CustomEvent('mujian:unauthorized'));
  } catch (e) { /* ignore */ }
}

// 供令牌门校验输入：用给定令牌探测一个需要鉴权的轻量接口。
export async function verifyAuthToken(token) {
  if (!token) return false;
  try {
    const res = await fetch(`${API_BASE}/api/stats`, {
      credentials: 'same-origin',
      headers: { 'X-Auth-Token': token }
    });
    return res.ok;
  } catch (e) {
    return false;
  }
}

async function request(path, options = {}) {
  const url = `${API_BASE}${path}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15000);
  try {
    const res = await fetch(url, {
      // same-origin：同源部署下与 include 等效，跨域部署时不会把 cookie
      // 泄漏给第三方站点（配合无 CSRF token 的现状，这是更安全的默认值）。
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', ...authHeaders(), ...(options.headers || {}) },
      ...options,
      signal: controller.signal
    });

    if (!res.ok) {
      if (res.status === 401) markUnauthorized();
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
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...authHeaders(), ...(options.headers || {}) },
    ...options
  });
  if (!res.ok) {
    if (res.status === 401) markUnauthorized();
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
  listRecords: async (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== '' && v != null) q.set(k, v);
    }
    const data = await request(`/api/records?${q.toString()}`);
    // 后端返回 {records: [...], total: N}；兼容旧版返回纯数组
    if (Array.isArray(data)) {
      return { records: data, total: data.length };
    }
    return { records: data.records ?? [], total: data.total ?? 0 };
  },
  // 地图专用精简投影：仅含带坐标的记录与地图渲染所需字段，
  // 体积约为 /api/records 全量响应的 1/12。
  mapPoints: async () => {
    const data = await request('/api/map/points');
    return Array.isArray(data) ? data : (data.points ?? []);
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
    const res = await fetch(`${API_BASE}/api/records/import`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: authHeaders(),
      body: form
    });
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
  reorderCategories: (ids) => request('/api/categories/reorder', { method: 'POST', body: JSON.stringify({ ids }) }),

  listDramas: () => request('/api/dramas'),
  getDramaTree: () => request('/api/dramas/tree'),
  createDrama: (data) => request('/api/dramas', { method: 'POST', body: JSON.stringify(data) }),
  getDrama: (id) => request(`/api/dramas/${id}`),
  updateDrama: (id, data) => request(`/api/dramas/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteDrama: (id) => request(`/api/dramas/${id}`, { method: 'DELETE' }),
  reorderDramas: (ids) => request('/api/dramas/reorder', { method: 'POST', body: JSON.stringify({ ids }) }),
  createZhezi: (dramaId, data) => request(`/api/dramas/${dramaId}/zhezis`, { method: 'POST', body: JSON.stringify(data) }),
  updateZhezi: (id, data) => request(`/api/zhezis/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteZhezi: (id) => request(`/api/zhezis/${id}`, { method: 'DELETE' }),
  reorderZhezis: (dramaId, ids) => request(`/api/dramas/${dramaId}/zhezis/reorder`, { method: 'POST', body: JSON.stringify({ ids }) }),

  listArtists: () => request('/api/artists'),
  getArtist: (id) => request(`/api/artists/${id}`),
  createArtist: (data) => request('/api/artists', { method: 'POST', body: JSON.stringify(data) }),
  updateArtist: (id, data) => request(`/api/artists/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteArtist: (id) => request(`/api/artists/${id}`, { method: 'DELETE' }),
  reorderArtists: (ids) => request('/api/artists/reorder', { method: 'POST', body: JSON.stringify({ ids }) }),

  getStats: () => request('/api/stats'),
  getDashboard: () => request('/api/dashboard'),
  getAnalytics: () => request('/api/analytics'),
  getCalendar: (year, month) => request(`/api/calendar?year=${year}&month=${month}`),
  // ICS 订阅地址：服务端启用 token 鉴权时日历客户端无法带请求头，
  // 改以 ?token= 查询参数传递。extra 用于合并附加参数（如 dl=1），
  // 避免调用方手拼第二个 "?" 把 token 值污染掉。
  getICSUrl: (extra = {}) => {
    const q = new URLSearchParams();
    const t = getAuthToken();
    if (t) q.set('token', t);
    for (const [k, v] of Object.entries(extra)) {
      if (v !== '' && v != null) q.set(k, v);
    }
    const qs = q.toString();
    return `${API_BASE}/api/calendar.ics${qs ? `?${qs}` : ''}`;
  },

  getAutocomplete: (field) => request(`/api/autocomplete/${field}`),
  getByField: (field, value) => request(`/api/field/${field}/${encodeURIComponent(value)}`),

  getSettings: () => request('/api/settings'),
  updateSettings: (data) => request('/api/settings', { method: 'PUT', body: JSON.stringify(data) }),
  testS3Connection: (data) => request('/api/settings/test-s3', { method: 'POST', body: JSON.stringify(data) }),

  backupRun: () => request('/api/backup/run', { method: 'POST' }),
  listRecordPhotos: (id) => request(`/api/records/${id}/photos`),
  addRecordPhoto: (id, key) => request(`/api/records/${id}/photos`, { method: 'POST', body: JSON.stringify({ key }) }),
  deleteRecordPhoto: (id, pid) => request(`/api/records/${id}/photos/${pid}`, { method: 'DELETE' }),
  reorderRecordPhotos: (id, ids) => request(`/api/records/${id}/photos/reorder`, { method: 'POST', body: JSON.stringify({ ids }) }),
  listDeletedRecords: () => request('/api/records/deleted'),
  restoreRecord: (id) => request(`/api/records/${id}/restore`, { method: 'POST' }),
  purgeRecord: (id) => request(`/api/records/${id}/purge`, { method: 'DELETE' }),
  purgeRecordsTrash: () => request('/api/records/trash/purge', { method: 'POST' }),
  backupList: () => request('/api/backup/list'),
  backupDownloadUrl: (file) => `${API_BASE}/api/backup/download?file=${encodeURIComponent(file)}`,
  backupRestoreFrom: (file) => request('/api/backup/restore-from', { method: 'POST', body: JSON.stringify({ file }) }),
  backupDelete: (file) => request(`/api/backup?file=${encodeURIComponent(file)}`, { method: 'DELETE' }),

  uploadFile: async (file) => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${API_BASE}/api/upload`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: authHeaders(),
      body: form
    });
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
  regenerateThumbs: (onProgress) =>
    streamRequest('/api/covers/thumbs', { method: 'POST' }, onProgress),
  convertCover: (key, format) => request('/api/covers/convert', { method: 'POST', body: JSON.stringify({ key, format }) }),
  migrateCoversToS3: (onProgress) =>
    streamRequest('/api/storage/migrate-to-s3', { method: 'POST' }, onProgress),
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

// Drop the cached storage info so subsequent coverUrl() calls re-read
// /api/settings. Used after saving settings on the settings page.
export function resetStorageInfo() {
  storageLoaded = false;
}

export function coverUrl(coverFile) {
  if (!coverFile) return '';
  // A data: URI (or raw base64, e.g. "/9j/...") is already renderable by the
  // browser; never prepend /uploads/ to it (that yields a bogus URL + 500).
  if (coverFile.startsWith('data:')) return coverFile;
  if (isRawBase64(coverFile)) return `data:image/jpeg;base64,${coverFile.replace(/^\//, '')}`;
  if (coverFile.startsWith('http')) return coverFile;
  if (coverFile.startsWith('/uploads/')) return `${API_BASE}${coverFile}`;
  if (storageInfo.storage_type === 's3' && storageInfo.s3_public_url) {
    return `${storageInfo.s3_public_url}/${coverFile}`;
  }
  return `${API_BASE}/uploads/${coverFile}`;
}

// isRawBase64 reports whether the value looks like a bare base64-encoded image
// (legacy data where the whole cover was stored in the field instead of a
// storage key). Such values are long, contain only base64 alphabet chars, and
// are not a storage key (no "covers/" prefix, no dot+extension segment).
function isRawBase64(v) {
  if (v.length < 200) return false;
  if (v.startsWith('/uploads/') || v.startsWith('covers/') || v.includes('/')) return false;
  return /^[A-Za-z0-9+/=]+\/?[A-Za-z0-9+/=]*$/.test(v);
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
