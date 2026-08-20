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

  onMount(load);
</script>

<div class="fade-up">
  <div class="page-head">
    <h1>分类</h1>
    <p class="sub">管理演出分类，点击分类名可查看该类目下的记录</p>
  </div>

  <div class="card add-bar">
    <input class="input" placeholder="新建分类名称，回车快速添加" bind:value={newName} on:keydown={(e) => e.key === 'Enter' && add()} />
    <button class="btn primary" on:click={add} disabled={adding || !newName.trim()}>{adding ? '添加中…' : '＋ 添加'}</button>
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
      {#each categories as c (c.id)}
        <div class="card cat">
          <a class="cat-name" href={`/?category=${encodeURIComponent(c.name)}`}>{c.name}</a>
          <span class="cnt">{c.recordCount ?? 0} 条</span>
          <button class="del" title="删除分类" on:click={() => remove(c.id, c.name)}>✕</button>
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
</style>
