<script>
  import { goto } from '$app/navigation';

  // 返回上一页：优先走浏览器历史（保留来源页的筛选/滚动状态），
  // 没有站内历史时（直接打开 / 刷新后首跳）回退到 fallback 列表页。
  let { fallback = '/', label = '← 返回' } = $props();

  function onClick(e) {
    const idx = window.history.state?.['sveltekit:index'] ?? 0;
    if (idx > 0) {
      e.preventDefault();
      history.back();
    } else {
      e.preventDefault();
      goto(fallback);
    }
  }
</script>

<a class="back" href={fallback} onclick={onClick}>{label}</a>

<style>
  .back {
    display: inline-flex;
    color: var(--text-muted);
    font-size: 13.5px;
    margin-bottom: 12px;
    text-decoration: none;
    transition: color var(--t-fast, 0.15s) ease;
  }
  .back:hover { color: var(--accent); }
</style>
