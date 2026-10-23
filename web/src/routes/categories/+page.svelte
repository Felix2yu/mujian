<script>
  import Banner from '#lib/ui/Banner.svelte';
  import Skeleton from '#lib/ui/Skeleton.svelte';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import { askConfirm } from '#lib/confirm.js';
  import EmptyState from '#lib/ui/EmptyState.svelte';
  import IconButton from '#lib/ui/IconButton.svelte';

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

  // 删除确认统一走 askConfirm（ConfirmModal 挂在 +layout，自带 Esc/焦点/滚动锁）。
  async function askRemove(c) {
    const ok = await askConfirm({
      title: `删除剧种「${c.name}」`,
      message:
        (c.recordCount ?? 0) > 0
          ? `该剧种下有 ${c.recordCount} 条演出。删除后这些演出仍会保留「${c.name}」标签（成为无主引用，不再出现在剧种列表中），演出记录本身不会被删除。`
          : '确定删除该剧种吗？演出记录不会被删除。',
      confirmLabel: '删除',
      impact: (c.recordCount ?? 0) > 0 ? c.recordCount : null
    });
    if (!ok) return;
    error = '';
    try {
      await api.deleteCategory(c.id);
      await load();
    } catch (e) {
      error = e.message;
    }
  }

  onMount(load);
</script>

<svelte:head><title>剧种 - 幕间</title></svelte:head>

<div class="fade-up">
  <div class="page-head">
    <h1>剧种</h1>
    <p class="sub">
      管理演出剧种；点击剧种名可查看该剧种下的剧目，点击右侧条数可查看演出记录。列表按演出数从高到低排列
    </p>
  </div>

  <div class="card add-bar">
    <input
      class="input"
      placeholder="新建剧种名称，回车快速添加"
      bind:value={newName}
      onkeydown={(e) => e.key === 'Enter' && add()}
    />
    <button class="btn primary" onclick={add} disabled={adding || !newName.trim()}
      >{adding ? '添加中…' : '＋ 添加'}</button
    >
  </div>

  {#if error}<Banner tone="error">{error}</Banner>{/if}

  {#if loading}
    <div class="grid">
      <Skeleton count={6} height="84px" />
    </div>
  {:else if categories.length === 0}
    <EmptyState title="还没有剧种" hint="在上方输入名称添加第一个剧种">
      {#snippet icon()}<span>❏</span>{/snippet}
    </EmptyState>
  {:else}
    <div class="stagger grid">
      {#each categories as c (c.id)}
        <div class="card cat">
          <a
            class="cat-name"
            href={resolve(`/dramas?cat=${encodeURIComponent(c.name)}`)}
            title="查看该剧种下的剧目">{c.name}</a
          >
          <a
            class="cnt"
            href={resolve(`/?category=${encodeURIComponent(c.name)}`)}
            title="查看该剧种下的演出记录">{c.recordCount ?? 0} 条</a
          >
          <IconButton label="删除剧种" tone="danger" size="sm" onclick={() => askRemove(c)}
            >✕</IconButton
          >
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .add-bar {
    display: flex;
    gap: 10px;
    padding: 12px;
    margin-bottom: 16px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 10px;
  }
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
  .cat-name:hover {
    color: var(--accent);
  }
  .cnt {
    font-size: 12px;
    color: var(--text-muted);
    flex: 0 0 auto;
  }
  /* 条数可点：跳转到该剧种下的演出记录 */
  a.cnt {
    transition: color var(--t-fast) var(--ease);
  }
  a.cnt:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .cat {
    position: relative;
  }

  /* 删除确认弹窗已统一走 askConfirm / ConfirmModal，本页不再自带外壳 */
</style>
