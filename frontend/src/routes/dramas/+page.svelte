<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';

  let dramas = $state([]);
  let loading = $state(true);
  let error = $state('');
  let adding = $state(false);
  let form = $state({ name: '', remark: '' });

  async function load() {
    loading = true;
    error = '';
    try {
      dramas = await api.listDramas();
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
      const d = await api.createDrama({ name, remark: form.remark.trim() });
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
    const next = dramas.slice();
    const [moved] = next.splice(dragIdx, 1);
    next.splice(targetIdx, 0, moved);
    resetDrag();
    error = '';
    api.reorderDramas(next.map((x) => x.id))
      .then(() => {
        dramas = next;
      })
      .catch((e) => (error = e.message));
  }

  function resetDrag() {
    dragIdx = -1;
    overIdx = -1;
  }

  onMount(load);
</script>

<div class="fade-up">
  <div class="page-head">
    <h1>剧目</h1>
    <p class="sub">管理剧目档案与折子。点击剧目进入详情，可添加、排序并设置折子别名；拖动卡片可调整剧目显示顺序</p>
  </div>

  <div class="card add-bar">
    <input class="input grow" placeholder="剧目名称（必填），如：牡丹亭" bind:value={form.name} onkeydown={(e) => e.key === 'Enter' && add()} />
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
    <div class="grid stagger">
      {#each dramas as d, i (d.id)}
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
                {#each d.categoryNames as cn}<span class="d-cat">{cn}</span>{/each}
              {/if}
              {#if d.remark}<span class="remark" title={d.remark}>{d.remark}</span>{/if}
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
</div>

<style>
  .add-bar { display: flex; gap: 10px; padding: 12px; margin-bottom: 16px; align-items: center; flex-wrap: wrap; }
  .add-bar .grow { flex: 1 1 160px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 10px; }
  .drama { display: flex; align-items: center; gap: 8px; padding: 14px 16px; }
  .drama-main { flex: 1; min-width: 0; text-decoration: none; transition: color var(--t-fast) var(--ease); }
  .drama-main:hover .d-title { color: var(--accent); }
  .d-title { font-weight: 600; font-size: 15.5px; font-family: var(--font-serif); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .d-meta { display: flex; gap: 6px; font-size: 12px; color: var(--text-muted); margin-top: 3px; flex-wrap: wrap; }
  .d-cat {
    background: var(--accent-soft);
    color: var(--accent);
    border-radius: 999px;
    padding: 0 8px;
    line-height: 1.7;
    white-space: nowrap;
  }
  .d-meta .remark { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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