<script>
  // 图标按钮：无文字、只有 aria-label 说明用途的那类按钮。
  // label 必填——没有它屏幕阅读器只会念「按钮」，等于没有。
  let { label, title = '', tone = 'default', size = 'md', onclick, children } = $props();
</script>

<button
  type="button"
  class="ib"
  class:ib-danger={tone === 'danger'}
  class:ib-sm={size === 'sm'}
  class:ib-lg={size === 'lg'}
  aria-label={label}
  title={title || label}
  {onclick}
>
  {@render children()}
</button>

<style>
  .ib {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 28px;
    height: 28px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: transparent;
    color: var(--text-3);
    font-size: 13px;
    line-height: 1;
    cursor: pointer;
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease);
  }
  .ib:hover {
    background: var(--surface-3);
    color: var(--text);
  }
  .ib-danger:hover {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .ib-sm {
    width: 24px;
    height: 24px;
    font-size: 12px;
  }
  .ib-lg {
    width: 34px;
    height: 34px;
  }
  .ib:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }
</style>
