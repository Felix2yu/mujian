<script>
  // 横幅提示：统一「语气色 + 前缀符号」。
  // 视觉沿用全局 .banner（app.css）；前缀符号按语气给默认值，
  // 也可以用 icon 覆盖，或传 icon={false} 完全不要图标。
  let { tone = 'error', icon, children } = $props();

  const DEFAULT_ICON = { error: '⚠', success: '✓', info: '💡', warn: '⏳' };
  let glyph = $derived(icon === false ? '' : (icon ?? DEFAULT_ICON[tone] ?? ''));
</script>

<div
  class="banner"
  class:error={tone === 'error'}
  class:success={tone === 'success'}
  class:info={tone === 'info'}
>
  {#if glyph}<span class="banner-glyph" aria-hidden="true">{glyph}</span>{/if}
  {@render children()}
</div>

<style>
  .banner-glyph {
    flex: none;
  }
</style>
