<script>
  import { confirmState, resolveConfirm } from '$lib/confirm.js';
  import { fade } from 'svelte/transition';

  function onKey(e) {
    if (e.key === 'Escape') resolveConfirm(false);
  }
</script>

{#if $confirmState.open}
  <div
    class="cm-backdrop"
    role="presentation"
    onclick={() => resolveConfirm(false)}
    onkeydown={onKey}
    transition:fade={{ duration: 120 }}
  >
    <div
      class="cm-card"
      role="alertdialog"
      aria-modal="true"
      aria-label={$confirmState.title}
      onclick={(e) => e.stopPropagation()}
    >
      <h3 class="cm-title" class:danger={$confirmState.danger}>{$confirmState.title}</h3>
      {#if $confirmState.message}<p class="cm-msg">{$confirmState.message}</p>{/if}
      {#if $confirmState.impact != null}
        <div class="cm-impact">影响面：<b>{$confirmState.impact}</b> {$confirmState.impactLabel}</div>
      {/if}
      <div class="cm-actions">
        <button class="btn" type="button" onclick={() => resolveConfirm(false)}>{$confirmState.cancelLabel}</button>
        <button
          class="btn {$confirmState.danger ? 'danger' : 'primary'}"
          type="button"
          onclick={() => resolveConfirm(true)}
        >{$confirmState.confirmLabel}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .cm-backdrop {
    position: fixed;
    inset: 0;
    z-index: 2000;
    background: rgba(0, 0, 0, 0.42);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
  }
  .cm-card {
    width: min(420px, 100%);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg, 16px);
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.16);
    padding: 22px 22px 18px;
  }
  .cm-title { margin: 0 0 10px; font-size: 17px; }
  .cm-title.danger { color: var(--danger); }
  .cm-msg {
    margin: 0 0 6px;
    color: var(--text-2);
    line-height: 1.6;
    font-size: 14px;
    white-space: pre-wrap;
  }
  .cm-impact {
    margin: 10px 0 4px;
    padding: 10px 12px;
    border-radius: var(--radius, 10px);
    background: var(--surface-3);
    font-size: 14px;
  }
  .cm-impact b { color: var(--danger); font-size: 16px; }
  .cm-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px; }
</style>
