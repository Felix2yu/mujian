const CACHE_NAME = 'mujian-v3';
const STATIC_CACHE = 'mujian-static-v3';
// v3: no longer caches /api/* at all. Search/filter results are
// parameter-dependent — any cache replay returns wrong data (e.g. /api/records
// cached once is incorrectly served for /api/records?q=蔡安, returning the
// full 298-row list instead of the filtered empty result).

const STATIC_ASSETS = [
  '/',
  '/manifest.json',
  '/favicon.svg'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(STATIC_CACHE)
      .then(cache => cache.addAll(STATIC_ASSETS))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(
        // Drop every cache from prior SW versions (incl. the now-unused API
        // cache from v2) so any stale entries can't replay under a new key.
        keys.filter(key => key !== STATIC_CACHE)
          .map(key => caches.delete(key))
      )
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // API requests are NEVER intercepted — they're parameter-dependent, live
  // data; serving a cached response would silently return stale results and
  // break the UI (e.g. cached /api/records replayed as /api/records?q=…).
  if (url.pathname.startsWith('/api/') || url.pathname === '/uploads/') {
    return;
  }

  // Everything else (static SPA assets, covers served from /uploads/ via the
  // catch-all in main.go): network-first, fall back to cache when offline.
  if (request.method !== 'GET') return;
  event.respondWith(
    fetch(request)
      .then(response => {
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
