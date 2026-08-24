import { writable } from 'svelte/store';
import { browser } from '$app/environment';

const darkMQ = browser ? window.matchMedia('(prefers-color-scheme: dark)') : null;

function applyTheme(theme) {
  if (!browser) return;
  const root = document.documentElement;
  const dark = theme === 'auto' ? darkMQ.matches : theme === 'dark';
  root.classList.toggle('dark', dark);
  root.classList.toggle('light', !dark);
  // 浏览器 UI（地址栏/状态栏）同步跟随：亮色保持品牌红，暗色用深底
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', dark ? '#1c1917' : '#991B1B');
}

function currentTheme() {
  return browser ? localStorage.getItem('theme') || 'auto' : 'auto';
}

function createThemeStore() {
  const stored = currentTheme();
  const { subscribe, set } = writable(stored);

  if (browser) {
    subscribe(value => {
      localStorage.setItem('theme', value);
      applyTheme(value);
    });
    applyTheme(stored);

    // 系统亮暗切换：auto 模式下实时跟随
    darkMQ.addEventListener('change', () => {
      if (currentTheme() === 'auto') applyTheme('auto');
    });

    // 兜底：部分环境（如 iOS PWA standalone）系统切换后不派发 change 事件，
    // 页面重新可见 / 窗口聚焦时重新应用，免去手动刷新
    const recheck = () => {
      if (currentTheme() === 'auto') applyTheme('auto');
    };
    document.addEventListener('visibilitychange', () => { if (!document.hidden) recheck(); });
    window.addEventListener('focus', recheck);
  }

  return { subscribe, set };
}

export const theme = createThemeStore();
