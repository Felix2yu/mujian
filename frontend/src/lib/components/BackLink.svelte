<script>
  import { goto } from '$app/navigation';

  // 返回导航策略：
  // - 传了 href（规范化上级页，如 /dramas、/artists）时，直接跳转到该页。
  //   这样「← 剧目 / ← 演员」等按钮的标签与目的地始终一致，不会误返回到
  //   上一条演出详情。
  // - 未传 href（如演出详情页）时，优先走浏览器历史（history.back），
  //   让首页的筛选状态随历史记录保留；没有站内历史时回退到首页。
  let { href = '', label = '← 返回' } = $props();

  function onClick(e) {
    e.preventDefault();
    if (href) {
      goto(href);
      return;
    }
    const idx = window.history.state?.['sveltekit:index'] ?? 0;
    if (idx > 0) {
      history.back();
    } else {
      goto('/');
    }
  }
</script>

<a class="back" href={href || '/'} onclick={onClick}>{label}</a>

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
