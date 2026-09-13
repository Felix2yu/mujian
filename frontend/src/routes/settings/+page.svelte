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
    huozhi_enabled: false,
    huozhi_domain: '',
    huozhi_api_key: '',
    huozhi_bill_path: '/bills/{id}',
    default_start_time: '19:30',
    reminder_mode: 'hours_before',
    reminder_before_hours: 12,
    reminder_daily_hour: 10,
    reminder_daily_minute: 0
  });
  // GET /api/settings 返回的 secret 是掩码值（如 "sk12****"）；保存时若未改动
  // 就不回传该字段，避免把掩码存成真实密钥（后端也会兜底忽略）。
  let loadedS3Secret = $state('');
  // GET /api/settings 返回的 AI key 是掩码值（如 "sk-...****"）；未改动则不回传。
  let loadedAIApiKey = $state('');
  // 货殖 API Key 同理：掩码值表示未改动。
  let loadedHuozhiApiKey = $state('');
  // 设置卡片的两列归属：按预估高度最短列优先分配，列高大致均衡。
  // 新增/调整卡片时同步这里的权重即可。
  const CARD_COLS = (() => {
    // 权重 = 实测卡片高度（含间距，px）；内容变化时按需更新
    const cards = [
      ['theme', 162], ['storage', 158], ['s3', 823], ['encode', 305], ['ics', 210], ['caldav', 470],
      ['fields', 389], ['status', 178], ['list', 224], ['backup', 988], ['security', 274], ['map', 435],
      ['ai', 360], ['huozhi', 470]
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
  let saving = $state(false);
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

  // ===== 费用补全 =====
  //
  // 语义（与后端 internal/db/cost_review.go 一致）：
  //   · 字段值 > 0                  → 已填写，永不进入待确认清单，也永不被改写；
  //   · 值 NULL（未填写）或 0       → 进入待确认清单；
  //   · 用户对某字段做出确认后，服务端记一条 cost_reviews，该条目随即离开清单。
  //
  // 关键点：历史版本把「未填写」默认存成了 0，所以清单里的 0 元无法与真实
  // 免费区分。本模块不猜测，而是把区分动作交给用户逐项确认 —— 确认记录可撤销，
  // 补全操作只写当前为 NULL/0 的字段，因此不可能误改有效金额。
  const COST_FIELDS = [
    { key: 'price', label: '票价', desc: '票面价格', zeroLabel: '免费', zeroAction: '标记为免费' },
    { key: 'pay_price', label: '实付', desc: '实际支付金额', zeroLabel: '无实付', zeroAction: '标记为无实付' },
    { key: 'other_cost', label: '其他花费', desc: '交通、餐饮等额外支出', zeroLabel: '无支出', zeroAction: '标记为无支出' }
  ];

  let costLoading = $state(false);
  let costError = $state('');
  let costNotice = $state('');
  let costBusy = $state(false);
  let costRecords = $state([]);   // 仍待确认的记录（服务端已排除已标记项）
  let costSummary = $state(null); // 全量概览计数
  let costFieldFilter = $state('all'); // 'all' | 字段 key
  let costValueFilter = $state('pending'); // 'pending' | 'empty' | 'zero'
  let costVenue = $state('');
  let costQuery = $state('');
  let costExpanded = $state({ price: true, pay_price: true, other_cost: true });
  let costSel = $state({ price: [], pay_price: [], other_cost: [] });
  let costRowAmount = $state({}); // `${field}:${id}` -> 行内输入金额

  function costRowKey(field, id) { return `${field}:${id}`; }

  // 费用补全列表里的日期：只到天，避免时区把 19:30 的演出显示成第二天。
  function fmtCostDate(ts) {
    if (!ts) return '—';
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  }

  // 场馆候选（按待确认条数降序）
  let costVenueOptions = $derived.by(() => {
    const map = new Map();
    for (const r of costRecords) {
      if (r.address) map.set(r.address, (map.get(r.address) || 0) + 1);
    }
    return [...map.entries()].sort((a, b) => b[1] - a[1]).map(([address, count]) => ({ address, count }));
  });

  // 场馆 / 关键词筛选
  let costFiltered = $derived.by(() => {
    const q = costQuery.trim().toLowerCase();
    return costRecords.filter((r) => {
      if (costVenue && r.address !== costVenue) return false;
      if (q) {
        const hay = `${r.name || ''} ${r.address || ''} ${r.city || ''}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  });

  // 把记录「按字段展开」成行：一行 = 一条记录的一个待确认字段。
  // kind 用于区分两类待确认：empty = 列为 NULL（确定未填写）；
  // zero = 值为 0（可能是真实免费，也可能是历史默认的脏数据）。
  let costRows = $derived.by(() => {
    const out = {};
    for (const f of COST_FIELDS) out[f.key] = [];
    for (const r of costFiltered) {
      const pending = r.pending_fields || [];
      for (const f of COST_FIELDS) {
        if (!pending.includes(f.key)) continue;
        const raw = r[f.key];
        const kind = raw === null || raw === undefined ? 'empty' : 'zero';
        if (costValueFilter !== 'pending' && costValueFilter !== kind) continue;
        out[f.key].push({ rec: r, kind });
      }
    }
    return out;
  });

  let costVisibleFields = $derived(
    costFieldFilter === 'all' ? COST_FIELDS : COST_FIELDS.filter((f) => f.key === costFieldFilter)
  );

  let costVisibleCount = $derived(
    COST_FIELDS.reduce((n, f) => n + (costRows[f.key]?.length || 0), 0)
  );

  let costPendingCounts = $derived(costSummary?.pending || {});
  let costReviewedCounts = $derived(costSummary?.reviewed || {});
  let costReviewedTotal = $derived(costSummary?.reviewed_total || 0);

  // 生效目标：优先取勾选项，否则作用于当前筛选出的全部行。始终以「当前可见」
  // 取交集，避免筛选切换后残留的选择被误伤。
  function costTargets(field) {
    const visible = costRows[field].map((row) => row.rec.id);
    const sel = (costSel[field] || []).filter((id) => visible.includes(id));
    return sel.length > 0 ? sel : visible;
  }

  function costToggleSel(field, id) {
    const cur = costSel[field] || [];
    costSel = { ...costSel, [field]: cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id] };
  }

  function costSelectAll(field) {
    const visible = costRows[field].map((row) => row.rec.id);
    const cur = (costSel[field] || []).filter((id) => visible.includes(id));
    costSel = { ...costSel, [field]: cur.length === visible.length ? [] : visible };
  }

  async function loadCosts() {
    costLoading = true;
    costError = '';
    try {
      const res = await api.costPending();
      costRecords = res.records || [];
      costSummary = res.summary || null;
      // 丢掉已不在清单中的选择（例如刚被标记掉的记录）
      const alive = new Set(costRecords.map((r) => r.id));
      costSel = Object.fromEntries(
        COST_FIELDS.map((f) => [f.key, (costSel[f.key] || []).filter((id) => alive.has(id))])
      );
    } catch (e) {
      costError = '加载失败：' + e.message;
    } finally {
      costLoading = false;
    }
  }

  // 提交一次确认操作并刷新清单。所有入口都汇总到这里，保证「操作 → 重新拉取」
  // 的单一真相：清单永远来自服务端，前端不做乐观推断，避免本地状态与服务端
  // 的「已标记」判定漂移。
  async function costSubmit(payload, notice) {
    costBusy = true;
    costError = '';
    costNotice = '';
    try {
      await api.costReview(payload);
      costNotice = notice;
      costSel = { price: [], pay_price: [], other_cost: [] };
      await loadCosts();
    } catch (e) {
      costError = '操作失败：' + e.message;
    } finally {
      costBusy = false;
    }
  }

  // 分组批量：标记为 0 元（免费 / 无实付 / 无支出）
  function costMarkZero(field) {
    const ids = costTargets(field);
    if (ids.length === 0) return;
    const f = COST_FIELDS.find((x) => x.key === field);
    costSubmit({ ids, field, action: 'zero' }, `已将 ${ids.length} 条记录的${f.label}标记为${f.zeroLabel}`);
  }

  // 分组批量：跳过（暂不处理，列值不变，仅移出清单）
  function costMarkSkip(field) {
    const ids = costTargets(field);
    if (ids.length === 0) return;
    const f = COST_FIELDS.find((x) => x.key === field);
    costSubmit({ ids, field, action: 'skip' }, `已跳过 ${ids.length} 条${f.label}（列值保持不变）`);
  }

  // 分组批量：填入统一金额
  function costSetAmount(field) {
    const raw = String(costRowAmount[costRowKey(field, '__batch__')] ?? '').trim();
    const amount = Number(raw);
    if (raw === '' || !Number.isFinite(amount) || amount < 0) {
      costError = '请先填写一个不小于 0 的金额';
      return;
    }
    const ids = costTargets(field);
    if (ids.length === 0) return;
    const f = COST_FIELDS.find((x) => x.key === field);
    costSubmit({ ids, field, action: 'amount', amount }, `已为 ${ids.length} 条${f.label}填入 ${amount} 元`);
  }

  // 单行：填入该行输入的金额
  function costSetRowAmount(row) {
    const field = row.field;
    const k = costRowKey(field, row.rec.id);
    const raw = String(costRowAmount[k] ?? '').trim();
    const amount = Number(raw);
    if (raw === '' || !Number.isFinite(amount) || amount < 0) {
      costError = '请填写一个不小于 0 的金额';
      return;
    }
    costSubmit({ ids: [row.rec.id], field, action: 'amount', amount }, `已填入 ${amount} 元`);
  }

  // 单行：标记为 0 元
  function costRowZero(row) {
    const f = COST_FIELDS.find((x) => x.key === row.field);
    costSubmit({ ids: [row.rec.id], field: row.field, action: 'zero' },
      `已标记为${f.zeroLabel}`);
  }

  // 单行：跳过
  function costRowSkip(row) {
    costSubmit({ ids: [row.rec.id], field: row.field, action: 'skip' }, '已跳过该条（列值保持不变）');
  }

  // 撤销：单个字段全部标记
  async function costUndoField(field) {
    const f = COST_FIELDS.find((x) => x.key === field);
    costBusy = true;
    costError = '';
    costNotice = '';
    try {
      const res = await api.costUnreview({ all: true, field });
      costNotice = `已撤销 ${res.restored} 条${f.label}标记，它们重新回到待确认清单`;
      await loadCosts();
    } catch (e) {
      costError = '撤销失败：' + e.message;
    } finally {
      costBusy = false;
    }
  }

  // 撤销：全部字段的所有标记
  async function costUndoAll() {
    costBusy = true;
    costError = '';
    costNotice = '';
    try {
      const res = await api.costUnreview({ all: true });
      costNotice = `已撤销全部 ${res.restored} 条标记`;
      await loadCosts();
    } catch (e) {
      costError = '撤销失败：' + e.message;
    } finally {
      costBusy = false;
    }
  }

  // S3 连接自检：用当前（合并掩码后的）配置做一次真实读写探测，验证连通性 /
  // 凭据 / 桶存在 / path-style 寻址；不落库。
  let testingS3 = $state(false);
  let s3TestOk = $state(false);
  let s3TestError = $state('');

  // 货殖连接自检：可选地带一个账单 ID 真实拉取一次；不落库。
  let testingHuozhi = $state(false);
  let huozhiTestOk = $state(false);
  let huozhiTestError = $state('');
  let huozhiTestMsg = $state('');
  let huozhiTestBillId = $state('');

  // 日历订阅链接（同源部署，取当前站点地址拼出完整 URL）
  let icsUrl = $state('');
  let icsCopied = $state(false);
  // CalDAV 账户地址（iOS/macOS 原生日历，只读同步，支持地图卡片）
  let caldavUrl = $state('');
  let caldavCopied = $state(false);

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
      // 货殖账单关联
      settings.huozhi_enabled = settings.huozhi_enabled === true;
      settings.huozhi_domain = settings.huozhi_domain || '';
      settings.huozhi_bill_path = settings.huozhi_bill_path || '/bills/{id}';
      loadedHuozhiApiKey = settings.huozhi_api_key || '';
      settings.huozhi_api_key = settings.huozhi_api_key || '';
      settings.huozhi_ticket_categories = settings.huozhi_ticket_categories || '';
      if (typeof settings.show_friends !== 'boolean') settings.show_friends = true;
      if (typeof settings.show_pay_price !== 'boolean') settings.show_pay_price = true;
      if (typeof settings.show_other_cost !== 'boolean') settings.show_other_cost = true;
      if (typeof settings.multi_currency !== 'boolean') settings.multi_currency = true;
      if (!settings.default_start_time) settings.default_start_time = '19:30';
      if (!['hours_before', 'same_day'].includes(settings.reminder_mode)) settings.reminder_mode = 'hours_before';
      if (typeof settings.reminder_before_hours !== 'number' || settings.reminder_before_hours < 0 || settings.reminder_before_hours > 72) settings.reminder_before_hours = 12;
      if (typeof settings.reminder_daily_hour !== 'number' || settings.reminder_daily_hour < 0 || settings.reminder_daily_hour > 23) settings.reminder_daily_hour = 10;
      if (typeof settings.reminder_daily_minute !== 'number' || settings.reminder_daily_minute < 0 || settings.reminder_daily_minute > 59) settings.reminder_daily_minute = 0;
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
      loadCosts();
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
    saving = true;
    try {
      localStorage.setItem('mujian:auth_token', authToken || '');
    } catch (e) { /* ignore */ }
    // 令牌变了，订阅链接里的 ?token= 也要跟着刷新
    icsUrl = `${window.location.origin}${api.getICSUrl()}`;
    caldavUrl = `${window.location.origin}/caldav/user/calendars/mujian/`;
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
        reminder_mode: settings.reminder_mode || 'hours_before',
        reminder_before_hours: Math.max(0, Math.min(72, Number(settings.reminder_before_hours) || 12)),
        reminder_daily_hour: Math.max(0, Math.min(23, Number(settings.reminder_daily_hour) || 10)),
        reminder_daily_minute: Math.max(0, Math.min(59, Number(settings.reminder_daily_minute) || 0)),
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
      // 货殖账单关联：域名/路径始终提交；密钥掩码（含 ****）表示未改动，不回传。
      payload.huozhi_enabled = !!settings.huozhi_enabled;
      payload.huozhi_domain = settings.huozhi_domain.trim();
      payload.huozhi_bill_path = settings.huozhi_bill_path.trim() || '/bills/{id}';
      payload.huozhi_ticket_categories = settings.huozhi_ticket_categories.trim();
      payload.huozhi_api_key = settings.huozhi_api_key;
      await api.updateSettings(payload);
      resetStorageInfo();
      saved = true;
      setTimeout(() => (saved = false), 2400);
      const fresh = await api.getSettings().catch(() => null);
      if (fresh) lastBackupAt = fresh.last_backup_at || 0;
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
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

  async function testHuozhi() {
    testingHuozhi = true;
    huozhiTestOk = false;
    huozhiTestError = '';
    huozhiTestMsg = '';
    try {
      const payload = {
        huozhi_domain: settings.huozhi_domain.trim(),
        huozhi_bill_path: settings.huozhi_bill_path.trim() || '/bills/{id}'
      };
      // 掩码值（含 ****）说明用户没有改密钥，不回传；后端用已保存的真实值兜底
      if (settings.huozhi_api_key && !settings.huozhi_api_key.includes('****')) {
        payload.huozhi_api_key = settings.huozhi_api_key;
      }
      const bid = String(huozhiTestBillId || '').trim();
      if (bid) payload.bill_id = bid;
      const res = await api.testHuozhiConnection(payload);
      huozhiTestOk = !!res.ok;
      if (res.ok) {
        huozhiTestMsg = res.message || '连接成功';
      } else {
        huozhiTestError = res.error || '货殖连接测试失败';
      }
    } catch (e) {
      huozhiTestError = e.message;
    } finally {
      testingHuozhi = false;
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
    caldavUrl = `${window.location.origin}/caldav/user/calendars/mujian/`;
    statusFilter = loadStatusFilter();
    jumpNowPref = !!loadJsonPref('mujian:home_jump_now', false);
    load();
  });
</script>
<svelte:head><title>设置 - 幕间</title></svelte:head>

{#snippet costWizardCard()}
<div class="card sec cost-wizard">
  <h3>费用补全</h3>
  <p class="tiny muted cost-intro">
    集中处理「未填写」与「0 元」的费用字段。历史版本把「未填写」默认存成了 0，因此清单里的
    0 元既可能是真实的免费 / 无支出，也可能是遗留的脏数据 —— 请逐项确认。确认后的条目不会再
    出现在这里（可随时撤销），列值大于 0 的记录则完全不受本模块影响。
  </p>

  {#if costError}<div class="banner error">⚠ {costError}</div>{/if}
  {#if costNotice}<div class="banner success">✓ {costNotice}</div>{/if}

  <!-- 概览：服务端全量计数，不受本地筛选与列表截断影响 -->
  <div class="cost-overview">
    <div class="cost-stat">
      <span class="k">待确认记录</span>
      <span class="v">{costSummary?.pending_records ?? '—'}</span>
    </div>
    {#each COST_FIELDS as f (f.key)}
      <div class="cost-stat">
        <span class="k">{f.label}待确认</span>
        <span class="v accent">{costPendingCounts[f.key] ?? 0}</span>
      </div>
    {/each}
    <div class="cost-stat">
      <span class="k">已标记</span>
      <span class="v">{costReviewedTotal}</span>
    </div>
    <button class="btn sm ghost cost-refresh" onclick={loadCosts} disabled={costLoading || costBusy}>
      {costLoading ? '加载中…' : '刷新'}
    </button>
  </div>

  <!-- 筛选：「未填写」(NULL) 与「值为 0」(疑似历史脏数据) 分开看，是查漏补缺的关键 -->
  <div class="cost-filters">
    <div class="cost-seg" role="group" aria-label="字段筛选">
      <button class="cost-seg-btn" class:on={costFieldFilter === 'all'} onclick={() => (costFieldFilter = 'all')}>全部字段</button>
      {#each COST_FIELDS as f (f.key)}
        <button class="cost-seg-btn" class:on={costFieldFilter === f.key} onclick={() => (costFieldFilter = f.key)}>{f.label}</button>
      {/each}
    </div>
    <div class="cost-seg" role="group" aria-label="取值筛选" title="区分「确实是 0 元」与「历史默认写成了 0」">
      <button class="cost-seg-btn" class:on={costValueFilter === 'pending'} onclick={() => (costValueFilter = 'pending')}>全部</button>
      <button class="cost-seg-btn" class:on={costValueFilter === 'empty'} onclick={() => (costValueFilter = 'empty')} title="列为 NULL，确定未填写">未填写</button>
      <button class="cost-seg-btn" class:on={costValueFilter === 'zero'} onclick={() => (costValueFilter = 'zero')} title="值为 0：可能是真实免费，也可能是历史遗留的脏数据">值为 0</button>
    </div>
    <select class="input cost-venue-select" bind:value={costVenue} disabled={costBusy} aria-label="场馆筛选">
      <option value="">全部场馆</option>
      {#each costVenueOptions as v (v.address)}
        <option value={v.address}>{v.address}（{v.count}）</option>
      {/each}
    </select>
    <input class="input cost-search" type="search" placeholder="搜索剧目 / 场馆 / 城市" bind:value={costQuery} disabled={costBusy} />
    <span class="tiny muted cost-visible-count">当前筛选 {costVisibleCount} 条</span>
  </div>

  {#if costLoading && !costSummary}
    <div class="banner info" style="margin-top: 12px;">加载中…</div>
  {:else if (costSummary?.pending_records ?? 0) === 0}
    <div class="banner success" style="margin-top: 12px;">✓ 所有费用字段均已补全</div>
  {/if}

  {#each costVisibleFields as f (f.key)}
    {@const rows = costRows[f.key] ?? []}
    {@const sel = (costSel[f.key] || []).filter((id) => rows.some((r) => r.rec.id === id))}
    {@const targets = sel.length > 0 ? sel : rows.map((r) => r.rec.id)}
    <div class="cost-group">
      <div class="cost-group-head">
        <button
          class="cost-group-toggle"
          onclick={() => (costExpanded = { ...costExpanded, [f.key]: !costExpanded[f.key] })}
          disabled={costBusy}
        >
          <span class="chevron" class:open={costExpanded[f.key]}>▶</span>
          <span class="cost-group-title">{f.label}</span>
          <span class="tiny muted">{f.desc}</span>
        </button>
        <span class="badge">{costPendingCounts[f.key] ?? 0} 待确认</span>
      </div>

      {#if costExpanded[f.key]}
        <div class="cost-group-body">
          <div class="cost-bulk">
            <label class="cost-selectall">
              <input
                type="checkbox"
                checked={rows.length > 0 && sel.length === rows.length}
                onchange={() => costSelectAll(f.key)}
                disabled={costBusy || rows.length === 0}
              />
              <span>{sel.length > 0 ? `已选 ${sel.length} 条` : `共 ${rows.length} 条`}</span>
            </label>
            <button class="btn sm primary" disabled={costBusy || targets.length === 0} onclick={() => costMarkZero(f.key)}>
              {f.zeroAction}（{targets.length}）
            </button>
            <button class="btn sm" disabled={costBusy || targets.length === 0} onclick={() => costMarkSkip(f.key)}>
              跳过（{targets.length}）
            </button>
            <span class="cost-amount-group">
              <input
                class="input cost-mini-input"
                type="number" min="0" step="0.01" placeholder="金额"
                bind:value={costRowAmount[costRowKey(f.key, '__batch__')]}
                disabled={costBusy}
              />
              <button class="btn sm" disabled={costBusy || targets.length === 0} onclick={() => costSetAmount(f.key)}>填入</button>
            </span>
            {#if (costReviewedCounts[f.key] ?? 0) > 0}
              <button class="btn sm ghost" disabled={costBusy} onclick={() => costUndoField(f.key)}>
                撤销已标记 {costReviewedCounts[f.key]}
              </button>
            {/if}
          </div>

          {#if rows.length === 0}
            <div class="tiny muted" style="padding: 6px 0;">当前筛选条件下该字段无待确认条目</div>
          {:else}
            <div class="cost-rows">
              {#each rows as row (row.rec.id)}
                <div class="cost-row">
                  <input
                    type="checkbox"
                    checked={sel.includes(row.rec.id)}
                    onchange={() => costToggleSel(f.key, row.rec.id)}
                    disabled={costBusy}
                  />
                  <span class="cost-row-date">{fmtCostDate(row.rec.date)}</span>
                  <span class="cost-row-name" title={row.rec.name}>{row.rec.name}</span>
                  {#if row.rec.address}
                    <span class="cost-row-venue" title={row.rec.address}>{row.rec.address}</span>
                  {/if}
                  <span
                    class="cost-kind"
                    class:zero={row.kind === 'zero'}
                    title={row.kind === 'empty' ? '未填写（列为空）' : '值为 0：需确认是真实免费还是历史脏数据'}
                  >{row.kind === 'empty' ? '未填写' : '0 元'}</span>
                  <span class="cost-row-ops">
                    <input
                      class="input cost-mini-input"
                      type="number" min="0" step="0.01" placeholder="金额"
                      bind:value={costRowAmount[costRowKey(f.key, row.rec.id)]}
                      disabled={costBusy}
                    />
                    <button class="btn sm" disabled={costBusy} onclick={() => costSetRowAmount({ ...row, field: f.key })}>填入</button>
                    <button class="btn sm" disabled={costBusy} onclick={() => costRowZero({ ...row, field: f.key })}>{f.zeroLabel}</button>
                    <button class="btn sm ghost" disabled={costBusy} onclick={() => costRowSkip({ ...row, field: f.key })}>跳过</button>
                  </span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/each}
</div>
{/snippet}


<div class="fade-up">
  <div class="page-head">
    <h1>设置</h1>
    <p class="sub">偏好、同步与外发设置</p>
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
        <p class="hint-row">切换存储方式不会自动迁移已有封面：切到 S3 后旧图仍留在本地磁盘，新上传的才会写入 S3；<br />可在下方 S3 卡片中执行「把本地封面上传到 S3」实现无缝衔接</p>
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

{#snippet icsCard()}
<div class="card sec">
      <h3>ICS 订阅</h3>
      <p class="hint" style="margin-bottom: 12px;">
        把演出记录导出为标准 <b>.ics</b> 日历订阅源，兼容性最好：Google 日历、Outlook 及大部分第三方日历客户端都能订阅。限制：Apple 日历对订阅源（subscription）一律不渲染地图卡片（LOCATION 不可点），这是 Apple 平台限制，无法绕过；且为只读，演出改动需等客户端轮询刷新。
      </p>
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
        <span class="hint">订阅链接（自动附带 ?token=，日历客户端无法自定义请求头，故直接拼在 URL 上）。地址含令牌，请勿公开分享。两种方式详细对比见仓库 docs/calendar.md。</span>
      </div>
    </div>
{/snippet}

{#snippet caldavCard()}
<div class="card sec">
      <h3>CalDAV 账户</h3>
      <p class="hint" style="margin-bottom: 12px;">
        以账户方式把演出同步进 iOS / macOS 原生日历（推荐方式）。一个 CalDAV 账户会同步两类内容：<b>日历事件</b>——演出作为本地一等事件，LOCATION 显示可点地图卡片，随记录自动增量更新；<b>提醒事项任务清单「幕间·提醒」</b>——每场演出是一条到期提醒，在提醒事项 App 勾选完成即标记「已观看 / 已到场」并同步回幕间。所有内容只读，日历侧的增删改会被服务端拒绝。
      </p>
      <div class="cal-actions">
        <div class="cal-subscribe">
          <input class="input" readonly value={caldavUrl} onfocus={(e) => e.currentTarget.select()} />
          <button
            type="button"
            class="btn"
            onclick={async () => {
              try {
                await navigator.clipboard.writeText(caldavUrl);
                caldavCopied = true;
                setTimeout(() => (caldavCopied = false), 2000);
              } catch {
                // 剪贴板不可用时退化为选中文本手动复制
              }
            }}
          >
            {caldavCopied ? '已复制' : '复制'}
          </button>
        </div>
        <span class="hint">账户地址：iOS「设置 → 日历 → 账户 → 其他 → 添加 CalDAV 账户」/ macOS「系统设置 → 互联网账户」，服务器只填域名，用户名随意，密码填上面的访问令牌。<br />
        需 HTTPS；与 ICS 订阅并存会导致事件重复，配好后请退订旧订阅。</span>
      </div>

      <h4 style="margin: 18px 0 8px; font-size: 15px;">演出提醒（提醒事项）</h4>
      <p class="hint" style="margin-bottom: 12px;">
        控制「幕间·提醒」任务清单中每条演出的提醒时机：可选「演出前 N 小时」或「当天固定时刻」两种方式。保存后下次日历同步生效。
      </p>
      <div class="status-row">
        <label class="status-opt" class:on={settings.reminder_mode === 'hours_before'}>
          <input type="radio" name="reminder_mode" value="hours_before" checked={settings.reminder_mode === 'hours_before'} onchange={() => (settings.reminder_mode = 'hours_before')} />
          <span>演出前 N 小时</span>
        </label>
        <label class="status-opt" class:on={settings.reminder_mode === 'same_day'}>
          <input type="radio" name="reminder_mode" value="same_day" checked={settings.reminder_mode === 'same_day'} onchange={() => (settings.reminder_mode = 'same_day')} />
          <span>当天固定时刻</span>
        </label>
      </div>

      {#if settings.reminder_mode === 'hours_before'}
        <label class="field" style="margin-top: 12px;">
          <span>提前小时数（0–72）</span>
          <input class="input" type="number" min="0" max="72" bind:value={settings.reminder_before_hours} style="max-width: 110px;" />
          <span class="hint">演出开始前这么多个小时提醒。例：下午 2 点的演出设 2 小时，则中午 12 点提醒，而非凌晨。</span>
        </label>
      {:else}
        <label class="field" style="margin-top: 12px;">
          <span>当天提醒时刻</span>
          <input class="input" type="time"
            value={String(settings.reminder_daily_hour).padStart(2, '0') + ':' + String(settings.reminder_daily_minute).padStart(2, '0')}
            onchange={(e) => { const [h, m] = e.currentTarget.value.split(':').map(Number); settings.reminder_daily_hour = h || 0; settings.reminder_daily_minute = m || 0; }}
            style="max-width: 130px;" />
          <span class="hint">无论演出几点开始，统一在这一天这个时刻弹出提醒（按服务端时区）。例：设为 10:00，则每天上午 10 点提醒当天全部演出。</span>
        </label>
      {/if}
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
        在服务器上配置环境变量 MJ_AUTH_TOKEN（或设置文件中的 auth_token）即启用鉴权，此后所有 API/MCP 请求都必须携带这个令牌。<br />
        在上方填入与服务器相同的值并保存即可正常使用：本页只把它记在本机浏览器（localStorage），每次请求自动附带；
        保存时不会把令牌发回服务器——服务器已经从环境变量拿到了这个值，无需再存一份。
      </p>
      {#if authRequired}
      <p class="hint" style="margin-top: 8px;">服务端已启用鉴权：未填写或填错时，除本页外的所有页面和接口都会提示 401 未授权，填对后立即恢复，无需重启。</p>
      {/if}
      <p class="hint" style="margin-top: 8px;">ICS 订阅与 CalDAV 地址会自动附带 ?token= 参数（日历客户端无法自定义请求头，故直接拼在 URL 上）。</p>
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
          <span class="hint">前端从该地址直接加载封面，要求桶可公开读取或挂了 CDN。<br />留空则回退到 /uploads/ 路径（仅当反向代理把该路径映射到桶时可用）</span>
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

{#snippet huozhiCard()}
<div class="card sec">
      <h3>货殖账单关联</h3>
      <p class="tiny muted" style="margin: 0 0 10px;">
        货殖是自建的个人记账系统。<a href="https://github.com/Felix2yu/huozhi" target="_blank" rel="noopener noreferrer" style="color: var(--accent); text-decoration: underline; text-underline-offset: 2px;">项目主页 ↗</a><br />
        配置后，演出记录可绑定货殖账单 ID：详情页会展示这些账单的合计金额（悬停看描述 / 账户等明细，点击跳转账单页），并在货殖有数据时优先采用接口金额而非内建费用。<br />
        历史演出不做回溯关联，只在新建 / 编辑时手动绑定。
      </p>
      <label class="switch-row">
        <span>启用货殖关联</span>
        <input type="checkbox" bind:checked={settings.huozhi_enabled} />
      </label>
      <div class="s3-grid" style="margin-top: 12px;">
        <label class="field">
          <span>货殖域名</span>
          <input class="input" type="text" bind:value={settings.huozhi_domain} placeholder="https://huozhi.example.com" spellcheck="false" autocomplete="off" />
          <span class="hint">站点根地址；接口按「域名 + /api/public/bills/账单ID」拼接，只填主机名时自动补 https://</span>
        </label>
        <label class="field">
          <span>API Key</span>
          <input class="input" type="password" bind:value={settings.huozhi_api_key} placeholder={loadedHuozhiApiKey || '未设置'} autocomplete="new-password" />
          <span class="hint">以 X-API-Key 请求头发送，仅存服务端；保持不变即保留原值，清空后保存可移除</span>
        </label>
        <label class="field">
          <span>账单页面路径</span>
          <input class="input" type="text" bind:value={settings.huozhi_bill_path} placeholder={'/bills/{id}'} spellcheck="false" autocomplete="off" />
          <span class="hint">详情页跳转用的网页路径模板，其中的 id 会替换成账单 ID</span>
        </label>
        <label class="field">
          <span>门票分类关键词</span>
          <input class="input" type="text" bind:value={settings.huozhi_ticket_categories} placeholder="娱乐,演出,门票" spellcheck="false" autocomplete="off" />
          <span class="hint">货殖账单的分类（category）匹配这些关键词（逗号分隔）则归为「门票实付」，其余（交通/酒店等）归为「其他开销」</span>
        </label>
      </div>
      <div class="huozhi-test">
        <input
          class="input"
          type="text"
          bind:value={huozhiTestBillId}
          placeholder="账单 ID（可选，如 123）"
          spellcheck="false"
          autocomplete="off"
          style="max-width: 220px;"
        />
        <button class="btn sm" onclick={testHuozhi} disabled={testingHuozhi}>
          {testingHuozhi ? '测试中…' : '测试连接'}
        </button>
        <span class="hint">填写账单 ID 会真实拉取一次账单以验证接口返回；留空则只探测连通性</span>
      </div>
      {#if huozhiTestOk}
        <div class="banner success">✓ {huozhiTestMsg}</div>
      {/if}
      {#if huozhiTestError}
        <div class="banner error">⚠ 货殖连接失败：{huozhiTestError}</div>
      {/if}
      {#if settings.huozhi_enabled && !settings.huozhi_domain.trim()}
        <div class="banner error">⚠ 已启用但未填写货殖域名，保存后不会生效</div>
      {/if}
    </div>
{/snippet}

  <!-- 两列按卡片高度权重最短列优先分配（CARD_COLS），保证列高大致均衡 -->
  <div class="col">
    {#each CARD_COLS[0] as key (key)}
			{#if key === "theme"}{@render themeCard()}
			{:else if key === "storage"}{@render storageCard()}
			{:else if key === "s3"}{@render s3Card()}
			{:else if key === "encode"}{@render encodeCard()}
			{:else if key === "ics"}{@render icsCard()}
			{:else if key === "caldav"}{@render caldavCard()}
			{:else if key === "fields"}{@render fieldsCard()}
			{:else if key === "status"}{@render statusCard()}
			{:else if key === "list"}{@render listCard()}
			{:else if key === "backup"}{@render backupCard()}
			{:else if key === "security"}{@render securityCard()}
			{:else if key === "map"}{@render mapCard()}
			{:else if key === "ai"}{@render aiCard()}
			{:else if key === "huozhi"}{@render huozhiCard()}
			{/if}
    {/each}
  </div>
  <div class="col">
    {#each CARD_COLS[1] as key (key)}
			{#if key === "theme"}{@render themeCard()}
			{:else if key === "storage"}{@render storageCard()}
			{:else if key === "s3"}{@render s3Card()}
			{:else if key === "encode"}{@render encodeCard()}
			{:else if key === "ics"}{@render icsCard()}
			{:else if key === "caldav"}{@render caldavCard()}
			{:else if key === "fields"}{@render fieldsCard()}
			{:else if key === "status"}{@render statusCard()}
			{:else if key === "list"}{@render listCard()}
			{:else if key === "backup"}{@render backupCard()}
			{:else if key === "security"}{@render securityCard()}
			{:else if key === "map"}{@render mapCard()}
			{:else if key === "ai"}{@render aiCard()}
			{:else if key === "huozhi"}{@render huozhiCard()}
			{/if}
    {/each}
  </div>
</div>
  <p class="hint" style="margin: 4px 0 14px;">除标注「本地偏好，立即生效」的项外，所有改动需点击「保存设置」才会写入服务端。</p>
  <div class="save-row">
    <button class="btn primary" onclick={save} disabled={saving}>{saving ? '保存中…' : '保存设置'}</button>
    {#if saved}<span class="save-ok">已保存 ✓</span>{/if}
    {#if error}<span class="save-err">{error}</span>{/if}
  </div>

  <!-- 费用补全：数据清理工具，操作立即生效（与「保存设置」无关），故独立于
       上方两列网格，占满整行以便承载较宽的行内编辑区。 -->
  {@render costWizardCard()}
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
  .huozhi-test {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin-top: 14px;
  }
  .huozhi-test .hint { margin-top: 0; }
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

  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--accent-softer);
    color: var(--accent);
    font-size: 12px;
    font-weight: 600;
    line-height: 1.4;
  }
  .chevron {
    display: inline-block;
    font-size: 10px;
    transition: transform var(--t-fast) var(--ease);
    color: var(--text-3);
  }
  .chevron.open { transform: rotate(90deg); }

  /* ---------- 费用补全 ---------- */
  .cost-wizard { margin-top: 14px; }
  .cost-intro { margin: 0 0 12px; line-height: 1.65; }
  .cost-overview {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px 20px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface-2);
  }
  .cost-stat { display: flex; flex-direction: column; gap: 2px; min-width: 66px; }
  .cost-stat .k { font-size: 11.5px; color: var(--text-3); white-space: nowrap; }
  .cost-stat .v {
    font-size: 16px;
    font-weight: 700;
    color: var(--text-2);
    line-height: 1.2;
    font-variant-numeric: tabular-nums;
  }
  .cost-stat .v.accent { color: var(--accent); }
  .cost-refresh { margin-left: auto; }

  .cost-filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    margin-top: 12px;
  }
  .cost-seg {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
  }
  .cost-seg-btn {
    padding: 5px 11px;
    font-size: 12.5px;
    background: var(--surface);
    color: var(--text-3);
    border: none;
    border-right: 1px solid var(--border);
    cursor: pointer;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .cost-seg-btn:last-child { border-right: none; }
  .cost-seg-btn:hover { color: var(--accent); }
  .cost-seg-btn.on { background: var(--accent-softer); color: var(--accent); font-weight: 600; }
  .cost-venue-select {
    flex: 0 1 220px;
    min-width: 150px;
    height: 30px;
    font-size: 12.5px;
    padding: 0 8px;
  }
  .cost-search {
    flex: 1 1 180px;
    min-width: 140px;
    height: 30px;
    font-size: 12.5px;
    padding: 0 10px;
  }
  .cost-visible-count { white-space: nowrap; }

  .cost-group {
    margin-top: 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
  }
  .cost-group-head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    background: var(--surface-2);
  }
  .cost-group-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    min-width: 0;
    padding: 0;
    background: none;
    border: none;
    cursor: pointer;
    font-size: 13.5px;
    font-weight: 600;
    color: var(--text-2);
    text-align: left;
  }
  .cost-group-toggle:disabled { cursor: default; opacity: 0.7; }
  .cost-group-title { flex: none; }
  .cost-group-body { padding: 10px 12px 12px; border-top: 1px solid var(--border); }

  .cost-bulk {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
  }
  .cost-selectall {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    color: var(--text-3);
    cursor: pointer;
    user-select: none;
  }
  .cost-selectall input { accent-color: var(--accent); cursor: pointer; }
  .cost-amount-group { display: inline-flex; align-items: center; gap: 6px; }

  .cost-rows {
    border: 1px solid var(--border);
    border-radius: 6px;
    max-height: 360px;
    overflow-y: auto;
  }
  .cost-row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    padding: 6px 10px;
    font-size: 12.5px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .cost-row:last-child { border-bottom: none; }
  .cost-row input[type='checkbox'] { accent-color: var(--accent); cursor: pointer; flex: none; }
  .cost-row-date { color: var(--text-3); flex: none; font-variant-numeric: tabular-nums; }
  .cost-row-name {
    color: var(--text-2);
    flex: 1 1 140px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cost-row-venue {
    color: var(--text-3);
    font-size: 11.5px;
    flex: 0 1 140px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* 区分两类待确认：未填写（NULL，确定缺失）与值为 0（疑似历史脏数据） */
  .cost-kind {
    flex: none;
    padding: 1px 7px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    background: var(--accent-softer);
    color: var(--accent);
    white-space: nowrap;
  }
  .cost-kind.zero { background: var(--danger-soft); color: var(--danger); }
  /* 行内操作区可换行：窄屏（≤380px）时若固定不折行，会把整行顶出容器。 */
  .cost-row-ops {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex: 0 1 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
    margin-left: auto;
  }
  .cost-mini-input { width: 82px; height: 28px; font-size: 12.5px; padding: 0 8px; }
  @media (max-width: 420px) {
    /* 极窄屏下让操作区独占一行，避免与剧目/场馆挤在同一行。 */
    .cost-row-ops { flex-basis: 100%; margin-left: 0; justify-content: flex-start; }
  }
  .save-row {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .save-ok {
    color: #4ade80;
    font-size: 13.5px;
    font-weight: 600;
  }
  .save-err {
    color: #f87171;
    font-size: 13.5px;
    font-weight: 600;
    max-width: 520px;
  }
</style>
