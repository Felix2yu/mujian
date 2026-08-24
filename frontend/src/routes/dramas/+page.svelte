<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import CategoryTags from '$lib/components/CategoryTags.svelte';

  let dramas = $state([]);
  let categories = $state([]);
  let loading = $state(true);
  let error = $state('');
  let adding = $state(false);
  let form = $state({ name: '', categoryNames: [], remark: '' });

  // 筛选与排序：默认按演出数降序；「手动排序」即拖拽顺序（后端 sort_order）
  let filterQuery = $state('');
  let filterCat = $state('');
  let sortBy = $state('records');

  const allCats = $derived.by(() => {
    const set = new Set();
    for (const d of dramas) for (const c of d.categoryNames || []) set.add(c);
    return [...set].sort((a, b) => a.localeCompare(b, 'zh'));
  });

  const visibleDramas = $derived.by(() => {
    let list = dramas;
    const q = filterQuery.trim().toLowerCase();
    if (q) list = list.filter((d) => (d.name || '').toLowerCase().includes(q) || (d.remark || '').toLowerCase().includes(q));
    if (filterCat) list = list.filter((d) => d.categoryNames?.includes(filterCat));
    if (sortBy === 'records') return [...list].sort((a, b) => b.recordCount - a.recordCount || a.name.localeCompare(b.name, 'zh'));
    if (sortBy === 'zhezis') return [...list].sort((a, b) => b.zheziCount - a.zheziCount || a.name.localeCompare(b.name, 'zh'));
    if (sortBy === 'name') return [...list].sort((a, b) => a.name.localeCompare(b.name, 'zh'));
    return list; // manual：后端已按 sort_order 排序
  });

  async function load() {
    loading = true;
    error = '';
    try {
      [dramas, categories] = await Promise.all([api.listDramas(), api.listCategories().catch(() => [])]);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function add() {
    const name = form.name.trim();
    if (!name || adding) return;
    adding = true;
    error = '';
    try {
      const d = await api.createDrama({ name, categoryNames: form.categoryNames.slice(), remark: form.remark.trim() });
      location.href = `/dramas/${d.id}`;
    } catch (e) {
      error = e.message;
      adding = false;
    }
  }

  async function remove(id, name) {
    if (!confirm(`删除剧目「${name}」？其下所有折子也会一并删除，演出记录不受影响。`)) return;
    error = '';
    try {
      await api.deleteDrama(id);
      await load();
    } catch (e) {
      error = e.message;
    }
  }

  // 拖拽排序状态（与折子页一致）
  let dragIdx = $state(-1);
  let overIdx = $state(-1);

  function onDragStart(i) {
    dragIdx = i;
  }

  function onDrop(targetIdx) {
    if (dragIdx < 0 || dragIdx === targetIdx) {
      resetDrag();
      return;
    }
    // 以当前可见顺序为基础重排；提交完整顺序后自动切到「手动排序」视图。
    // 筛选状态下拖拽：未显示的剧目追加在尾部，保持相对顺序不变。
    const next = visibleDramas.slice();
    const [moved] = next.splice(dragIdx, 1);
    next.splice(targetIdx, 0, moved);
    resetDrag();
    error = '';
    const shown = new Set(next.map((d) => d.id));
    const full = [...next, ...dramas.filter((d) => !shown.has(d.id))];
    api.reorderDramas(full.map((x) => x.id))
      .then(() => {
        sortBy = 'manual';
        const order = new Map(full.map((d, i) => [d.id, i]));
        dramas = [...dramas].sort((a, b) => (order.get(a.id) ?? 0) - (order.get(b.id) ?? 0));
      })
      .catch((e) => (error = e.message));
  }

  function resetDrag() {
    dragIdx = -1;
    overIdx = -1;
  }

  onMount(load);
</script>
<svelte:head><title>剧目 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>剧目</h1>
    <p class="sub">管理剧目档案与折子。点击剧目进入详情，可添加、排序并设置折子别名；拖动卡片可调整剧目显示顺序</p>
  </div>

  <div class="card add-bar">
    <input class="input grow" placeholder="剧目名称（必填），如：牡丹亭" bind:value={form.name} onkeydown={(e) => e.key === 'Enter' && add()} />
    <div class="grow">
      <CategoryTags bind:values={form.categoryNames} {categories} placeholder="剧种（可多个，可留空自动统计）" />
    </div>
    <input class="input grow" placeholder="备注（可选）" bind:value={form.remark} />
    <button class="btn primary" onclick={add} disabled={adding || !form.name.trim()}>{adding ? '创建中…' : '＋ 创建剧目'}</button>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  {#if loading}
    <div class="grid">
      {#each Array(6) as _}<div class="skeleton" style="height: 96px;"></div>{/each}
    </div>
  {:else if dramas.length === 0}
    <div class="empty card">
      <div class="ico">戏</div>
      <div class="t">还没有剧目</div>
      <div class="h">在上方输入剧目名称创建第一个档案</div>
    </div>
  {:else}
    <div class="card filter-bar">
      <input class="input grow" placeholder="🔍 搜索剧目名称 / 备注…" bind:value={filterQuery} />
      <select class="input" bind:value={filterCat}>
        <option value="">全部剧种</option>
        {#each allCats as c}<option value={c}>{c}</option>{/each}
      </select>
      <select class="input" bind:value={sortBy} title="选择「手动排序」后可拖动卡片调整顺序">
        <option value="records">按演出数</option>
        <option value="zhezis">按折子数</option>
        <option value="name">按名称</option>
        <option value="manual">手动排序</option>
      </select>
      <span class="muted small">{visibleDramas.length}/{dramas.length}</span>
    </div>

    {#if visibleDramas.length === 0}
      <div class="empty card"><div class="t">无匹配剧目</div><div class="h">调整筛选条件试试</div></div>
    {:else}
    <div class="grid stagger">
      {#each visibleDramas as d, i (d.id)}
        <div
          class="card drama"
          draggable="true"
          class:dragging={dragIdx === i}
          ondragstart={(e) => { onDragStart(i); e.dataTransfer.effectAllowed = 'move'; }}
          ondragover={(e) => { e.preventDefault(); overIdx = i; }}
          ondragleave={() => { if (overIdx === i) overIdx = -1; }}
          ondrop={() => onDrop(i)}
          ondragend={() => resetDrag()}
        >
          <a class="drama-main" href={`/dramas/${d.id}`}>
            <div class="d-title">{d.name}</div>
            <div class="d-meta">
              {#if d.categoryNames?.length}
                <div class="d-cats">
                  {#each d.categoryNames.slice(0, 4) as cn}<span class="d-cat">{cn}</span>{/each}
                  {#if d.categoryNames.length > 4}
                    <span class="d-cat more" title={d.categoryNames.join(' / ')}>+{d.categoryNames.length - 4}</span>
                  {/if}
                </div>
              {/if}
              {#if d.remark}<span class="d-remark" title={d.remark}>{d.remark}</span>{/if}
            </div>
          </a>
          <div class="d-stats">
            <span class="stat" title="折子数"><b>{d.zheziCount}</b> 折</span>
            <span class="stat" title="演出数"><b>{d.recordCount}</b> 场</span>
          </div>
          <button class="del" title="删除剧目" onclick={() => remove(d.id, d.name)}>✕</button>
        </div>
      {/each}
    </div>
    {/if}
  {/if}
</div>

<style>
  .add-bar { display: flex; gap: 10px; padding: 12px; margin-bottom: 16px; align-items: center; flex-wrap: wrap; }
  .add-bar .grow { flex: 1 1 160px; }
  .filter-bar {
    display: flex; gap: 10px; padding: 10px 12px; margin-bottom: 14px;
    align-items: center; flex-wrap: wrap;
  }
  .filter-bar .input { width: auto; min-width: 130px; }
  .filter-bar .grow { flex: 1 1 180px; }
  .filter-bar select { max-width: 150px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 10px; }
  .drama { display: flex; align-items: center; gap: 8px; padding: 14px 16px; }
  .drama-main { flex: 1; min-width: 0; text-decoration: none; transition: color var(--t-fast) var(--ease); }
  .drama-main:hover .d-title { color: var(--accent); }
  .d-title { font-weight: 600; font-size: 15.5px; font-family: var(--font-serif); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .d-meta { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--text-muted); margin-top: 3px; min-width: 0; }
  .d-cats { display: flex; flex-wrap: nowrap; gap: 6px; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .d-cat {
    background: var(--accent-soft);
    color: var(--accent);
    border-radius: 999px;
    padding: 0 8px;
    line-height: 1.7;
    white-space: nowrap;
  }
  .d-cat.more { background: var(--surface-3); color: var(--text-muted); }
  .d-remark { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .d-stats { display: flex; gap: 8px; flex: 0 0 auto; }
  .stat { font-size: 12px; color: var(--text-muted); }
  .stat b { color: var(--accent); font-size: 13px; }
  .del {
    border: none; background: none; color: var(--text-3); cursor: pointer; font-size: 12px;
    width: 24px; height: 24px; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;
    transition: all var(--t-fast) var(--ease); flex: 0 0 auto;
  }
  .del:hover { background: var(--danger-soft); color: var(--danger); }
  .drama.dragging { opacity: 0.4; cursor: grabbing; }
  .drama { cursor: grab; }

  @media (max-width: 560px) {
    .grid { grid-template-columns: 1fr; }
  }
</style>