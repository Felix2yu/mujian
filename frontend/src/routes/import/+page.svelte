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

<div class="fade-up">
  <div class="page-head">
    <h1>导入 / 导出</h1>
    <p class="sub">上传「记录现场」导出的压缩包（.zip，含 data.json 与 covers/），或单独的 data.json；记录按 id 覆盖更新</p>
  </div>

  <div
    class="card dropzone"
    class:dragover
    on:dragover|preventDefault={() => (dragover = true)}
    on:dragleave={() => (dragover = false)}
    on:drop={onDrop}
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
    <input class="dz-input" type="file" accept=".json,.zip" on:change={onFile} title="选择文件" />
  </div>

  <div class="btn-row">
    <button class="btn primary lg" on:click={runImport} disabled={busy || !file}>
      {busy ? '导入中…' : '开始导入'}
    </button>
    {#if file}<button class="btn" on:click={() => (file = null)}>清除</button>{/if}
  </div>

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
    <h3>说明</h3>
    <ul class="tips">
      <li><b>推荐</b>：直接上传「记录现场」导出的 <code>JI_LU_XIAN_CHANG.android.zip</code>，自动解压 <code>JI_LU_XIAN_CHANG.android</code>（raw-deflate JSON）并解码 <code>covers/</code> 里的 base64 封面，一键完成数据 + 封面关联</li>
      <li>也支持已转换的 <code>data.json</code> 或含 <code>data.json + covers/</code> 的 zip</li>
      <li>记录按 <code>id</code> 覆盖更新，重复导入不会产生重复数据</li>
      <li>封面文件落盘在数据目录的 <code>uploads/covers/</code> 下，Docker 卷挂载后重启不丢失</li>
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

  .sec { padding: 18px 20px; margin-top: 16px; }
  .sec h3 { margin: 0 0 10px; font-size: 15.5px; }
  .tips { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 6px; font-size: 13.5px; color: var(--text-2); }
  code { background: var(--surface-3); padding: 1px 6px; border-radius: 5px; font-size: 12.5px; }
  .flink { color: var(--accent); font-weight: 600; margin-left: 6px; }
</style>
