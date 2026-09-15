<script>
  import { goto } from '$app/navigation';

  // 返回导航策略：
  // - 浏览器历史栈存在上一页（history.length > 1）→ history.back() 回到
  //   真正的上一页（日历 / 列表 / 主页等，由 history 栈决定），符合预期。
  // - 如果 history.back() 后 URL 未变化（历史栈中有重复条目）→ goto(fallback)。
  // - 直接打开本页 URL（history.length <= 1，无站内历史）→ goto(fallback)。
  let { fallback = '/', label = '← 返回' } = $props();

  function onClick(e) {
    e.preventDefault();
    if (window.history.length > 1) {
      const before = location.href;
      history.back();
      // 如果 history.back() 后 URL 没变化（历史栈中有重复条目），
      // 用 goto 导航到 fallback 页面
      setTimeout(() => {
        if (location.href === before) goto(fallback);
      }, 80);
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
