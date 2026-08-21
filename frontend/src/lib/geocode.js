// 高德 JS SDK 地理编码封装：把「场馆/地址」解析成 { lat, lng }
// 复用设置页里已有的高德 Key（localStorage: mujian:map_custom_key）

let amapPromise = null;
const cache = new Map();

function getMapKey() {
  try {
    return localStorage.getItem('mujian:map_custom_key') || '';
  } catch (e) {
    return '';
  }
}

function ensureAMap(key) {
  if (amapPromise) return amapPromise;
  amapPromise = new Promise((resolve, reject) => {
    if (typeof window === 'undefined') {
      reject(new Error('no-window'));
      return;
    }
    if (window.AMap && window.AMap.Geocoder) {
      resolve(window.AMap);
      return;
    }
    // 可选：高德 2.0 安全密钥（若用户 Key 需要）
    try {
      const sec = localStorage.getItem('mujian:amap_security') || '';
      if (sec) window._AMapSecurityConfig = { securityJsCode: sec };
    } catch (e) { /* 忽略 */ }

    const s = document.createElement('script');
    s.src = `https://webapi.amap.com/maps?v=2.0&key=${encodeURIComponent(key)}&plugin=AMap.Geocoder`;
    s.async = true;
    s.onload = () => {
      if (window.AMap && window.AMap.Geocoder) resolve(window.AMap);
      else reject(new Error('amap-load-failed'));
    };
    s.onerror = () => reject(new Error('amap-load-failed'));
    document.head.appendChild(s);
  });
  // 失败时清空，允许下次重试
  amapPromise.catch(() => { amapPromise = null; });
  return amapPromise;
}

// 返回 { lat, lng } 或抛带 code 的错误：nokey / error / notfound
export async function geocodeAddress(query, city) {
  const q = (query || '').trim();
  if (!q) return null;
  const cacheKey = city ? `${city}||${q}` : q;
  if (cache.has(cacheKey)) return cache.get(cacheKey);

  const key = getMapKey();
  if (!key) {
    const err = new Error('nokey');
    err.code = 'nokey';
    throw err;
  }

  let AMap;
  try {
    AMap = await ensureAMap(key);
  } catch (e) {
    const err = new Error('error');
    err.code = 'error';
    throw err;
  }

  const result = await new Promise((resolve, reject) => {
    let g;
    try {
      g = new AMap.Geocoder();
    } catch (e) {
      reject(e);
      return;
    }
    if (city) { try { g.setCity(city); } catch (e) { /* 忽略 */ } }
    g.getLocation(q, (status, res) => {
      if (status === 'complete' && res && res.geocodes && res.geocodes.length > 0) {
        const loc = res.geocodes[0].location;
        resolve({ lat: Number(loc.lat), lng: Number(loc.lng) });
      } else {
        reject(new Error('notfound'));
      }
    });
  });

  cache.set(cacheKey, result);
  return result;
}
