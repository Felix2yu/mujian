<script>
  import { api } from '$lib/api.js';

  // kind: 'drama' | 'artist'；selfId/selfName 为「保留方」（target）。
  // 合并方向：把所选的「重复项」（source）并入当前 this 实体。
  let { kind = 'drama', selfId, selfName } = $props();

  const norm = (s) => (s || '').trim().replace(/\s+/g, ' ').toLowerCase();
  const labelFor = (k) => (k === 'drama' ? '剧目' : '演员');

  let open = $state(false);
  let list = $state([]);
  let loading = $state(false);
  let error = $state('');
  let selectedId = $state('');
  let preview = $state(null);
  let busy = $state(false);
  let done = $state('');

  // 同名（归一化后）的其它实体 = 高置信重复；其余进手动下拉。
  let candidates = $derived(
    list.filter((x) => x.id !== selfId && norm(x.name) === norm(selfName))
  );
  let others = $derived(
    list.filter((x) => x.id !== selfId && norm(x.name) !== norm(selfName))
  );

  async function toggle() {
    if (open) {
      open = false;
      reset();
      return;
    }
    open = true;
    await load();
  }

  function reset() {
    selectedId = '';
    preview = null;
    error = '';
    done = '';
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const data = kind === 'drama' ? await api.listDramas() : await api.listArtists();
      list = data || [];
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  const mergeFn = (dryRun) =>
    kind === 'drama'
      ? api.mergeDramas(selectedId, selfId, { dryRun })
      : api.mergeArtists(selectedId, selfId, { dryRun });

  async function showPreview() {
    if (!selectedId) return;
    busy = true;
    error = '';
    preview = null;
    try {
      preview = await mergeFn(true);
    } catch (e) {
      error = e.message;
    } finally {
      busy = false;
    }
  }

  async function doMerge() {
    if (!selectedId) return;
    busy = true;
    error = '';
    try {
      const res = await mergeFn(false);
      const src = preview?.source?.name || '';
      const moved = res.records_repointed ?? res.records_repoint ?? 0;
      const dedup = res.records_deduped ?? 0;
      done = `已合并「${src}」→「${selfName}」：重挂 ${moved} 条、去重 ${dedup} 条记录${kind === 'drama' && (res.zhezis_moved || res.zhezis_deduped) ? `，折子移动 ${res.zhezis_moved ?? 0} / 去重 ${res.zhezis_deduped ?? 0}` : ''}。`;
      preview = null;
      // 被删的是 source，当前页仍是保留方，刷新以反映别名/折子变化。
      setTimeout(() => location.reload(), 700);
    } catch (e) {
      error = e.message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="merge-wrap">
  <button class="btn ghost sm" onclick={toggle} disabled={busy}>
    {open ? '收起' : '查重 / 合并'}
  </button>

  {#if open}
    <div class="card merge-panel">
      <p class="tiny muted">把另一个重复{labelFor(kind)}并入当前「{selfName}」。先预览影响面，确认后再执行；被合并项会被删除。</p>

      {#if loading}
        <p class="tiny">载入{labelFor(kind)}列表…</p>
      {:else if error}
        <div class="banner error">⚠ {error}</div>
      {:else}
        <div class="row">
          <select class="input" bind:value={selectedId} onchange={() => (preview = null)}>
            <option value="">选择要合并进来的重复{labelFor(kind)}…</option>
            {#if candidates.length}
              <optgroup label="同名（最可能是重复）">
                {#each candidates as c}
                  <option value={c.id}>{c.name}{c.recordCount ? ` · ${c.recordCount} 场` : ''}</option>
                {/each}
              </optgroup>
            {/if}
            <optgroup label="其他{labelFor(kind)}">
              {#each others as o}
                <option value={o.id}>{o.name}{o.recordCount ? ` · ${o.recordCount} 场` : ''}</option>
              {/each}
            </optgroup>
          </select>
        </div>

        {#if !selectedId}
          {#if candidates.length === 0}
            <p class="tiny warn-hint">未检测到同名重复项。如确有重复，可在上方下拉手动选择。</p>
          {/if}
        {:else if preview}
          <div class="preview">
            <div class="pv-row"><span>来源</span><b>{preview.source?.name}</b>（{preview.source?.record_count ?? 0} 场）</div>
            <div class="pv-row"><span>并入</span><b>{preview.target?.name}</b>（{preview.target?.record_count ?? 0} 场）</div>
            <div class="pv-stats">
              <span class="pstat"><b>{preview.records_repoint}</b> 重挂</span>
              <span class="pstat"><b>{preview.records_dedupe}</b> 去重</span>
              {#if kind === 'drama'}
                <span class="pstat"><b>{preview.zhezis_moved}</b> 折子移动</span>
                <span class="pstat"><b>{preview.zhezis_deduped}</b> 折子去重</span>
              {/if}
            </div>
            {#if preview.aliases_added?.length}
              <p class="tiny">将并入别名：{preview.aliases_added.join('、')}</p>
            {/if}
            <div class="btn-row">
              <button class="btn danger sm" onclick={doMerge} disabled={busy}>{busy ? '合并中…' : '确认合并'}</button>
              <button class="btn sm" onclick={() => (preview = null)} disabled={busy}>重新选择</button>
            </div>
          </div>
        {:else}
          <div class="btn-row">
            <button class="btn primary sm" onclick={showPreview} disabled={busy}>{busy ? '计算中…' : '预览影响'}</button>
          </div>
        {/if}
      {/if}

      {#if done}<div class="banner success">{done}</div>{/if}
    </div>
  {/if}
</div>

<style>
  .merge-wrap { display: inline-block; }
  .merge-panel {
    margin-top: 10px;
    padding: 14px 16px;
    text-align: left;
  }
  .row { display: flex; gap: 10px; margin-top: 6px; }
  .row .input { flex: 1; min-width: 0; }
  .warn-hint { color: var(--text-muted); margin: 8px 0 0; }
  .preview { margin-top: 12px; border-top: 1px dashed var(--border); padding-top: 12px; }
  .pv-row { display: flex; gap: 8px; font-size: 13px; margin-bottom: 4px; }
  .pv-row span { color: var(--text-muted); width: 32px; flex: none; }
  .pv-stats { display: flex; flex-wrap: wrap; gap: 8px; margin: 8px 0; }
  .pstat {
    display: inline-flex; align-items: baseline; gap: 5px;
    padding: 4px 10px; border-radius: 999px; background: var(--surface-3);
    font-size: 13px; color: var(--text-2);
  }
  .pstat b { font-size: 14px; color: var(--text); }
  .btn-row { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
</style>
