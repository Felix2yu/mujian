<script>
  import { goto } from '$app/navigation';
  import ShowForm from '$lib/components/ShowForm.svelte';

  function handleSaved() {
    goto('/shows');
  }

  function handleCancel() {
    goto('/shows');
  }
</script>

<div class="new-show">
  <h1>添加演出</h1>
  <ShowForm on:saved={handleSaved} on:cancel={handleCancel} />
</div>

<style>
  .new-show {
    max-width: 800px;
    margin: 0 auto;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 24px;
  }
</style>
