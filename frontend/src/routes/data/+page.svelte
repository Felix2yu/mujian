<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import { askConfirm } from '$lib/confirm.js';
  import BackupPanel from '$lib/components/BackupPanel.svelte';
  import CostCompletion from '$lib/components/CostCompletion.svelte';
  import CoverMaintenance from '$lib/components/CoverMaintenance.svelte';
  import VenuePanel from '$lib/components/VenuePanel.svelte';

  let file = $state(null);
  let result = $state(null);
  // 导入预览（?dry_run=1 的返回）：只解析校验、不落库。
  let preview = $state(null);
  let error = $state('');
  let busy = $state(false);
  let previewBusy = $state(false);
  let dragover = $state(false);

  // 回收站（软删除 30 天）
  let trash = $state([]);
  let trashTotal = $state(0);
  let trashBusy = $state('');

  const RETAIN_DAYS = 30;
  function daysLeft(deletedAt) {
    const elapsed = Math.floor(Date.now() / 1000) - deletedAt;
    return Math.max(0, RETAIN_DAYS - Math.floor(elapsed / 86400));
  }
  async function loadTrash() {
    try {
      const res = await api.listDeletedRecords();
      trash = res.records || [];
      trashTotal = res.total || 0;
    } catch (e) { /* 页面其他功能不受影响 */ }
  }
  async function restoreTrashed(id) {
    trashBusy = id;
    try {
      await api.restoreRecord(id);
      await loadTrash();
    } catch (e) {
      error = '恢复失败：' + e.message;
    } finally {
      trashBusy = '';
    }
  }
  async function purgeTrashed(id) {
    const ok = await askConfirm({
      title: '彻底删除记录',
      message: '这条演出将从回收站永久删除，不可恢复。',
      confirmLabel: '彻底删除',
      danger: true,
      impact: 1,
      impactLabel: '条记录'
    });
    if (!ok) return;
    trashBusy = id;
    try {
      await api.purgeRecord(id);
      await loadTrash();
    } catch (e) {
      error = '删除失败：' + e.message;
    } finally {
      trashBusy = '';
    }
  }
  async function emptyTrash() {
    const ok = await askConfirm({
      title: '清空回收站',
      message: `将永久删除回收站中的全部 ${trashTotal} 条记录，不可恢复。`,
      confirmLabel: `清空 ${trashTotal} 条`,
      danger: true,
      impact: trashTotal,
      impactLabel: '条记录'
    });
    if (!ok) return;
    trashBusy = 'all';
    try {
      await api.purgeRecordsTrash();
      await loadTrash();
    } catch (e) {
      error = '清空失败：' + e.message;
    } finally {
      trashBusy = '';
    }
  }

  function resetImportState() {
    result = null;
    preview = null;
    error = '';
  }

  function onFile(e) {
    file = e.target.files?.[0] || null;
    resetImportState();
  }

  function onDrop(e) {
    e.preventDefault();
    dragover = false;
    const f = e.dataTransfer?.files?.[0];
    if (f) {
      file = f;
      resetImportState();
    }
  }

  // 第一步：试运行。只解析 + 校验，回报新增/覆盖/跳过与拒收明细，不改动数据库。
  async function runPreview() {
    if (!file) {
      error = '请选择文件（.json 或 .zip 压缩包）';
      return;
    }
    previewBusy = true;
    error = '';
    result = null;
    try {
      preview = await api.importRecords(file, { dryRun: true });
    } catch (e) {
      error = e.message;
      preview = null;
    } finally {
      previewBusy = false;
    }
  }

  // 第二步：确认后真正写入。
  async function confirmImport() {
    if (!file) {
      error = '请选择文件（.json 或 .zip 压缩包）';
      return;
    }
    busy = true;
    error = '';
    try {
      result = await api.importRecords(file);
      preview = null;
    } catch (e) {
      error = e.message;
    } finally {
      busy = false;
    }
  }

  onMount(loadTrash);
</script>
<svelte:head><title>数据 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>数据管理</h1>
    <p class="sub">导入 / 导出、自动备份、回收站、费用补全与封面维护</p>
  </div>

  <!-- ============ 导入 / 导出 ============ -->
  <!-- 导入与导出是同一套文件格式（data.json / 含 covers 的 zip）的两个方向，
       放在同一分区，用户不必在两个分区之间来回找。 -->
  <section class="data-sec">
    <div class="data-sec-head">
      <h2>导入 / 导出 / 自动备份</h2>
      <p class="tiny muted">从「记录现场」备份包或单独的 data.json 还原数据（按 id 覆盖更新）；也可把当前数据导出为文件带走；或设置自动备份定时留存。</p>
    </div>

    <div
      class="card dropzone"
      class:dragover
      ondragover={(e) => { e.preventDefault(); dragover = true; }}
      ondragleave={() => (dragover = false)}
      ondrop={onDrop}
      role="button"
      tabindex="0"
    >
      <div class="dz-ico">⇪</div>
      {#if file}
        <div class="dz-file">
          <span class="fname">{file.name}</span>
          <span class="tiny">{(file.size / 1024 / 1024).toFixed(2)} MB</span>
        </div>
      {:else}
        <div class="dz-text">
          <div class="t">拖放 .zip 压缩包或 data.json 到此处</div>
          <div class="h">或点击选择文件</div>
        </div>
      {/if}
      <input class="dz-input" type="file" accept=".json,.zip" onchange={onFile} title="选择文件" />
    </div>

    <div class="btn-row">
      <button class="btn primary lg" onclick={runPreview} disabled={busy || previewBusy || !file}>
        {previewBusy ? '检查中…' : '预览导入'}
      </button>
      {#if file && !busy && !previewBusy}
        <button class="btn" onclick={() => { file = null; resetImportState(); }}>清除</button>
      {/if}
    </div>

    {#if previewBusy}<p class="tiny warn-hint">正在解析并试运行校验（不写入数据库）；大备份（含封面）可能需要数分钟。</p>{/if}
    {#if busy}<p class="tiny warn-hint">导入进行中，请勿刷新或关闭页面、不要重复点击；大备份（含封面）可能需要数分钟。</p>{/if}

    {#if error}<div class="banner error">⚠ {error}</div>{/if}

    {#if preview}
      <div class="card sec preview">
        <h3>导入预览 <span class="tiny muted">试运行 · 尚未写入</span></h3>
        <div class="pstats">
          <span class="pstat"><b>{preview.new_records ?? 0}</b> 新增</span>
          <span class="pstat"><b>{preview.updated_records ?? 0}</b> 覆盖</span>
          <span class="pstat" class:warn={preview.skipped > 0}><b>{preview.skipped ?? 0}</b> 跳过</span>
          <span class="pstat"><b>{preview.records ?? 0}</b> 记录</span>
          <span class="pstat"><b>{preview.categories ?? 0}</b> 分类</span>
          {#if preview.covers_imported}<span class="pstat"><b>{preview.covers_imported}</b> 封面</span>{/if}
          {#if preview.covers_missing}<span class="pstat warn"><b>{preview.covers_missing}</b> 封面缺失</span>{/if}
        </div>
        {#if preview.warnings?.length}
          <ul class="plist">
            {#each preview.warnings as w}<li>提示：{w.reason}（{w.count} 条）</li>{/each}
          </ul>
        {/if}
        {#if preview.issues?.length}
          <ul class="plist">
            {#each preview.issues as it}<li>跳过第 {it.index + 1} 条{#if it.id}（id {it.id}）{/if}：{it.reason}</li>{/each}
          </ul>
        {/if}
        <div class="btn-row">
          <button class="btn primary" onclick={confirmImport} disabled={busy}>
            {busy ? '导入中…' : `确认导入（${preview.records ?? 0} 条）`}
          </button>
          <button class="btn" onclick={() => (preview = null)} disabled={busy}>取消</button>
        </div>
      </div>
    {/if}
    {#if result}
      <div class="banner success">
        ✓ 导入完成：记录 {result.records} 条，分类 {result.categories} 个
        {#if result.covers_imported != null}，封面关联 {result.covers_imported} 张{/if}
        {#if result.covers_missing}（缺失 {result.covers_missing} 张）{/if}。
        <a class="flink" href="/">查看记录 →</a>
      </div>
    {/if}

    <!-- 两张说明卡并排：内容互不依赖且都不长，宽屏并排可省下约 170px 竖向空间，
         窄于 ~660px 自动回退单列。 -->
    <div class="refs-grid">
      <div class="card sec">
        <h3>支持的文件</h3>
        <ul class="tips">
          <li><b>记录现场备份（推荐）</b>：上传 <code>JI_LU_XIAN_CHANG.android.zip</code>，包内包含数据文件 <code>JI_LU_XIAN_CHANG.android</code> 与封面目录 <code>covers/</code>，一键还原数据与封面关联</li>
          <li><b>纯数据</b>：单独的 <code>data.json</code>（不含封面）</li>
          <li><b>数据 + 封面</b>：含 <code>data.json</code> 与 <code>covers/</code> 的压缩包</li>
        </ul>
      </div>

      <div class="card sec">
        <h3>关于去重</h3>
        <ul class="tips">
          <li><b>演出记录</b>：按 <code>id</code> 覆盖更新。导入自己导出的备份是幂等的——重复导入不会新增重复记录；但来自「记录现场」等外部源、且每条不含 <code>id</code> 的文件，重复导入会产生重复。</li>
          <li><b>封面</b>：按图片内容哈希自动去重，字节完全相同的封面只保存一份，不重复占用空间。</li>
          <li><b>注意</b>：导入是按 <code>id</code> 更新，会覆盖同 <code>id</code> 的已有数据；导入本质是“恢复”而非“追加”。</li>
        </ul>
      </div>
    </div>

    <div class="card sec">
      <h3>手动导出</h3>
      <p class="tiny">导出全量数据与封面，打包为 zip（导入时可直接还原）；也可单独导出 JSON。</p>
      <div class="btn-row">
        <a class="btn primary" href={api.getExportUrl('zip')}>⇩ 导出 ZIP（数据 + 封面）</a>
        <a class="btn" href={api.getExportUrl()}>⇩ 导出 JSON</a>
      </div>
    </div>

    <BackupPanel />
  </section>

  <!-- ============ 回收站 ============ -->
  <section class="data-sec">
    <div class="data-sec-head">
      <h2>回收站</h2>
      <p class="tiny muted">删除的演出在这里保留 {RETAIN_DAYS} 天，之后自动永久清除。</p>
    </div>
    <div class="card sec">
      {#if trash.length === 0}
        <p class="tiny muted">回收站是空的。</p>
      {:else}
        <div class="trash-list">
          {#each trash as t (t.id)}
            <div class="trash-row">
              <span class="trash-name">{t.name}</span>
              <span class="trash-meta">{t.dateText ? t.dateText.slice(0, 10) : ''}{t.city ? ' · ' + t.city : ''} · 剩 {daysLeft(t.deleted_at)} 天</span>
              <span class="trash-ops">
                <button type="button" class="btn sm" disabled={trashBusy === t.id} onclick={() => restoreTrashed(t.id)}>
                  {trashBusy === t.id ? '…' : '恢复'}
                </button>
                <button type="button" class="btn sm danger" disabled={trashBusy === t.id} onclick={() => purgeTrashed(t.id)}>
                  {trashBusy === t.id ? '…' : '彻底删除'}
                </button>
              </span>
            </div>
          {/each}
        </div>
        <div class="btn-row" style="margin-top: 10px;">
          <button type="button" class="btn sm danger" disabled={trashBusy === 'all' || trash.length === 0} onclick={emptyTrash}>
            {trashBusy === 'all' ? '清空中…' : `清空回收站（${trashTotal} 条）`}
          </button>
        </div>
      {/if}
    </div>
  </section>
</div>

<!-- 费用补全 / 封面维护：数据清理工具，操作立即生效（与「保存设置」无关）。
     都挂在 .fade-up 之外 —— 组件内的浮窗（演出预览、封面灯箱）是
     position:fixed，而 .fade-up 的入场动画（fill-mode: both）会残留
     transform，成为 fixed 元素的包含块，使定位基准从视口变成该元素。
     组件自身已带 fade-up 入场动画与分区标题。 -->
<CostCompletion />
<CoverMaintenance />
<VenuePanel />

<style>
  /* 分区：与费用补全组件（.cost-section）保持同一套节奏——28px 上间距 +
     18px 内间距 + 1px 分隔线，使四个分区视觉层级一致。 */
  .data-sec {
    margin-top: 28px;
    padding-top: 18px;
    border-top: 1px solid var(--border);
  }
  /* 第一个分区紧跟页面标题，无需再画分隔线 */
  .data-sec:first-of-type {
    margin-top: 0;
    padding-top: 0;
    border-top: none;
  }
  .data-sec-head { margin-bottom: 14px; }
  .data-sec-head h2 {
    margin: 0 0 4px;
    font-size: 16px;
    font-weight: 600;
    color: var(--text);
  }
  .data-sec-head p { margin: 0; }

  .dropzone {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 44px 20px;
    border: 2px dashed var(--border-strong);
    border-radius: var(--radius-lg);
    cursor: pointer;
    transition: border-color var(--t-med) var(--ease), background var(--t-med) var(--ease);
    box-shadow: none;
    background: var(--surface-2);
  }
  .dropzone:hover, .dropzone.dragover {
    border-color: var(--accent);
    background: var(--accent-softer);
  }
  .dz-ico { font-size: 34px; color: var(--accent); opacity: 0.8; }
  .dz-text { text-align: center; }
  .dz-text .t { font-weight: 600; font-size: 15px; }
  .dz-text .h { font-size: 13px; color: var(--text-muted); }
  .dz-file { display: flex; flex-direction: column; align-items: center; gap: 2px; }
  .fname { font-weight: 600; word-break: break-all; text-align: center; }
  .dz-input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
  }

  /* 允许换行：导出按钮文案较长，窄屏下不换行会把整页顶出视口 */
  .btn-row { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 14px; }
  .warn-hint { color: var(--text-muted); margin-top: 8px; }

  .sec { padding: 18px 20px; margin-top: 16px; }
  .sec h3 { margin: 0 0 10px; font-size: 15.5px; }
  .tips { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 6px; font-size: 13.5px; color: var(--text-2); }

  /* 导入预览：试运行统计条 + 提示/拒收明细 */
  .preview { margin-top: 16px; }
  .preview h3 { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; }
  .pstats { display: flex; flex-wrap: wrap; gap: 8px; margin: 2px 0; }
  .pstat {
    display: inline-flex;
    align-items: baseline;
    gap: 5px;
    padding: 4px 10px;
    border-radius: 999px;
    background: var(--surface-3);
    font-size: 13px;
    color: var(--text-2);
  }
  .pstat b { font-size: 14px; color: var(--text); }
  .pstat.warn { background: rgba(180, 83, 9, 0.12); color: var(--warn, #b45309); }
  .pstat.warn b { color: inherit; }
  /* 明细可滚动：一次导入最多回报 50 条拒收，不撑爆页面 */
  .plist {
    margin: 10px 0 0;
    padding-left: 18px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--text-2);
    max-height: 220px;
    overflow-y: auto;
  }

  /* 说明卡并排（支持的文件 / 关于去重）。min() 兜底：容器窄于 320px 时轨道下限
     降为容器宽度，避免 auto-fit 的硬下限把窄屏顶破；窄于 ~660px 自动回退单列。 */
  .refs-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
    align-items: start;
    gap: 16px;
    margin-top: 16px;
  }
  /* 网格自己管间距，避免与 .sec 的 margin-top 叠加（不塌陷，会变成双倍）。 */
  .refs-grid .sec { margin-top: 0; }
  code { background: var(--surface-3); padding: 1px 6px; border-radius: 5px; font-size: 12.5px; }
  .flink { color: var(--accent); font-weight: 600; margin-left: 6px; }

  .trash-list { border: 1px solid var(--border); border-radius: var(--radius, 10px); overflow: hidden; }
  .trash-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .trash-row:last-child { border-bottom: none; }
  .trash-name { font-weight: 500; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
  .trash-meta { color: var(--text-3); font-size: 12px; flex: none; }
  .trash-ops { display: flex; gap: 6px; flex: none; }
</style>
