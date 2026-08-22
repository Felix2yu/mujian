<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api } from '$lib/api.js';
  import RecordCard from '$lib/components/RecordCard.svelte';
  import BatchEditModal from '$lib/components/BatchEditModal.svelte';

  let records = $state([]);
  let categories = $state([]);
  let cities = $state([]);
  let loading = $state(true);
  let error = $state('');
  let filters = $state({ q: '', category: '', city: '', year: '', month: '', drama: '', zhezi: '' });
  let zheziNames = $state(new Map());
  let searchTimer;

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

  async function load() {
    loading = true;
    error = '';
    try {
      records = await api.listRecords(filters);
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
    clearTimeout(searchTimer);
    searchTimer = setTimeout(load, 260);
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
    loadMeta();
    load();
  });
</script>

<div class="home fade-up">
  <div class="hero">
    <div class="search-wrap">
      <span class="search-ico">⌕</span>
      <input
        class="search"
        placeholder="搜索演出名称、演员、城市、剧团、备注…"
        bind:value={filters.q}
        oninput={onSearchInput}
      />
      {#if filters.q}<button class="search-clear" onclick={() => clearChip('q')}>✕</button>{/if}
    </div>
    {#if selectionMode}
      <button class="btn" onclick={toggleSelectMode}>完成</button>
    {:else}
      <button class="btn ghost" onclick={toggleSelectMode}>批量</button>
      <a class="btn primary" href="/records/new">＋ 新建记录</a>
    {/if}
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

  <div class="filter-bar card">
    <select class="input" bind:value={filters.category} onchange={load}>
      <option value="">全部分类</option>
      {#each categories as c}<option value={c.name}>{c.name}</option>{/each}
    </select>
    <select class="input" bind:value={filters.city} onchange={load}>
      <option value="">全部城市</option>
      {#each cities as c}<option value={c}>{c}</option>{/each}
    </select>
    <input class="input" type="number" placeholder="年份" bind:value={filters.year} onchange={load} />
    <select class="input" bind:value={filters.month} onchange={load}>
      <option value="">月份</option>
      {#each Array(12) as _, i}<option value={i + 1}>{i + 1} 月</option>{/each}
    </select>
    {#if activeChips.length}
      <button class="btn ghost sm" onclick={resetFilters}>清除全部</button>
    {/if}
  </div>

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
          <div class="ico">🎭</div>
          <div class="t">{activeChips.length ? '没有符合条件的记录' : '还没有记录'}</div>
          <div class="h">{activeChips.length ? '试试调整筛选条件，或清除全部筛选' : '前往「导入」上传 recordlive_export 的 data.json，或点击右上角新建第一条记录'}</div>
          {#if activeChips.length}<button class="btn sm" onclick={resetFilters}>清除筛选</button>{/if}
        </div>
      {:else}
        <div class="grid stagger">
          {#each records as r (r.id)}
            <div
              class="record-card-wrapper"
              class:select-mode={selectionMode}
              class:selected={selectedIds.has(r.id)}
              role={selectionMode ? 'button' : undefined}
              tabindex={selectionMode ? 0 : undefined}
              aria-pressed={selectionMode ? selectedIds.has(r.id) : undefined}
              onclick={(e) => { if (selectionMode) { e.preventDefault(); toggleSelect(r.id); } }}
              onkeydown={(e) => { if (selectionMode && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); toggleSelect(r.id); } }}
            >
              {#if selectionMode}
                <span class="record-check" class:checked={selectedIds.has(r.id)} aria-hidden="true">
                  {#if selectedIds.has(r.id)}✓{/if}
                </span>
              {/if}
              <RecordCard record={r} />
            </div>
          {/each}
        </div>
      {/if}
</div>

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

  .hero { display: flex; gap: 10px; align-items: center; }
  .search-wrap {
    position: relative;
    flex: 1;
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

  .filter-bar {
    display: flex;
    gap: 8px;
    padding: 10px;
    align-items: center;
    flex-wrap: wrap;
  }
  .filter-bar .input { width: auto; flex: 1 1 130px; padding: 7px 10px; font-size: 13.5px; }

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
  .record-card-wrapper.select-mode {
    padding-left: 36px;
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
  .record-check {
    position: absolute;
    left: 8px;
    top: 8px;
    z-index: 10;
    width: 20px;
    height: 20px;
    border-radius: 6px;
    background: var(--surface);
    border: 2px solid var(--border-strong, #c9c2b8);
    box-shadow: var(--shadow-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    color: #fff;
    font-size: 13px;
    font-weight: 700;
    line-height: 1;
  }
  .record-check.checked {
    background: var(--accent);
    border-color: var(--accent);
  }

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
    .hero { flex-direction: column; align-items: stretch; }
  }
</style>
