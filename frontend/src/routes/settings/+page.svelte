<script>
  import { onMount } from 'svelte';
  import { api, resetStorageInfo } from '$lib/api.js';
  import { theme } from '$lib/stores.js';
  import { STATUS_LABELS, ALL_STATUSES, loadStatusFilter, saveStatusFilter } from '$lib/statusPrefs.js';
  import { loadPref as loadJsonPref, savePref as saveJsonPref } from '$lib/prefs.js';

  let settings = $state({
    storage_type: 'local',
    theme: 'auto',
    image_format: 'avif',
    allow_local_storage: true,
    s3_endpoint: '',
    s3_bucket: '',
    s3_region: 'us-east-1',
    s3_access_key: '',
    s3_secret_key: '',
    s3_public_url: ''
  });
  // GET /api/settings 返回的 secret 是掩码值（如 "sk12****"）；保存时若未改动
  // 就不回传该字段，避免把掩码存成真实密钥（后端也会兜底忽略）。
  let loadedS3Secret = $state('');
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

  // 本地封面 → S3 迁移（幂等，已存在的对象自动跳过）
  let migrating = $state(false);
  let migrateResult = $state(null);
  let migrateError = $state('');
  let migrateProgress = $state({ processed: 0, total: 0 });

  // 日历订阅链接（同源部署，取当前站点地址拼出完整 URL）
  let icsUrl = $state('');
  let icsCopied = $state(false);

  // 首页列表显示哪些演出状态（本地偏好，不进服务端设置）
  let statusFilter = $state([0, 1, 2, 3]);
  function toggleStatus(v) {
    const set = new Set(statusFilter);
    if (set.has(v)) set.delete(v);
    else set.add(v);
    statusFilter = [...set].sort();
    saveStatusFilter(statusFilter);
  }

  // 进入首页自动定位到当前时间（本地偏好，不进服务端设置）
  let jumpNowPref = $state(false);
  function setJumpNowPref(v) {
    jumpNowPref = !!v;
    saveJsonPref('mujian:home_jump_now', jumpNowPref);
  }

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

  // 访问令牌（可选）：服务端通过 MJ_AUTH_TOKEN 或设置文件启用鉴权后，
  // 前端所有 API 请求需携带该令牌。仅保存在本机 localStorage，不回传服务器。
  let authToken = $state('');
  let authRequired = $state(false);

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
      if (typeof settings.allow_local_storage !== 'boolean') settings.allow_local_storage = true;
      for (const k of ['s3_endpoint', 's3_bucket', 's3_region', 's3_access_key', 's3_secret_key', 's3_public_url']) {
        if (typeof settings[k] !== 'string') settings[k] = '';
      }
      if (!settings.s3_region) settings.s3_region = 'us-east-1';
      loadedS3Secret = settings.s3_secret_key;
      if (typeof settings.show_friends !== 'boolean') settings.show_friends = true;
      if (typeof settings.show_pay_price !== 'boolean') settings.show_pay_price = true;
      if (typeof settings.show_other_cost !== 'boolean') settings.show_other_cost = true;
      if (typeof settings.multi_currency !== 'boolean') settings.multi_currency = true;
      mapSource = loadPref('mujian:map_source', 'osm');
      mapKey = loadPref('mujian:map_custom_key', '');
      mapCustomUrl = loadPref('mujian:map_custom_url', '');
      authToken = loadPref('mujian:auth_token', '');
      authRequired = settings.auth_required === true;
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
      localStorage.setItem('mujian:auth_token', authToken || '');
    } catch (e) { /* ignore */ }
    try {
      const payload = {
        theme: currentTheme,
        storage_type: settings.storage_type,
        image_format: settings.image_format,
        show_friends: settings.show_friends,
        show_pay_price: settings.show_pay_price,
        show_other_cost: settings.show_other_cost,
        multi_currency: settings.multi_currency
      };
      if (settings.storage_type === 's3') {
        payload.s3_endpoint = settings.s3_endpoint.trim();
        payload.s3_bucket = settings.s3_bucket.trim();
        payload.s3_region = settings.s3_region.trim() || 'us-east-1';
        payload.s3_access_key = settings.s3_access_key.trim();
        payload.s3_public_url = settings.s3_public_url.trim();
        // 掩码值（含 ****）说明用户没有改密钥，不回传；后端同样会忽略。
        if (settings.s3_secret_key && !settings.s3_secret_key.includes('****')) {
          payload.s3_secret_key = settings.s3_secret_key;
        }
      }
      await api.updateSettings(payload);
      resetStorageInfo();
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

  async function runMigrateToS3() {
    migrating = true;
    migrateError = '';
    migrateResult = null;
    migrateProgress = { processed: 0, total: 0 };
    try {
      const r = await api.migrateCoversToS3((p) => {
        if (typeof p.total === 'number') migrateProgress.total = p.total;
        if (p.phase === 'item') migrateProgress.processed = p.processed;
      });
      migrateResult = r;
    } catch (e) {
      migrateError = e.message;
    } finally {
      migrating = false;
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
    icsUrl = `${window.location.origin}${api.getICSUrl()}`;
    statusFilter = loadStatusFilter();
    jumpNowPref = !!loadJsonPref('mujian:home_jump_now', false);
    load();
  });
</script>
<svelte:head><title>设置 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>设置</h1>
    <p class="sub">外观与存储偏好</p>
  </div>

  {#if loading}
    <div class="skeleton" style="height: 220px;"></div>
  {:else}
    <div class="settings-grid">
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
        <option value="local" disabled={!settings.allow_local_storage}>本地存储</option>
        <option value="s3">S3 对象存储</option>
      </select>

      {#if settings.storage_type === 's3'}
        <div class="s3-grid">
          <label class="field">
            <span>S3 Endpoint</span>
            <input class="input" type="text" bind:value={settings.s3_endpoint} placeholder="https://<accountid>.r2.cloudflarestorage.com" autocomplete="off" spellcheck="false" />
            <span class="hint">兼容 AWS S3 / Cloudflare R2 / MinIO / OSS 等 S3 协议端点；AWS 官方 S3 可留空</span>
          </label>
          <label class="field">
            <span>Bucket</span>
            <input class="input" type="text" bind:value={settings.s3_bucket} placeholder="mujian" autocomplete="off" spellcheck="false" />
          </label>
          <label class="field">
            <span>Region</span>
            <input class="input" type="text" bind:value={settings.s3_region} placeholder="us-east-1" autocomplete="off" spellcheck="false" />
          </label>
          <label class="field">
            <span>Access Key ID</span>
            <input class="input" type="text" bind:value={settings.s3_access_key} autocomplete="off" spellcheck="false" />
            <span class="hint">已配置的 Access Key 会以掩码显示；保持不变即可保留原值，填入新值可覆盖</span>
          </label>
          <label class="field">
            <span>Secret Access Key</span>
            <input class="input" type="password" bind:value={settings.s3_secret_key} placeholder={loadedS3Secret || '未设置'} autocomplete="new-password" />
            <span class="hint">已配置的密钥会以掩码显示；保持不变即可保留原密钥，清空后保存可移除</span>
          </label>
          <label class="field">
            <span>公网访问地址（Public URL）</span>
            <input class="input" type="text" bind:value={settings.s3_public_url} placeholder="https://cdn.example.com/mujian" spellcheck="false" />
            <span class="hint">前端从该地址直接加载封面，要求桶可公开读取或挂了 CDN；留空则回退到 /uploads/ 路径（仅当反向代理把该路径映射到桶时可用）</span>
          </label>
        </div>

        {#if !settings.s3_bucket.trim() || !settings.s3_access_key.trim()}
          <div class="banner error">⚠ Bucket 与 Access Key 未填写，重启后仍会退回本地存储</div>
        {:else}
          <p class="hint-row">✓ 配置已就绪，保存并重启服务后生效</p>
        {/if}
        <p class="hint-row">切换存储方式不会自动迁移已有封面：切到 S3 后旧图仍留在本地磁盘（配好 Public URL 前可能无法显示），新上传的才会写入 S3</p>

        {#if settings.s3_bucket.trim() && settings.s3_access_key.trim()}
          <div class="convert-actions">
            <button
              class="btn"
              class:disabled={migrating}
              onclick={runMigrateToS3}
              disabled={migrating}
            >
              {#if migrating}
                迁移中…
              {:else}
                把本地封面上传到 S3
              {/if}
            </button>
            <span class="hint">按原 key 上传本地 covers/ 下的全部文件（含缩略图）；S3 中已存在的对象自动跳过，可重复执行。建议在切换存储方式前先运行，实现无缝衔接</span>
          </div>

          {#if migrating && migrateProgress.total > 0}
            <div class="banner info">
              <span>⏳ 迁移中… {migrateProgress.processed}/{migrateProgress.total}</span>
            </div>
          {/if}

          {#if migrateResult}
            <div class="banner success">
              ✓ 共 {migrateResult.total} 个文件：新上传 {migrateResult.migrated}，已存在跳过 {migrateResult.skipped}{#if migrateResult.failed}，失败 {migrateResult.failed}{/if}
            </div>
          {/if}
          {#if migrateError}
            <div class="banner error">⚠ {migrateError}</div>
          {/if}
        {/if}
      {:else}
        <p class="hint-row">封面文件保存在服务器 uploads 目录中</p>
      {/if}
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
      <h3>日历</h3>
      <p class="hint" style="margin-bottom: 12px;">将演出记录同步到系统日历：下载 .ics 文件导入，或复制订阅链接添加到日历应用（如 Apple 日历 → 订阅日历），内容会随记录更新</p>
      <div class="cal-actions">
        <a class="btn" href={`${api.getICSUrl()}?dl=1`}>⇩ 导出日历 (.ics)</a>
        <div class="cal-subscribe">
          <input class="input" readonly value={icsUrl} onfocus={(e) => e.currentTarget.select()} />
          <button
            type="button"
            class="btn"
            onclick={async () => {
              try {
                await navigator.clipboard.writeText(icsUrl);
                icsCopied = true;
                setTimeout(() => (icsCopied = false), 2000);
              } catch {
                // 剪贴板不可用时退化为选中文本手动复制
              }
            }}
          >
            {icsCopied ? '已复制' : '复制'}
          </button>
        </div>
        <span class="hint">订阅链接为完整地址，可在手机 / 电脑的日历应用中直接添加订阅；该地址无鉴权，请勿公开分享</span>
      </div>
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
      <label class="switch-row">
        <span>启用多币种（金额可单独选择币种）</span>
        <input type="checkbox" bind:checked={settings.multi_currency} />
      </label>
    </div>

    <div class="card sec">
      <h3>演出状态</h3>
      <p class="hint" style="margin-bottom: 12px;">控制首页列表显示哪些状态的演出（本地偏好，立即生效）</p>
      <div class="status-row">
        {#each ALL_STATUSES as sv}
          <label class="status-opt" class:on={statusFilter.includes(sv)}>
            <input type="checkbox" checked={statusFilter.includes(sv)} onchange={() => toggleStatus(sv)} />
            <span>{STATUS_LABELS[sv]}</span>
          </label>
        {/each}
      </div>
    </div>

    <div class="card sec">
      <h3>列表</h3>
      <p class="hint" style="margin-bottom: 12px;">首页浏览辅助（本地偏好，立即生效）</p>
      <label class="switch-row">
        <span>进入首页自动定位到当前时间</span>
        <input type="checkbox" checked={jumpNowPref} onchange={(e) => setJumpNowPref(e.target.checked)} />
      </label>
      <p class="hint" style="margin-top: 8px;">开启后每次打开演出列表会自动滚动到最近已发生（含今天）的演出；列表右下角也随时提供手动定位按钮</p>
    </div>

    <div class="card sec">
      <h3>访问安全</h3>
      <label>
        访问令牌（可选）
        {#if authRequired}<span class="hint" style="color: var(--warn, #b45309);">· 服务端已启用鉴权</span>{/if}
      </label>
      <input class="input" type="password" bind:value={authToken} placeholder="未设置" autocomplete="new-password" style="max-width: 320px;" />
      <p class="hint" style="margin-top: 8px;">
        服务端设置 MJ_AUTH_TOKEN 环境变量（或在设置文件中写入 auth_token）后，所有 API/MCP 请求都需携带该令牌；
        在此填入与服务器一致的值即可正常使用。令牌仅保存在本机浏览器，不会上传到服务器。
        日历订阅地址会自动附带 ?token= 参数。
      </p>
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
    </div>

    {#if error}<div class="banner error">⚠ {error}</div>{/if}
    {#if saved}<div class="banner success">✓ 已保存</div>{/if}

    <button class="btn primary" onclick={save}>保存设置</button>
  {/if}
</div>

<style>
  /* 宽屏下卡片两列铺排，窄屏自动退化为单列 */
  .settings-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
    gap: 14px;
    margin-bottom: 14px;
  }
  .sec { padding: 18px 20px; margin-bottom: 0; }
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
    transition: border-color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
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
  .s3-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    gap: 12px 16px;
    margin-top: 14px;
  }
  .field { display: flex; flex-direction: column; gap: 4px; font-size: 13.5px; color: var(--text-2); }

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
  .cal-actions { display: flex; flex-direction: column; gap: 8px; }
  .cal-subscribe { display: flex; gap: 8px; }
  .cal-subscribe .input { flex: 1; font-size: 12.5px; color: var(--text-2); }
  .cal-subscribe .btn { white-space: nowrap; }
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
  .status-row { display: flex; gap: 8px; flex-wrap: wrap; }
  .status-opt {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 7px 14px;
    border-radius: 999px;
    border: 1.5px solid var(--border);
    background: var(--surface-2);
    cursor: pointer;
    font-size: 13.5px;
    color: var(--text-2);
    transition: border-color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
    user-select: none;
  }
  .status-opt input { accent-color: var(--accent); cursor: pointer; }
  .status-opt.on { border-color: var(--accent); background: var(--accent-softer); color: var(--accent); font-weight: 600; }
</style>
