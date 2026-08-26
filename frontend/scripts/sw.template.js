const BUILD_VERSION = '__BUILD_VERSION__';
const CACHE_NAME = 'mujian-' + BUILD_VERSION;
const STATIC_CACHE = 'mujian-static-' + BUILD_VERSION;

// 每次部署 BUILD_VERSION 随 git sha 变化，浏览器会检测到 sw.js 字节变化并重新
// 安装本 SW，activate 阶段删除旧版本缓存，从而保证已安装的 PWA 能更新到新资源。

const STATIC_ASSETS = [
  '/',
  '/manifest.json',
  '/favicon.svg'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(STATIC_CACHE)
      .then(cache => cache.addAll(STATIC_ASSETS))
      // 不自动 skipWaiting：等前端提示用户刷新后再接管，避免开着不关的页面被强制刷新
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(
        // 只保留当前版本的缓存，删除所有旧版本，避免旧资源被回放
        keys.filter(key => key !== STATIC_CACHE)
          .map(key => caches.delete(key))
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

  // 同源 GET 统一「网络优先 + 失败回退缓存」：
  // - 页面外壳、静态资源、封面 → 离线可打开 App
  // - /api/* 数据 → 离线可翻看上次加载过的演出/剧目/演员
  // 缓存键为完整 URL（含 query），/api/records?q=蔡安 只命中它本身，不会串味
  event.respondWith(
    fetch(request)
      .then(response => {
        // 仅缓存成功响应（2xx）；失败时回退到缓存中的旧数据
        if (response.ok) {
          const clone = response.clone();
          caches.open(STATIC_CACHE).then(cache => cache.put(request, clone));
        }
        return response;
      })
      .catch(() => caches.match(request))
  );
});

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
