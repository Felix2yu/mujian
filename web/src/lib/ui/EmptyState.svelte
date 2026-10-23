<script>
  // 空态：统一「图标 + 标题 + 说明 + 可选操作」的结构与间距。
  // 视觉沿用全局 .empty（app.css），这里只收拢结构与传参方式。
  // 图标通过 snippet 注入，不做全局图标体系。
  let { icon, title = '', hint = '', small = false, bare = false, action, children } = $props();
</script>

<div class="es" class:card={!bare} class:es-small={small}>
  {#if icon}
    <div class="ico">{@render icon()}</div>
  {/if}
  {#if title}<div class="t">{title}</div>{/if}
  {#if hint}<div class="h">{hint}</div>{/if}
  {#if children}{@render children()}{/if}
  {#if action}<div class="es-act">{@render action()}</div>{/if}
</div>

<style>
  /* .es 复用的就是全局 .empty 的排版（app.css），这里只补卡片外观与 small 变体，
     免得每个页面再抄一遍「empty + card」的组合。 */
  .es {
    padding: 56px 24px;
  }
  .es-small {
    padding: 28px 20px;
  }
  .es-small .t {
    font-size: 14px;
  }
  .es-act {
    margin-top: 6px;
  }
</style>
