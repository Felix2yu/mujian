<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { theme } from '$lib/stores';

  let settings = {
    theme: 'auto',
    storage_type: 'local',
    allow_local_storage: true,
    s3_endpoint: '',
    s3_bucket: '',
    s3_region: 'us-east-1',
    s3_access_key: '',
    s3_secret_key: '',
    s3_public_url: ''
  };

  let categories = [];
  let saving = false;
  let message = '';
  let newCatName = '';
  let newCatColor = '#4A90D9';
  let dragIndex = null;
  let restoring = false;
  let restoreResult = null;

  onMount(async () => {
    const [s, c] = await Promise.all([api.getSettings(), api.listCategories()]);
    settings = s;
    categories = c || [];
  });

  async function handleRestore(e) {
    const file = e.target.files[0];
    if (!file) return;

    if (!confirm('恢复将追加数据到现有记录，确定继续吗？')) {
      e.target.value = '';
      return;
    }

    restoring = true;
    restoreResult = null;
    try {
      restoreResult = await api.restoreBackup(file);
      const c = await api.listCategories();
      categories = c || [];
    } catch (err) {
      alert('恢复失败: ' + err.message);
    } finally {
      restoring = false;
      e.target.value = '';
    }
  }

  async function saveSettings() {
    saving = true;
    message = '';
    try {
      const res = await api.updateSettings(settings);
      settings = res;
      if (settings.theme) {
        theme.set(settings.theme);
      }
      message = '设置已保存';
    } catch (e) {
      message = '保存失败: ' + e.message;
    } finally {
      saving = false;
    }
  }

  async function addCategory() {
    if (!newCatName.trim()) return;
    try {
      const cat = await api.createCategory({ name: newCatName, color: newCatColor });
      categories = [...categories, cat];
      newCatName = '';
      newCatColor = '#4A90D9';
    } catch (e) {
      alert('添加失败: ' + e.message);
    }
  }

  async function updateCategory(cat) {
    try {
      await api.updateCategory(cat.id, { name: cat.name, color: cat.color });
    } catch (e) {
      alert('更新失败: ' + e.message);
    }
  }

  async function deleteCategory(id) {
    const cat = categories.find(c => c.id === id);
    if (cat && cat.show_count > 0 && !confirm(`该分类下有 ${cat.show_count} 场演出，确定删除吗？`)) return;
    try {
      await api.deleteCategory(id);
      categories = categories.filter(c => c.id !== id);
    } catch (e) {
      alert('删除失败: ' + e.message);
    }
  }

  function handleDragStart(e, index) {
    dragIndex = index;
    e.dataTransfer.effectAllowed = 'move';
    e.target.style.opacity = '0.5';
  }

  function handleDragEnd(e) {
    dragIndex = null;
    e.target.style.opacity = '1';
    document.querySelectorAll('.cat-item').forEach(el => {
      el.classList.remove('drag-over');
    });
  }

  function handleDragOver(e, index) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (index !== dragIndex) {
      e.target.closest('.cat-item')?.classList.add('drag-over');
    }
  }

  function handleDragLeave(e) {
    e.target.closest('.cat-item')?.classList.remove('drag-over');
  }

  async function handleDrop(e, dropIndex) {
    e.preventDefault();
    e.target.closest('.cat-item')?.classList.remove('drag-over');

    if (dragIndex === null || dragIndex === dropIndex) return;

    const newCategories = [...categories];
    const [moved] = newCategories.splice(dragIndex, 1);
    newCategories.splice(dropIndex, 0, moved);
    categories = newCategories;

    try {
      await api.updateCategorySort(categories.map(c => c.id));
    } catch (err) {
      alert('排序保存失败: ' + err.message);
    }
  }

  $: canUseLocal = settings.allow_local_storage;
  $: isS3 = settings.storage_type === 's3';
</script>

<div class="settings-page">
  <h1>设置</h1>

  {#if message}
    <div class="message" class:error={message.includes('失败')}>{message}</div>
  {/if}

  <div class="section">
    <h2>外观</h2>
    <div class="form-group">
      <label for="theme">主题</label>
      <select id="theme" bind:value={settings.theme}>
        <option value="light">亮色</option>
        <option value="dark">暗色</option>
        <option value="auto">跟随系统</option>
      </select>
    </div>
  </div>

  <div class="section">
    <h2>海报存储</h2>
    <div class="form-group">
      <label for="storage">存储方式</label>
      <select id="storage" bind:value={settings.storage_type} disabled={!canUseLocal && settings.storage_type !== 's3'}>
        {#if canUseLocal}
          <option value="local">本地磁盘</option>
        {/if}
        <option value="s3">S3 兼容 OSS</option>
      </select>
      {#if !canUseLocal}
        <span class="hint">本地存储已被管理员禁用</span>
      {/if}
    </div>

    {#if isS3}
      <div class="s3-form">
        <div class="form-row">
          <div class="form-group">
            <label for="s3_endpoint">Endpoint</label>
            <input type="text" id="s3_endpoint" bind:value={settings.s3_endpoint} placeholder="https://s3.amazonaws.com" />
          </div>
          <div class="form-group">
            <label for="s3_bucket">Bucket</label>
            <input type="text" id="s3_bucket" bind:value={settings.s3_bucket} />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="s3_region">Region</label>
            <input type="text" id="s3_region" bind:value={settings.s3_region} />
          </div>
          <div class="form-group">
            <label for="s3_public_url">Public URL</label>
            <input type="text" id="s3_public_url" bind:value={settings.s3_public_url} placeholder="https://cdn.example.com" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="s3_access_key">Access Key</label>
            <input type="text" id="s3_access_key" bind:value={settings.s3_access_key} />
          </div>
          <div class="form-group">
            <label for="s3_secret_key">Secret Key</label>
            <input type="password" id="s3_secret_key" bind:value={settings.s3_secret_key} />
          </div>
        </div>
      </div>
    {/if}

    <button class="btn-save" on:click={saveSettings} disabled={saving}>
      {saving ? '保存中...' : '保存设置'}
    </button>
  </div>

  <div class="section">
    <h2>分类管理</h2>
    <div class="categories-list">
      {#each categories as cat, index (cat.id)}
        <div class="cat-item"
          draggable="true"
          on:dragstart={(e) => handleDragStart(e, index)}
          on:dragend={handleDragEnd}
          on:dragover={(e) => handleDragOver(e, index)}
          on:dragleave={handleDragLeave}
          on:drop={(e) => handleDrop(e, index)}
        >
          <span class="drag-handle">⠿</span>
          <input type="color" bind:value={cat.color} on:change={() => updateCategory(cat)} />
          <input type="text" bind:value={cat.name} on:blur={() => updateCategory(cat)} class="cat-name" />
          <span class="cat-count">{cat.show_count} 场</span>
          <button class="btn-delete" on:click={() => deleteCategory(cat.id)}>×</button>
        </div>
      {/each}
    </div>
    <div class="add-cat">
      <input type="color" bind:value={newCatColor} />
      <input type="text" bind:value={newCatName} placeholder="新分类名称" on:keydown={(e) => e.key === 'Enter' && addCategory()} />
      <button class="btn-add" on:click={addCategory}>添加</button>
    </div>
  </div>

  <div class="section">
    <h2>数据备份</h2>
    <p class="backup-desc">备份包含所有分类和演出数据，可用于迁移或恢复。</p>
    <div class="backup-actions">
      <a href={api.getBackupUrl()} class="btn-backup" download>📥 下载备份</a>
      <div class="restore-area">
        <label class="btn-restore">
          📤 恢复备份
          <input type="file" accept=".json" on:change={handleRestore} hidden />
        </label>
        {#if restoring}
          <span class="restore-status">恢复中...</span>
        {:else if restoreResult}
          <span class="restore-status success">已恢复 {restoreResult.categories} 个分类、{restoreResult.shows} 场演出</span>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .settings-page {
    max-width: 700px;
    margin: 0 auto;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 24px;
  }

  .message {
    padding: 12px 16px;
    border-radius: 8px;
    background: #d4edda;
    color: #155724;
    margin-bottom: 20px;
  }

  .message.error {
    background: #f8d7da;
    color: #721c24;
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

  .form-group {
    margin-bottom: 16px;
  }

  .form-group label {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: #666;
    margin-bottom: 6px;
  }

  .form-group select, .form-group input {
    width: 100%;
    max-width: 400px;
  }

  .hint {
    display: block;
    font-size: 12px;
    color: #999;
    margin-top: 4px;
  }

  .s3-form {
    margin-top: 16px;
    padding: 16px;
    background: #f8f8f8;
    border-radius: 8px;
  }

  .form-row {
    display: flex;
    gap: 16px;
  }

  .form-row .form-group {
    flex: 1;
  }

  .btn-save {
    padding: 10px 24px;
    background: #4A90D9;
    color: #fff;
    border-radius: 8px;
    font-weight: 500;
    margin-top: 8px;
    transition: background 0.2s;
  }

  .btn-save:hover:not(:disabled) {
    background: #3a7bc8;
  }

  .btn-save:disabled {
    opacity: 0.6;
  }

  .categories-list {
    margin-bottom: 16px;
  }

  .cat-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
    border-bottom: 1px solid #eee;
    cursor: grab;
    transition: background 0.2s;
  }

  .cat-item:last-child {
    border-bottom: none;
  }

  .cat-item.drag-over {
    border-top: 2px solid #4A90D9;
  }

  .cat-item:active {
    cursor: grabbing;
  }

  .drag-handle {
    color: #ccc;
    font-size: 16px;
    cursor: grab;
    user-select: none;
  }

  .drag-handle:hover {
    color: #999;
  }

  .cat-count {
    font-size: 12px;
    color: #999;
    min-width: 40px;
    text-align: right;
  }

  .cat-name {
    flex: 1;
    border: none;
    background: transparent;
    font-size: 15px;
  }

  .cat-name:focus {
    outline: 1px solid #4A90D9;
    border-radius: 4px;
    padding: 4px 8px;
  }

  .btn-delete {
    color: #c00;
    font-size: 18px;
    padding: 4px 8px;
  }

  .btn-delete:hover {
    background: #fee;
    border-radius: 4px;
  }

  .add-cat {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .add-cat input[type="text"] {
    flex: 1;
  }

  .btn-add {
    padding: 8px 16px;
    background: #27AE60;
    color: #fff;
    border-radius: 6px;
    font-weight: 500;
  }

  .btn-add:hover {
    background: #219a52;
  }

  .backup-desc {
    font-size: 13px;
    color: #666;
    margin-bottom: 16px;
  }

  .backup-actions {
    display: flex;
    gap: 16px;
    align-items: center;
  }

  .btn-backup {
    padding: 10px 24px;
    background: #4A90D9;
    color: #fff;
    border-radius: 8px;
    font-weight: 500;
    text-decoration: none;
    transition: background 0.2s;
  }

  .btn-backup:hover {
    background: #3a7bc8;
  }

  .restore-area {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .btn-restore {
    padding: 10px 24px;
    background: #f0f0f0;
    color: #333;
    border-radius: 8px;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.2s;
  }

  .btn-restore:hover {
    background: #e0e0e0;
  }

  .restore-status {
    font-size: 13px;
    color: #666;
  }

  .restore-status.success {
    color: #27AE60;
  }

  @media (max-width: 600px) {
    .form-row {
      flex-direction: column;
      gap: 0;
    }
  }
</style>
