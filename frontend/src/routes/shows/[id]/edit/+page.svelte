<script>
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import ShowForm from '$lib/components/ShowForm.svelte';

  let show = null;
  let loading = true;
  let error = '';

  $: id = $page.params.id;

  onMount(async () => {
    try {
      show = await api.getShow(id);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function handleSaved() {
    goto(`/shows/${id}`);
  }

  function handleCancel() {
    goto(`/shows/${id}`);
  }
</script>

<div class="edit-show">
  {#if loading}
    <div class="loading">加载中...</div>
  {:else if error}
    <div class="error">{error}</div>
  {:else if show}
    <h1>编辑演出</h1>
    <ShowForm {show} on:saved={handleSaved} on:cancel={handleCancel} />
  {/if}
</div>

<style>
  .edit-show {
    max-width: 800px;
    margin: 0 auto;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 24px;
  }

  .loading, .error {
    text-align: center;
    padding: 60px 20px;
    color: #666;
  }

  .error {
    color: #c00;
    background: #fee;
    border-radius: 8px;
  }
</style>
