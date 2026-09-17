<script>
  import { untrack, tick } from 'svelte';
  import { api, coverUrl } from '$lib/api.js';

  let { open, onSelect, onClose } = $props();

  const limit = 30;
  const debounceMs = 280;
  const requestTimeoutMs = 15000;

  // kw = 输入框里的实时文本；q = 真正提交给后端的搜索词（防抖后同步）。
  // 两者分离是为了让「输入」不会被任何副作用回填，避免搜索框被打回空字符串。
  let kw = $state('');
  let q = $state('');
  let page = $state(0);
  let covers = $state([]);
  let total = $state(0);
  let loading = $state(false);
  let error = $state('');
  let loaded = $state(false);
  let broken = $state({});

  let gridEl = $state(null);
  let inputEl = $state(null);

  // 请求序号 + AbortController：只认最后一次请求的结果，杜绝旧请求覆盖新结果。
  let seq = 0;
  let controller = null;
  let debounceTimer = null;
  let timeoutTimer = null;

  let totalPages = $derived(Math.max(1, Math.ceil(total / limit)));
  let hasResult = $derived(covers.length > 0);

  // open 是唯一依赖：组件内所有状态读写都走 untrack，避免 effect 被
  // q/page 的变化反复触发（把搜索框清空、页码归零）。这是之前搜索与翻页
  // 全部失效的根因。
  $effect(() => {
    if (!open) return;
    const restoreScroll = lockBodyScroll();
    const active = document.activeElement;
    untrack(() => {
      resetState();
      load();
    });
    tick().then(() => inputEl?.focus({ preventScroll: true }));
    return () => {
      restoreScroll();
      cancelInFlight();
      if (active instanceof HTMLElement && active.isConnected) {
        try { active.focus({ preventScroll: true }); } catch (e) { /* ignore */ }
      }
    };
  });

  function lockBodyScroll() {
    if (typeof document === 'undefined') return () => {};
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = prev; };
  }

  function cancelInFlight() {
    clearTimeout(debounceTimer);
    clearTimeout(timeoutTimer);
    seq++;
    controller?.abort();
    controller = null;
  }

  function resetState() {
    kw = '';
    q = '';
    page = 0;
    covers = [];
    total = 0;
    error = '';
    loaded = false;
    broken = {};
  }

  async function load() {
    const my = ++seq;
    clearTimeout(timeoutTimer);
    controller?.abort();
    const mine = new AbortController();
    controller = mine;
    timeoutTimer = setTimeout(() => mine.abort(new DOMException('timeout', 'TimeoutError')), requestTimeoutMs);

    loading = true;
    error = '';
    try {
      const res = await api.listCovers({ q, page, limit }, mine.signal);
      if (my !== seq) return; // 已有更新请求，丢弃本次结果
      covers = res?.covers || [];
      total = res?.total || 0;
      loaded = true;
      // 越界保护：页数变少时自动回退到最后一页并保持网格可用。
      const maxPage = Math.max(0, Math.ceil(total / limit) - 1);
      if (covers.length === 0 && page > maxPage) {
        page = maxPage;
        await load();
        return;
      }
    } catch (e) {
      if (my !== seq) return;
      covers = [];
      total = 0;
      // 走到这里的一定是「当前」请求超时（被替代的请求在上一层已被过滤）。
      error = e?.name === 'TimeoutError' || e?.name === 'AbortError'
        ? '请求超时，请重试'
        : (e?.message || '封面加载失败');
    } finally {
      if (my === seq) {
        loading = false;
        clearTimeout(timeoutTimer);
      }
    }
    await tick();
    gridEl?.scrollTo({ top: 0 });
  }

  function onKeywordInput() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(submitSearch, debounceMs);
  }

  function submitSearch() {
    clearTimeout(debounceTimer);
    const term = kw.trim();
    if (term === q && page === 0) return;
    q = term;
    page = 0;
    load();
  }

  function clearSearch() {
    clearTimeout(debounceTimer);
    kw = '';
    if (q === '' && page === 0) return;
    q = '';
    page = 0;
    load();
  }

  function goto(p) {
    const next = Math.min(Math.max(p, 0), totalPages - 1);
    if (next === page) return;
    page = next;
    load();
  }

  function pick(c) {
    if (!c?.file_name) return;
    cancelInFlight();
    onSelect?.(c);
    onClose?.();
  }

  function requestClose() {
    clearTimeout(debounceTimer);
    cancelInFlight();
    onClose?.();
  }

  function onWindowKeydown(e) {
    if (!open) return;
    const t = e.target;
    const typing = t instanceof HTMLElement &&
      (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable);
    if (e.key === 'Escape') {
      e.preventDefault();
      requestClose();
      return;
    }
    if (typing) return;
    if (e.key === 'ArrowLeft') { e.preventDefault(); goto(page - 1); }
    else if (e.key === 'ArrowRight') { e.preventDefault(); goto(page + 1); }
  }

  // 浮层必须是 body 的直接子节点：本组件挂在 .fade-up 内，而 fadeUp 动画的
  // fill-mode:both 会残留 transform，使 .fade-up 成为 fixed 元素的包含块，
  // 定位基准不再是视口（页面滚动后弹窗会跑到可见区之外）。
  function portal(node) {
    if (typeof document !== 'undefined') document.body.appendChild(node);
    return { destroy() { node.remove(); } };
  }

  function reload() {
    load();
  }
</script>

<svelte:window on:keydown={onWindowKeydown} />

{#if open}
  <div class="overlay" role="presentation" onclick={requestClose} use:portal>
    <div
      class="panel card"
      role="dialog"
      aria-modal="true"
      aria-labelledby="cover-picker-title"
      aria-busy={loading}
      onclick={(e) => e.stopPropagation()}
    >
      <div class="head">
        <h3 id="cover-picker-title">从已有演出引用封面</h3>
        <button type="button" class="x" onclick={requestClose} aria-label="关闭">✕</button>
      </div>

      <div class="search-row">
        <input
          class="input"
          bind:this={inputEl}
          bind:value={kw}
          oninput={onKeywordInput}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submitSearch(); } }}
          placeholder="搜索演出名称或分类…"
          aria-label="搜索封面"
          autocomplete="off"
        />
        <button type="button" class="btn" onclick={submitSearch}>搜索</button>
        {#if kw || q}
          <button type="button" class="btn sm ghost" onclick={clearSearch}>清除</button>
        {/if}
      </div>

      <div class="meta tiny" aria-live="polite">
        {#if loading}
          正在加载…
        {:else if loaded}
          共 {total} 张封面{#if q}（匹配「{q}」）{/if}
        {:else}
           
        {/if}
      </div>

      {#if error}
        <div class="banner error picker-error">
          <span>⚠ {error}</span>
          <button type="button" class="btn sm" onclick={reload}>重试</button>
        </div>
      {:else if !loaded && loading}
        <div class="grid" bind:this={gridEl}>
          {#each Array.from({ length: 8 }) as _, i (i)}
            <div class="skeleton sk-item"></div>
          {/each}
        </div>
      {:else if hasResult}
        <div class="grid" bind:this={gridEl} class:refreshing={loading}>
          {#each covers as c (c.file_name)}
            <button
              type="button"
              class="item"
              onclick={() => pick(c)}
              title={c.sample_name || c.file_name}
              aria-label={`使用封面：${c.sample_name || c.file_name}`}
            >
              {#if broken[c.file_name]}
                <div class="img-fallback">🖼</div>
              {:else}
                <img
                  src={coverUrl(c.thumb || c.file_name)}
                  alt={c.sample_name || '封面'}
                  loading="lazy"
                  decoding="async"
                  onerror={() => (broken[c.file_name] = true)}
                />
              {/if}
              <span class="iname">{c.sample_name || c.file_name}</span>
              <span class="iref">{c.ref_count} 条引用</span>
            </button>
          {/each}
        </div>
      {:else}
        <div class="empty">
          <div class="ico">🖼</div>
          <div class="t">{q ? '没有匹配的封面' : '还没有任何封面'}</div>
          <div class="h">{q ? '换个关键词试试，或清除搜索查看全部封面' : '先上传封面，或从「记录现场」导入数据'}</div>
          {#if q}
            <button type="button" class="btn sm" onclick={clearSearch}>清除搜索</button>
          {/if}
        </div>
      {/if}

      {#if totalPages > 1}
        <div class="pager">
          <button type="button" class="btn sm" disabled={page === 0 || loading} onclick={() => goto(0)} aria-label="第一页">«</button>
          <button type="button" class="btn sm" disabled={page === 0 || loading} onclick={() => goto(page - 1)}>上一页</button>
          <span class="tiny pager-label">{page + 1} / {totalPages}</span>
          <button type="button" class="btn sm" disabled={page >= totalPages - 1 || loading} onclick={() => goto(page + 1)}>下一页</button>
          <button type="button" class="btn sm" disabled={page >= totalPages - 1 || loading} onclick={() => goto(totalPages - 1)} aria-label="最后一页">»</button>
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
    min-height: 0;
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

  .search-row { display: flex; flex-wrap: wrap; gap: 8px; }
  .search-row .input { flex: 1 1 200px; min-width: 0; }

  .meta { margin: 8px 2px 10px; min-height: 16px; }

  .picker-error { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 10px; }

  /* flex 子项默认 min-height:auto 会拒绝收缩，导致内容溢出 max-height；
     显式 min-height:0 让网格自己滚动而不是把面板顶出视口。 */
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(120px, 100%), 1fr));
    gap: 10px;
    overflow-y: auto;
    overscroll-behavior: contain;
    flex: 1 1 auto;
    min-height: 0;
    padding: 2px;
    transition: opacity var(--t-fast) var(--ease);
  }
  .grid.refreshing { opacity: 0.55; }

  .item {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--surface-2);
    padding: 0;
    cursor: pointer;
    min-width: 0;
    display: flex;
    flex-direction: column;
    transition: border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease), box-shadow var(--t-fast) var(--ease);
    text-align: left;
  }
  .item:hover { border-color: var(--accent); transform: translateY(-2px); box-shadow: var(--shadow-md); }
  .item:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .item img, .item .img-fallback { width: 100%; aspect-ratio: 3/4; object-fit: cover; display: block; background: var(--surface-3); }
  .item .img-fallback { display: grid; place-items: center; font-size: 22px; opacity: 0.5; }
  .iname {
    font-size: 12px;
    padding: 6px 8px 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .iref { font-size: 11px; color: var(--text-muted); padding: 2px 8px 8px; }

  .sk-item { aspect-ratio: 3/4; border-radius: var(--radius); }

  .pager { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
  .pager-label { min-width: 64px; text-align: center; }

  @media (max-width: 640px) {
    .overlay { padding: 0; }
    .panel { width: 100%; height: 100%; max-height: 100vh; border-radius: 0; }
    .grid { grid-template-columns: repeat(auto-fill, minmax(min(96px, 100%), 1fr)); }
  }
</style>
