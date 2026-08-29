<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';

  let file = $state(null);
  let result = $state(null);
  let error = $state('');
  let busy = $state(false);
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
    if (!confirm('彻底删除这条演出？此操作不可恢复。')) return;
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
    if (!confirm(`清空回收站（${trashTotal} 条）？此操作不可恢复。`)) return;
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

  function onFile(e) {
    file = e.target.files?.[0] || null;
    result = null;
    error = '';
  }

  function onDrop(e) {
    e.preventDefault();
    dragover = false;
    const f = e.dataTransfer?.files?.[0];
    if (f) {
      file = f;
      result = null;
      error = '';
    }
  }

  async function runImport() {
    if (!file) {
      error = '请选择文件（.json 或 .zip 压缩包）';
      return;
    }
    busy = true;
    error = '';
    try {
      result = await api.importRecords(file);
    } catch (e) {
      error = e.message;
    } finally {
      busy = false;
    }
  }

  onMount(loadTrash);
</script>
<svelte:head><title>导入 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>导入 / 导出</h1>
    <p class="sub">上传「记录现场」导出的 JI_LU_XIAN_CHANG.android.zip（内含数据与封面），或单独的 data.json；记录按 id 覆盖更新，重复导入不会产生重复</p>
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
    <button class="btn primary lg" onclick={runImport} disabled={busy || !file}>
      {busy ? '导入中…' : '开始导入'}
    </button>
    {#if file && !busy}<button class="btn" onclick={() => (file = null)}>清除</button>{/if}
  </div>

  {#if busy}<p class="tiny warn-hint">导入进行中，请勿刷新或关闭页面、不要重复点击；大备份（含封面）可能需要数分钟。</p>{/if}

  {#if error}<div class="banner error">⚠ {error}</div>{/if}
  {#if result}
    <div class="banner success">
      ✓ 导入完成：记录 {result.records} 条，分类 {result.categories} 个
      {#if result.covers_imported != null}，封面关联 {result.covers_imported} 张{/if}
      {#if result.covers_missing}（缺失 {result.covers_missing} 张）{/if}。
      <a class="flink" href="/">查看记录 →</a>
    </div>
  {/if}

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

  <div class="card sec">
    <h3>回收站</h3>
    <p class="tiny">删除的演出在这里保留 {RETAIN_DAYS} 天，之后自动永久清除。</p>
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

  <div class="card sec">
    <h3>导出 / 备份</h3>
    <p class="tiny">可导出为单独 JSON，或打包为含封面的 zip（导入时可直接还原）。</p>
    <div class="btn-row">
      <a class="btn" href={api.getExportUrl('zip')}>⇩ 导出 ZIP（数据 + 封面）</a>
      <a class="btn" href={api.getExportUrl()}>⇩ 导出 JSON</a>
    </div>
  </div>
</div>

<style>
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

  .btn-row { display: flex; gap: 10px; margin-top: 14px; }
  .warn-hint { color: var(--text-muted); margin-top: 8px; }

  .sec { padding: 18px 20px; margin-top: 16px; }
  .sec h3 { margin: 0 0 10px; font-size: 15.5px; }
  .tips { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 6px; font-size: 13.5px; color: var(--text-2); }
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
