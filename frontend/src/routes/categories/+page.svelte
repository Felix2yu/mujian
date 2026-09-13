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

  // 页内删除确认状态：{ id, name, count }；原生 confirm 在预览浏览器中被屏蔽，故用页内弹窗。
  let pendingDelete = $state(null);

  function askRemove(c) {
    pendingDelete = { id: c.id, name: c.name, count: c.recordCount ?? 0 };
  }

  function cancelRemove() {
    pendingDelete = null;
  }

  async function confirmRemove() {
    if (!pendingDelete) return;
    const { id } = pendingDelete;
    pendingDelete = null;
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
  let overBefore = $state(true);

  function onDragStart(i) {
    dragIdx = i;
  }

  function onDragOver(e, i) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    overIdx = i;
    // Grid flows left-to-right (row-major): split by X, not Y.
    overBefore = e.clientX < rect.left + rect.width / 2;
  }

  function onDrop(targetIdx) {
    if (dragIdx < 0 || dragIdx === targetIdx) {
      resetDrag();
      return;
    }
    const next = categories.slice();
    const [moved] = next.splice(dragIdx, 1);
    // Honor the left/right split: before => insert at target, after => insert
    // after target. Account for the index shift caused by removal.
    let insertAt = overBefore ? targetIdx : targetIdx + 1;
    if (dragIdx < targetIdx) insertAt -= 1;
    next.splice(insertAt, 0, moved);
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
<svelte:head><title>剧种 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>剧种</h1>
    <p class="sub">管理演出剧种；点击剧种名可查看该剧种下的剧目，点击右侧条数可查看演出记录。拖动卡片可调整显示顺序</p>
  </div>

  <div class="card add-bar">
    <input class="input" placeholder="新建剧种名称，回车快速添加" bind:value={newName} onkeydown={(e) => e.key === 'Enter' && add()} />
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
      <div class="t">还没有剧种</div>
      <div class="h">在上方输入名称添加第一个剧种</div>
    </div>
  {:else}
    <div class="grid stagger">
      {#each categories as c, i (c.id)}
        <div
          class="card cat"
          draggable="true"
          class:dragging={dragIdx === i}
          class:drop-before={overIdx === i && dragIdx !== i && overBefore}
          class:drop-after={overIdx === i && dragIdx !== i && !overBefore}
          ondragstart={(e) => { onDragStart(i); e.dataTransfer.effectAllowed = 'move'; }}
          ondragover={(e) => onDragOver(e, i)}
          ondragleave={() => { if (overIdx === i) overIdx = -1; }}
          ondrop={() => onDrop(i)}
          ondragend={() => resetDrag()}
        >
          <a class="cat-name" href={`/dramas?cat=${encodeURIComponent(c.name)}`} title="查看该剧种下的剧目">{c.name}</a>
          <a class="cnt" href={`/?category=${encodeURIComponent(c.name)}`} title="查看该剧种下的演出记录">{c.recordCount ?? 0} 条</a>
          <button class="del" title="删除剧种" onclick={() => askRemove(c)}>✕</button>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if pendingDelete}
  <div class="modal-mask" onclick={cancelRemove} role="presentation">
    <div class="modal" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
      <h3 class="modal-title">删除剧种「{pendingDelete.name}」</h3>
      {#if pendingDelete.count > 0}
        <p class="modal-text">
          该剧种下有 <b>{pendingDelete.count}</b> 条演出。删除后这些演出仍会保留「{pendingDelete.name}」标签
          （成为无主引用，不再出现在剧种列表中），演出记录本身不会被删除。
        </p>
      {:else}
        <p class="modal-text">确定删除该剧种吗？演出记录不会被删除。</p>
      {/if}
      <div class="modal-actions">
        <button class="btn" onclick={cancelRemove}>取消</button>
        <button class="btn danger" onclick={confirmRemove}>删除</button>
      </div>
    </div>
  </div>
{/if}

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
  /* 条数可点：跳转到该剧种下的演出记录 */
  a.cnt { transition: color var(--t-fast) var(--ease); }
  a.cnt:hover { color: var(--accent); text-decoration: underline; }
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
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
    flex: 0 0 auto;
  }
  .del:hover { background: var(--danger-soft); color: var(--danger); }
  .cat { position: relative; cursor: grab; }
  .cat.dragging { opacity: 0.4; cursor: grabbing; }
  /* 拖拽插入指示：行优先网格用竖线（左=插前，右=插后） */
  .cat.drop-before::before,
  .cat.drop-after::after {
    content: '';
    position: absolute;
    top: 8px;
    bottom: 8px;
    width: 3px;
    border-radius: 2px;
    background: var(--accent);
    pointer-events: none;
    z-index: 1;
  }
  .cat.drop-before::before { left: -6px; }
  .cat.drop-after::after { right: -6px; }

  /* 页内删除确认弹窗（替代被预览浏览器屏蔽的原生 confirm） */
  .modal-mask {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
    padding: 16px;
  }
  .modal {
    background: var(--bg, #fff);
    border-radius: 12px;
    padding: 22px 22px 18px;
    max-width: 380px;
    width: 100%;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.2);
  }
  .modal-title { margin: 0 0 10px; font-size: 16px; font-family: var(--font-serif); }
  .modal-text { margin: 0 0 18px; font-size: 14px; line-height: 1.6; color: var(--text-2, #555); }
  .modal-actions { display: flex; justify-content: flex-end; gap: 10px; }
  .btn.danger { background: var(--danger, #d33); color: #fff; border: none; }
  .btn.danger:hover { background: var(--danger-dark, #b22); }
</style>
