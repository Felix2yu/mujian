<script>
  import { api } from '$lib/api.js';

  let file = $state(null);
  let result = $state(null);
  let error = $state('');
  let busy = $state(false);
  let dragover = $state(false);

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
    transition: all var(--t-med) var(--ease);
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
</style>
