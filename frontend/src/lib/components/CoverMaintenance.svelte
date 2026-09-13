<script>
  import { api, coverUrl } from '$lib/api.js';

  let groups = $state([]);
  let groupsLoading = $state(false);
  let dupScanned = $state(false);
  let selectedHashes = $state(new Set());
  let merging = $state(false);

  let orphans = $state([]);
  let orphansLoading = $state(false);
  // 区分「尚未扫描」与「已扫描但没有未引用封面」——两者的空态文案不同
  let orphansScanned = $state(false);
  let selectedOrphans = $state(new Set());
  let cleaning = $state(false);
  let purging = $state(false);

  let thumbsBusy = $state(false);
  let thumbsProgress = $state({ processed: 0, total: 0, updated: 0 });
  let error = $state('');
  let info = $state('');

  // 灯箱：点击封面放大查看
  let lightbox = $state(false);
  let lightboxSrc = $state('');
  function openLightbox(src) { lightboxSrc = src; lightbox = true; }
  function closeLightbox() { lightbox = false; lightboxSrc = ''; }

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
      dupScanned = true;
      selectedHashes = new Set(groups.map((g) => g.hash));
    } catch (e) {
      error = e.message;
    } finally {
      groupsLoading = false;
    }
  }

  function selectAllHashes() {
    selectedHashes = new Set(groups.map((g) => g.hash));
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
      orphansScanned = true;
      selectedOrphans = new Set(orphans.map((o) => o.file_name));
    } catch (e) {
      error = e.message;
    } finally {
      orphansLoading = false;
    }
  }

  function selectAllOrphans() {
    selectedOrphans = new Set(orphans.map((o) => o.file_name));
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
    if (!confirm('彻底清空封面回收站？此操作不可恢复。')) return;
    purging = true;
    error = '';
    try {
      const res = await api.purgeTrash();
      info = `封面回收站已清空（${res.purged} 个文件）`;
    } catch (e) {
      error = e.message;
    } finally {
      purging = false;
    }
  }

  async function runThumbs() {
    thumbsBusy = true;
    error = '';
    info = '';
    thumbsProgress = { processed: 0, total: 0, updated: 0 };
    try {
      // Streams NDJSON progress from the server; each line updates the live
      // counter below instead of waiting (and risking a timeout) for the end.
      const res = await api.regenerateThumbs((p) => {
        if (typeof p.total === 'number') thumbsProgress.total = p.total;
        if (typeof p.updated === 'number') thumbsProgress.updated = p.updated;
        if (p.phase === 'item') thumbsProgress.processed = p.index + 1;
      });
      info = `已为 ${res.updated} 条记录生成缩略图`;
    } catch (e) {
      error = e.message;
    } finally {
      thumbsBusy = false;
    }
  }

  function onWindowKeydown(e) {
    if (e.key === 'Escape') closeLightbox();
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<!-- 分区容器。三张卡片的空态一律做成单行内联文案：扫描出结果后才展开列表，
     不再事先用 .empty（约 200px）或 120px 骨架屏预留一大块空白。 -->
<section class="fade-up cover-section" aria-labelledby="cover-section-title">
  <div class="cover-section-head">
    <h2 id="cover-section-title">封面维护</h2>
    <p class="tiny muted">去重合并、清理未引用封面、统一缩略图。</p>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}
  {#if info}<div class="banner success">✓ {info}</div>{/if}

  <div class="card sec">
    <div class="sec-head">
      <h3>① 重复封面合并</h3>
      <div class="sec-actions">
        {#if groups.length}<button class="btn ghost sm" onclick={selectAllHashes}>全选</button>{/if}
        <button class="btn sm" onclick={scanDuplicates} disabled={groupsLoading}>
          {groupsLoading ? '扫描中…' : (groups.length || dupScanned ? '重新扫描' : '扫描重复封面')}
        </button>
      </div>
    </div>

    {#if groupsLoading}
      <p class="tiny scanning">正在扫描重复封面…</p>
    {:else if groups.length}
      <p class="tiny">检测到 {groups.length} 组重复封面（内容相同、仅存多份），合并后仅保留一份。</p>
      <div class="glist">
        {#each groups as g}
          <label class="grow card">
            <input type="checkbox" checked={selectedHashes.has(g.hash)} onchange={() => toggleHash(g.hash)} />
            <button type="button" class="cover-th" onclick={() => openLightbox(coverUrl(g.records[0]?.cover_file))} aria-label="放大查看封面">
              <img src={coverUrl(g.records[0]?.cover_file)} alt="" loading="lazy" />
            </button>
            <div class="ginfo">
              <div class="gname">共 {g.count} 条记录引用相同内容 · {fmtSize(g.size)}</div>
              <div class="grecs">{g.records.map((r) => r.name).join(' / ')}</div>
              <div class="ghash tiny">{g.hash.slice(0, 12)}…</div>
            </div>
          </label>
        {/each}
      </div>
      <div class="sec-actions">
        <button class="btn primary" onclick={runMerge} disabled={merging || !selectedHashes.size}>
          {merging ? '合并中…' : `合并选中（${selectedHashes.size} 组）`}
        </button>
      </div>
    {:else if dupScanned}
      <p class="tiny ok-line">✓ 未找到重复封面——没有内容相同却存为多份的文件。</p>
    {:else}
      <p class="tiny muted">内容相同却存了多份的封面会在这里列出，可勾选合并、只保留一份。</p>
    {/if}
  </div>

  <div class="card sec">
    <div class="sec-head">
      <h3>② 未引用封面清理</h3>
      <div class="sec-actions">
        {#if orphans.length}<button class="btn ghost sm" onclick={selectAllOrphans}>全选</button>{/if}
        <button class="btn sm" onclick={scanOrphans} disabled={orphansLoading}>
          {orphansLoading ? '扫描中…' : (orphans.length || orphansScanned ? '重新扫描' : '扫描未引用封面')}
        </button>
        <button class="btn danger sm" onclick={runPurge} disabled={purging}>清空封面回收站</button>
      </div>
    </div>

    {#if orphansLoading}
      <p class="tiny scanning">正在扫描未引用封面…</p>
    {:else if orphans.length}
      <p class="tiny">发现 {orphans.length} 张未被任何演出引用的封面，共 {fmtSize(orphans.reduce((a, o) => a + o.size, 0))}。清理前会移入回收站，可恢复。</p>
      <div class="olist">
        {#each orphans as o}
          <label class="orow">
            <input type="checkbox" checked={selectedOrphans.has(o.file_name)} onchange={() => toggleOrphan(o.file_name)} />
            <button type="button" class="cover-th-sm" onclick={() => openLightbox(coverUrl(`covers/${o.file_name}`))} aria-label="放大查看封面">
              <img src={coverUrl(`covers/${o.file_name}`)} alt="" loading="lazy" />
            </button>
            <span class="oname">{o.file_name}</span>
            <span class="osize tiny">{fmtSize(o.size)}</span>
          </label>
        {/each}
      </div>
      <div class="sec-actions">
        <button class="btn primary" onclick={runCleanup} disabled={cleaning || !selectedOrphans.size}>
          {cleaning ? '清理中…' : `移入回收站（${selectedOrphans.size} 张）`}
        </button>
      </div>
    {:else if orphansScanned}
      <p class="tiny ok-line">✓ 没有未引用的封面——所有封面文件仍被演出引用。</p>
    {:else}
      <p class="tiny muted">不再被任何演出引用的封面文件会在这里列出；清理时先移入回收站，可恢复。</p>
    {/if}
  </div>

  <div class="card sec">
    <div class="sec-head">
      <h3>③ 统一缩略图</h3>
      <button class="btn sm" onclick={runThumbs} disabled={thumbsBusy}>{thumbsBusy ? '生成中…' : '重新生成缩略图'}</button>
    </div>
    {#if thumbsBusy && thumbsProgress.total > 0}
      <p class="tiny">⏳ 已处理 {thumbsProgress.processed}/{thumbsProgress.total} 个封面，更新 {thumbsProgress.updated} 条记录</p>
    {/if}
    <p class="tiny">为所有有封面的演出按当前编码格式重新生成缩略图（宽 ≤400px），并清理旧格式缩略图文件。</p>
  </div>
</section>

<!-- 灯箱必须渲染在 .fade-up 之外：fadeUp 动画 fill-mode:both 会残留
     transform，使 .fade-up 成为 fixed 元素的包含块，定位基准随之错位。 -->
{#if lightbox && lightboxSrc}
  <button type="button" class="lightbox" onclick={closeLightbox} aria-label="关闭大图">
    <img src={lightboxSrc} alt="" />
  </button>
{/if}

<style>
  /* 分区节奏与 .cost-section / .data-sec 一致（28px 上间距 + 18px 内间距
     + 1px 分隔线），使数据页五个分区视觉层级统一。 */
  .cover-section {
    margin-top: 28px;
    padding-top: 18px;
    border-top: 1px solid var(--border);
  }
  .cover-section-head { margin-bottom: 14px; }
  .cover-section-head h2 {
    margin: 0 0 4px;
    font-size: 16px;
    font-weight: 600;
    color: var(--text);
  }
  .cover-section-head p { margin: 0; }

  /* .sec 是页面级样式，不跨组件生效，故组件内自带卡片内边距。 */
  .sec { padding: 18px 20px; }
  .sec + .sec { margin-top: 14px; }
  /* flex-wrap：窄屏下按钮组换行，而不是把标题挤成两行 */
  .sec-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 10px; }
  .sec-head h3 { margin: 0; font-size: 15.5px; }
  .sec-actions { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }

  /* 空态一律单行，避免事先预留大块空白 */
  .scanning { margin: 6px 0 0; color: var(--text-3); }
  .ok-line { margin: 6px 0 0; color: var(--success); }
  .sec > p.tiny { margin: 8px 0; }

  .glist { display: flex; flex-direction: column; gap: 8px; margin: 10px 0 12px; }
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
  .grow img { width: 44px; aspect-ratio: 3 / 4; object-fit: cover; border-radius: 6px; flex: 0 0 auto; }
  .ginfo { min-width: 0; }
  .gname { font-weight: 600; font-size: 14px; }
  .grecs { font-size: 12.5px; color: var(--text-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ghash { color: var(--text-3); }

  .olist { display: flex; flex-direction: column; gap: 6px; margin: 10px 0 12px; }
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
  .orow img { width: 30px; aspect-ratio: 3 / 4; object-fit: cover; border-radius: 5px; flex: 0 0 auto; }
  .oname { flex: 1; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-mono, monospace); }
  .osize { flex: 0 0 auto; }

  /* 封面图可点击放大 */
  .cover-th, .cover-th-sm {
    flex: 0 0 auto;
    padding: 0;
    border: none;
    background: transparent;
    cursor: zoom-in;
  }
  .cover-th img { width: 44px; aspect-ratio: 3 / 4; object-fit: cover; border-radius: 6px; display: block; }
  .cover-th-sm img { width: 30px; aspect-ratio: 3 / 4; object-fit: cover; border-radius: 5px; display: block; }

  /* 灯箱 */
  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 100;
    border: none;
    padding: 24px;
    margin: 0;
    background: rgba(0, 0, 0, 0.86);
    backdrop-filter: blur(4px);
    cursor: zoom-out;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .lightbox img {
    max-width: min(92vw, 720px);
    max-height: 88vh;
    width: auto;
    height: auto;
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
  }
</style>
