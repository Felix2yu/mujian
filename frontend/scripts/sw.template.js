const BUILD_VERSION = '__BUILD_VERSION__';
const CACHE_NAME = 'mujian-' + BUILD_VERSION;
const STATIC_CACHE = 'mujian-static-' + BUILD_VERSION;
// 固定名称缓存：不随 BUILD_VERSION 变化，跨部署持久保留
const COVERS_CACHE = 'mujian-covers'; // 封面/缩略图
const DATA_CACHE = 'mujian-data';     // API 数据（列表/分类/设置等）

// 每次部署 BUILD_VERSION 随 git sha 变化，浏览器会检测到 sw.js 字节变化并重新
// 安装本 SW，activate 阶段删除旧版本静态缓存，从而保证已安装的 PWA 能更新到新
// 页面/资源。COVERS_CACHE / DATA_CACHE 刻意不带版本号，不被此处清理，
// 这样重新部署后离线仍能看到封面缩略图和上次加载过的列表数据。

// 持久数据缓存的条目上限：API 响应会随搜索/筛选参数不断产生新条目，
// 不设上限会长期无限增长，超出浏览器配额后被整体回收。
const MAX_DATA_ENTRIES = 200;

// 不写入持久数据缓存的端点：导出 ZIP / ICS 体积大且每次内容都不同，缓存无意义
const API_NO_CACHE = new Set(['/api/export', '/api/calendar.ics']);

// install 阶段预热的首屏必需接口，保证「重新部署后离线打开也能出列表」
const WARM_API = [
  '/api/settings',
  '/api/categories',
  '/api/records?offset=0&limit=30'
];

const STATIC_ASSETS = [
  '/',
  '/manifest.json',
  '/favicon.svg'
];

// 把一个 cover 字段值解析为可走同源 /uploads/ 预缓存的请求路径；
// 内联 data:/裸 base64、跨域（S3）或非法值返回 null（跳过）。
// storageType 来自 /api/settings，'s3' 时封面走 s3_public_url（跨域），SW 不缓存。
function coverUploadUrl(value, storageType) {
  if (!value) return null;
  if (value.startsWith('data:')) return null;          // 内联，无需网络
  if (value.startsWith('http')) return null;            // 跨域（如 S3），SW 不缓存
  if (storageType === 's3') return null;               // S3 走 s3_public_url，跨域
  // 本地存储：coverThumb/coverFile 形如 "covers/xxx.thumb.avif"
  const key = value.startsWith('/uploads/')
    ? value.slice('/uploads/'.length)
    : value.replace(/^\//, '');
  if (!key || key.indexOf('.') === -1) return null;     // 裸 base64 无扩展名，跳过
  return '/uploads/' + key;
}

// install：预缓存外壳 + 预热首屏接口 + 预取最近封面缩略图
// （三者都是尽力而为，任意失败都不影响安装完成）
self.addEventListener('install', (event) => {
  event.waitUntil((async () => {
    const cache = await caches.open(STATIC_CACHE);
    await cache.addAll(STATIC_ASSETS).catch(() => {});
    await warmApiCache().catch(() => {});
    await prefetchCovers().catch(() => {});
    // 不自动 skipWaiting：等前端提示用户刷新后再接管，避免开着不关的页面被强制刷新
  })());
});

// 预热首屏必需的 API 到持久缓存。注意：SW 内部发起的 fetch 不会触发自身的
// fetch 事件，因此必须显式写入，不能依赖运行时拦截。
async function warmApiCache() {
  const cache = await caches.open(DATA_CACHE);
  for (const url of WARM_API) {
    try {
      const resp = await fetch(url);
      if (resp && resp.ok) await cache.put(url, resp.clone());
    } catch (_) { /* 离线或接口异常，跳过 */ }
  }
  await pruneCache(cache, MAX_DATA_ENTRIES);
}

// 预取最近若干条记录的封面缩略图进 COVERS_CACHE（持久、跨部署）。
async function prefetchCovers() {
  // 1) 读取存储类型，S3 跨域不可缓存则直接返回
  let storageType = 'local';
  try {
    const s = await (await fetch('/api/settings')).json();
    if (s && s.storage_type) storageType = s.storage_type;
  } catch (_) { /* 离线/失败：按本地处理 */ }
  if (storageType === 's3') return;

  // 2) 拉取最近记录列表，收集封面缩略图 URL
  const res = await fetch('/api/records?limit=60');
  if (!res.ok) return;
  const data = await res.json();
  const recs = Array.isArray(data) ? data : (data.records || []);
  if (!recs.length) return;

  const cache = await caches.open(COVERS_CACHE);
  const seen = new Set();
  for (const r of recs) {
    const val = r.coverThumb || r.coverFile || '';
    const url = coverUploadUrl(val, storageType);
    if (!url || seen.has(url)) continue;
    seen.add(url);
    // 逐条抓取，单条失败（404/网络抖动）不中断整体预取
    try {
      const resp = await fetch(url);
      if (resp.ok) await cache.put(url, resp.clone());
    } catch (_) { /* 跳过该封面 */ }
  }
}

// 把缓存裁剪到 max 条：keys() 按写入顺序返回（最旧在前），删掉多余的旧条目。
async function pruneCache(cache, max) {
  try {
    const keys = await cache.keys();
    if (keys.length <= max) return;
    for (let i = 0; i < keys.length - max; i++) {
      await cache.delete(keys[i]);
    }
  } catch (_) { /* 裁剪失败不影响主流程 */ }
}

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(
        // 保留当前版本静态缓存与两个持久缓存，删除其余所有旧缓存
        keys.filter(key =>
          key !== STATIC_CACHE && key !== COVERS_CACHE && key !== DATA_CACHE
        ).map(key => caches.delete(key))
      )
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return; // 写操作（POST/PUT/DELETE）绝不缓存，直接走网络

  const url = new URL(request.url);
  // 跨域资源（如地图瓦片）不接管，直接走网络
  if (url.origin !== self.location.origin) return;

  // 封面/上传图片：走持久化的 COVERS_CACHE，cache-first + 后台刷新。
  // 封面 key 唯一且与内容绑定（重新生成会得到新 key），cache-first 不会回放旧封面；
  // 命中缓存即返回（离线/秒开），同时后台用网络响应更新缓存。
  if (url.pathname.startsWith('/uploads/')) {
    event.respondWith(serveCover(request));
    return;
  }

  event.respondWith(servePageOrApi(request, url));
});

// 页面外壳与 API：网络优先（保证在线时数据新鲜），成功后额外写一份到持久缓存，
// 供离线兜底。离线时先精确匹配，再退化为同 pathname 的任意历史数据。
async function servePageOrApi(request, url) {
  const persist = url.pathname.startsWith('/api/') && !API_NO_CACHE.has(url.pathname);
  try {
    const response = await fetch(request);
    if (response && response.ok) {
      // 必须在任何 await 之前同步 clone，否则响应体可能已被消费而 clone 抛错
      const forStatic = response.clone();
      const forPersistent = persist ? response.clone() : null;
      caches.open(STATIC_CACHE).then(c => c.put(request, forStatic)).catch(() => {});
      if (forPersistent) {
        (async () => {
          try {
            const dc = await caches.open(DATA_CACHE);
            await dc.put(request, forPersistent);
            await pruneCache(dc, MAX_DATA_ENTRIES);
          } catch (_) { /* 写入失败不影响已返回的响应 */ }
        })();
      }
    }
    return response;
  } catch (_) {
    // 离线/网络失败：caches.match 会遍历所有缓存（含持久缓存）
    const exact = await caches.match(request);
    if (exact) return exact;
    if (persist) {
      // 最后退路：部署后前端若改了查询参数（如 offset/limit），精确 key 会失效，
      // 此时回落同 pathname 的历史数据，至少能出列表而不是空白。
      const loose = await matchByPathname(DATA_CACHE, url.pathname);
      if (loose) return loose;
    }
    return Response.error();
  }
}

// 在指定缓存里按 pathname（忽略 query）查找第一条匹配项
async function matchByPathname(cacheName, pathname) {
  try {
    const cache = await caches.open(cacheName);
    const keys = await cache.keys();
    const hit = keys.find(k => new URL(k.url).pathname === pathname);
    return hit ? await cache.match(hit) : undefined;
  } catch (_) {
    return undefined;
  }
}

// 封面图的 cache-first 策略：命中即返回并后台刷新；未命中则走网络，失败回退缓存或报错响应。
async function serveCover(request) {
  const cache = await caches.open(COVERS_CACHE);
  const cached = await cache.match(request);
  if (cached) {
    // 后台静默刷新，不阻塞当前响应
    fetch(request)
      .then(resp => { if (resp && resp.ok) cache.put(request, resp.clone()); })
      .catch(() => {});
    return cached;
  }
  try {
    const response = await fetch(request);
    if (response.ok) cache.put(request, response.clone());
    return response;
  } catch (_) {
    return cached || Response.error();
  }
}

self.addEventListener('push', (event) => {
  if (!event.data) return;

  const data = event.data.json();
  const title = data.title || '幕间提醒';
  const options = {
    body: data.body || '',
    icon: '/icons/icon-192.svg',
    badge: '/icons/icon-192.svg',
    vibrate: [200, 100, 200],
    tag: data.tag || 'mujian-notification',
    data: data.url || '/',
    requireInteraction: true
  };

  event.waitUntil(
    self.registration.showNotification(title, options)
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const url = event.notification.data || '/';

  event.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true })
      .then(clientList => {
        for (const client of clientList) {
          if (client.url.includes(self.location.origin) && 'focus' in client) {
            client.navigate(url);
            return client.focus();
          }
        }
        return clients.openWindow(url);
      })
  );
});

self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});
