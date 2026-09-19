<script>
  import { onMount } from 'svelte';
  import { api, coverUrl } from '$lib/api.js';
  import OperaIcon from '$lib/components/OperaIcon.svelte';

  let artists = $state([]);
  let loading = $state(true);
  let error = $state('');
  let adding = $state(false);
  let form = $state({ name: '', aliases: '', bio: '' });

  async function load() {
    loading = true;
    error = '';
    try {
      artists = await api.listArtists();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  const splitList = (s) => (s || '').split(/[,，、]/).map((x) => x.trim()).filter(Boolean);

  async function add() {
    const name = form.name.trim();
    if (!name || adding) return;
    adding = true;
    error = '';
    try {
      const a = await api.createArtist({ name, aliases: splitList(form.aliases), bio: form.bio.trim() });
      location.href = `/artists/${a.id}`;
    } catch (e) {
      error = e.message;
      adding = false;
    }
  }

  async function remove(id, name) {
    if (!confirm(`删除演员「${name}」？关联的演出记录不受影响，仅移除演员档案与关联。`)) return;
    error = '';
    try {
      await api.deleteArtist(id);
      await load();
    } catch (e) {
      error = e.message;
    }
  }

  onMount(load);
</script>
<svelte:head><title>演员 - 幕间</title></svelte:head>


<div class="fade-up">
  <div class="page-head">
    <h1>演员</h1>
    <p class="sub">演员档案（实体）。点击进入演员主页查看其参演演出；列表按参演场次（演出数）从高到低排列。</p>
  </div>

  <div class="card add-bar">
    <input class="input" placeholder="新建演员名，回车快速创建" bind:value={form.name} onkeydown={(e) => e.key === 'Enter' && add()} />
    <button class="btn primary" onclick={add} disabled={adding || !form.name.trim()}>{adding ? '创建中…' : '＋ 添加'}</button>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  {#if loading}
    <div class="grid">
      {#each Array(8) as _}<div class="skeleton" style="height: 150px;"></div>{/each}
    </div>
  {:else if artists.length === 0}
    <div class="empty card">
      <div class="ico"><OperaIcon size={44} /></div>
      <div class="t">还没有演员</div>
      <div class="h">在上方输入姓名添加第一个演员档案</div>
    </div>
  {:else}
    <div class="grid stagger">
      {#each artists as a (a.id)}
        <div class="card actor">
          <a class="actor-link" href={`/artists/${a.id}`}>
            {#if a.coverFile}
              <img class="avatar coverable" src={coverUrl(a.coverFile)} alt={a.name} />
            {:else}
              <div class="avatar placeholder"><OperaIcon size={26} /></div>
            {/if}
            <div class="meta">
              <div class="name">{a.name}</div>
              <div class="cnt">{a.recordCount ?? 0} 场演出</div>
            </div>
          </a>
          <button class="del" title="删除演员" onclick={() => remove(a.id, a.name)}>✕</button>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .add-bar { display: flex; gap: 10px; padding: 12px; margin-bottom: 16px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
  .actor {
    position: relative;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 14px;
    cursor: grab;
  }
  .actor-link { display: flex; align-items: center; gap: 10px; flex: 1; min-width: 0; text-decoration: none; color: inherit; }
  .avatar { width: 48px; aspect-ratio: 1 / 1; border-radius: 50%; object-fit: cover; flex-shrink: 0; background: var(--surface-3); }
  .avatar.placeholder { display: flex; align-items: center; justify-content: center; font-size: 22px; }
  .meta { min-width: 0; }
  .name { font-weight: 600; font-size: 14.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cnt { font-size: 12px; color: var(--text-muted); white-space: nowrap; }
  .del {
    border: none; background: none; color: var(--text-3); cursor: pointer; font-size: 12px;
    width: 24px; height: 24px; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease); flex-shrink: 0;
  }
  .del:hover { background: var(--danger-soft); color: var(--danger); }
</style>
