<script>
  import { api, coverUrl } from '$lib/api.js';
  let { open, onSelect, onClose } = $props();

  let covers = $state([]);
  let total = $state(0);
  let q = $state('');
  let page = $state(0);
  let loading = $state(false);
  const limit = 30;

  async function load() {
    loading = true;
    try {
      const res = await api.listCovers({ q, page, limit });
      covers = res.covers || [];
      total = res.total || 0;
    } catch (e) {
      covers = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (open) {
      q = '';
      page = 0;
      load();
    }
  });

  function search() {
    page = 0;
    load();
  }

  function pick(c) {
    onSelect?.(c);
    onClose?.();
  }
</script>

{#if open}
  <div class="overlay" onclick={onClose}>
    <div class="panel card" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
      <div class="head">
        <h3>从已有演出引用封面</h3>
        <button class="x" onclick={onClose} aria-label="关闭">✕</button>
      </div>
      <div class="search-row">
        <input
          class="input"
          placeholder="搜索演出名称或分类…"
          bind:value={q}
          onkeydown={(e) => e.key === 'Enter' && search()}
        />
        <button class="btn" onclick={search}>搜索</button>
      </div>

      {#if loading}
        <div class="grid">
          {#each Array(6) as _}<div class="skeleton sk-item"></div>{/each}
        </div>
      {:else if covers.length === 0}
        <div class="empty">
          <div class="ico">🖼</div>
          <div class="t">{q ? '没有匹配的封面' : '还没有任何封面'}</div>
          <div class="h">先上传封面，或从「记录现场」导入数据</div>
        </div>
      {:else}
        <div class="grid">
          {#each covers as c}
            <button class="item" onclick={() => pick(c)} title={c.sample_name || c.file_name}>
              <img src={coverUrl(c.thumb || c.file_name)} alt={c.sample_name} loading="lazy" />
              <span class="iname">{c.sample_name || c.file_name}</span>
              <span class="iref">{c.ref_count} 条引用</span>
            </button>
          {/each}
        </div>
      {/if}

      {#if total > limit}
        <div class="pager">
          <button class="btn sm" disabled={page === 0} onclick={() => { page--; load(); }}>上一页</button>
          <span class="tiny">{page + 1} / {Math.ceil(total / limit)}</span>
          <button class="btn sm" disabled={(page + 1) * limit >= total} onclick={() => { page++; load(); }}>下一页</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    background: rgba(0, 0, 0, 0.45);
    -webkit-backdrop-filter: blur(4px);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
    animation: fadeIn 0.2s var(--ease);
  }
  .panel {
    width: min(720px, 100%);
    max-height: 82vh;
    display: flex;
    flex-direction: column;
    padding: 16px;
    animation: fadeUp 0.25s var(--ease);
  }
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
  .head h3 { margin: 0; font-size: 16px; }
  .x {
    border: none; background: none; font-size: 15px; cursor: pointer;
    color: var(--text-muted); width: 30px; height: 30px; border-radius: 50%;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .x:hover { background: var(--surface-3); color: var(--text); }

  .search-row { display: flex; gap: 8px; margin-bottom: 12px; }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap: 10px;
    overflow-y: auto;
    padding: 2px;
  }
  .item {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--surface-2);
    padding: 0;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    transition: border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease), box-shadow var(--t-fast) var(--ease);
    text-align: left;
  }
  .item:hover { border-color: var(--accent); transform: translateY(-2px); box-shadow: var(--shadow-md); }
  .item img { width: 100%; aspect-ratio: 3/4; object-fit: cover; display: block; background: var(--surface-3); }
  .iname {
    font-size: 12px;
    padding: 6px 8px 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .iref { font-size: 11px; color: var(--text-muted); padding: 2px 8px 8px; }

  .sk-item { aspect-ratio: 3/4; border-radius: var(--radius); }

  .pager { display: flex; align-items: center; justify-content: center; gap: 12px; margin-top: 12px; }

  @media (max-width: 640px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); }
  }
</style>
