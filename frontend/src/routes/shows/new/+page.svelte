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
  <div class="page-header">
    <a href="/shows" class="back-link">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      返回
    </a>
    <h1>添加演出</h1>
  </div>
  <ShowForm onsaved={handleSaved} oncancel={handleCancel} />
</div>

<style>
  .new-show {
    max-width: 800px;
    margin: 0 auto;
  }

  .page-header {
    display: flex;
    flex-direction: column;
    gap: 16px;
    margin-bottom: 24px;
  }

  .back-link {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
    color: var(--text-muted);
    text-decoration: none;
    font-weight: 500;
    transition: color 0.15s;
    width: fit-content;
  }

  .back-link:hover {
    color: var(--accent);
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
</style>
