<script>
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';

  let file = $state(null);
  let importing = $state(false);
  let result = $state(null);
  let error = $state('');
  let dragover = $state(false);

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
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
      <h2>导入说明</h2>
    </div>
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
            <li>状态 / status（正常/已取消/待开票/未赴约）</li>
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
        <a href="javascript:void(0)" onclick={downloadTemplate}>下载导入模板</a>
        — 模板包含表头和示例数据
      </p>
    </div>
  </div>

  <div class="section">
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>
      <h2>选择文件</h2>
    </div>
    <div
      class="dropzone"
      class:dragover
      class:has-file={file}
      ondrop={handleDrop}
      ondragover={handleDragOver}
      ondragleave={handleDragLeave}
    >
      {#if file}
        <div class="file-info">
          <div class="file-icon-wrap">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>
          </div>
          <div class="file-details">
            <span class="file-name">{file.name}</span>
            <span class="file-size">({(file.size / 1024).toFixed(1)} KB)</span>
          </div>
          <button class="btn-remove" onclick={() => { file = null; result = null; }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      {:else}
        <div class="dropzone-content">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          <p>拖拽文件到此处，或</p>
          <label class="btn-select">
            选择文件
            <input type="file" accept=".xlsx,.xls" onchange={handleFileSelect} hidden />
          </label>
        </div>
      {/if}
    </div>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    {#if result}
      <div class="result" class:has-failed={result.failed > 0}>
        <h3>导入完成</h3>
        <div class="result-stats">
          <div class="result-stat">
            <span class="result-stat-value">{result.total}</span>
            <span class="result-stat-label">总计</span>
          </div>
          <div class="result-stat success">
            <span class="result-stat-value">{result.success}</span>
            <span class="result-stat-label">成功</span>
          </div>
          {#if result.failed > 0}
            <div class="result-stat failed">
              <span class="result-stat-value">{result.failed}</span>
              <span class="result-stat-label">失败</span>
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
          <a href="/shows" class="primary-btn">查看演出列表</a>
          <button class="secondary-btn" onclick={() => { file = null; result = null; error = ''; }}>继续导入</button>
        </div>
      </div>
    {/if}

    <div class="actions">
      <button class="import-btn" onclick={handleImport} disabled={!file || importing}>
        {#if importing}
          <div class="spinner"></div>
          导入中...
        {:else}
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          开始导入
        {/if}
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
    margin-bottom: 28px;
    letter-spacing: -0.02em;
  }

  .section {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 24px;
    margin-bottom: 16px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 20px;
  }

  .section-header svg {
    color: var(--accent);
  }

  h2 {
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .instructions p {
    margin-bottom: 12px;
    color: var(--text-secondary);
    line-height: 1.6;
  }

  .columns-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 20px;
    margin-bottom: 16px;
  }

  .col-group strong {
    display: block;
    margin-bottom: 8px;
    color: var(--text-primary);
    font-size: 13px;
  }

  .col-group ul {
    list-style: none;
    padding: 0;
  }

  .col-group li {
    font-size: 13px;
    color: var(--text-secondary);
    padding: 2px 0;
  }

  .tip {
    font-size: 13px;
    color: var(--accent);
    font-weight: 500;
  }

  .tip a {
    text-decoration: underline;
    cursor: pointer;
  }

  .dropzone {
    border: 2px dashed var(--border);
    border-radius: var(--radius-md);
    padding: 40px 20px;
    text-align: center;
    transition: all 0.2s ease;
    cursor: pointer;
  }

  .dropzone:hover, .dropzone.dragover {
    border-color: var(--accent);
    background: var(--accent-bg);
  }

  .dropzone.has-file {
    border-color: var(--success);
    background: var(--success-bg);
    border-style: solid;
  }

  .dropzone-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .dropzone-content svg {
    color: var(--text-muted);
    opacity: 0.4;
  }

  .dropzone-content p {
    color: var(--text-secondary);
    margin: 0;
    font-size: 14px;
  }

  .btn-select {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 20px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-weight: 500;
    font-size: 13px;
    transition: all 0.2s;
  }

  .btn-select:hover {
    background: var(--accent-light);
  }

  .file-info {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
  }

  .file-icon-wrap {
    width: 44px;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--accent-bg);
    border-radius: var(--radius-sm);
    color: var(--accent);
  }

  .file-details {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }

  .file-name {
    font-weight: 600;
    color: var(--text-primary);
    font-size: 14px;
  }

  .file-size {
    color: var(--text-muted);
    font-size: 12px;
  }

  .btn-remove {
    color: var(--text-muted);
    padding: 6px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    transition: all 0.15s;
  }

  .btn-remove:hover {
    background: var(--danger-bg);
    color: var(--danger-text);
  }

  .error-msg {
    margin-top: 12px;
    padding: 12px 16px;
    background: var(--danger-bg);
    color: var(--danger-text);
    border-radius: var(--radius-sm);
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .error-msg::before {
    content: '⚠';
  }

  .result {
    margin-top: 16px;
    padding: 24px;
    background: var(--success-bg);
    border-radius: var(--radius-md);
    border: 1px solid var(--success);
  }

  .result.has-failed {
    background: var(--warning-bg);
    border-color: var(--warning);
  }

  .result h3 {
    margin-bottom: 16px;
    font-size: 16px;
    font-weight: 600;
  }

  .result-stats {
    display: flex;
    gap: 24px;
    margin-bottom: 16px;
  }

  .result-stat {
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .result-stat-value {
    font-size: 28px;
    font-weight: 700;
    color: var(--text-primary);
    letter-spacing: -0.02em;
  }

  .result-stat.success .result-stat-value {
    color: var(--success);
  }

  .result-stat.failed .result-stat-value {
    color: var(--danger-text);
  }

  .result-stat-label {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .error-list {
    margin-top: 12px;
    padding: 16px;
    background: rgba(255,255,255,0.5);
    border-radius: var(--radius-sm);
    max-height: 200px;
    overflow-y: auto;
  }

  .error-list strong {
    display: block;
    margin-bottom: 8px;
    font-size: 13px;
    color: var(--text-primary);
  }

  .error-list ul {
    margin: 0;
    padding-left: 20px;
  }

  .error-list li {
    font-size: 12px;
    color: var(--text-secondary);
    margin-bottom: 4px;
  }

  .result-actions {
    display: flex;
    gap: 12px;
    margin-top: 16px;
  }

  .primary-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 20px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 13px;
    text-decoration: none;
    transition: all 0.2s;
  }

  .primary-btn:hover {
    background: var(--accent-light);
  }

  .secondary-btn {
    padding: 8px 20px;
    background: var(--bg-surface);
    color: var(--text-secondary);
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 13px;
    border: 1px solid var(--border);
    transition: all 0.2s;
  }

  .secondary-btn:hover {
    background: var(--bg-surface-hover);
  }

  .actions {
    margin-top: 20px;
  }

  .import-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 10px 32px;
    background: var(--success);
    color: #fff;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 14px;
    transition: all 0.2s;
  }

  .import-btn:hover:not(:disabled) {
    opacity: 0.9;
    transform: translateY(-1px);
  }

  .import-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .spinner {
    width: 16px;
    height: 16px;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: #fff;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

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
      justify-content: center;
    }
  }
</style>
