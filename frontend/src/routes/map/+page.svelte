<script>
  import { onMount } from 'svelte';
  import { api, coverUrl } from '$lib/api.js';
  import 'leaflet/dist/leaflet.css';
  import 'leaflet.markercluster/dist/MarkerCluster.css';
  import 'leaflet.markercluster/dist/MarkerCluster.Default.css';

  function loadPref(key, fallback) {
    try {
      const v = localStorage.getItem(key);
      return v || fallback;
    } catch (e) {
      return fallback;
    }
  }

  let loading = $state(true);
  let error = $state('');
  let records = $state([]);
  // 默认改为标准底图（OSM）
  let source = $state(loadPref('mujian:map_source', 'osm'));
  let customUrl = $state(loadPref('mujian:map_custom_url', ''));
  let customKey = $state(loadPref('mujian:map_custom_key', ''));

  // Leaflet 运行时引用（仅客户端）
  let L = $state(null);

  const SOURCES = {
    osm: {
      name: '标准',
      attribution: '© OpenStreetMap contributors',
      url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
      subdomains: ''
    },
    gaode: {
      name: '高德',
      attribution: '© 高德地图',
      url: 'https://webrd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}',
      subdomains: '1234'
    },
    tencent: {
      name: '腾讯',
      attribution: '© 腾讯地图',
      url: 'https://rt{s}.map.gtimg.com/tile?z={z}&x={x}&y={y}&styleid=3&version=197',
      subdomains: '0123'
    },
    custom: {
      name: '自定义',
      attribution: '© 自定义瓦片',
      url: '',
      subdomains: ''
    }
  };

  let map = null;
  let tileLayer = null;
  let clusterGroup = null;
  let mapEl = $state(null);

  onMount(async () => {
    try {
      // 动态引入 leaflet（仅客户端，避免 SSR 依赖 window）
      L = (await import('leaflet')).default || L;
      // 预引入 markercluster（会自动注册到 L）
      await import('leaflet.markercluster');
      records = await api.listRecords({});
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  // 响应式：leaflet 就绪、容器已渲染、数据已加载后初始化地图（修复空白）
  $effect(() => {
    if (L && mapEl && records && records.length > 0 && !map) {
      initMap();
    }
  });

  function initMap() {
    if (!mapEl) return;

    map = L.map(mapEl, {
      zoomControl: true,
      attributionControl: true,
      scrollWheelZoom: true
    });

    applySource(source);
    plotMarkers();

    // 自适应视野到所有有坐标的记录
    const bounds = [];
    for (const r of records) {
      if (r.coordinate && r.coordinate.latitude && r.coordinate.longitude) {
        bounds.push([r.coordinate.latitude, r.coordinate.longitude]);
      }
    }
    if (bounds.length > 0) {
      map.fitBounds(bounds, { padding: [40, 40], maxZoom: 13 });
    } else {
      // 兜底：中国范围
      map.setView([35.0, 108.0], 3);
    }
  }

  function applySource(key) {
    const s = SOURCES[key];
    if (!s) return;
    source = key;
    let url = s.url;
    if (key === 'custom') {
      url = customUrl || SOURCES.osm.url;
    } else if ((key === 'gaode' || key === 'tencent') && customKey) {
      url += (url.includes('?') ? '&' : '?') + 'key=' + encodeURIComponent(customKey);
    }
    if (!url) url = SOURCES.osm.url;
    if (tileLayer) tileLayer.remove();
    const opts = { attribution: s.attribution };
    if (s.subdomains) opts.subdomains = s.subdomains;
    tileLayer = L.tileLayer(url, opts).addTo(map);
  }

  function plotMarkers() {
    if (clusterGroup) {
      clusterGroup.clearLayers();
      clusterGroup.remove();
    }
    clusterGroup = L.markerClusterGroup({
      maxClusterRadius: 60,
      spiderfyOnMaxZoom: true,
      showCoverageOnHover: false,
      zoomToBoundsOnClick: true,
      removeOutsideVisibleBounds: true,
      animate: true
    });

    const groups = new Map();
    for (const r of records) {
      if (!r.coordinate) continue;
      const key = `${r.coordinate.latitude.toFixed(4)},${r.coordinate.longitude.toFixed(4)}`;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(r);
    }

    for (const [, grp] of groups) {
      const first = grp[0];
      const lat = first.coordinate.latitude;
      const lng = first.coordinate.longitude;
      const count = grp.length;

      const icon = L.divIcon({
        className: 'muj-mark',
        html: count > 1
          ? `<div class="pin multi"><span>${count}</span></div>`
          : '<div class="pin"></div>',
        iconSize: [36, 40],
        iconAnchor: [18, 38],
        popupAnchor: [0, -38]
      });

      const mk = L.marker([lat, lng], { icon });
      const cities = new Set(grp.map((r) => r.city).filter(Boolean));
      const cityText = cities.size ? [...cities].join(' · ') : '未知地点';

      const list = grp
        .slice(0, 8)
        .map(
          (r) => {
            const cov = r.coverFile ? coverUrl(r.coverFile) : coverUrl(r.coverThumb);
            const dateStr = r.dateText ? r.dateText.split(' ')[0].replace('年', '.').replace('月', '.').replace('日', '') : '';
            const catStr = r.categoryName ? ' · ' + r.categoryName : '';
            return `
              <a class="mrow" href="/records/${r.id}">
                ${cov ? `<img class="mcov" src="${cov}" alt=""/>` : '<div class="mcov ph"></div>'}
                <div class="minfo">
                  <div class="mtitle">${r.name}</div>
                  <div class="msub">${dateStr}${catStr}</div>
                </div>
              </a>`;
          }
        )
        .join('');

      const more = count > 8 ? `<a class="mmore" href="/?q=${encodeURIComponent(first.name)}">还有 ${count - 8} 场…</a>` : '';

      mk.bindPopup(() =>
        createPopup(first, cityText, count, list, more),
        { maxWidth: 340 }
      );

      clusterGroup.addLayer(mk);
    }
    clusterGroup.addTo(map);
  }

  function createPopup(first, cityText, count, list, more) {
    const cover = first.coverFile ? coverUrl(first.coverFile) : coverUrl(first.coverThumb);
    const titleHtml = count > 1
      ? `<b>${cityText}</b> · ${count} 场`
      : `<b>${first.name}</b>`;
    const address = first.address ? `<div class="paddr">${first.address}</div>` : '';
    return `
      <div class="pc">
        <div class="phead">
          ${cover ? `<img src="${cover}" alt=""/>` : ''}
          <div class="ptext">${titleHtml}${address}</div>
        </div>
        <div class="plist">${list}</div>
        ${more}
      </div>`;
  }

  function switchSource(key) {
    source = key;
    try {
      localStorage.setItem('mujian:map_source', key);
    } catch (e) { /* ignore */ }
    if (map) applySource(key);
  }
</script>

<svelte:head><title>地图 - 幕间</title></svelte:head>

<div class="fade-up">
  <div class="page-head">
    <h1>演出地图</h1>
    <p class="sub">按演出地点聚合展示，点击标记查看该地点的演出</p>
  </div>

  <div class="map-toolbar">
    <span class="src-label">底图</span>
    <div class="seg">
      {#each Object.entries(SOURCES) as [key, s]}
        <button class="seg-btn {source === key ? 'active' : ''}" onclick={() => switchSource(key)}>
          {s.name}
        </button>
      {/each}
    </div>
  </div>

  {#if error}
    <div class="banner error">⚠ {error}</div>
  {/if}

  {#if loading}
    <div class="skeleton" style="height: 60vh; border-radius: var(--radius-lg);"></div>
  {:else}
    <div class="map-shell" style="position:relative; height: calc(100vh - 260px); min-height: 420px;">
      <div bind:this={mapEl} id="map" style="position:absolute; inset:0; z-index:0;"></div>
    </div>
  {/if}
</div>

<style>
  .page-head h1 { margin-bottom: 2px; }
  .map-toolbar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; flex-wrap: wrap; }
  .src-label { font-size: 13px; color: var(--text-muted); }
  .seg { display: inline-flex; background: var(--surface-3); border-radius: 999px; padding: 3px; gap: 2px; }
  .seg-btn {
    border: none;
    background: transparent;
    color: var(--text-2);
    padding: 5px 14px;
    border-radius: 999px;
    font-size: 13px;
    cursor: pointer;
    transition: all var(--t-fast) var(--ease);
  }
  .seg-btn:hover { color: var(--text); }
  .seg-btn.active { background: var(--surface); color: var(--accent); font-weight: 600; box-shadow: var(--shadow-sm); }

  .map-shell { border-radius: var(--radius-lg); overflow: hidden; box-shadow: var(--shadow-md); border: 1px solid var(--border); }

  /* Leaflet 标记与弹窗（Leaflet 在组件外渲染，需全局） */
  :global(.muj-mark) { background: transparent; border: none; }
  :global(.muj-mark .pin) {
    width: 38px;
    height: 42px;
    background: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='38' height='42' viewBox='0 0 38 42'%3E%3Cpath d='M19 1C9.6 1 2 8.5 2 18c0 12.6 17 23 17 23s17-10.4 17-23C36 8.5 28.4 1 19 1z' fill='%23b42318'/%3E%3Ccircle cx='19' cy='18' r='6' fill='white'/%3E%3C/svg%3E") no-repeat center / 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  :global(.muj-mark .pin.multi) { background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='38' height='42' viewBox='0 0 38 42'%3E%3Cpath d='M19 1C9.6 1 2 8.5 2 18c0 12.6 17 23 17 23s17-10.4 17-23C36 8.5 28.4 1 19 1z' fill='%23991b1b'/%3E%3Ccircle cx='19' cy='18' r='7' fill='white'/%3E%3C/svg%3E"); }
  :global(.muj-mark .pin.multi span) { color: #991b1b; font-weight: 800; font-size: 13px; transform: translateY(-26px); position: relative; z-index: 2; }

  :global(.muj-popup .leaflet-popup-content-wrapper) {
    border-radius: 14px;
    box-shadow: var(--shadow-lg);
    padding: 0;
  }
  :global(.muj-popup .leaflet-popup-content) { margin: 0; width: 320px !important; }
  :global(.muj-popup .pc) { padding: 0; }
  :global(.muj-popup .phead) { display: flex; gap: 10px; padding: 12px 14px; align-items: center; border-bottom: 1px solid var(--border); }
  :global(.muj-popup .phead img) { width: 40px; height: 52px; object-fit: cover; border-radius: 6px; flex: 0 0 auto; }
  :global(.muj-popup .ptext b) { font-family: var(--font-serif); font-size: 15px; }
  :global(.muj-popup .paddr) { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
  :global(.muj-popup .plist) { padding: 4px 6px; max-height: 260px; overflow-y: auto; }
  :global(.muj-popup .mrow) {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 8px;
    border-radius: 8px;
    text-decoration: none;
    color: inherit;
    margin-bottom: 2px;
    transition: background var(--t-fast) var(--ease);
  }
  :global(.muj-popup .mrow:hover) { background: var(--surface-3); }
  :global(.muj-popup .mcov) {
    width: 36px;
    height: 48px;
    object-fit: cover;
    border-radius: 5px;
    flex: 0 0 auto;
    background: var(--surface-3);
    display: block;
  }
  :global(.muj-popup .mcov.ph) {
    background: linear-gradient(135deg, var(--surface-3), var(--surface-2));
    border: 1px solid var(--border);
  }
  :global(.muj-popup .minfo) { flex: 1 1 auto; min-width: 0; }
  :global(.muj-popup .mtitle) {
    color: var(--text);
    font-weight: 600;
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  :global(.muj-popup .msub) { font-size: 11.5px; color: var(--text-muted); margin-top: 2px; }
  :global(.muj-popup .mmore) { display: block; padding: 8px 14px 12px; color: var(--accent); font-size: 13px; text-decoration: none; text-align: center; }
</style>