<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { theme } from '$lib/stores';

  let settings = $state({
    theme: 'auto',
    allow_local_storage: true,
    storage_type: 'local'
  });
  let categories = $state([]);
  let saving = $state(false);
  let message = $state('');
  let newCatName = $state('');
  let dragIndex = $state(null);
  let restoring = $state(false);
  let restoreResult = $state(null);
  let sceneSorts = $state([]);
  let expandedPlay = $state('');
  let sceneDragIndex = $state(null);

  onMount(async () => {
    const [s, c, ss] = await Promise.all([api.getSettings(), api.listCategories(), api.getSceneSorts()]);
    settings = s;
    categories = c || [];
    sceneSorts = ss || [];
  });

  function extractPlaysFromSetlists(shows) {
    const playScenes = {};
    shows.forEach(show => {
      if (!show.setlist) return;
      show.setlist.split('\n').forEach(line => {
        const trimmed = line.trim();
        if (!trimmed) return;
        const idx = trimmed.indexOf('•');
        if (idx === -1) {
          if (!playScenes[trimmed]) playScenes[trimmed] = new Set();
        } else {
          const play = trimmed.substring(0, idx).trim();
          if (!playScenes[play]) playScenes[play] = new Set();
          trimmed.substring(idx + 1).split('•').forEach(s => {
            const scene = s.trim();
            if (scene) playScenes[play].add(scene);
          });
        }
      });
    });
    return playScenes;
  }

  function applySortPrefs(playScenes) {
    const sortMap = {};
    sceneSorts.forEach(ss => {
      try { sortMap[ss.play] = JSON.parse(ss.scenes); } catch {}
    });
    const result = {};
    Object.entries(playScenes).forEach(([play, scenes]) => {
      const arr = [...scenes];
      const sorted = sortMap[play];
      if (sorted && Array.isArray(sorted)) {
        const sortedSet = new Set(sorted);
        const preferred = sorted.filter(s => arr.includes(s));
        arr.forEach(s => { if (!sortedSet.has(s)) preferred.push(s); });
        result[play] = preferred;
      } else {
        result[play] = arr;
      }
    });
    return result;
  }

  let scenePlayData = $state({});
  let sortedPlays = $state([]);

  async function loadSceneSorts() {
    const [shows, sorts] = await Promise.all([api.listAllShows(), api.getSceneSorts()]);
    sceneSorts = sorts || [];
    const raw = extractPlaysFromSetlists(shows || []);
    scenePlayData = applySortPrefs(raw);
    sortedPlays = Object.keys(scenePlayData).sort();
  }

  async function saveSceneSort(play) {
    const scenes = scenePlayData[play] || [];
    try {
      await api.updateSceneSort(play, JSON.stringify(scenes));
    } catch (e) {
      alert('保存失败: ' + e.message);
    }
  }

  function moveScene(play, fromIdx, toIdx) {
    const scenes = [...scenePlayData[play]];
    const [moved] = scenes.splice(fromIdx, 1);
    scenes.splice(toIdx, 0, moved);
    scenePlayData = { ...scenePlayData, [play]: scenes };
    saveSceneSort(play);
  }

  function sceneDragStart(e, play, index) {
    sceneDragIndex = { play, index };
    e.dataTransfer.effectAllowed = 'move';
    e.target.style.opacity = '0.5';
  }

  function sceneDragEnd(e) {
    sceneDragIndex = null;
    e.target.style.opacity = '1';
  }

  function sceneDragOver(e, play, index) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  }

  function sceneDrop(e, play, dropIndex) {
    e.preventDefault();
    if (!sceneDragIndex || sceneDragIndex.play !== play) return;
    moveScene(play, sceneDragIndex.index, dropIndex);
    sceneDragIndex = null;
  }

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
      const cat = await api.createCategory({ name: newCatName });
      categories = [...categories, cat];
      newCatName = '';
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

  let canUseLocal = $derived(settings.allow_local_storage);
  let isS3 = $derived(settings.storage_type === 's3');
</script>

<div class="settings-page">
  <h1>设置</h1>

  {#if message}
    <div class="message" class:error={message.includes('失败')}>{message}</div>
  {/if}

  <div class="section">
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
      <h2>外观</h2>
    </div>
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
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
      <h2>海报存储</h2>
    </div>
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

    <button class="primary-btn" onclick={saveSettings} disabled={saving}>
      {saving ? '保存中...' : '保存设置'}
    </button>
  </div>

  <div class="section">
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
      <h2>分类管理</h2>
    </div>
    <div class="categories-list">
      {#each categories as cat, index (cat.id)}
        <div class="cat-item"
          draggable="true"
          ondragstart={(e) => handleDragStart(e, index)}
          ondragend={handleDragEnd}
          ondragover={(e) => handleDragOver(e, index)}
          ondragleave={handleDragLeave}
          ondrop={(e) => handleDrop(e, index)}
        >
          <span class="drag-handle">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="9" cy="5" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="15" cy="19" r="1"/></svg>
          </span>
          <input type="text" bind:value={cat.name} onblur={() => updateCategory(cat)} class="cat-name" />
          <span class="cat-count">{cat.show_count} 场</span>
          <button class="btn-delete" onclick={() => deleteCategory(cat.id)}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      {/each}
    </div>
    <div class="add-cat">
      <input type="text" bind:value={newCatName} placeholder="新分类名称" onkeydown={(e) => e.key === 'Enter' && addCategory()} />
      <button class="add-btn" onclick={addCategory}>添加</button>
    </div>
  </div>

  <div class="section">
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
      <h2>折子排序</h2>
      <button class="load-scenes-btn" onclick={loadSceneSorts}>加载剧目数据</button>
    </div>
    {#if sortedPlays.length === 0}
      <p class="backup-desc">点击"加载剧目数据"从所有演出中提取剧目和折子信息。</p>
    {:else}
      <div class="scene-plays">
        {#each sortedPlays as play}
          <div class="scene-play-item">
            <button class="scene-play-header" onclick={() => expandedPlay = expandedPlay === play ? '' : play}>
              <svg class="play-arrow" class:expanded={expandedPlay === play} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
              <span class="scene-play-name">{play}</span>
              <span class="scene-play-count">{scenePlayData[play]?.length || 0} 折</span>
            </button>
            {#if expandedPlay === play}
              <div class="scene-drag-list">
                {#each (scenePlayData[play] || []) as scene, idx}
                  <div
                    class="scene-drag-item"
                    draggable="true"
                    ondragstart={(e) => sceneDragStart(e, play, idx)}
                    ondragend={sceneDragEnd}
                    ondragover={(e) => sceneDragOver(e, play, idx)}
                    ondrop={(e) => sceneDrop(e, play, idx)}
                  >
                    <span class="scene-drag-handle">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="9" cy="5" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="15" cy="19" r="1"/></svg>
                    </span>
                    <span class="scene-drag-name">{scene}</span>
                    <span class="scene-drag-idx">{idx + 1}</span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="section">
    <div class="section-header">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
      <h2>数据备份</h2>
    </div>
    <p class="backup-desc">备份包含所有分类和演出数据，可用于迁移或恢复。</p>
    <div class="backup-actions">
      <a href={api.getBackupUrl()} class="backup-btn" download>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        下载备份
      </a>
      <div class="restore-area">
        <label class="restore-btn">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
          恢复备份
          <input type="file" accept=".json" onchange={handleRestore} hidden />
        </label>
        {#if restoring}
          <span class="restore-status">恢复中...</span>
        {:else if restoreResult}
          <span class="restore-status success">已恢复 {restoreResult.categories} 个分类、{restoreResult.shows} 场演出{#if restoreResult.skipped > 0}（跳过 {restoreResult.skipped} 场重复）{/if}</span>
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
    margin-bottom: 28px;
    letter-spacing: -0.02em;
  }

  .message {
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    background: var(--success-bg);
    color: var(--success);
    margin-bottom: 20px;
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .message::before { content: '✓'; font-weight: 700; }

  .message.error {
    background: var(--danger-bg);
    color: var(--danger-text);
  }

  .message.error::before { content: '✕'; }

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

  .form-group {
    margin-bottom: 16px;
  }

  .form-group label {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-muted);
    margin-bottom: 8px;
  }

  .form-group select, .form-group input {
    width: 100%;
    max-width: 400px;
  }

  .hint {
    display: block;
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 6px;
  }

  .s3-form {
    margin-top: 16px;
    padding: 20px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
  }

  .form-row {
    display: flex;
    gap: 16px;
  }

  .form-row .form-group {
    flex: 1;
  }

  .primary-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 10px 24px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 14px;
    margin-top: 8px;
    transition: all 0.2s;
  }

  .primary-btn:hover:not(:disabled) {
    background: var(--accent-light);
    transform: translateY(-1px);
  }

  .primary-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .categories-list {
    margin-bottom: 16px;
  }

  .cat-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 0;
    cursor: grab;
    transition: all 0.2s;
    border-bottom: 1px solid var(--border);
  }

  .cat-item:last-child {
    border-bottom: none;
  }

  .cat-item.drag-over {
    border-top: 2px solid var(--accent);
  }

  .cat-item:active {
    cursor: grabbing;
  }

  .drag-handle {
    color: var(--text-muted);
    cursor: grab;
    user-select: none;
    display: flex;
    align-items: center;
    padding: 4px;
    border-radius: 4px;
    transition: all 0.15s;
  }

  .drag-handle:hover {
    color: var(--text-secondary);
    background: var(--bg-surface);
  }

  .cat-count {
    font-size: 12px;
    color: var(--text-muted);
    min-width: 40px;
    text-align: right;
    font-weight: 500;
  }

  .cat-name {
    flex: 1;
    border: none;
    background: transparent;
    font-size: 14px;
    padding: 4px 8px;
    border-radius: 4px;
    transition: all 0.15s;
    color: var(--text-primary);
  }

  .cat-name:hover {
    background: var(--bg-surface);
  }

  .cat-name:focus {
    outline: 2px solid var(--accent);
    background: var(--bg-input);
  }

  .btn-delete {
    color: var(--text-muted);
    padding: 4px 8px;
    border-radius: 4px;
    transition: all 0.15s;
    display: flex;
    align-items: center;
  }

  .btn-delete:hover {
    background: var(--danger-bg);
    color: var(--danger-text);
  }

  .add-cat {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .add-cat input[type="text"] {
    flex: 1;
  }

  .add-btn {
    padding: 8px 18px;
    background: var(--success);
    color: #fff;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 13px;
    transition: all 0.2s;
  }

  .add-btn:hover {
    opacity: 0.9;
    transform: translateY(-1px);
  }

  .load-scenes-btn {
    margin-left: auto;
    padding: 6px 14px;
    font-size: 12px;
    font-weight: 500;
    background: var(--bg-surface);
    color: var(--text-secondary);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    transition: all 0.2s;
  }
  .load-scenes-btn:hover { background: var(--bg-surface-hover); }
  .scene-plays { display: flex; flex-direction: column; gap: 4px; }
  .scene-play-item { border-bottom: 1px solid var(--border); }
  .scene-play-item:last-child { border-bottom: none; }
  .scene-play-header {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 10px 0;
    background: none;
    border: none;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-primary);
    transition: color 0.15s;
  }
  .scene-play-header:hover { color: var(--accent); }
  .play-arrow { transition: transform 0.2s; color: var(--text-muted); flex-shrink: 0; }
  .play-arrow.expanded { transform: rotate(90deg); }
  .scene-play-name { flex: 1; text-align: left; }
  .scene-play-count { font-size: 12px; color: var(--text-muted); font-weight: 400; }
  .scene-drag-list { padding: 0 0 8px 22px; }
  .scene-drag-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    border-radius: var(--radius-sm);
    cursor: grab;
    transition: background 0.15s;
    font-size: 13px;
  }
  .scene-drag-item:hover { background: var(--bg-surface); }
  .scene-drag-item:active { cursor: grabbing; }
  .scene-drag-handle { color: var(--text-muted); display: flex; align-items: center; }
  .scene-drag-name { flex: 1; }
  .scene-drag-idx { font-size: 11px; color: var(--text-muted); min-width: 20px; text-align: right; }
  .backup-desc {
    font-size: 13px;
    color: var(--text-muted);
    margin-bottom: 16px;
    line-height: 1.6;
  }

  .backup-actions {
    display: flex;
    gap: 12px;
    align-items: center;
  }

  .backup-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 10px 24px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 14px;
    text-decoration: none;
    transition: all 0.2s;
  }

  .backup-btn:hover {
    background: var(--accent-light);
    transform: translateY(-1px);
  }

  .restore-area {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .restore-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 10px 24px;
    background: var(--bg-surface);
    color: var(--text-secondary);
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 14px;
    cursor: pointer;
    transition: all 0.2s;
    border: 1px solid var(--border);
  }

  .restore-btn:hover {
    background: var(--bg-surface-hover);
  }

  .restore-status {
    font-size: 13px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .restore-status.success {
    color: var(--success);
  }

  @media (max-width: 768px) {
    .settings-page {
      padding: 0;
    }

    h1 {
      font-size: 20px;
    }

    .section {
      padding: 16px;
      margin-bottom: 12px;
    }

    .form-group select, .form-group input {
      max-width: 100%;
    }

    .s3-form .form-row {
      flex-direction: column;
      gap: 0;
    }

    .cat-item {
      padding: 10px 0;
    }

    .add-cat {
      flex-wrap: wrap;
    }

    .add-cat input[type="text"] {
      flex: 1 1 100%;
    }

    .backup-actions {
      flex-direction: column;
      align-items: stretch;
    }

    .backup-btn, .restore-btn {
      justify-content: center;
    }
  }

  @media (max-width: 600px) {
    .form-row {
      flex-direction: column;
      gap: 0;
    }
  }
</style>
