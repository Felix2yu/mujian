<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import { theme } from '$lib/stores.js';

  let settings = $state({ storage_type: 'local', theme: 'auto', image_format: 'avif' });
  let error = $state('');
  let saved = $state(false);
  let loading = $state(true);
  let currentTheme = $state('auto');

  let converting = $state(false);
  let convertResult = $state(null);
  let convertError = $state('');
  let convertProgress = $state({ processed: 0, total: 0, converted: 0, skipped: 0, freed_bytes: 0 });

  let aligning = $state(false);
  let alignResult = $state(null);
  let alignError = $state('');

  const MAP_SOURCES = [
    { k: 'osm', label: '标准' },
    { k: 'gaode', label: '高德' },
    { k: 'tencent', label: '腾讯' },
    { k: 'custom', label: '自定义瓦片' }
  ];

  const IMAGE_FORMATS = [
    { k: 'avif', label: 'AVIF', hint: '体积最小，现代浏览器原生支持（推荐）' },
    { k: 'webp', label: 'WebP', hint: '兼容性最好的现代格式' },
    { k: 'jpeg', label: 'JPEG', hint: '兼容性最广，体积相对较大' }
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
      if (!settings.storage_type) settings.storage_type = 'local';
      if (!settings.theme) settings.theme = 'auto';
      if (!settings.image_format) settings.image_format = 'avif';
      if (typeof settings.show_friends !== 'boolean') settings.show_friends = true;
      if (typeof settings.show_pay_price !== 'boolean') settings.show_pay_price = true;
      if (typeof settings.show_other_cost !== 'boolean') settings.show_other_cost = true;
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
      await api.updateSettings({
        theme: currentTheme,
        storage_type: settings.storage_type,
        image_format: settings.image_format,
        show_friends: settings.show_friends,
        show_pay_price: settings.show_pay_price,
        show_other_cost: settings.show_other_cost
      });
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

  async function runBatchConvert(format) {
    converting = true;
    convertError = '';
    convertResult = null;
    convertProgress = { processed: 0, total: 0, converted: 0, skipped: 0, freed_bytes: 0 };
    try {
      // Batch conversion streams NDJSON progress lines from the server; each
      // line updates the live counter below instead of waiting for the end.
      const r = await api.convertBatchCovers(format, (p) => {
        if (typeof p.total === 'number') convertProgress.total = p.total;
        if (typeof p.converted === 'number') convertProgress.converted = p.converted;
        if (typeof p.skipped === 'number') convertProgress.skipped = p.skipped;
        if (typeof p.freed_bytes === 'number') convertProgress.freed_bytes = p.freed_bytes;
        if (p.phase === 'item') convertProgress.processed = p.index + 1;
      });
      convertResult = r;
    } catch (e) {
      convertError = e.message;
    } finally {
      converting = false;
    }
  }

  async function runAlignVenues() {
    aligning = true;
    alignError = '';
    alignResult = null;
    try {
      alignResult = await api.alignVenues();
    } catch (e) {
      alignError = e.message;
    } finally {
      aligning = false;
    }
  }

  const themes = [
    { v: 'auto', label: '跟随系统' },
    { v: 'light', label: '亮色' },
    { v: 'dark', label: '暗色' }
  ];

  onMount(() => {
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
            <span class="tico">
              {#if t.v === 'light'}
                <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <circle cx="12" cy="12" r="4" />
                  <line x1="12" y1="2" x2="12" y2="4.5" /><line x1="12" y1="19.5" x2="12" y2="22" />
                  <line x1="2" y1="12" x2="4.5" y2="12" /><line x1="19.5" y1="12" x2="22" y2="12" />
                  <line x1="4.6" y1="4.6" x2="6.3" y2="6.3" /><line x1="17.7" y1="17.7" x2="19.4" y2="19.4" />
                  <line x1="4.6" y1="19.4" x2="6.3" y2="17.7" /><line x1="17.7" y1="6.3" x2="19.4" y2="4.6" />
                </svg>
              {:else if t.v === 'dark'}
                <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true">
                  <path d="M21 12.8A8.5 8.5 0 1 1 11.2 3a6.6 6.6 0 0 0 9.8 9.8z" />
                </svg>
              {:else}
                <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
                  <circle cx="12" cy="12" r="9" />
                  <path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor" stroke="none" />
                </svg>
              {/if}
            </span>
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
      <h3>封面编码</h3>
      <label>默认编码格式
        <span class="hint">新上传海报会使用所选格式保存；更改后立即生效，无需重启</span>
      </label>
      <select class="input" bind:value={settings.image_format} style="max-width: 280px;">
        {#each IMAGE_FORMATS as f}
          <option value={f.k}>{f.label}</option>
        {/each}
      </select>
      {#each IMAGE_FORMATS as f}
        {#if settings.image_format === f.k}
          <div class="hint-row">💡 {f.hint}</div>
        {/if}
      {/each}

      <div class="convert-actions">
        <button
          class="btn"
          class:disabled={converting}
          onclick={() => runBatchConvert(settings.image_format)}
          disabled={converting}
        >
          {#if converting}
            转换中…
          {:else}
            批量转换已有海报为 {settings.image_format.toUpperCase()}
          {/if}
        </button>
        <span class="hint">会重新编码所有历史海报文件并自动更新数据库引用，此操作不可撤销</span>
      </div>

      {#if converting && convertProgress.total > 0}
        <div class="banner info">
          <span>⏳ 转换中… {convertProgress.processed}/{convertProgress.total}</span>
          <span class="hint" style="margin: 0;">已转换 {convertProgress.converted}，跳过 {convertProgress.skipped}，释放 {Math.round(convertProgress.freed_bytes / 1024)} KB</span>
        </div>
      {/if}

      {#if convertResult}
        <div class="banner success">
          ✓ 已转换 {convertResult.converted} 个文件，跳过 {convertResult.skipped} 个（已是目标格式），释放 {Math.round(convertResult.freed_bytes / 1024)} KB
        </div>
      {/if}
      {#if convertError}
        <div class="banner error">⚠ {convertError}</div>
      {/if}
    </div>

    <div class="card sec">
      <h3>记录字段</h3>
      <p class="hint" style="margin-bottom: 12px;">控制新建 / 编辑演出时是否显示以下字段，不常用的可按需隐藏</p>
      <label class="switch-row">
        <span>显示「同行人」</span>
        <input type="checkbox" bind:checked={settings.show_friends} />
      </label>
      <label class="switch-row">
        <span>显示「实付金额」</span>
        <input type="checkbox" bind:checked={settings.show_pay_price} />
      </label>
      <label class="switch-row">
        <span>显示「其他花费」</span>
        <input type="checkbox" bind:checked={settings.show_other_cost} />
      </label>
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
      <input class="input" type="text" bind:value={mapCustomUrl} placeholder={'https://{s}.example.com/tiles/{z}/{x}/{y}.png'} style="max-width: 420px;" />

      <div class="convert-actions">
        <button
          class="btn"
          class:disabled={aligning}
          onclick={() => runAlignVenues()}
          disabled={aligning}
        >
          {#if aligning}
            对齐中…
          {:else}
            对齐同场馆坐标
          {/if}
        </button>
        <span class="hint">按地址分组，用每组已有坐标回填同场馆其他演出，统一历史数据</span>
      </div>

      {#if alignResult}
        <div class="banner success">
          ✓ 已对齐 {alignResult.groups_aligned}/{alignResult.groups_total} 组场馆，更新 {alignResult.records_updated} 条记录
        </div>
      {/if}
      {#if alignError}
        <div class="banner error">⚠ {alignError}</div>
      {/if}
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
  .tico { font-size: 18px; display: inline-flex; align-items: center; justify-content: center; }
  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; display: block; margin-top: 4px; }
  .hint-row { margin-top: 8px; font-size: 12.5px; color: var(--text-3); }

  .convert-actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 14px;
    padding-top: 14px;
    border-top: 1px dashed var(--border);
  }
  .convert-actions .btn { width: fit-content; }
  .btn.disabled, .btn:disabled { opacity: 0.6; cursor: not-allowed; }
  .switch-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 10px 0;
    font-size: 14px;
    color: var(--text-2);
    border-top: 1px solid var(--border);
  }
  .switch-row:first-of-type { border-top: none; }
  .switch-row input { width: 18px; height: 18px; accent-color: var(--accent); cursor: pointer; }
</style>
