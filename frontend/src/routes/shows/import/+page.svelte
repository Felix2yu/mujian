<script>
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';

  let file = null;
  let importing = false;
  let result = null;
  let error = '';
  let dragover = false;

  function handleFileSelect(e) {
    file = e.target.files[0];
    result = null;
    error = '';
  }

  function handleDrop(e) {
    e.preventDefault();
    dragover = false;
    const droppedFile = e.dataTransfer.files[0];
    if (droppedFile && (droppedFile.name.endsWith('.xlsx') || droppedFile.name.endsWith('.xls'))) {
      file = droppedFile;
      result = null;
      error = '';
    } else {
      error = '请上传 .xlsx 或 .xls 文件';
    }
  }

  function handleDragOver(e) {
    e.preventDefault();
    dragover = true;
  }

  function handleDragLeave() {
    dragover = false;
  }

  async function handleImport() {
    if (!file) {
      error = '请先选择文件';
      return;
    }

    importing = true;
    error = '';
    result = null;

    try {
      result = await api.importShows(file);
    } catch (e) {
      error = e.message;
    } finally {
      importing = false;
    }
  }

  function downloadTemplate() {
    window.open(api.getImportTemplate(), '_blank');
  }
</script>

<div class="import-page">
  <h1>批量导入演出</h1>

  <div class="section">
    <h2>导入说明</h2>
    <div class="instructions">
      <p>支持从 Excel 文件 (.xlsx) 批量导入演出数据。文件需包含表头行，以下列名均可识别：</p>
      <div class="columns-grid">
        <div class="col-group">
          <strong>必填</strong>
          <ul>
            <li>名称 / name / title</li>
          </ul>
        </div>
        <div class="col-group">
          <strong>可选</strong>
          <ul>
            <li>场地 / venue</li>
            <li>日期 / date（支持多种格式）</li>
            <li>时长 / duration（分钟）</li>
            <li>状态 / status（计划中/已观看/已取消）</li>
            <li>分类 / category（名称或ID）</li>
            <li>剧团 / company</li>
            <li>阵容 / cast</li>
            <li>同行 / friends</li>
            <li>评分 / rating（1-5）</li>
            <li>座位 / seat</li>
            <li>门票 / ticket_cost</li>
            <li>其他花费 / other_cost</li>
            <li>剧目 / setlist</li>
            <li>剧评 / review</li>
            <li>备注 / notes</li>
          </ul>
        </div>
      </div>
      <p class="tip">
        <a href="javascript:void(0)" on:click={downloadTemplate}>下载导入模板</a>
        — 模板包含表头和示例数据
      </p>
    </div>
  </div>

  <div class="section">
    <h2>选择文件</h2>
    <div
      class="dropzone"
      class:dragover
      class:has-file={file}
      on:drop={handleDrop}
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
    >
      {#if file}
        <div class="file-info">
          <span class="file-icon">📄</span>
          <span class="file-name">{file.name}</span>
          <span class="file-size">({(file.size / 1024).toFixed(1)} KB)</span>
          <button class="btn-remove" on:click={() => { file = null; result = null; }}>×</button>
        </div>
      {:else}
        <div class="dropzone-content">
          <span class="dropzone-icon">📁</span>
          <p>拖拽文件到此处，或</p>
          <label class="btn-select">
            选择文件
            <input type="file" accept=".xlsx,.xls" on:change={handleFileSelect} hidden />
          </label>
        </div>
      {/if}
    </div>

    {#if error}
      <div class="error">{error}</div>
    {/if}

    {#if result}
      <div class="result" class:has-failed={result.failed > 0}>
        <h3>导入完成</h3>
        <div class="result-stats">
          <div class="stat">
            <span class="stat-value">{result.total}</span>
            <span class="stat-label">总计</span>
          </div>
          <div class="stat success">
            <span class="stat-value">{result.success}</span>
            <span class="stat-label">成功</span>
          </div>
          {#if result.failed > 0}
            <div class="stat failed">
              <span class="stat-value">{result.failed}</span>
              <span class="stat-label">失败</span>
            </div>
          {/if}
        </div>
        {#if result.errors && result.errors.length > 0}
          <div class="error-list">
            <strong>错误详情：</strong>
            <ul>
              {#each result.errors as err}
                <li>{err}</li>
              {/each}
            </ul>
          </div>
        {/if}
        <div class="result-actions">
          <a href="/shows" class="btn-view">查看演出列表</a>
          <button class="btn-import-more" on:click={() => { file = null; result = null; error = ''; }}>继续导入</button>
        </div>
      </div>
    {/if}

    <div class="actions">
      <button class="btn-import" on:click={handleImport} disabled={!file || importing}>
        {importing ? '导入中...' : '开始导入'}
      </button>
    </div>
  </div>
</div>

<style>
  .import-page {
    max-width: 700px;
    margin: 0 auto;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 24px;
  }

  .section {
    background: #fff;
    border-radius: 12px;
    padding: 24px;
    margin-bottom: 20px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  h2 {
    font-size: 18px;
    font-weight: 600;
    margin-bottom: 16px;
  }

  .instructions p {
    margin-bottom: 12px;
    color: #666;
    line-height: 1.6;
  }

  .columns-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    margin-bottom: 16px;
  }

  .col-group strong {
    display: block;
    margin-bottom: 8px;
    color: #333;
  }

  .col-group ul {
    list-style: none;
    padding: 0;
  }

  .col-group li {
    font-size: 13px;
    color: #666;
    padding: 2px 0;
  }

  .tip {
    font-size: 13px;
    color: #4A90D9;
  }

  .tip a {
    text-decoration: underline;
    cursor: pointer;
  }

  .dropzone {
    border: 2px dashed #ddd;
    border-radius: 12px;
    padding: 40px 20px;
    text-align: center;
    transition: border-color 0.2s, background 0.2s;
    cursor: pointer;
  }

  .dropzone:hover, .dropzone.dragover {
    border-color: #4A90D9;
    background: #f0f7ff;
  }

  .dropzone.has-file {
    border-color: #27AE60;
    background: #f0fff4;
  }

  .dropzone-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .dropzone-icon {
    font-size: 48px;
  }

  .dropzone-content p {
    color: #666;
    margin: 0;
  }

  .btn-select {
    display: inline-block;
    padding: 8px 20px;
    background: #4A90D9;
    color: #fff;
    border-radius: 8px;
    cursor: pointer;
    font-weight: 500;
    transition: background 0.2s;
  }

  .btn-select:hover {
    background: #3a7bc8;
  }

  .file-info {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }

  .file-icon {
    font-size: 24px;
  }

  .file-name {
    font-weight: 500;
    color: #333;
  }

  .file-size {
    color: #999;
    font-size: 13px;
  }

  .btn-remove {
    color: #c00;
    font-size: 20px;
    padding: 4px 8px;
  }

  .btn-remove:hover {
    background: #fee;
    border-radius: 4px;
  }

  .error {
    margin-top: 12px;
    padding: 12px 16px;
    background: #f8d7da;
    color: #721c24;
    border-radius: 8px;
  }

  .result {
    margin-top: 16px;
    padding: 20px;
    background: #f0fff4;
    border-radius: 12px;
    border: 1px solid #d4edda;
  }

  .result.has-failed {
    background: #fff3cd;
    border-color: #ffc107;
  }

  .result h3 {
    margin-bottom: 16px;
    font-size: 16px;
  }

  .result-stats {
    display: flex;
    gap: 24px;
    margin-bottom: 16px;
  }

  .stat {
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .stat-value {
    font-size: 28px;
    font-weight: 700;
    color: #333;
  }

  .stat.success .stat-value {
    color: #27AE60;
  }

  .stat.failed .stat-value {
    color: #c00;
  }

  .stat-label {
    font-size: 12px;
    color: #666;
  }

  .error-list {
    margin-top: 12px;
    padding: 12px;
    background: rgba(255,255,255,0.7);
    border-radius: 8px;
    max-height: 200px;
    overflow-y: auto;
  }

  .error-list strong {
    display: block;
    margin-bottom: 8px;
    font-size: 13px;
  }

  .error-list ul {
    margin: 0;
    padding-left: 20px;
  }

  .error-list li {
    font-size: 12px;
    color: #666;
    margin-bottom: 4px;
  }

  .result-actions {
    display: flex;
    gap: 12px;
    margin-top: 16px;
  }

  .btn-view {
    padding: 8px 20px;
    background: #4A90D9;
    color: #fff;
    border-radius: 8px;
    font-weight: 500;
  }

  .btn-view:hover {
    background: #3a7bc8;
  }

  .btn-import-more {
    padding: 8px 20px;
    background: #f0f0f0;
    color: #666;
    border-radius: 8px;
    font-weight: 500;
  }

  .btn-import-more:hover {
    background: #e0e0e0;
  }

  .actions {
    margin-top: 20px;
  }

  .btn-import {
    padding: 10px 32px;
    background: #27AE60;
    color: #fff;
    border-radius: 8px;
    font-weight: 500;
    font-size: 15px;
    transition: background 0.2s;
  }

  .btn-import:hover:not(:disabled) {
    background: #219a52;
  }

  .btn-import:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  @media (max-width: 768px) {
    .import-page {
      padding: 0;
    }

    .columns-grid {
      grid-template-columns: 1fr;
    }

    .dropzone {
      padding: 30px 16px;
    }

    .result-stats {
      flex-wrap: wrap;
      gap: 16px;
    }

    .result-actions {
      flex-direction: column;
    }

    .result-actions a,
    .result-actions button {
      width: 100%;
      text-align: center;
    }
  }

  :global(.dark) .btn-import-more { background: #333; color: #ccc; }
  :global(.dark) .btn-import-more:hover { background: #444; }
  :global(.dark) h2 { color: #e0e0e0; }
  :global(.dark) .col-group strong { color: #e0e0e0; }
  :global(.dark) .col-group li { color: #999; }
  :global(.dark) .dropzone { border-color: #444; }
  :global(.dark) .dropzone:hover, :global(.dark) .dropzone.dragover { background: #1a2a3a; }
  :global(.dark) .instructions p { color: #999; }
  :global(.dark) .result-stats { color: #999; }
</style>
