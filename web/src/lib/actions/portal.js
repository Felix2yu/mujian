/**
 * 把节点挂到 document.body 下。
 *
 * 为什么需要它：.fade-up 的入场动画用 fill-mode:both，结束后会残留 transform，
 * 使它成为内部 position:fixed 元素的包含块 —— 浮层会按列宽/容器高度定位，
 * 页面一滚动就跑到可见区之外。挂到 body 才能以视口为基准。
 *
 * SSR 安全：没有 document 时什么都不做。
 */
export function portal(node) {
  if (typeof document === 'undefined') return;
  document.body.appendChild(node);
  return {
    destroy() {
      node.remove();
    }
  };
}
