<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { page } from '$app/stores';
  import { api } from '$lib/api.js';
  import { loadStatusFilter, ALL_STATUSES } from '$lib/statusPrefs.js';
  import { loadPref } from '$lib/prefs.js';
  import RecordCard from '$lib/components/RecordCard.svelte';
  import OperaIcon from '$lib/components/OperaIcon.svelte';
  // BatchEditModal 改为按需动态加载（见 openBatchEdit），不进首页关键路径。

  let records = $state([]);
  let categories = $state([]);
  let cities = $state([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let error = $state('');
  let total = $state(0);
  const PAGE_SIZE = 30;
  let offset = 0;
  let hasMore = $state(false);
  // 所有演出总数（不受筛选影响），来自 /api/stats.total_records。
  let allTotal = $state(0);
  let filters = $state({
    q: '', category: '', city: '', year: '', month: '',
    drama: '', zhezi: '', artist: '',
    channel: '', company: '',
    start: '', end: '',
    rating_min: '', price_min: '', price_max: '',
    status: '', exact: false,
    missing: []
  });

  // 可筛选的「缺失字段」清单。value 必须与后端 buildMissingPredicate 的 token 一致。
  // 用于数据治理：快速找出某项未填写的演出（例如未填分类、未传封面）。
  const MISSING_FIELDS = [
    { value: 'category', label: '分类' },
    { value: 'city', label: '城市' },
    { value: 'cover', label: '封面' },
    { value: 'rating', label: '评分' },
    { value: 'price', label: '票价' },
    { value: 'company', label: '剧团' },
    { value: 'channel', label: '渠道' },
    { value: 'artist', label: '演员' },
    { value: 'drama', label: '剧目' },
    { value: 'zhezi', label: '折子' },
    { value: 'coordinate', label: '坐标' },
    { value: 'remark', label: '备注' },
    { value: 'friends', label: '戏友' },
    { value: 'guest', label: '嘉宾' },
    { value: 'play', label: '剧目别名' },
    { value: 'seat', label: '座位' }
  ];
  const missingLabel = (v) => (MISSING_FIELDS.find((x) => x.value === v) || {}).label || v;

  // 状态标签（active_status: 0 正常 / 1 想看 / 2 已取消 / 3 未赴约）
  const STATUS_LABELS = { '0': '正常', '1': '想看', '2': '已取消', '3': '未赴约' };
  const statusLabel = (v) => STATUS_LABELS[v] || ('状态' + v);

  // 筛选面板的下拉候选（剧目/折子/演员/渠道/剧团）。剧目树较大（~36KB），
  // 仅首次打开筛选面板时才加载，避免污染首屏。
  let dramaList = $state([]);
  let zheziList = $state([]);
  let artistList = $state([]);
  let channels = $state([]);
  let companies = $state([]);
  let filterDataLoaded = false;
  async function loadFilterData() {
    if (filterDataLoaded) return;
    filterDataLoaded = true;
    try {
      const [tree, arts, chans, comps] = await Promise.all([
        api.getDramaTree(),
        api.listArtists(),
        api.getAutocomplete('channel'),
        api.getAutocomplete('company')
      ]);
      const d = [], z = [];
      for (const dr of tree || []) {
        d.push({ id: dr.id, name: dr.name });
        for (const zz of dr.zhezis || []) z.push({ id: zz.id, name: zz.name, dramaName: dr.name });
      }
      dramaList = d;
      zheziList = z;
      artistList = (arts || []).map((a) => ({ id: a.id, name: a.name }));
      channels = chans || [];
      companies = comps || [];
    } catch (e) { /* 候选缺失时不阻塞筛选面板 */ }
  }
  const dramaLabel = (id) => (dramaList.find((x) => x.id === id) || {}).name || '剧目';
  const artistLabel = (id) => (artistList.find((x) => x.id === id) || {}).name || '演员';
  let showFilter = $state(false);
  let showMissing = $state(false); // 缺失字段分组默认折叠（数据治理高级维度）
  let zheziNames = $state(new Map());
  let searchTimer;
  let searchComposing = false;
  let sentinelEl = $state(null);
  let sentinelObserver = null;

  // 定位到当前时间：锚点为最近已发生（含今天）的演出；全为未来演出时取最接近现在的
  let showJump = $state(false);
  let flashId = $state('');
  let flashTimer;

  const nowAnchorId = $derived.by(() => {
    if (!records.length) return '';
    const d = new Date();
    d.setHours(23, 59, 59, 999);
    const endOfToday = Math.floor(d.getTime() / 1000);
    for (const r of records) {
      if (r.date <= endOfToday) return r.id;
    }
    return records[records.length - 1].id;
  });

  function jumpToNow(smooth = true) {
    if (!nowAnchorId) return;
    const el = document.getElementById('rec-' + nowAnchorId);
    if (!el) return;
    el.scrollIntoView({ behavior: smooth ? 'smooth' : 'auto', block: 'center' });
    if (smooth) {
      flashId = nowAnchorId;
      clearTimeout(flashTimer);
      flashTimer = setTimeout(() => (flashId = ''), 1500);
    }
  }

  function onScroll() {
    showJump = window.scrollY > 320;
  }

  function autoJumpOnLoad() {
    if (loadPref('mujian:home_jump_now', false)) {
      tick().then(() => jumpToNow(false));
    }
  }

  // 批量操作状态
  let selectionMode = $state(false);
  let selectedIds = $state(new Set());
  let showBatchEdit = $state(false);
  let batchError = $state('');

  const allSelected = $derived(records.length > 0 && selectedIds.size === records.length);

  function toggleSelectMode() {
    selectionMode = !selectionMode;
    if (!selectionMode) {
      selectedIds.clear();
    }
  }

  function toggleSelect(id) {
    if (selectedIds.has(id)) {
      selectedIds.delete(id);
    } else {
      selectedIds.add(id);
    }
    selectedIds = new Set(selectedIds);
  }

  function toggleSelectAll() {
    if (allSelected) {
      selectedIds.clear();
    } else {
      selectedIds = new Set(records.map((r) => r.id));
    }
  }

  async function batchDelete() {
    if (selectedIds.size === 0) return;
    if (!confirm(`确定删除 ${selectedIds.size} 条记录？此操作不可恢复。`)) return;
    try {
      await api.batchDelete([...selectedIds]);
      selectedIds.clear();
      selectionMode = false;
      load();
    } catch (e) {
      batchError = e.message;
    }
  }

  let BatchEditModal = $state(null);
  let batchModalLoading = $state(false);

  async function openBatchEdit() {
    if (selectedIds.size === 0) return;
    // 680 行的批量编辑弹窗只在进入批量模式后才需要：按需加载，
    // 把它移出首页首屏关键路径（约 -12KB JS）。
    if (!BatchEditModal) {
      batchModalLoading = true;
      try {
        BatchEditModal = (await import('$lib/components/BatchEditModal.svelte')).default;
      } finally {
        batchModalLoading = false;
      }
    }
    showBatchEdit = true;
  }

  function onBatchSaved() {
    showBatchEdit = false;
    selectedIds.clear();
    selectionMode = false;
    load();
  }

  function zheziLabel(id) {
    const z = zheziNames.get(id);
    return z ? `${z.dramaName} · ${z.name}` : '折子';
  }

  let baseChips = $derived(
    [
      filters.q ? { k: 'q', label: `搜索：${filters.q}` } : null,
      filters.category ? { k: 'category', label: `分类：${filters.category}` } : null,
      filters.city ? { k: 'city', label: `城市：${filters.city}` } : null,
      filters.year ? { k: 'year', label: `年份：${filters.year}` } : null,
      filters.month ? { k: 'month', label: `月份：${filters.month}` } : null,
      filters.drama ? { k: 'drama', label: `剧目：${dramaLabel(filters.drama)}` } : null,
      filters.zhezi ? { k: 'zhezi', label: `折子：${zheziLabel(filters.zhezi)}` } : null,
      filters.artist ? { k: 'artist', label: `演员：${artistLabel(filters.artist)}` } : null,
      filters.channel ? { k: 'channel', label: `渠道：${filters.channel}` } : null,
      filters.company ? { k: 'company', label: `剧团：${filters.company}` } : null,
      filters.start ? { k: 'start', label: `起：${filters.start}` } : null,
      filters.end ? { k: 'end', label: `止：${filters.end}` } : null,
      filters.rating_min ? { k: 'rating_min', label: `评分≥${filters.rating_min}` } : null,
      (filters.price_min || filters.price_max) ? { k: 'price', label: `票价 ${filters.price_min || '0'}~${filters.price_max || '∞'}` } : null,
      filters.status ? { k: 'status', label: `状态：${statusLabel(filters.status)}` } : null,
      filters.exact ? { k: 'exact', label: '精确匹配' } : null
    ].filter(Boolean)
  );
  // 每个被勾选的「缺失字段」各占一个独立 chip，可单独取消。
  let missingChips = $derived(
    filters.missing.map((m) => ({ k: 'missing:' + m, label: `缺失：${missingLabel(m)}` }))
  );
  let activeChips = $derived([...baseChips, ...missingChips]);
  // 是否处于筛选状态（筛选面板的条件，或设置里的状态偏好未全选）。
  // 只有筛选时才显示「筛选总数 / 全部总数」。
  const statusPrefsNow = loadStatusFilter();
  let isFiltered = $derived(
    activeChips.length > 0 || statusPrefsNow.length < ALL_STATUSES.length
  );

  // 将当前筛选条件同步进 URL（不新增历史记录），使点开某条演出后「← 返回」
  // 能回到带筛选的首页，而不是未筛选页面。
  let urlReady = $state(false);

  function buildFilterQuery() {
    const params = new URLSearchParams();
    const keys = ['q', 'category', 'city', 'year', 'month', 'drama', 'zhezi', 'artist', 'channel', 'company', 'start', 'end', 'rating_min', 'price_min', 'price_max', 'status'];
    for (const k of keys) if (filters[k]) params.set(k, filters[k]);
    if (filters.exact) params.set('exact', '1');
    if (filters.missing.length) params.set('missing', filters.missing.join(','));
    return params.toString();
  }

  // 筛选变化后把状态写回地址栏；与当前 URL 相同则跳过，避免无意义写入与循环。
  $effect(() => {
    if (!urlReady) return;
    // 依赖：读取全部筛选项以触发更新
    const _ = [
      filters.q, filters.category, filters.city, filters.year, filters.month,
      filters.drama, filters.zhezi, filters.artist, filters.channel, filters.company,
      filters.start, filters.end, filters.rating_min, filters.price_min, filters.price_max,
      filters.status, filters.exact, filters.missing.join(',')
    ];
    const qs = buildFilterQuery();
    const url = qs ? `/?${qs}` : '/';
    const cur = location.pathname + location.search;
    if (url !== cur) history.replaceState(history.state, '', url);
  });

  // 合并筛选参数 + 分页参数
  function buildQuery(off = 0, limit = PAGE_SIZE) {
    const q = {};
    const keys = ['q', 'category', 'city', 'year', 'month', 'drama', 'zhezi', 'artist', 'channel', 'company', 'start', 'end', 'rating_min', 'price_min', 'price_max', 'status'];
    for (const k of keys) if (filters[k]) q[k] = filters[k];
    if (filters.exact) q.exact = '1';
    if (filters.missing.length) q.missing = filters.missing.join(',');
    // 状态偏好（设置里勾选要显示的状态）交给服务端过滤：这样 total 只统计
    // 用户真正会看到的记录，列表计数不再与滚动加载后的条数对不上。
    const statusPrefs = loadStatusFilter();
    if (statusPrefs.length && statusPrefs.length < ALL_STATUSES.length) {
      q.active_status = statusPrefs.join(',');
    }
    q.offset = off;
    q.limit = limit;
    return q;
  }

  // 请求序号：连续触发 load（防抖外还有各 select 的 onchange 直发）时，
  // 慢的旧响应返回会覆盖新筛选的结果，用序号丢弃过期响应。
  let listReqSeq = 0;

  async function load() {
    const seq = ++listReqSeq;
    loading = true;
    loadingMore = false;
    offset = 0;
    error = '';
    try {
      const [listRes, stats] = await Promise.all([
        api.listRecords(buildQuery(0, PAGE_SIZE)),
        api.getStats().catch(() => null) // 全部总数拿不到时只隐藏第二个数字
      ]);
      if (seq !== listReqSeq) return; // 已有更新的请求，丢弃本次响应
      records = listRes.records;
      total = listRes.total;
      if (stats) allTotal = stats.total_records ?? 0;
      hasMore = PAGE_SIZE < listRes.total;
    } catch (e) {
      if (seq === listReqSeq) error = e.message;
    } finally {
      if (seq === listReqSeq) loading = false;
    }
  }

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    const seq = ++listReqSeq;
    loadingMore = true;
    try {
      const { records: page } = await api.listRecords(buildQuery(offset + PAGE_SIZE, PAGE_SIZE));
      if (seq !== listReqSeq) return; // 期间筛选已变，旧页不得 append
      offset += PAGE_SIZE;
      records = [...records, ...page];
      hasMore = offset + PAGE_SIZE < total;
    } catch (e) {
      // 静默失败，保留已加载的
    } finally {
      if (seq === listReqSeq) loadingMore = false;
    }
  }

  async function loadMeta() {
    try {
      const [cats, cityList] = await Promise.all([
        api.listCategories(),
        api.getAutocomplete('city')
      ]);
      categories = cats;
      cities = cityList;
    } catch (e) { /* 非关键，忽略 */ }
  }

  // 剧目/折子树（36KB）只为把 zhezi id 解析成「剧目 · 折子」显示名，
  // 仅当 URL 带 ?zhezi= 筛选时才有用。此前无条件随首屏加载，
  // 是首屏关键路径上最大的一笔无效请求。
  let zheziNamesLoading = false;
  async function ensureZheziNames() {
    if (zheziNames.size || zheziNamesLoading) return;
    zheziNamesLoading = true;
    try {
      const tree = await api.getDramaTree();
      const m = new Map();
      for (const d of tree) for (const z of d.zhezis || []) m.set(z.id, { name: z.name, dramaName: d.name });
      zheziNames = m;
    } catch (e) { /* 显示名缺失时兜底为「折子」 */ }
    finally { zheziNamesLoading = false; }
  }

  function onSearchInput() {
    // 中文输入法组词中不触发搜索，等 compositionend 上屏后再查，
    // 避免拼音中间态被当作关键词查出无结果。
    if (searchComposing) return;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(load, 260);
  }

  function onSearchCompositionStart() {
    searchComposing = true;
  }

  function onSearchCompositionEnd() {
    searchComposing = false;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(load, 120);
  }

  function clearChip(k) {
    if (k.startsWith('missing:')) {
      const m = k.slice('missing:'.length);
      filters = { ...filters, missing: filters.missing.filter((x) => x !== m) };
      load();
      return;
    }
    if (k === 'price') {
      filters = { ...filters, price_min: '', price_max: '' };
      load();
      return;
    }
    if (k === 'exact') {
      filters = { ...filters, exact: false };
      load();
      return;
    }
    filters = { ...filters, [k]: '' };
    load();
  }

  function toggleMissing(v) {
    const has = filters.missing.includes(v);
    filters = {
      ...filters,
      missing: has ? filters.missing.filter((x) => x !== v) : [...filters.missing, v]
    };
    load();
  }

  function resetFilters() {
    filters = {
      q: '', category: '', city: '', year: '', month: '',
      drama: '', zhezi: '', artist: '',
      channel: '', company: '',
      start: '', end: '',
      rating_min: '', price_min: '', price_max: '',
      status: '', exact: false,
      missing: []
    };
    load();
  }

  function setupSentinelObserver() {
    if (sentinelObserver) return;
    sentinelObserver = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting && hasMore && !loading && !loadingMore) {
            loadMore();
          }
        }
      },
      { rootMargin: '400px 0px' }
    );
  }

  // sentinelEl 就绪后开始观察（列表渲染完才会有 sentinelEl）
  $effect(() => {
    if (sentinelEl) {
      setupSentinelObserver();
      sentinelObserver.disconnect();
      sentinelObserver.observe(sentinelEl);
    }
  });

  onMount(() => {
    const sp = new URLSearchParams($page.url.search);
    filters = {
      q: sp.get('q') || '',
      category: sp.get('category') || '',
      city: sp.get('city') || '',
      year: sp.get('year') || '',
      month: sp.get('month') || '',
      drama: sp.get('drama') || '',
      zhezi: sp.get('zhezi') || '',
      artist: sp.get('artist') || '',
      channel: sp.get('channel') || '',
      company: sp.get('company') || '',
      start: sp.get('start') || '',
      end: sp.get('end') || '',
      rating_min: sp.get('rating_min') || '',
      price_min: sp.get('price_min') || '',
      price_max: sp.get('price_max') || '',
      status: sp.get('status') || '',
      exact: sp.get('exact') === '1' || sp.get('exact') === 'true',
      missing: (sp.get('missing') || '').split(',').map((s) => s.trim()).filter(Boolean)
    };
    if (filters.missing.length) showMissing = true; // 携带缺失筛选进入时自动展开该分组
    urlReady = true;
    loadMeta();
    if (filters.zhezi) ensureZheziNames();
    load().then(autoJumpOnLoad);
  });

  // 组件销毁时清理观察器与防抖/提示定时器，避免残留监听。
  onDestroy(() => {
    if (sentinelObserver) {
      sentinelObserver.disconnect();
      sentinelObserver = null;
    }
    clearTimeout(searchTimer);
    clearTimeout(flashTimer);
  });
</script>
<svelte:head><title>演出 - 幕间</title></svelte:head>
<svelte:window onscroll={onScroll} />


<div class="home fade-up">
  <div class="hero">
    <div class="search-wrap">
      <span class="search-ico">⌕</span>
      <input
        class="search"
        placeholder="搜索演出名称、演员、城市、剧团、备注…"
        bind:value={filters.q}
        oninput={onSearchInput}
        oncompositionstart={onSearchCompositionStart}
        oncompositionend={onSearchCompositionEnd}
      />
      {#if filters.q}<button class="search-clear" onclick={() => clearChip('q')}>✕</button>{/if}
    </div>
      <div class="action-row">
        <button
          type="button"
          class="btn ghost filter-toggle"
          class:active={activeChips.length > 0}
          onclick={() => { showFilter = !showFilter; if (showFilter) loadFilterData(); }}
          aria-expanded={showFilter}
          aria-haspopup="dialog"
        >
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M3 5h18M6 12h12M10 19h4" />
          </svg>
          <span>筛选</span>
          {#if activeChips.length}<span class="filter-count">{activeChips.length}</span>{/if}
        </button>
        {#if selectionMode}
          <button class="btn" onclick={toggleSelectMode}>完成</button>
        {:else}
          <button class="btn ghost" onclick={toggleSelectMode}>批量</button>
          <a class="btn primary" href="/records/new">＋ 新建记录</a>
        {/if}
      </div>
  </div>

  {#if selectionMode}
    <div class="batch-bar card">
      <label class="select-all">
        <input type="checkbox" checked={allSelected} onchange={toggleSelectAll} />
        <span>{allSelected ? '取消全选' : '全选'}</span>
      </label>
      <span class="batch-count">已选 {selectedIds.size} 条</span>
      <div class="batch-actions">
        <button class="btn primary sm" onclick={openBatchEdit} disabled={selectedIds.size === 0}>批量编辑</button>
        <button class="btn danger sm" onclick={batchDelete} disabled={selectedIds.size === 0}>批量删除</button>
      </div>
    </div>
  {/if}

  {#if activeChips.length}
    <div class="chips">
      {#each activeChips as chip}
        <button class="chip" onclick={() => clearChip(chip.k)}>{chip.label} ✕</button>
      {/each}
    </div>
  {/if}

  <div class="count-row">
    <h2>记录 <span class="num">{total}</span>{#if isFiltered && allTotal > 0} / {allTotal}{/if}</h2>
  </div>

      {#if batchError}
        <div class="banner error">⚠ {batchError}</div>
      {/if}

      {#if loading}
        <div class="grid">
          {#each Array(8) as _}
            <div class="skel-card"><div class="skeleton skel-cover"></div><div class="skeleton skel-line"></div><div class="skeleton skel-line short"></div></div>
          {/each}
        </div>
      {:else if records.length === 0}
        <div class="empty card">
          <div class="ico"><OperaIcon size={44} /></div>
          <div class="t">{activeChips.length ? '没有符合条件的记录' : '还没有记录'}</div>
          <div class="h">{activeChips.length ? '试试调整筛选条件，或清除全部筛选' : '前往「导入」上传 recordlive_export 的 data.json，或点击右上角新建第一条记录'}</div>
          {#if activeChips.length}<button class="btn sm" onclick={resetFilters}>清除筛选</button>{/if}
        </div>
      {:else}
        <div class="grid stagger">
          {#each records as r (r.id)}
            <div
              id={'rec-' + r.id}
              class="record-card-wrapper"
              class:select-mode={selectionMode}
              class:selected={selectedIds.has(r.id)}
              class:flash={flashId === r.id}
              role={selectionMode ? 'button' : undefined}
              tabindex={selectionMode ? 0 : undefined}
              aria-pressed={selectionMode ? selectedIds.has(r.id) : undefined}
              onclick={(e) => { if (selectionMode) { e.preventDefault(); toggleSelect(r.id); } }}
              onkeydown={(e) => { if (selectionMode && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); toggleSelect(r.id); } }}
            >
              <RecordCard record={r} selectionMode={selectionMode} selected={selectedIds.has(r.id)} />
            </div>
          {/each}
        </div>
        <!-- 无限滚动哨兵 -->
        {#if hasMore}
          <div bind:this={sentinelEl} class="sentinel" aria-hidden="true">
            {#if loadingMore}
              <div class="sentinel-loader"><span></span><span></span><span></span></div>
            {/if}
          </div>
        {/if}
      {/if}
</div>

{#if showFilter}
  <div class="filter-mask" onclick={() => (showFilter = false)} transition:fade={{ duration: 140 }} aria-hidden="true"></div>
  <div class="filter-panel card" role="dialog" aria-modal="true" aria-label="筛选选项" transition:fly={{ y: 40, duration: 200 }}>
    <div class="filter-panel-head">
      <span class="filter-panel-title">筛选</span>
      <button type="button" class="filter-close" onclick={() => (showFilter = false)} aria-label="关闭筛选">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <path d="M6 6l12 12M18 6L6 18" />
        </svg>
      </button>
    </div>
    <div class="filter-fields">
      <label class="filter-field">
        <span class="filter-label">分类</span>
        <select class="input" bind:value={filters.category} onchange={load}>
          <option value="">全部分类</option>
          {#each categories as c}<option value={c.name}>{c.name}</option>{/each}
        </select>
      </label>
      <label class="filter-field">
        <span class="filter-label">城市</span>
        <select class="input" bind:value={filters.city} onchange={load}>
          <option value="">全部城市</option>
          {#each cities as c}<option value={c}>{c}</option>{/each}
        </select>
      </label>
      <label class="filter-field">
        <span class="filter-label">年份</span>
        <input class="input" type="number" placeholder="年份" bind:value={filters.year} onchange={load} />
      </label>
      <label class="filter-field">
        <span class="filter-label">月份</span>
        <select class="input" bind:value={filters.month} onchange={load}>
          <option value="">月份</option>
          {#each Array(12) as _, i}<option value={i + 1}>{i + 1} 月</option>{/each}
        </select>
      </label>
    </div>
    <div class="filter-section">
      <button type="button" class="filter-section-title collapsible" onclick={() => (showMissing = !showMissing)} aria-expanded={showMissing}>
        <span>缺失字段（数据治理）</span>
        {#if filters.missing.length}<span class="missing-count">{filters.missing.length}</span>{/if}
        <svg class="chev" class:open={showMissing} viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6"/></svg>
      </button>
      {#if showMissing}
        <div class="missing-grid">
          {#each MISSING_FIELDS as f}
            <label class="missing-opt">
              <input type="checkbox" checked={filters.missing.includes(f.value)} onchange={() => toggleMissing(f.value)} />
              <span>{f.label}</span>
            </label>
          {/each}
        </div>
      {/if}
    </div>

    <div class="filter-section">
      <span class="filter-section-title">剧目 / 折子 / 演员</span>
      <div class="filter-fields">
        <label class="filter-field">
          <span class="filter-label">剧目</span>
          <select class="input" bind:value={filters.drama} onchange={load}>
            <option value="">全部剧目</option>
            {#each dramaList as d}<option value={d.id}>{d.name}</option>{/each}
          </select>
        </label>
        <label class="filter-field">
          <span class="filter-label">折子</span>
          <select class="input" bind:value={filters.zhezi} onchange={load}>
            <option value="">全部折子</option>
            {#each zheziList as z}<option value={z.id}>{z.dramaName} · {z.name}</option>{/each}
          </select>
        </label>
        <label class="filter-field">
          <span class="filter-label">演员</span>
          <select class="input" bind:value={filters.artist} onchange={load}>
            <option value="">全部演员</option>
            {#each artistList as a}<option value={a.id}>{a.name}</option>{/each}
          </select>
        </label>
      </div>
    </div>

    <div class="filter-section">
      <span class="filter-section-title">时间区间（起 / 止）</span>
      <div class="filter-hint">可只填其中一项：仅起始=该日及之后，仅结束=该日及之前。</div>
      <div class="filter-fields">
        <label class="filter-field">
          <span class="filter-label">起始日期</span>
          <input class="input" type="date" bind:value={filters.start} onchange={load} />
        </label>
        <label class="filter-field">
          <span class="filter-label">结束日期</span>
          <input class="input" type="date" bind:value={filters.end} onchange={load} />
        </label>
      </div>
    </div>

    <div class="filter-section">
      <span class="filter-section-title">渠道 / 剧团</span>
      <div class="filter-fields">
        <label class="filter-field">
          <span class="filter-label">渠道</span>
          <input class="input" list="flt-channel" placeholder="渠道" bind:value={filters.channel} onchange={load} />
          <datalist id="flt-channel">{#each channels as c}<option value={c}>{c}</option>{/each}</datalist>
        </label>
        <label class="filter-field">
          <span class="filter-label">剧团</span>
          <input class="input" list="flt-company" placeholder="剧团" bind:value={filters.company} onchange={load} />
          <datalist id="flt-company">{#each companies as c}<option value={c}>{c}</option>{/each}</datalist>
        </label>
      </div>
    </div>

    <div class="filter-section">
      <span class="filter-section-title">评分 / 票价</span>
      <div class="filter-fields">
        <label class="filter-field">
          <span class="filter-label">评分 ≥</span>
          <input class="input" type="number" min="0" max="10" step="1" placeholder="评分下限" bind:value={filters.rating_min} onchange={load} />
        </label>
        <div class="filter-field">
          <span class="filter-label">票价区间（¥）</span>
          <div class="range-row">
            <input class="input" type="number" min="0" step="1" placeholder="最低" bind:value={filters.price_min} onchange={load} />
            <span class="range-sep">~</span>
            <input class="input" type="number" min="0" step="1" placeholder="最高" bind:value={filters.price_max} onchange={load} />
          </div>
        </div>
      </div>
    </div>

    <div class="filter-section">
      <span class="filter-section-title">状态 / 匹配方式</span>
      <div class="filter-fields">
        <label class="filter-field">
          <span class="filter-label">状态</span>
          <select class="input" bind:value={filters.status} onchange={load}>
            <option value="">全部状态</option>
            <option value="0">正常</option>
            <option value="1">想看</option>
            <option value="2">已取消</option>
            <option value="3">未赴约</option>
          </select>
        </label>
        <label class="missing-opt exact-opt">
          <input type="checkbox" bind:checked={filters.exact} onchange={load} />
          <span>关键词精确匹配（按名称全等，非模糊）</span>
        </label>
      </div>
    </div>

    <div class="filter-panel-actions">
      <button class="btn ghost" onclick={resetFilters}>清除全部</button>
      <button class="btn primary" onclick={() => (showFilter = false)}>完成</button>
    </div>
  </div>
{/if}

{#if showJump && !loading && records.length && nowAnchorId}
  <button class="jump-now" onclick={() => jumpToNow()} title="定位到当前时间" aria-label="定位到当前时间">
    <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="7" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3" />
    </svg>
  </button>
{/if}

{#if showBatchEdit && BatchEditModal}
  <BatchEditModal
    selectedIds={[...selectedIds]}
    records={records}
    onClose={() => showBatchEdit = false}
    onSaved={onBatchSaved}
  />
{/if}

<style>
  .home { display: flex; flex-direction: column; gap: 14px; }

  .hero { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
  .action-row { display: flex; gap: 8px; align-items: center; flex-shrink: 0; flex-wrap: wrap; }
  .action-row .btn { white-space: nowrap; }
  .search-wrap {
    position: relative;
    flex: 1 1 240px;
    min-width: 0;
    display: flex;
    align-items: center;
  }
  .search-ico {
    position: absolute;
    left: 14px;
    font-size: 18px;
    color: var(--text-3);
    pointer-events: none;
  }
  .search {
    width: 100%;
    padding: 11px 40px 11px 40px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    font-size: 14.5px;
    box-shadow: var(--shadow-xs);
    transition: border-color var(--t-fast) var(--ease), box-shadow var(--t-fast) var(--ease);
  }
  .search:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft), var(--shadow-sm);
  }
  .search::placeholder { color: var(--text-3); }
  .search-clear {
    position: absolute;
    right: 12px;
    border: none;
    background: var(--surface-3);
    color: var(--text-muted);
    width: 22px;
    height: 22px;
    border-radius: 50%;
    font-size: 11px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .search-clear:hover { background: var(--accent-soft); color: var(--accent); }

  .filter-toggle { display: inline-flex; align-items: center; gap: 6px; }
  .filter-toggle.active { color: var(--accent); border-color: var(--accent); background: var(--accent-soft); }
  .filter-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 999px;
    background: var(--accent);
    color: #fff;
    font-size: 11px;
    font-weight: 600;
    line-height: 1;
  }

  /* 筛选弹出层（移动端/桌面端均为底部抽屉式） */
  .filter-mask {
    position: fixed;
    inset: 0;
    z-index: 60;
    border: none;
    padding: 0;
    background: rgba(0, 0, 0, 0.42);
    cursor: default;
  }
  .filter-panel {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    margin: 0 auto;
    z-index: 61;
    width: min(560px, 100%);
    max-height: 80vh;
    overflow-y: auto;
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
    padding: 16px 16px calc(20px + env(safe-area-inset-bottom, 0px));
    box-shadow: var(--shadow-lg);
  }
  .filter-panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }
  .filter-panel-title { font-family: var(--font-serif); font-size: 17px; font-weight: 600; }
  .filter-close {
    border: none;
    background: none;
    color: var(--text-muted);
    width: 34px;
    height: 34px;
    border-radius: 50%;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .filter-close:hover { background: var(--surface-3); color: var(--text); }
  .filter-fields {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 12px;
  }
  .filter-field { display: flex; flex-direction: column; gap: 6px; margin: 0; }
  .filter-label { font-size: 13px; font-weight: 500; color: var(--text-2); }
  .filter-field .input { width: 100%; min-width: 0; }

  /* 缺失字段分组 */
  .filter-section {
    margin-top: 14px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }
  .filter-section-title {
    display: block;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-2);
    margin-bottom: 8px;
  }
  .filter-hint {
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-3, #9aa0a6);
    margin: -4px 0 10px;
  }
  .missing-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
    gap: 6px 10px;
  }
  .missing-opt {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    line-height: 1.2;
    color: var(--text);
    cursor: pointer;
    user-select: none;
    white-space: nowrap;
  }
  .missing-opt input[type="checkbox"] {
    width: 15px;
    height: 15px;
    margin: 0;
    accent-color: var(--accent);
    flex-shrink: 0;
  }
  .exact-opt {
    align-self: end;
    padding-bottom: 2px;
    white-space: normal;
  }
  .range-row { display: flex; align-items: center; gap: 6px; }
  .range-row .input { width: 100%; min-width: 0; }
  .range-sep { color: var(--text-3); font-size: 13px; flex-shrink: 0; }
  .filter-panel-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
    margin-top: 16px;
  }

  /* 桌面端：加宽筛选面板并放宽网格列数，承载更多维度而不显拥挤 */
  @media (min-width: 640px) {
    .filter-panel {
      width: min(860px, 94vw);
      max-height: 88vh;
      border-top-left-radius: 16px;
      border-top-right-radius: 16px;
    }
    .filter-fields { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  }

  /* 缺失字段分组可折叠 */
  .collapsible {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    margin: 0 0 8px;
    padding: 7px 10px;
    border: none;
    border-radius: 9px;
    background: var(--surface-3);
    color: var(--text-2);
    font: inherit;
    font-size: 13px;
    font-weight: 600;
    text-align: left;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .collapsible:hover { background: var(--accent-soft); color: var(--accent); }
  .collapsible > span:first-child { flex: 1; min-width: 0; }
  .missing-count {
    flex-shrink: 0;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 999px;
    background: var(--accent);
    color: #fff;
    font-size: 11px;
    font-weight: 600;
    line-height: 18px;
    text-align: center;
  }
  .chev { flex-shrink: 0; transition: transform var(--t-fast) var(--ease); transform: rotate(-90deg); }
  .chev.open { transform: rotate(0deg); }

  .chips { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip {
    padding: 4px 12px;
    border-radius: 999px;
    border: 1px solid var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 12.5px;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .chip:hover { background: var(--accent); color: #fff; }

  .count-row h2 { font-size: 20px; margin: 4px 0 0; }
  .count-row .num { color: var(--accent); font-family: var(--font-sans); font-weight: 700; font-size: 18px; margin-left: 4px; }

  .batch-bar {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 12px 16px;
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .select-all {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    font-size: 13.5px;
    line-height: 1;
    color: var(--text);
    user-select: none;
    white-space: nowrap;
    margin: 0;
  }
  .select-all input[type="checkbox"] {
    width: 16px;
    height: 16px;
    margin: 0;
    accent-color: var(--accent);
  }
  .batch-count {
    font-size: 13px;
    line-height: 1;
    color: var(--text-2);
    flex: 1;
    white-space: nowrap;
  }
  .batch-actions { display: flex; gap: 8px; flex-shrink: 0; }

  .record-card-wrapper {
    position: relative;
    display: block;
  }
  .record-card-wrapper.flash::after {
    content: '';
    position: absolute;
    inset: 0;
    border: 2px solid var(--accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 0 0 6px var(--accent-soft);
    pointer-events: none;
    animation: flash-fade 1.5s var(--ease) forwards;
  }
  @keyframes flash-fade {
    from { opacity: 1; }
    to { opacity: 0; }
  }

  .jump-now {
    position: fixed;
    right: 18px;
    bottom: calc(18px + env(safe-area-inset-bottom, 0px));
    width: 44px;
    height: 44px;
    border-radius: 50%;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--accent);
    font-size: 22px;
    line-height: 1;
    box-shadow: var(--shadow-md);
    cursor: pointer;
    z-index: 40;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease);
  }
  .jump-now:hover {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
  }
  .record-card-wrapper.select-mode {
    cursor: pointer;
  }
  .record-card-wrapper[role='button']:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
    border-radius: var(--radius-lg);
  }
  /* 批量模式下整卡可点选，悬停不再放大海报 */
  .record-card-wrapper.select-mode:hover .cover img { transform: none; }
  .record-card-wrapper.selected::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    right: 0;
    bottom: 0;
    border: 2px solid var(--accent);
    border-radius: var(--radius-lg);
    pointer-events: none;
    box-sizing: border-box;
  }
  /* 选择框已移入 RecordCard 封面右下角（避开左上剧种角标/右上评分角标），样式见组件内部 */

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(172px, 1fr));
    gap: 14px;
  }

  .skel-card { display: flex; flex-direction: column; gap: 8px; }
  .skel-cover { aspect-ratio: 3 / 4; border-radius: var(--radius-lg); }
  .skel-line { height: 14px; }
  .skel-line.short { width: 60%; }

  @media (max-width: 560px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
    /* 搜索框整行，筛选 / 批量 / 新建 同行水平排列 */
    .search-wrap { flex: 1 1 100%; }
    .action-row { flex: 1 1 100%; }
    .action-row .btn { flex: 1 1 auto; justify-content: center; }
  }

  /* 无限滚动哨兵 */
  .sentinel {
    grid-column: 1 / -1;
    display: flex;
    justify-content: center;
    align-items: center;
    min-height: 48px;
    padding: 16px 0 32px;
  }
  .sentinel-loader {
    display: inline-flex;
    gap: 5px;
    align-items: center;
  }
  .sentinel-loader span {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    opacity: 0.3;
    animation: sentinel-pulse 1.2s ease-in-out infinite;
  }
  .sentinel-loader span:nth-child(2) { animation-delay: 0.15s; }
  .sentinel-loader span:nth-child(3) { animation-delay: 0.3s; }

  @keyframes sentinel-pulse {
    0%, 80%, 100% { opacity: 0.25; transform: scale(0.85); }
    40% { opacity: 1; transform: scale(1.1); }
  }
</style>
