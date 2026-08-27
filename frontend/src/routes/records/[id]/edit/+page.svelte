<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api } from '$lib/api.js';
  import BackLink from '$lib/components/BackLink.svelte';
  import RecordForm from '$lib/components/RecordForm.svelte';

  const id = $page.params.id;
  let record = $state(null);
  let categories = $state([]);
  let loading = $state(true);
  let error = $state('');

  async function onSubmit(payload) {
    try {
      await api.updateRecord(id, payload);
      location.href = `/records/${id}`;
    } catch (e) {
      error = e.message;
      throw e;
    }
  }

  onMount(async () => {
    try {
      [categories, record] = await Promise.all([api.listCategories(), api.getRecord(id)]);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });
</script>
<svelte:head><title>{record ? `编辑 ${record.name} - 幕间` : "编辑演出 - 幕间"}</title></svelte:head>


<div class="fade-up">
  <BackLink fallback={`/records/${id}`} label="← 返回详情" />
  <div class="page-head">
    <h1>编辑记录</h1>
  </div>
  {#if loading}
    <div class="skeleton" style="height: 320px; border-radius: var(--radius-lg);"></div>
  {:else if error}
    <div class="banner error">⚠ {error}</div>
  {:else if record}
    <RecordForm {record} {categories} onSubmit={onSubmit} onCancel={() => (location.href = `/records/${id}`)} />
  {/if}
</div>

<style>
  .fade-up { max-width: 1200px; margin: 0 auto; }
  .back { display: inline-flex; color: var(--text-muted); font-size: 13.5px; margin-bottom: 10px; }
  .back:hover { color: var(--accent); }
</style>
