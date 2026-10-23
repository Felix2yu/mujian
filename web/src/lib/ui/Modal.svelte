<script>
  import { fade } from 'svelte/transition';
  import { portal } from '../actions/portal.js';

  let {
    open = $bindable(false),
    title = '',
    label = '',
    escClose = true,
    padded = true,
    busy = false,
    labelledby = '',
    variant = 'dialog', // dialog = 居中对话框；sheet = 底部抽屉
    width = '560px',
    maxHeight = 'min(84vh, 720px)',
    onclose,
    children,
    headExtra,
    footer
  } = $props();

  let panelEl = $state(null);
  const uid = $props.id();
  let titleId = $derived(`modal-title-${uid}`);

  const FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  function close() {
    if (typeof onclose === 'function') onclose();
    open = false;
  }

  // 面板入场：居中对话框用轻微放大，底部抽屉用上滑。
  // 自绘而不是引 transition: —— 同一个元素上不能挂两个 in: 指令，
  // 而 variant 是运行时才定的。
  function panelAnim(_node) {
    if (variant === 'sheet') {
      return {
        duration: 180,
        css: (t) => `transform: translateY(${(1 - t) * 60}px); opacity: ${t};`
      };
    }
    return {
      duration: 200,
      css: (t) => `transform: scale(${0.96 + 0.04 * t}); opacity: ${t};`
    };
  }

  // 对话框契约：滚动锁 + Esc 关闭 + Tab 循环陷阱 + 初始焦点 + 关闭还焦。
  // 全部由本组件负责，调用方不需要、也不应该再写一遍。
  $effect(() => {
    if (!open) return;
    const panel = panelEl;
    if (!panel) return; // 面板挂载前的一次空转（模板尚未渲染）

    const prevOverflow = document.body.style.overflow;
    const prevActive = document.activeElement;
    document.body.style.overflow = 'hidden';

    const onKey = (e) => {
      if (e.key === 'Escape') {
        if (!escClose) return;
        e.preventDefault();
        e.stopPropagation(); // 别让页面级的 Esc 处理器跟着一起动
        close();
        return;
      }
      if (e.key !== 'Tab') return;
      const nodes = [...panel.querySelectorAll(FOCUSABLE)].filter((el) => el.offsetParent !== null);
      if (nodes.length === 0) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || !nodes.includes(active)) {
          e.preventDefault();
          last.focus({ preventScroll: true });
        }
      } else if (active === last) {
        e.preventDefault();
        first.focus({ preventScroll: true });
      }
    };
    document.addEventListener('keydown', onKey, true);

    // 等过渡结束再送焦点，否则会被 transform 动画带着滚动。
    const focusTimer = setTimeout(() => {
      const target = panel.querySelector('[data-autofocus]') ?? panel.querySelector(FOCUSABLE);
      target?.focus({ preventScroll: true });
    }, 220);

    return () => {
      document.removeEventListener('keydown', onKey, true);
      clearTimeout(focusTimer);
      document.body.style.overflow = prevOverflow;
      if (prevActive instanceof HTMLElement && prevActive.isConnected) {
        prevActive.focus({ preventScroll: true });
      }
    };
  });
</script>

{#if open}
  <!-- 遮罩是带 aria-label 的 <button>：既承接点击，又不会把一堆链接塞进按钮里 -->
  <div
    class="m-mask"
    class:m-sheet={variant === 'sheet'}
    transition:fade={{ duration: 140 }}
    use:portal
  >
    <button class="m-bg" type="button" tabindex="-1" aria-label="关闭" onclick={close}></button>
    <div
      class="m-panel"
      class:m-flush={!padded}
      role="dialog"
      aria-modal="true"
      aria-busy={busy || undefined}
      aria-labelledby={labelledby || (title ? titleId : undefined)}
      aria-label={labelledby || title ? undefined : label || '对话框'}
      bind:this={panelEl}
      style:width={variant === 'sheet' ? undefined : width}
      style:max-height={maxHeight}
      in:panelAnim
      out:panelAnim
    >
      {#if title}
        <div class="m-head">
          <h2 id={titleId}>{title}</h2>
          <div class="m-head-extra">{@render headExtra?.()}</div>
          <button class="m-x" type="button" aria-label="关闭" onclick={close}>✕</button>
        </div>
      {/if}
      <div class="m-body">{@render children()}</div>
      {#if footer}
        <div class="m-foot">{@render footer()}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* 层级阶梯：顶栏 50 < 抽屉 61 < 工具浮层 70 < Modal 100 < 确认框 2000 < 灯箱 9999 */
  .m-mask {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
    background: rgba(0, 0, 0, 0.45);
    -webkit-backdrop-filter: blur(4px);
    backdrop-filter: blur(4px);
  }

  .m-bg {
    position: absolute;
    inset: 0;
    border: none;
    padding: 0;
    background: none;
    cursor: default;
  }

  /* 底部抽屉：贴底、通栏、只留上圆角，窄屏保持 82vh 而不占满 */
  .m-mask.m-sheet {
    align-items: flex-end;
    padding: 0;
  }
  .m-sheet .m-panel {
    width: 100%;
    max-width: 560px;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }

  .m-panel {
    position: relative;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--surface);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
  }

  .m-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 14px 18px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .m-head h2 {
    margin: 0;
    font-family: var(--font-serif);
    font-size: 16px;
    font-weight: 600;
  }

  .m-head-extra {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-muted);
    font-size: 13px;
  }

  .m-x {
    flex: none;
    width: 28px;
    height: 28px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-2);
    font-size: 13px;
    line-height: 1;
    cursor: pointer;
  }

  .m-x:hover {
    background: var(--surface-3);
    color: var(--text);
  }

  .m-body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    display: flex;
    flex-direction: column;
    padding: 18px 20px;
    -webkit-overflow-scrolling: touch;
  }

  /* padded=false：内边距完全交给调用方（封面选择器、批量编辑各自有布局） */
  .m-flush .m-body {
    padding: 0;
  }

  .m-foot {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    padding: 14px 18px;
    border-top: 1px solid var(--border);
  }

  @media (max-width: 640px) {
    /* 窄屏全屏只对居中对话框生效；底部抽屉保持贴底 + 82vh */
    .m-mask:not(.m-sheet) {
      padding: 0;
    }
    .m-mask:not(.m-sheet) .m-panel {
      width: 100% !important;
      max-width: 100%;
      max-height: 100vh;
      height: 100%;
      border-radius: 0;
    }
  }
</style>
