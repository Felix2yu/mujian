<script>
  import { api, coverUrl } from '$lib/api.js';

  let groups = $state([]);
  let groupsLoading = $state(false);
  let selectedHashes = $state(new Set());
  let merging = $state(false);

  let orphans = $state([]);
  let orphansLoading = $state(false);
  let selectedOrphans = $state(new Set());
  let cleaning = $state(false);
  let purging = $state(false);

  let thumbsBusy = $state(false);
  let error = $state('');
  let info = $state('');

  function fmtSize(n) {
    if (!n) return '0 B';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    return (n / 1024 / 1024).toFixed(2) + ' MB';
  }

  async function scanDuplicates() {
    groupsLoading = true;
    error = '';
    try {
      const res = await api.getCoverDuplicates();
      groups = res.groups || [];
      selectedHashes = new Set(groups.map((g) => g.hash));
    } catch (e) {
      error = e.message;
    } finally {
      groupsLoading = false;
    }
  }

  function toggleHash(hash) {
    const s = new Set(selectedHashes);
    s.has(hash) ? s.delete(hash) : s.add(hash);
    selectedHashes = s;
  }

  async function runMerge() {
    const hashes = [...selectedHashes];
    if (!hashes.length) return;
    if (!confirm(`合并 ${hashes.length} 组重复封面？合并后这些演出将共享同一份封面文件。`)) return;
    merging = true;
    error = '';
    info = '';
    try {
      const res = await api.mergeCovers(hashes);
      info = `合并完成：${res.merged_groups} 组，更新 ${res.updated_records} 条记录，释放 ${fmtSize(res.freed_bytes)}`;
      await scanDuplicates();
    } catch (e) {
      error = e.message;
    } finally {
      merging = false;
    }
  }

  async function scanOrphans() {
    orphansLoading = true;
    error = '';
    try {
      const res = await api.getCoverOrphans();
      orphans = res.files || [];
      selectedOrphans = new Set(orphans.map((o) => o.file_name));
    } catch (e) {
      error = e.message;
    } finally {
      orphansLoading = false;
    }
  }

  function toggleOrphan(name) {
    const s = new Set(selectedOrphans);
    s.has(name) ? s.delete(name) : s.add(name);
    selectedOrphans = s;
  }

  async function runCleanup() {
    if (!selectedOrphans.size) return;
    if (!confirm(`将 ${selectedOrphans.size} 张未引用的封面移入回收站（可恢复）？`)) return;
    cleaning = true;
    error = '';
    info = '';
    try {
      const res = await api.cleanupCovers({ files: [...selectedOrphans] });
      info = `已移入回收站 ${res.moved} 张，释放 ${fmtSize(res.freed_bytes)}`;
      await scanOrphans();
    } catch (e) {
      error = e.message;
    } finally {
      cleaning = false;
    }
  }

  async function runPurge() {
    if (!confirm('彻底清空回收站？此操作不可恢复。')) return;
    purging = true;
    error = '';
    try {
      const res = await api.purgeTrash();
      info = `回收站已清空（${res.purged} 个文件）`;
    } catch (e) {
      error = e.message;
    } finally {
      purging = false;
    }
  }

  async function runThumbs() {
    thumbsBusy = true;
    error = '';
    try {
      const res = await api.regenerateThumbs();
      info = `已为 ${res.updated} 条记录生成统一缩略图`;
    } catch (e) {
      error = e.message;
    } finally {
      thumbsBusy = false;
    }
  }
</script>

<div class="fade-up">
  <div class="page-head">
    <h1>封面管理</h1>
    <p class="sub">去重合并、清理未引用封面、统一缩略图</p>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}
  {#if info}<div class="banner success">✓ {info}</div>{/if}

  <div class="card sec">
    <div class="sec-head">
      <h3>① 重复封面合并</h3>
      <div class="sec-actions">
        {#if groups.length}<button class="btn ghost sm" on:click={() => (selectedHashes = new Set(groups.map((g) => g.hash)))}>全选</button>{/if}
        <button class="btn sm" on:click={scanDuplicates} disabled={groupsLoading}>{groupsLoading ? '扫描中…' : (groups.length ? '重新扫描' : '扫描重复封面')}</button>
      </div>
    </div>

    {#if groups.length}
      <p class="tiny">检测到 {groups.length} 组重复封面（内容相同、仅存多份），合并后仅保留一份。</p>
      <div class="glist">
        {#each groups as g}
          <label class="grow card">
            <input type="checkbox" checked={selectedHashes.has(g.hash)} on:change={() => toggleHash(g.hash)} />
            <img src={coverUrl(g.records[0]?.cover_file)} alt="" loading="lazy" />
            <div class="ginfo">
              <div class="gname">共 {g.count} 条记录引用相同内容 · {fmtSize(g.size)}</div>
              <div class="grecs">{g.records.map((r) => r.name).join(' / ')}</div>
              <div class="ghash tiny">{g.hash.slice(0, 12)}…</div>
            </div>
          </label>
        {/each}
      </div>
      <div class="sec-actions">
        <button class="btn primary" on:click={runMerge} disabled={merging || !selectedHashes.size}>
          {merging ? '合并中…' : `合并选中（${selectedHashes.size} 组）`}
        </button>
      </div>
    {:else if groupsLoading}
      <div class="skeleton" style="height: 120px;"></div>
    {:else}
      <div class="empty">
        <div class="ico">🖼</div>
        <div class="t">尚未扫描</div>
        <div class="h">点击「扫描重复封面」检测内容相同的封面</div>
      </div>
    {/if}
  </div>

  <div class="card sec">
    <div class="sec-head">
      <h3>② 未引用封面清理</h3>
      <div class="sec-actions">
        {#if orphans.length}<button class="btn ghost sm" on:click={() => (selectedOrphans = new Set(orphans.map((o) => o.file_name)))}>全选</button>{/if}
        <button class="btn sm" on:click={scanOrphans} disabled={orphansLoading}>{orphansLoading ? '扫描中…' : (orphans.length ? '重新扫描' : '扫描未引用封面')}</button>
        <button class="btn danger sm" on:click={runPurge} disabled={purging}>清空回收站</button>
      </div>
    </div>

    {#if orphans.length}
      <p class="tiny">发现 {orphans.length} 张未被任何演出引用的封面，共 {fmtSize(orphans.reduce((a, o) => a + o.size, 0))}。清理前会移入回收站，可恢复。</p>
      <div class="olist">
        {#each orphans as o}
          <label class="orow">
            <input type="checkbox" checked={selectedOrphans.has(o.file_name)} on:change={() => toggleOrphan(o.file_name)} />
            <img src={coverUrl(`covers/${o.file_name}`)} alt="" loading="lazy" />
            <span class="oname">{o.file_name}</span>
            <span class="osize tiny">{fmtSize(o.size)}</span>
          </label>
        {/each}
      </div>
      <div class="sec-actions">
        <button class="btn primary" on:click={runCleanup} disabled={cleaning || !selectedOrphans.size}>
          {cleaning ? '清理中…' : `移入回收站（${selectedOrphans.size} 张）`}
        </button>
      </div>
    {:else if orphansLoading}
      <div class="skeleton" style="height: 120px;"></div>
    {:else}
      <div class="empty">
        <div class="ico">🗑</div>
        <div class="t">尚未扫描</div>
        <div class="h">点击「扫描未引用封面」找出不再被引用的文件</div>
      </div>
    {/if}
  </div>

  <div class="card sec">
    <div class="sec-head">
      <h3>③ 统一缩略图</h3>
      <button class="btn sm" on:click={runThumbs} disabled={thumbsBusy}>{thumbsBusy ? '生成中…' : '重新生成缩略图'}</button>
    </div>
    <p class="tiny">为所有有封面的演出生成统一规格（宽 ≤400px JPEG）的缩略图，提升列表加载体验。</p>
  </div>
</div>

<style>
  .sec { padding: 18px 20px; margin-bottom: 14px; }
  .sec-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 12px; }
  .sec-head h3 { margin: 0; font-size: 15.5px; }
  .sec-actions { display: flex; gap: 8px; align-items: center; }

  .glist { display: flex; flex-direction: column; gap: 8px; margin-bottom: 12px; }
  .grow {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px;
    cursor: pointer;
    transition: border-color var(--t-fast) var(--ease);
  }
  .grow:hover { border-color: var(--accent); }
  .grow input, .orow input { accent-color: var(--accent); width: 16px; height: 16px; flex: 0 0 auto; }
  .grow img { width: 44px; height: 60px; object-fit: cover; border-radius: 6px; flex: 0 0 auto; }
  .ginfo { min-width: 0; }
  .gname { font-weight: 600; font-size: 14px; }
  .grecs { font-size: 12.5px; color: var(--text-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ghash { color: var(--text-3); }

  .olist { display: flex; flex-direction: column; gap: 6px; margin-bottom: 12px; }
  .orow {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 8px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: background var(--t-fast) var(--ease);
  }
  .orow:hover { background: var(--surface-2); }
  .orow img { width: 30px; height: 40px; object-fit: cover; border-radius: 5px; flex: 0 0 auto; }
  .oname { flex: 1; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-mono, monospace); }
  .osize { flex: 0 0 auto; }
</style>
