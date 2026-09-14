import { writable } from 'svelte/store';

// 全站「危险操作二次确认」状态。任意页面调用 askConfirm() 打开居中弹窗，
// resolve 为 true 表示用户确认。弹窗挂在 +layout 顶层（不在 .fade-up 内），
// 规避 fixed 元素被入场动画 transform 捕获为包含块的问题。
export const confirmState = writable({
  open: false,
  title: '请确认',
  message: '',
  confirmLabel: '确认',
  cancelLabel: '取消',
  danger: true,
  impact: null, // 可选影响面数字（如将删除的条数）
  impactLabel: '条'
});

let resolver = null;

// 返回 Promise<boolean>：用户点确认 => true，取消/Esc/点遮罩 => false。
export function askConfirm(opts = {}) {
  return new Promise((resolve) => {
    resolver = resolve;
    confirmState.set({
      open: true,
      title: opts.title || '请确认',
      message: opts.message || '',
      confirmLabel: opts.confirmLabel || '确认',
      cancelLabel: opts.cancelLabel || '取消',
      danger: opts.danger ?? true,
      impact: opts.impact ?? null,
      impactLabel: opts.impactLabel || '条'
    });
  });
}

export function resolveConfirm(value) {
  confirmState.update((s) => ({ ...s, open: false }));
  const r = resolver;
  resolver = null;
  if (r) r(value);
}
