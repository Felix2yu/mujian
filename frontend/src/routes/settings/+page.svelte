<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import { theme } from '$lib/stores.js';

  let settings = $state({ storage_type: 'local', theme: 'auto' });
  let error = $state('');
  let saved = $state(false);
  let loading = $state(true);
  let currentTheme = $state('auto');

  const MAP_SOURCES = [
    { k: 'osm', label: '标准' },
    { k: 'gaode', label: '高德' },
    { k: 'tencent', label: '腾讯' },
    { k: 'custom', label: '自定义瓦片' }
  ];

  let mapSource = $state('osm');
  let mapKey = $state('');
  let mapCustomUrl = $state('');

  function loadPref(key, fallback) {
    try {
      return localStorage.getItem(key) || fallback;
    } catch (e) {
      return fallback;
    }
  }

  function saveMapPrefs() {
    try {
      localStorage.setItem('mujian:map_source', mapSource);
      localStorage.setItem('mujian:map_custom_key', mapKey || '');
      localStorage.setItem('mujian:map_custom_url', mapCustomUrl || '');
    } catch (e) { /* ignore */ }
  }

  async function load() {
    loading = true;
    error = '';
    try {
      settings = await api.getSettings();
      // 确保 settings 有完整的默认值
      if (!settings.storage_type) settings.storage_type = 'local';
      if (!settings.theme) settings.theme = 'auto';
      // 加载地图偏好
      mapSource = loadPref('mujian:map_source', 'osm');
      mapKey = loadPref('mujian:map_custom_key', '');
      mapCustomUrl = loadPref('mujian:map_custom_url', '');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function save() {
    saved = false;
    error = '';
    saveMapPrefs();
    try {
      await api.updateSettings({ theme: currentTheme, storage_type: settings.storage_type });
      saved = true;
      setTimeout(() => (saved = false), 2400);
    } catch (e) {
      error = e.message;
    }
  }

  function setTheme(v) {
    currentTheme = v;
    theme.set(v);
  }

  const themes = [
    { v: 'auto', label: '跟随系统', ico: '◐' },
    { v: 'light', label: '亮色', ico: '☀️' },
    { v: 'dark', label: '暗色', ico: '🌙' }
  ];

  onMount(() => {
    // 订阅 theme store
    const unsub = theme.subscribe((val) => {
      currentTheme = val;
    });
    load();
  });
</script>

<div class="fade-up">
  <div class="page-head">
    <h1>设置</h1>
    <p class="sub">外观与存储偏好</p>
  </div>

  {#if loading}
    <div class="skeleton" style="height: 220px; max-width: 520px;"></div>
  {:else}
    <div class="card sec">
      <h3>主题</h3>
      <div class="theme-row">
        {#each themes as t}
          <button
            class="theme-opt"
            class:on={currentTheme === t.v}
            onclick={() => setTheme(t.v)}
          >
            <span class="tico">{t.ico}</span>
            <span>{t.label}</span>
          </button>
        {/each}
      </div>
    </div>

    <div class="card sec">
      <h3>存储</h3>
      <label>封面图片存储方式</label>
      <select class="input" bind:value={settings.storage_type} style="max-width: 280px;">
        <option value="local">本地存储</option>
        <option value="s3">S3 对象存储</option>
      </select>
    </div>

    <div class="card sec">
      <h3>地图</h3>
      <label>默认底图</label>
      <select class="input" bind:value={mapSource} style="max-width: 280px;">
        {#each MAP_SOURCES as s}
          <option value={s.k}>{s.label}</option>
        {/each}
      </select>

      <label style="margin-top: 12px;">
        平台 Key（可选）
        <span class="hint">高德/腾讯地图在网页应用需填写 key 才会出图；可留空使用公共瓦片</span>
      </label>
      <input class="input" type="text" bind:value={mapKey} placeholder="例如高德或腾讯 JS 应用的 key" style="max-width: 320px;" />

      <label style="margin-top: 12px;">自定义瓦片 URL（可选）</label>
      <input class="input" type="text" bind:value={mapCustomUrl} placeholder="https://{s}.example.com/tiles/{z}/{x}/{y}.png" style="max-width: 420px;" />
    </div>

    {#if error}<div class="banner error">⚠ {error}</div>{/if}
    {#if saved}<div class="banner success">✓ 已保存</div>{/if}

    <button class="btn primary" onclick={save}>保存设置</button>
  {/if}
</div>

<style>
  .sec { padding: 18px 20px; margin-bottom: 14px; max-width: 520px; }
  .sec h3 { margin: 0 0 12px; font-size: 15.5px; }

  .theme-row { display: flex; gap: 8px; }
  .theme-opt {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 14px 8px;
    border-radius: var(--radius);
    border: 1.5px solid var(--border);
    background: var(--surface-2);
    cursor: pointer;
    font-size: 13.5px;
    color: var(--text-2);
    transition: all var(--t-fast) var(--ease);
  }
  .theme-opt:hover { border-color: var(--border-strong); }
  .theme-opt.on {
    border-color: var(--accent);
    background: var(--accent-softer);
    color: var(--accent);
    font-weight: 600;
  }
  .tico { font-size: 18px; }
  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; display: block; margin-top: 4px; }
</style>