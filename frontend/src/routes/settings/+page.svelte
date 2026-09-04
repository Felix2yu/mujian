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
    s3_public_url: '',
    ai_enabled: false,
    ai_base_url: '',
    ai_model: '',
    ai_api_key: '',
    default_start_time: '19:30'
  });
  // GET /api/settings 返回的 secret 是掩码值（如 "sk12****"）；保存时若未改动
  // 就不回传该字段，避免把掩码存成真实密钥（后端也会兜底忽略）。
  let loadedS3Secret = $state('');
  // GET /api/settings 返回的 AI key 是掩码值（如 "sk-...****"）；未改动则不回传。
  let loadedAIApiKey = $state('');
  // 设置卡片的两列归属：按预估高度最短列优先分配，列高大致均衡。
  // 新增/调整卡片时同步这里的权重即可。
  const CARD_COLS = (() => {
    // 权重 = 实测卡片高度（含间距，px）；内容变化时按需更新
    const cards = [
      ['theme', 162], ['storage', 158], ['s3', 823], ['encode', 305], ['calendar', 280],
      ['fields', 389], ['status', 178], ['list', 224], ['backup', 988], ['security', 274], ['map', 435],
      ['ai', 360]
    ];
    const cols = [[], []];
    const hs = [0, 0];
    for (const [k, w] of cards) {
      const c = hs[0] <= hs[1] ? 0 : 1;
      cols[c].push(k);
      hs[c] += w;
    }
    return cols;
  })();

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

  // S3 连接自检：用当前（合并掩码后的）配置做一次真实读写探测，验证连通性 /
  // 凭据 / 桶存在 / path-style 寻址；不落库。
  let testingS3 = $state(false);
  let s3TestOk = $state(false);
  let s3TestError = $state('');

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

  // 自动备份
  const BACKUP_INTERVALS = [
    { v: 0, label: '关闭' },
    { v: 24, label: '每天' },
    { v: 72, label: '每 3 天' },
    { v: 168, label: '每周' },
    { v: 336, label: '每 2 周' },
    { v: 720, label: '每月' },
    { v: 2160, label: '每季度' }
  ];
  const BACKUP_FORMATS = [
    { v: 'db', label: '数据库快照（.db）', hint: '单个 SQLite 库文件，停机后直接换回文件即可恢复' },
    { v: 'json', label: '纯数据（data.json）', hint: '体积小，可从「数据」页导入恢复，不含封面' },
    { v: 'zip', label: '数据 + 封面（.zip）', hint: 'data.json 加全部封面文件，可从「数据」页导入完整恢复' }
  ];
  let backupFormat = $state('db');
  let backupRemote = $state(false);
  const s3Ready = $derived(!!(settings.s3_bucket?.trim() && settings.s3_access_key?.trim()));
  let backupInterval = $state(0);
  // 存量值不在预设档位时（如旧配置的 6 小时）动态补一个选项，避免下拉显示空白
  let intervalOptions = $derived(
    BACKUP_INTERVALS.some((i) => i.v === backupInterval)
      ? BACKUP_INTERVALS
      : [...BACKUP_INTERVALS, { v: backupInterval, label: `每 ${backupInterval} 小时` }]
  );
  let backupKeep = $state(10);
  let lastBackupAt = $state(0);
  let backupRunning = $state(false);
  let backupMsg = $state('');
  let backups = $state([]);
  let restoringFile = $state('');

  function fmtSize(n) {
    if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB';
    if (n >= 1 << 10) return (n / (1 << 10)).toFixed(0) + ' KB';
    return n + ' B';
  }
  async function refreshBackups() {
    try {
      const res = await api.backupList();
      backups = res.backups || [];
    } catch (e) { /* 列表加载失败不打断页面 */ }
  }
  async function backupRestore(file) {
    if (!confirm(`用 ${file} 恢复数据？现有同 ID 记录会被覆盖。`)) return;
    restoringFile = file;
    try {
      await api.backupRestoreFrom(file);
      backupMsg = '已从 ' + file + ' 恢复';
      setTimeout(() => (backupMsg = ''), 6000);
    } catch (e) {
      backupMsg = '恢复失败：' + e.message;
    } finally {
      restoringFile = '';
    }
  }
  async function backupRemove(file) {
    if (!confirm(`删除备份 ${file}？此操作不可恢复。`)) return;
    await api.backupDelete(file).catch((e) => (backupMsg = '删除失败：' + e.message));
    refreshBackups();
  }

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
      settings.ai_enabled = settings.ai_enabled === true;
      settings.ai_base_url = settings.ai_base_url || '';
      settings.ai_model = settings.ai_model || '';
      loadedAIApiKey = settings.ai_api_key || '';
      settings.ai_api_key = settings.ai_api_key || '';
      if (typeof settings.show_friends !== 'boolean') settings.show_friends = true;
      if (typeof settings.show_pay_price !== 'boolean') settings.show_pay_price = true;
      if (typeof settings.show_other_cost !== 'boolean') settings.show_other_cost = true;
      if (typeof settings.multi_currency !== 'boolean') settings.multi_currency = true;
      if (!settings.default_start_time) settings.default_start_time = '19:30';
      mapSource = loadPref('mujian:map_source', 'osm');
      mapKey = loadPref('mujian:map_custom_key', '');
      mapCustomUrl = loadPref('mujian:map_custom_url', '');
      authToken = loadPref('mujian:auth_token', '');
      authRequired = settings.auth_required === true;
      backupInterval = typeof settings.backup_interval_hours === 'number' ? settings.backup_interval_hours : 0;
      backupKeep = typeof settings.backup_keep === 'number' ? settings.backup_keep : 10;
      backupFormat = ['db', 'json', 'zip'].includes(settings.backup_format) ? settings.backup_format : 'db';
      backupRemote = settings.backup_remote === true;
      lastBackupAt = typeof settings.last_backup_at === 'number' ? settings.last_backup_at : 0;
      refreshBackups();
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
    // 令牌变了，订阅链接里的 ?token= 也要跟着刷新
    icsUrl = `${window.location.origin}${api.getICSUrl()}`;
    try {
      const payload = {
        theme: currentTheme,
        storage_type: settings.storage_type,
        image_format: settings.image_format,
        show_friends: settings.show_friends,
        show_pay_price: settings.show_pay_price,
        show_other_cost: settings.show_other_cost,
        multi_currency: settings.multi_currency,
        default_start_time: settings.default_start_time || '19:30',
        backup_interval_hours: backupInterval,
        backup_keep: Math.max(1, Number(backupKeep) || 10),
        backup_format: backupFormat,
        backup_remote: backupRemote
      };
      // S3 凭据独立于存储方式：本地存储模式下也可配置（供备份推送等使用），
      // 始终提交。掩码值（含 ****）说明用户没有改密钥，不回传；后端同样会忽略。
      payload.s3_endpoint = settings.s3_endpoint.trim();
      payload.s3_bucket = settings.s3_bucket.trim();
      payload.s3_region = settings.s3_region.trim() || 'us-east-1';
      if (settings.s3_access_key.trim() && !settings.s3_access_key.includes('****')) {
        payload.s3_access_key = settings.s3_access_key.trim();
      }
      payload.s3_public_url = settings.s3_public_url.trim();
      if (settings.s3_secret_key && !settings.s3_secret_key.includes('****')) {
        payload.s3_secret_key = settings.s3_secret_key;
      }
      // AI 填写配置：始终提交 key 原值（含空字符串）；后端会忽略掩码值（含 ****），
      // 清空则真实移除，保持不变则保留已存密钥。
      payload.ai_enabled = !!settings.ai_enabled;
      payload.ai_base_url = settings.ai_base_url.trim();
      payload.ai_model = settings.ai_model.trim();
      payload.ai_api_key = settings.ai_api_key;
      await api.updateSettings(payload);
      resetStorageInfo();
      saved = true;
      setTimeout(() => (saved = false), 2400);
      const fresh = await api.getSettings().catch(() => null);
      if (fresh) lastBackupAt = fresh.last_backup_at || 0;
    } catch (e) {
      error = e.message;
    }
  }

  async function runBackupNow() {
    backupRunning = true;
    backupMsg = '';
    try {
      const res = await api.backupRun();
      backupMsg = `已生成备份 ${res.file}`;
      lastBackupAt = Math.floor(Date.now() / 1000);
      refreshBackups();
    } catch (e) {
      backupMsg = '备份失败：' + e.message;
    } finally {
      backupRunning = false;
      setTimeout(() => (backupMsg = ''), 6000);
    }
  }

  function fmtBackupTime(ts) {
    if (!ts) return '从未备份';
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
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

  async function testS3() {
    testingS3 = true;
    s3TestOk = false;
    s3TestError = '';
    try {
      const payload = {
        s3_endpoint: settings.s3_endpoint.trim(),
        s3_bucket: settings.s3_bucket.trim(),
        s3_region: settings.s3_region.trim() || 'us-east-1',
        s3_public_url: settings.s3_public_url.trim()
      };
      // 掩码值（含 ****）说明用户没有改密钥，不回传；后端用已保存的真实值兜底
      if (settings.s3_access_key.trim() && !settings.s3_access_key.includes('****')) {
        payload.s3_access_key = settings.s3_access_key.trim();
      }
      if (settings.s3_secret_key && !settings.s3_secret_key.includes('****')) {
        payload.s3_secret_key = settings.s3_secret_key;
      }
      const res = await api.testS3Connection(payload);
      if (res.ok) {
        s3TestOk = true;
      } else {
        s3TestError = res.error || 'S3 连接测试失败';
      }
    } catch (e) {
      s3TestError = e.message;
    } finally {
      testingS3 = false;
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
{#snippet themeCard()}
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
{/snippet}

{#snippet storageCard()}
<div class="card sec">
      <h3>存储</h3>
      <label>封面图片存储方式</label>
      <select class="input" bind:value={settings.storage_type} style="max-width: 280px;">
        <option value="local" disabled={!settings.allow_local_storage}>本地存储</option>
        <option value="s3">S3 对象存储</option>
      </select>

      {#if settings.storage_type === 's3'}
        {#if !settings.s3_bucket.trim() || !settings.s3_access_key.trim()}
          <div class="banner error">⚠ 存储方式为 S3，但下方「S3 对象存储」卡片的 Bucket / Access Key 未填写，保存后会退回本地存储</div>
        {:else}
          <p class="hint-row">✓ S3 凭据已配置（见下方「S3 对象存储」卡片），保存后立即生效，无需重启</p>
        {/if}
        <p class="hint-row">切换存储方式不会自动迁移已有封面：切到 S3 后旧图仍留在本地磁盘，新上传的才会写入 S3；可在下方 S3 卡片中执行「把本地封面上传到 S3」实现无缝衔接</p>
      {/if}
    </div>
{/snippet}

{#snippet encodeCard()}
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
{/snippet}

{#snippet calendarCard()}
<div class="card sec">
      <h3>日历</h3>
      <p class="hint" style="margin-bottom: 12px;">将演出记录同步到系统日历：下载 .ics 文件导入，或复制订阅链接添加到日历应用（如 Apple 日历 → 订阅日历），内容会随记录更新</p>
      <div class="cal-actions">
        <a class="btn" href={api.getICSUrl({ dl: '1' })}>⇩ 导出日历 (.ics)</a>
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
{/snippet}

{#snippet fieldsCard()}
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
      <div class="time-row">
        <span>默认演出开始时间</span>
        <input class="input" type="time" bind:value={settings.default_start_time} style="max-width: 120px;" />
      </div>
    </div>
{/snippet}

{#snippet statusCard()}
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
{/snippet}

{#snippet listCard()}
<div class="card sec">
      <h3>列表</h3>
      <p class="hint" style="margin-bottom: 12px;">首页浏览辅助（本地偏好，立即生效）</p>
      <label class="switch-row">
        <span>进入首页自动定位到当前时间</span>
        <input type="checkbox" checked={jumpNowPref} onchange={(e) => setJumpNowPref(e.target.checked)} />
      </label>
      <p class="hint" style="margin-top: 8px;">开启后每次打开演出列表会自动滚动到最近已发生（含今天）的演出；列表右下角也随时提供手动定位按钮</p>
    </div>
{/snippet}

{#snippet backupCard()}
<div class="card sec">
      <h3>自动备份</h3>
      <div class="s3-grid">
        <label class="field">
          <span>备份格式</span>
          <select class="input" bind:value={backupFormat} style="max-width: 260px;">
            {#each BACKUP_FORMATS as f (f.v)}
              <option value={f.v}>{f.label}</option>
            {/each}
          </select>
          <span class="hint">{BACKUP_FORMATS.find((f) => f.v === backupFormat)?.hint}</span>
        </label>
        <label class="field">
          <span>备份间隔</span>
          <select class="input" bind:value={backupInterval} style="max-width: 200px;">
            {#each intervalOptions as it (it.v)}
              <option value={it.v}>{it.label}</option>
            {/each}
          </select>
          <span class="hint">按间隔自动把所选格式的备份写入服务端 backups/ 目录</span>
        </label>
        <label class="field">
          <span>保留份数</span>
          <input class="input" type="number" min="1" max="100" bind:value={backupKeep} style="max-width: 120px;" />
          <span class="hint">超出后自动删除最旧的快照</span>
        </label>
        <label class="field">
          <span>上次备份</span>
          <input class="input" readonly value={fmtBackupTime(lastBackupAt)} />
        </label>
        <label class="field">
          <span>备份后上传到 S3</span>
          <input type="checkbox" bind:checked={backupRemote} disabled={!s3Ready} style="align-self: center;" />
          <span class="hint">
            {#if s3Ready}
              每次备份成功后把该文件推送到上方「S3 对象存储」卡片的桶内 backups/ 目录（本机备份仍保留）
            {:else}
              需要先在「S3 对象存储」卡片填写完整的 Bucket 与 Access Key
            {/if}
          </span>
        </label>
      </div>
      <div class="convert-actions" style="margin-top: 10px;">
        <button class="btn" disabled={backupRunning} onclick={runBackupNow}>
          {backupRunning ? '备份中…' : '立即备份'}
        </button>
        {#if backupMsg}<span class="hint" style="align-self: center;">{backupMsg}</span>{/if}
      </div>

      {#if backups.length}
        <div class="backup-list">
          {#each backups as b (b.file)}
            <div class="backup-row">
              <span class="backup-name" title={b.file}>{b.file}</span>
              <span class="backup-size">{fmtSize(b.size)}</span>
              <span class="backup-time">{fmtBackupTime(b.modified)}</span>
              <span class="backup-ops">
                <a class="btn sm" href={api.backupDownloadUrl(b.file)} download>下载</a>
                {#if b.file.endsWith('.json') || b.file.endsWith('.zip')}
                  <button type="button" class="btn sm" disabled={restoringFile === b.file} onclick={() => backupRestore(b.file)}>
                    {restoringFile === b.file ? '恢复中…' : '恢复'}
                  </button>
                {:else}
                  <button type="button" class="btn sm" disabled title=".db 快照需停机后替换数据库文件恢复">恢复</button>
                {/if}
                <button type="button" class="btn sm danger" onclick={() => backupRemove(b.file)}>删除</button>
              </span>
            </div>
          {/each}
        </div>
      {:else}
        <p class="hint" style="margin-top: 10px;">还没有备份文件：点「立即备份」生成第一份，或开启自动备份。</p>
      {/if}
      <p class="hint" style="margin-top: 8px;">修改间隔或保留份数后需点击页面底部的「保存」才会生效；重启服务不会重置备份节奏（按最近一份快照的时间续算）。</p>
    </div>
{/snippet}

{#snippet securityCard()}
<div class="card sec">
      <h3>访问安全</h3>
      <label>
        访问令牌（可选）
        {#if authRequired}<span class="hint" style="color: var(--warn, #b45309);">· 服务端已启用鉴权</span>{/if}
      </label>
      <input class="input" type="password" bind:value={authToken} placeholder="未设置" autocomplete="new-password" style="max-width: 320px;" />
      <p class="hint" style="margin-top: 8px;">
        在服务器上配置环境变量 MJ_AUTH_TOKEN（或设置文件中的 auth_token）即启用鉴权，此后所有 API/MCP 请求都必须携带这个令牌。
        在上方填入与服务器相同的值并保存即可正常使用：本页只把它记在本机浏览器（localStorage），每次请求自动附带；
        保存时不会把令牌发回服务器——服务器已经从环境变量拿到了这个值，无需再存一份。
        {#if authRequired}
          服务端已启用鉴权：未填写或填错时，除本页外的所有页面和接口都会提示 401 未授权，填对后立即恢复，无需重启。
        {/if}
        日历订阅地址会自动附带 ?token= 参数（日历客户端无法自定义请求头）。
      </p>
    </div>
{/snippet}

{#snippet mapCard()}
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
{/snippet}

{#snippet s3Card()}
<div class="card sec">
      <h3>S3 对象存储</h3>
      <p class="tiny muted" style="margin: 0 0 10px;">封面存储与自动备份的 S3 推送共用这组凭据；无论当前存储方式如何都可在此配置。</p>
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

      {#if settings.storage_type === 's3' && (!settings.s3_bucket.trim() || !settings.s3_access_key.trim())}
        <div class="banner error">⚠ 存储方式为 S3：Bucket 与 Access Key 必须填写，否则保存后退回本地存储</div>
      {:else if settings.s3_bucket.trim() && settings.s3_access_key.trim()}
        <p class="hint-row">✓ S3 配置已就绪，保存后立即生效，无需重启</p>
      {/if}

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
          <button
            class="btn"
            class:disabled={testingS3}
            onclick={testS3}
            disabled={testingS3}
          >
            {#if testingS3}
              测试中…
            {:else}
              测试 S3 连接
            {/if}
          </button>
          <span class="hint">「测试连接」会用当前配置（含已保存的掩码密钥）做一次真实读写探测，验证连通性、凭据与 path-style 寻址是否正确，不影响已存数据</span>
        </div>

        {#if testingS3}
          <div class="banner info"><span>⏳ 正在探测 S3 连接…</span></div>
        {/if}
        {#if s3TestOk}
          <div class="banner success">✓ S3 连接成功：已可向该桶写入并读取（path-style 寻址正常）</div>
        {/if}
        {#if s3TestError}
          <div class="banner error">⚠ S3 连接失败：{s3TestError}</div>
        {/if}

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
    </div>
{/snippet}

{#snippet aiCard()}
<div class="card sec">
      <h3>AI 填写</h3>
      <p class="tiny muted" style="margin: 0 0 10px;">配置一个 OpenAI 兼容的聊天模型接口后，新建演出时可粘贴演出信息（购票短信 / 宣传文案等），一键把内容填进对应字段。密钥仅保存在服务端，不会下发到浏览器。</p>
      <label class="switch-row">
        <span>启用 AI 填写</span>
        <input type="checkbox" bind:checked={settings.ai_enabled} />
      </label>
      <div class="s3-grid" style="margin-top: 12px;">
        <label class="field">
          <span>API 地址（Base URL）</span>
          <input class="input" type="text" bind:value={settings.ai_base_url} placeholder="https://api.openai.com/v1" spellcheck="false" autocomplete="off" />
          <span class="hint">OpenAI 兼容服务的 chat/completions 基址，如 https://api.openai.com/v1</span>
        </label>
        <label class="field">
          <span>模型（Model）</span>
          <input class="input" type="text" bind:value={settings.ai_model} placeholder="gpt-4o-mini" spellcheck="false" autocomplete="off" />
        </label>
        <label class="field">
          <span>API Key</span>
          <input class="input" type="password" bind:value={settings.ai_api_key} placeholder={loadedAIApiKey || '未设置'} autocomplete="new-password" />
          <span class="hint">已配置的密钥会以掩码显示；保持不变即可保留原值，清空后保存可移除</span>
        </label>
      </div>
    </div>
{/snippet}

  <!-- 两列按卡片高度权重最短列优先分配（CARD_COLS），保证列高大致均衡 -->
  <div class="col">
    {#each CARD_COLS[0] as key (key)}
			{#if key === "theme"}{@render themeCard()}
			{:else if key === "storage"}{@render storageCard()}
			{:else if key === "s3"}{@render s3Card()}
			{:else if key === "encode"}{@render encodeCard()}
			{:else if key === "calendar"}{@render calendarCard()}
			{:else if key === "fields"}{@render fieldsCard()}
			{:else if key === "status"}{@render statusCard()}
			{:else if key === "list"}{@render listCard()}
			{:else if key === "backup"}{@render backupCard()}
			{:else if key === "security"}{@render securityCard()}
			{:else if key === "map"}{@render mapCard()}
			{:else if key === "ai"}{@render aiCard()}
			{/if}
    {/each}
  </div>
  <div class="col">
    {#each CARD_COLS[1] as key (key)}
			{#if key === "theme"}{@render themeCard()}
			{:else if key === "storage"}{@render storageCard()}
			{:else if key === "s3"}{@render s3Card()}
			{:else if key === "encode"}{@render encodeCard()}
			{:else if key === "calendar"}{@render calendarCard()}
			{:else if key === "fields"}{@render fieldsCard()}
			{:else if key === "status"}{@render statusCard()}
			{:else if key === "list"}{@render listCard()}
			{:else if key === "backup"}{@render backupCard()}
			{:else if key === "security"}{@render securityCard()}
			{:else if key === "map"}{@render mapCard()}
			{:else if key === "ai"}{@render aiCard()}
			{/if}
    {/each}
  </div>
</div><button class="btn primary" onclick={save}>保存设置</button>
  {/if}
</div>

<style>
  /* 宽屏两列独立容器（按权重最短列优先分配，见 CARD_COLS），卡片保持
     自身高度；窄屏自动堆叠为单列。 */
  .settings-grid {
    display: flex;
    gap: 14px;
    align-items: flex-start;
    margin-bottom: 14px;
  }
  .settings-grid .col {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .sec { padding: 18px 20px; margin-bottom: 0; }
  @media (max-width: 859px) {
    .settings-grid { flex-direction: column; }
  }
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
  .cal-subscribe .input { flex: 1; font-size: 16px; color: var(--text-2); }
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
  .time-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 10px 0;
    font-size: 14px;
    color: var(--text-2);
    border-top: 1px solid var(--border);
  }

  /* ---------- 自动备份列表 ---------- */
  .backup-list { margin-top: 12px; border: 1px solid var(--border); border-radius: var(--radius, 10px); overflow: hidden; }
  .backup-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .backup-row:last-child { border-bottom: none; }
  .backup-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
  .backup-size { color: var(--text-3); font-size: 12px; flex: none; }
  .backup-time { color: var(--text-3); font-size: 12px; flex: none; }
  .backup-ops { display: flex; gap: 6px; flex: none; }
</style>
