<script>
  // 骨架屏：把「高度 / 圆角 / 重复次数」收成参数，消灭内联 style。
  // 视觉沿用全局 .skeleton（app.css 的 shimmer 动画）。
  let { height = '16px', width = '100%', radius = '', count = 1, gap = '10px' } = $props();
</script>

{#each Array(count) as _, i (i)}
  <div
    class="sk"
    style:height
    style:width
    style:margin-bottom={count > 1 && i < count - 1 ? gap : undefined}
    style:border-radius={radius || undefined}
    aria-hidden="true"
  ></div>
{/each}
