<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import BackLink from '$lib/components/BackLink.svelte';
  import RecordForm from '$lib/components/RecordForm.svelte';

  let categories = $state([]);
  let error = $state('');

  async function onSubmit(payload) {
    try {
      const rec = await api.createRecord(payload);
      location.href = `/records/${rec.id}`;
    } catch (e) {
      error = e.message;
      throw e;
    }
  }

  onMount(async () => {
    try { categories = await api.listCategories(); } catch (e) {}
  });
</script>

<div class="fade-up">
  <BackLink />
  <div class="page-head">
    <h1>新建记录</h1>
    <p class="sub">记录一次现场演出</p>
  </div>
  {#if error}<div class="banner error">⚠ {error}</div>{/if}
  <RecordForm {categories} onSubmit={onSubmit} onCancel={() => (location.href = '/')} />
</div>

<style>
  .fade-up { max-width: 860px; margin: 0 auto; }
  .back { display: inline-flex; color: var(--text-muted); font-size: 13.5px; margin-bottom: 10px; }
  .back:hover { color: var(--accent); }
</style>
