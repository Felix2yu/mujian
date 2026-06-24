<script>
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let actor = $state(null);
  let shows = $state([]);
  let loading = $state(true);
  let editing = $state(false);
  let editBio = $state('');
  let editPhoto = $state('');
  let saving = $state(false);
  let photoFile = $state(null);
  let photoPreview = $state('');
  let dragover = $state(false);

  let name = $derived(decodeURIComponent($page.params.name));

  onMount(async () => {
    try {
      const [actorData, showsData] = await Promise.all([
        api.getActor(name).catch(() => null),
        api.getActorShows(name)
      ]);
      actor = actorData;
      shows = showsData;
      editBio = actor?.bio || '';
      editPhoto = actor?.photo_url || '';
      if (actor?.photo_url) photoPreview = actor.photo_url;
    } catch (e) {
      console.error('Failed to load actor:', e);
    } finally {
      loading = false;
    }
  });

  function startEdit() {
    editBio = actor?.bio || '';
    editPhoto = actor?.photo_url || '';
    photoFile = null;
    photoPreview = actor?.photo_url || '';
    editing = true;
  }

  function cancelEdit() {
    editing = false;
    photoFile = null;
    photoPreview = actor?.photo_url || '';
  }

  function handleFileSelect(e) {
    if (e.target.files[0]) processFile(e.target.files[0]);
  }

  function handleDrop(e) {
    e.preventDefault();
    dragover = false;
    const f = e.dataTransfer.files[0];
    if (f && f.type.startsWith('image/')) processFile(f);
  }

  function handleDragOver(e) {
    e.preventDefault();
    dragover = true;
  }

  function handleDragLeave() {
    dragover = false;
  }

  function processFile(file) {
    photoFile = file;
    const reader = new FileReader();
    reader.onload = (e) => { photoPreview = e.target.result; };
    reader.readAsDataURL(file);
  }

  function removePhoto() {
    photoFile = null;
    photoPreview = '';
    editPhoto = '';
  }

  async function saveEdit() {
    saving = true;
    try {
      if (photoFile) {
        const res = await api.uploadFile(photoFile);
        editPhoto = res.url;
      }
      actor = await api.updateActor(name, { name, bio: editBio, photo_url: editPhoto });
      editing = false;
    } catch (e) {
      alert('保存失败: ' + e.message);
    } finally {
      saving = false;
    }
  }
</script>

<div class="actor-detail">
  {#if loading}
    <div class="loading"><div class="spinner"></div>加载中...</div>
  {:else}
    <a href="/cast" class="back-link">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      返回演员列表
    </a>

    <div class="actor-card">
      <div class="actor-header">
        <div class="photo-section">
          {#if editing}
            <div
              class="photo-upload"
              class:dragover
              ondrop={handleDrop}
              ondragover={handleDragOver}
              ondragleave={handleDragLeave}
            >
              {#if photoPreview}
                <img src={photoPreview} alt="预览" class="photo-preview" />
                <button type="button" class="btn-remove-photo" onclick={removePhoto}>移除</button>
              {:else}
                <div class="photo-placeholder">
                  <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
                  <span>拖拽或选择照片</span>
                  <label class="btn-select-photo">选择图片<input type="file" accept="image/*" onchange={handleFileSelect} hidden /></label>
                </div>
              {/if}
            </div>
          {:else if actor?.photo_url}
            <img src={actor.photo_url} alt={name} class="actor-photo" />
          {:else}
            <div class="actor-avatar">{name.charAt(0)}</div>
          {/if}
        </div>

        <div class="actor-info">
          <h1>{name}</h1>
          {#if actor?.bio && !editing}
            <p class="actor-bio">{actor.bio}</p>
          {/if}
          <span class="actor-count">{shows.length} 场演出</span>
        </div>

        <div class="actor-actions">
          {#if editing}
            <button class="cancel-btn" onclick={cancelEdit}>取消</button>
            <button class="save-btn" onclick={saveEdit} disabled={saving}>
              {saving ? '保存中...' : '保存'}
            </button>
          {:else}
            <button class="edit-btn" onclick={startEdit}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
              编辑信息
            </button>
          {/if}
        </div>
      </div>

      {#if editing}
        <div class="edit-form">
          <div class="form-group">
            <label for="actor-bio">简介</label>
            <textarea id="actor-bio" bind:value={editBio} placeholder="添加演员简介..." rows="4"></textarea>
          </div>
          <div class="form-group">
            <label for="actor-photo-url">照片URL</label>
            <input id="actor-photo-url" type="url" bind:value={editPhoto} placeholder="或输入照片URL" />
            <span class="field-hint">支持 JPG、PNG、WebP，或直接拖拽上传上方区域</span>
          </div>
        </div>
      {/if}
    </div>

    <div class="shows-section">
      <h2>参演记录</h2>
      {#if shows.length === 0}
        <div class="empty">暂无参演记录</div>
      {:else}
        <div class="shows-list">
          {#each shows as show (show.id)}
            <ShowCard {show} />
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .actor-detail { max-width: 1000px; margin: 0 auto; }

  .back-link {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
    color: var(--text-muted);
    text-decoration: none;
    font-weight: 500;
    margin-bottom: 16px;
    transition: color 0.15s;
  }
  .back-link:hover { color: var(--accent); }

  .loading {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
  }

  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

  .actor-card {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 32px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
    margin-bottom: 28px;
  }

  .actor-header {
    display: flex;
    align-items: flex-start;
    gap: 24px;
  }

  .photo-section {
    flex-shrink: 0;
  }

  .actor-photo {
    width: 100px;
    height: 100px;
    border-radius: 50%;
    object-fit: cover;
    border: 3px solid var(--bg-surface);
  }

  .actor-avatar {
    width: 100px;
    height: 100px;
    border-radius: 50%;
    background: var(--accent-bg);
    color: var(--accent);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 40px;
    font-weight: 700;
  }

  .photo-upload {
    width: 140px;
    height: 140px;
    border: 2px dashed var(--border);
    border-radius: var(--radius-md);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    background: var(--bg-surface);
    transition: all 0.2s;
    cursor: pointer;
  }

  .photo-upload:hover { border-color: var(--accent); background: var(--accent-bg); }
  .photo-upload.dragover { border-color: var(--accent); background: var(--accent-bg); }

  .photo-preview {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: calc(var(--radius-md) - 2px);
  }

  .btn-remove-photo {
    position: absolute;
    bottom: 8px;
    left: 50%;
    transform: translateX(-50%);
    padding: 4px 12px;
    background: var(--danger-bg);
    color: var(--danger-text);
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
  }

  .photo-placeholder {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    color: var(--text-muted);
  }

  .photo-placeholder span {
    font-size: 12px;
  }

  .btn-select-photo {
    padding: 6px 14px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.15s;
  }

  .btn-select-photo:hover { background: var(--accent-light); }

  .actor-info {
    flex: 1;
    min-width: 0;
  }

  .actor-info h1 {
    font-size: 28px;
    font-weight: 700;
    margin-bottom: 8px;
    letter-spacing: -0.02em;
  }

  .actor-bio {
    font-size: 14px;
    color: var(--text-secondary);
    line-height: 1.6;
    margin-bottom: 10px;
    padding: 12px 16px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
  }

  .actor-count {
    font-size: 13px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .actor-actions {
    flex-shrink: 0;
  }

  .edit-btn, .cancel-btn, .save-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 16px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    transition: all 0.2s;
  }

  .edit-btn {
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border);
  }
  .edit-btn:hover { background: var(--bg-surface-hover); }

  .cancel-btn {
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border);
    margin-right: 8px;
  }
  .cancel-btn:hover { background: var(--bg-surface-hover); }

  .save-btn {
    background: var(--accent);
    color: #fff;
  }
  .save-btn:hover { background: var(--accent-light); }
  .save-btn:disabled { opacity: 0.6; cursor: not-allowed; }

  .edit-form {
    margin-top: 24px;
    padding-top: 24px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .form-group label {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .form-group textarea, .form-group input {
    width: 100%;
    padding: 12px 14px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    border: 1.5px solid var(--border);
    background: var(--bg-input);
    color: var(--text-primary);
    resize: vertical;
    transition: all 0.2s;
  }

  .form-group textarea:hover, .form-group input:hover {
    border-color: var(--border-hover);
  }

  .form-group textarea:focus, .form-group input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
    outline: none;
  }

  .field-hint {
    font-size: 12px;
    color: var(--text-muted);
  }

  .shows-section {
    margin-top: 8px;
  }

  .shows-section h2 {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 16px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .shows-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .empty {
    text-align: center;
    padding: 48px 20px;
    color: var(--text-muted);
    font-size: 14px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
  }

  @media (max-width: 768px) {
    .actor-card { padding: 20px 16px; }
    .actor-header { flex-direction: column; align-items: center; text-align: center; }
    .actor-info h1 { font-size: 22px; }
    .actor-bio { text-align: left; }
    .photo-upload { width: 120px; height: 120px; }
  }
</style>
