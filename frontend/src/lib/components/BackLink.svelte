<script>
  import { goto } from '$app/navigation';

  // 返回导航策略：
  // - 浏览器历史栈存在上一页（history.length > 1）→ history.back() 回到
  //   真正的上一页（日历 / 列表 / 主页等，由 history 栈决定），符合预期。
  // - 直接打开本页 URL（history.length <= 1，无站内历史）→ goto(fallback)。
  // 注意：SvelteKit 2 的 history.state 不再携带 sveltekit:index，
  // 且 afterNavigate 在本环境未稳定触发，故采用 history.length 判断。
  let { fallback = '/', label = '← 返回' } = $props();

  function onClick(e) {
    e.preventDefault();
    if (window.history.length > 1) {
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
