<script>
  import { goto } from '$app/navigation';

  // 返回导航策略：
  // - 有站内历史（SvelteKit 记录了 index > 0）→ history.back()
  // - 没有历史（直接打开详情页、刷新、新标签页等）→ goto(fallback)
  let { fallback = '/', label = '← 返回' } = $props();

  function onClick(e) {
    e.preventDefault();
    const idx = window.history.state?.['sveltekit:index'] ?? 0;
    if (idx > 0) {
      history.back();
    } else {
      goto(fallback);
    }
  }
</script>

<a class="back" href={fallback} onclick={onClick} aria-label="返回">{label}</a>

<style>
  .back {
    display: inline-flex;
    color: var(--text-muted);
    font-size: 13.5px;
    margin-bottom: 12px;
    text-decoration: none;
    cursor: pointer;
    transition: color var(--t-fast, 0.15s) ease;
  }
  .back:hover { color: var(--accent); }
</style>
