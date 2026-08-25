<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api, coverUrl } from '$lib/api.js';
  import BackLink from '$lib/components/BackLink.svelte';
  import RecordCard from '$lib/components/RecordCard.svelte';

  const id = $page.params.id;
  let artist = $state(null);
  let records = $state([]);
  let loading = $state(true);
  let error = $state('');

  // inline editing
  let editing = $state(false);
  let form = $state({ name: '', aliases: '', remark: '', coverFile: '', coverThumb: '' });
  let uploading = $state(false);
  let fileInput = $state(null);
  let saving = $state(false);
  let deleting = $state(false);

  const splitList = (s) => (s || '').split(/[,，]/).map((x) => x.trim()).filter(Boolean);

  async function load() {
    loading = true;
    error = '';
    try {
      const a = await api.getArtist(id);
      artist = a;
      records = a.records || [];
      form = {
        name: a.name,
        aliases: (a.aliases || []).join(', '),
        remark: [a.bio, a.remark].filter(Boolean).join('\n'),
        coverFile: a.coverFile || '',
        coverThumb: a.coverThumb || ''
      };
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function startEdit() {
    form = {
      name: artist.name,
      aliases: (artist.aliases || []).join(', '),
      remark: [artist.bio, artist.remark].filter(Boolean).join('\n'),
      coverFile: artist.coverFile || '',
      coverThumb: artist.coverThumb || ''
    };
    editing = true;
  }

  async function save() {
    if (!form.name.trim() || saving) return;
    saving = true;
    error = '';
    try {
      artist = await api.updateArtist(id, {
        name: form.name.trim(),
        aliases: splitList(form.aliases),
        remark: form.remark.trim(),
        bio: '',
        coverFile: form.coverFile.trim(),
        coverThumb: form.coverThumb.trim()
      });
      editing = false;
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (!confirm(`删除演员「${artist.name}」？关联的演出记录不受影响。`)) return;
    deleting = true;
    try {
      await api.deleteArtist(id);
      location.href = '/artists';
    } catch (e) {
      error = e.message;
      deleting = false;
    }
  }

  function triggerUpload() {
    fileInput?.click();
  }
  async function handleUpload(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const res = await api.uploadFile(file);
      form.coverFile = res.key;
      if (res.thumb) form.coverThumb = res.thumb;
    } catch (err) {
      error = err.message;
    } finally {
      uploading = false;
    }
  }

  onMount(load);
</script>
<svelte:head><title>{artist ? `${artist.name} - 幕间` : "演员 - 幕间"}</title></svelte:head>


<div class="fade-up">
  <BackLink fallback="/artists" label="← 演员" />

  {#if loading}
    <div class="loading">
      <div class="skeleton" style="height: 160px; width: 100%;"></div>
      <div class="skeleton" style="height: 200px;"></div>
    </div>
  {:else if error}
    <div class="banner error">⚠ {error}</div>
    <a class="btn ghost" href="/artists">← 返回演员列表</a>
  {:else if artist}
    {#if error}<div class="banner error">⚠ {error}</div>{/if}

    <div class="card head-card">
      {#if editing}
        <div class="head-main">
          <div class="head-left">
            {#if form.coverFile}
              <img class="avatar lg" src={coverUrl(form.coverFile)} alt="头像预览" />
            {:else}
              <div class="avatar lg placeholder">🎭</div>
            {/if}
            <div class="edit-fields">
              <input class="input" placeholder="演员姓名" bind:value={form.name} />
              <input class="input" placeholder="别名（逗号分隔）" bind:value={form.aliases} />
              <textarea class="input" rows="3" placeholder="简介 / 备注" bind:value={form.remark}></textarea>
              <div>
                <button class="btn sm" onclick={triggerUpload} disabled={uploading}>{uploading ? '上传中…' : '⇪ 头像'}</button>
                <input type="file" accept="image/*" onchange={handleUpload} disabled={uploading} hidden bind:this={fileInput} />
              </div>
            </div>
          </div>
        </div>
        <div class="actions">
          <button class="btn primary sm" onclick={save} disabled={saving || !form.name.trim()}>{saving ? '保存中…' : '保存'}</button>
          <button class="btn ghost sm" onclick={() => { editing = false; }}>取消</button>
        </div>
      {:else}
        <div class="head-main">
          <div class="head-left">
            {#if artist.coverFile}
              <img class="avatar lg" src={coverUrl(artist.coverFile)} alt={artist.name} />
            {:else}
              <div class="avatar lg placeholder">🎭</div>
            {/if}
            <div>
              <h1>{artist.name}</h1>
              <div class="sub">
                {#if artist.aliases && artist.aliases.length}
                  <span class="aliases">别名：{artist.aliases.join(' / ')}</span>
                {/if}
                <span class="muted">{artist.recordCount} 场演出</span>
              </div>
              {#if artist.bio || artist.remark}<p class="remark">{artist.bio || ''}{artist.bio && artist.remark ? '\n' : ''}{artist.remark || ''}</p>{/if}
            </div>
          </div>
          <div class="head-actions">
            <button class="btn sm" onclick={startEdit}>编辑</button>
            <button class="btn danger sm" onclick={remove} disabled={deleting}>{deleting ? '删除中…' : '删除'}</button>
          </div>
        </div>
      {/if}
    </div>

    <div class="card section">
      <div class="sec-head"><h3>参演演出 <span class="num">{records.length}</span></h3></div>
      {#if records.length === 0}
        <div class="empty small card"><div class="h">还没有演出关联该演员</div></div>
      {:else}
        <div class="grid">
          {#each records as r (r.id)}
            <RecordCard record={r} />
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .back { display: inline-flex; color: var(--text-muted); font-size: 13.5px; margin-bottom: 12px; }
  .back:hover { color: var(--accent); }
  .loading { display: flex; flex-direction: column; gap: 12px; }

  .head-card { padding: 18px 20px; margin-bottom: 14px; }
  .head-main { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; }
  .head-left { display: grid; grid-template-columns: auto 1fr; gap: 16px; align-items: start; min-width: 0; flex: 1; }
  .avatar { width: 160px; height: 160px; border-radius: 50%; object-fit: cover; background: var(--surface-3); }
  .avatar.placeholder { display: flex; align-items: center; justify-content: center; font-size: 56px; }
  h1 { margin: 0 0 8px; font-size: 26px; line-height: 1.25; }
  .sub { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .aliases { font-size: 12.5px; color: var(--text-muted); }
  .muted { color: var(--text-muted); font-size: 13px; }
  .remark { margin: 10px 0 0; color: var(--text-2); white-space: pre-wrap; line-height: 1.6; }
  .head-actions { display: flex; gap: 8px; flex: 0 0 auto; }
  .edit-fields { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
  .edit-fields .input { width: 100%; }

  .actions { display: flex; gap: 8px; margin-top: 10px; }

  .section { padding: 18px 20px; margin-top: 14px; }
  .section h3 { margin: 0; font-size: 16px; display: flex; align-items: center; gap: 8px; }
  .num { color: var(--accent); font-family: var(--font-sans); font-weight: 700; font-size: 15px; }
  .sec-head { margin-bottom: 14px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(172px, 1fr)); gap: 14px; margin-top: 4px; }

  @media (max-width: 640px) {
    h1 { font-size: 22px; }
    .head-main { flex-direction: column; }
    .grid { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
  }
</style>
