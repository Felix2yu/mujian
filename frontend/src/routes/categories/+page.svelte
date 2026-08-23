<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';

  let categories = $state([]);
  let loading = $state(true);
  let error = $state('');
  let newName = $state('');
  let adding = $state(false);

  async function load() {
    loading = true;
    error = '';
    try {
      categories = await api.listCategories();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function add() {
    const name = newName.trim();
    if (!name || adding) return;
    adding = true;
    error = '';
    try {
      await api.createCategory({ name, activeIds: [], recordCount: 0 });
      newName = '';
      await load();
    } catch (e) {
      error = e.message;
    } finally {
      adding = false;
    }
  }

  async function remove(id, name) {
    if (!confirm(`删除分类「${name}」？记录不会被删除。`)) return;
    error = '';
    try {
      await api.deleteCategory(id);
      await load();
    } catch (e) {
      error = e.message;
    }
  }

  // 拖拽排序状态
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
    const next = categories.slice();
    const [moved] = next.splice(dragIdx, 1);
    next.splice(targetIdx, 0, moved);
    resetDrag();
    error = '';
    api.reorderCategories(next.map((x) => x.id))
      .then(() => {
        categories = next;
      })
      .catch((e) => (error = e.message));
  }

  function resetDrag() {
    dragIdx = -1;
    overIdx = -1;
  }

  onMount(load);
</script>
<svelte:head><title>分类 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>分类</h1>
    <p class="sub">管理演出分类，点击分类名可查看该类目下的记录；拖动卡片可调整显示顺序</p>
  </div>

  <div class="card add-bar">
    <input class="input" placeholder="新建分类名称，回车快速添加" bind:value={newName} onkeydown={(e) => e.key === 'Enter' && add()} />
    <button class="btn primary" onclick={add} disabled={adding || !newName.trim()}>{adding ? '添加中…' : '＋ 添加'}</button>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  {#if loading}
    <div class="grid">
      {#each Array(6) as _}<div class="skeleton" style="height: 84px;"></div>{/each}
    </div>
  {:else if categories.length === 0}
    <div class="empty card">
      <div class="ico">❏</div>
      <div class="t">还没有分类</div>
      <div class="h">在上方输入名称添加第一个分类</div>
    </div>
  {:else}
    <div class="grid stagger">
      {#each categories as c, i (c.id)}
        <div
          class="card cat"
          draggable="true"
          class:dragging={dragIdx === i}
          ondragstart={(e) => { onDragStart(i); e.dataTransfer.effectAllowed = 'move'; }}
          ondragover={(e) => { e.preventDefault(); overIdx = i; }}
          ondragleave={() => { if (overIdx === i) overIdx = -1; }}
          ondrop={() => onDrop(i)}
          ondragend={() => resetDrag()}
        >
          <a class="cat-name" href={`/?category=${encodeURIComponent(c.name)}`}>{c.name}</a>
          <span class="cnt">{c.recordCount ?? 0} 条</span>
          <button class="del" title="删除分类" onclick={() => remove(c.id, c.name)}>✕</button>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .add-bar { display: flex; gap: 10px; padding: 12px; margin-bottom: 16px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 10px; }
  .cat {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 14px 16px;
  }
  .cat-name {
    font-weight: 600;
    font-size: 15px;
    font-family: var(--font-serif);
    transition: color var(--t-fast) var(--ease);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cat-name:hover { color: var(--accent); }
  .cnt { font-size: 12px; color: var(--text-muted); flex: 0 0 auto; }
  .del {
    border: none;
    background: none;
    color: var(--text-3);
    cursor: pointer;
    font-size: 12px;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: all var(--t-fast) var(--ease);
    flex: 0 0 auto;
  }
  .del:hover { background: var(--danger-soft); color: var(--danger); }
  .cat { cursor: grab; }
  .cat.dragging { opacity: 0.4; cursor: grabbing; }
</style>
