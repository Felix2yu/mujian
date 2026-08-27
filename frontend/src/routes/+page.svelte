<script>
  import { onMount, tick } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { page } from '$app/stores';
  import { api } from '$lib/api.js';
  import { loadStatusFilter } from '$lib/statusPrefs.js';
  import { loadPref } from '$lib/prefs.js';
  import RecordCard from '$lib/components/RecordCard.svelte';
  import BatchEditModal from '$lib/components/BatchEditModal.svelte';
  import OperaIcon from '$lib/components/OperaIcon.svelte';

  let records = $state([]);
  let categories = $state([]);
  let cities = $state([]);
  let loading = $state(true);
  let error = $state('');
  let filters = $state({ q: '', category: '', city: '', year: '', month: '', drama: '', zhezi: '' });
  let showFilter = $state(false);
  let zheziNames = $state(new Map());
  let searchTimer;
  let searchComposing = false;

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

  function openBatchEdit() {
    if (selectedIds.size === 0) return;
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

  let activeChips = $derived(
    [
      filters.q ? { k: 'q', label: `搜索：${filters.q}` } : null,
      filters.category ? { k: 'category', label: `分类：${filters.category}` } : null,
      filters.city ? { k: 'city', label: `城市：${filters.city}` } : null,
      filters.year ? { k: 'year', label: `年份：${filters.year}` } : null,
      filters.month ? { k: 'month', label: `月份：${filters.month}` } : null,
      filters.drama ? { k: 'drama', label: '剧目' } : null,
      filters.zhezi ? { k: 'zhezi', label: `折子：${zheziLabel(filters.zhezi)}` } : null
    ].filter(Boolean)
  );

  // 将当前筛选条件同步进 URL（不新增历史记录），使点开某条演出后「← 返回」
  // 能回到带筛选的首页，而不是未筛选页面。
  let urlReady = $state(false);

  function buildFilterQuery() {
    const params = new URLSearchParams();
    if (filters.q) params.set('q', filters.q);
    if (filters.category) params.set('category', filters.category);
    if (filters.city) params.set('city', filters.city);
    if (filters.year) params.set('year', filters.year);
    if (filters.month) params.set('month', filters.month);
    if (filters.drama) params.set('drama', filters.drama);
    if (filters.zhezi) params.set('zhezi', filters.zhezi);
    return params.toString();
  }

  // 筛选变化后把状态写回地址栏；与当前 URL 相同则跳过，避免无意义写入与循环。
  $effect(() => {
    if (!urlReady) return;
    // 依赖：读取全部筛选项以触发更新
    const _ = [filters.q, filters.category, filters.city, filters.year, filters.month, filters.drama, filters.zhezi];
    const qs = buildFilterQuery();
    const url = qs ? `/?${qs}` : '/';
    const cur = location.pathname + location.search;
    if (url !== cur) history.replaceState(history.state, '', url);
  });

  async function load() {
    loading = true;
    error = '';
    try {
      const all = await api.listRecords(filters);
      const visible = new Set(loadStatusFilter());
      records = all.filter((r) => visible.has(r.active_status));
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function loadMeta() {
    try {
      const [cats, cityList, tree] = await Promise.all([
        api.listCategories(),
        api.getAutocomplete('city'),
        api.getDramaTree().catch(() => [])
      ]);
      categories = cats;
      cities = cityList;
      const m = new Map();
      for (const d of tree) for (const z of d.zhezis || []) m.set(z.id, { name: z.name, dramaName: d.name });
      zheziNames = m;
    } catch (e) { /* 非关键，忽略 */ }
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
    filters = { ...filters, [k]: '' };
    load();
  }

  function resetFilters() {
    filters = { q: '', category: '', city: '', year: '', month: '', drama: '', zhezi: '' };
    load();
  }

  onMount(() => {
    const sp = new URLSearchParams($page.url.search);
    filters = {
      q: sp.get('q') || '',
      category: sp.get('category') || '',
      city: sp.get('city') || '',
      year: sp.get('year') || '',
      month: sp.get('month') || '',
      drama: sp.get('drama') || '',
      zhezi: sp.get('zhezi') || ''
    };
    urlReady = true;
    loadMeta();
    load().then(autoJumpOnLoad);
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
          onclick={() => (showFilter = !showFilter)}
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
      <div class="filter-panel-actions">
        <button class="btn ghost" onclick={resetFilters}>清除全部</button>
        <button class="btn primary" onclick={() => (showFilter = false)}>完成</button>
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
    <h2>记录 <span class="num">{records.length}</span></h2>
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
      {/if}
</div>

{#if showJump && !loading && records.length && nowAnchorId}
  <button class="jump-now" onclick={() => jumpToNow()} title="定位到当前时间" aria-label="定位到当前时间">
    <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="7" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3" />
    </svg>
  </button>
{/if}

{#if showBatchEdit}
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
    transition: all var(--t-fast) var(--ease);
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
    transition: all var(--t-fast) var(--ease);
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
    left: 50%;
    bottom: 0;
    transform: translateX(-50%);
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
    transition: all var(--t-fast) var(--ease);
  }
  .filter-close:hover { background: var(--surface-3); color: var(--text); }
  .filter-fields {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .filter-field { display: flex; flex-direction: column; gap: 6px; margin: 0; }
  .filter-label { font-size: 13px; font-weight: 500; color: var(--text-2); }
  .filter-field .input { width: 100%; }
  .filter-panel-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
    margin-top: 16px;
  }

  .chips { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip {
    padding: 4px 12px;
    border-radius: 999px;
    border: 1px solid var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 12.5px;
    cursor: pointer;
    transition: all var(--t-fast) var(--ease);
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
    transition: all var(--t-fast) var(--ease);
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
</style>
