<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import { theme } from '$lib/stores.js';

  let settings = $state({ storage_type: 'local' });
  let error = $state('');
  let saved = $state(false);
  let loading = $state(true);

  async function load() {
    loading = true;
    try {
      settings = await api.getSettings();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function save() {
    saved = false;
    error = '';
    try {
      await api.updateSettings({ theme: $theme, storage_type: settings.storage_type });
      saved = true;
      setTimeout(() => (saved = false), 2400);
    } catch (e) {
      error = e.message;
    }
  }

  const themes = [
    { v: 'auto', label: '跟随系统', ico: '◐' },
    { v: 'light', label: '亮色', ico: '☀️' },
    { v: 'dark', label: '暗色', ico: '🌙' }
  ];

  onMount(load);
</script>

<div class="fade-up">
  <div class="page-head">
    <h1>设置</h1>
    <p class="sub">外观与存储偏好</p>
  </div>

  {#if loading}
    <div class="skeleton" style="height: 220px; max-width: 520px;"></div>
  {:else}
    <div class="card sec">
      <h3>主题</h3>
      <div class="theme-row">
        {#each themes as t}
          <button class="theme-opt" class:on={$theme === t.v} on:click={() => theme.set(t.v)}>
            <span class="tico">{t.ico}</span>
            <span>{t.label}</span>
          </button>
        {/each}
      </div>
    </div>

    <div class="card sec">
      <h3>存储</h3>
      <label>封面图片存储方式</label>
      <select class="input" bind:value={settings.storage_type} style="max-width: 280px;">
        <option value="local">本地存储</option>
        <option value="s3">S3 对象存储</option>
      </select>
    </div>

    {#if error}<div class="banner error">⚠ {error}</div>{/if}
    {#if saved}<div class="banner success">✓ 已保存</div>{/if}

    <button class="btn primary" on:click={save}>保存设置</button>
  {/if}
</div>

<style>
  .sec { padding: 18px 20px; margin-bottom: 14px; max-width: 520px; }
  .sec h3 { margin: 0 0 12px; font-size: 15.5px; }

  .theme-row { display: flex; gap: 8px; }
  .theme-opt {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 14px 8px;
    border-radius: var(--radius);
    border: 1.5px solid var(--border);
    background: var(--surface-2);
    cursor: pointer;
    font-size: 13.5px;
    color: var(--text-2);
    transition: all var(--t-fast) var(--ease);
  }
  .theme-opt:hover { border-color: var(--border-strong); }
  .theme-opt.on {
    border-color: var(--accent);
    background: var(--accent-softer);
    color: var(--accent);
    font-weight: 600;
  }
  .tico { font-size: 18px; }
</style>
