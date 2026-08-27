import { writable } from 'svelte/store';
import { browser } from '$app/environment';

/**
 * 主题（外观）单例模块。
 *
 * 设计目标：
 *  - 跟随系统：当偏好为 `auto` 时，实时监听操作系统「亮/暗」外观变化并立即更新，
 *    无需刷新页面。
 *  - 任意状态可切换：页面内的暗色切换控件（设置页的三选一）点击后立即生效，
 *    由 store 的订阅回调同步写入 <html> 的 class，不依赖任何刷新。
 *  - 跨标签页同步：在一个标签页修改主题后，其他已打开的标签页通过 `storage`
 *    事件实时跟随，同样无需刷新。
 *
 * 该模块被根布局 (+layout.svelte) 在 onMount 时通过 initTheme() 激活，
 * 因此无论停留在哪个路由，系统外观变化监听都始终在线。
 */

const STORAGE_KEY = 'theme';
export const THEMES = ['auto', 'light', 'dark'];

function readStored() {
  if (!browser) return 'auto';
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    return THEMES.includes(v) ? v : 'auto';
  } catch {
    return 'auto';
  }
}

// 模块级单例：仅在浏览器中创建一次，供 initTheme() 与 store 共享同一实例。
const darkMQ = browser ? window.matchMedia('(prefers-color-scheme: dark)') : null;

// 把偏好解析为「是否暗色」。auto 时以系统当前外观为准。
function resolveDark(value) {
  if (value === 'dark') return true;
  if (value === 'light') return false;
  return darkMQ ? darkMQ.matches : false; // 'auto'
}

/**
 * 将主题应用到 <html>：切换 `dark` class 并同步浏览器 UI（地址栏/状态栏）配色。
 * 纯 DOM 操作，同步执行，调用即生效。
 */
export function applyTheme(value) {
  if (!browser) return;
  const dark = resolveDark(value);
  const root = document.documentElement;
  root.classList.toggle('dark', dark);
  root.classList.toggle('light', !dark);
  // 让原生表单控件（select/input/scrollbar）跟随主题
  root.style.colorScheme = dark ? 'dark' : 'light';
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', dark ? '#1c1917' : '#991B1B');
}

function createThemeStore() {
  const stored = readStored();
  const store = writable(stored);

  // 每次变化（含 set）都立即持久化并应用到 DOM，点击控件即生效、无需刷新。
  store.subscribe((value) => {
    if (!browser) return;
    try {
      localStorage.setItem(STORAGE_KEY, value);
    } catch {
      /* localStorage 不可用时忽略，不影响本次渲染 */
    }
    applyTheme(value);
  });

  return store;
}

export const theme = createThemeStore();

let initialized = false;

/**
 * 激活全局主题监听。应在应用根布局挂载时调用一次（幂等）。
 * 负责：
 *  1. 跟随系统外观变化（auto 模式下 matchMedia 的 change 事件）；
 *  2. 跨标签页同步（storage 事件）；
 *  3. 兜底：部分环境（iOS PWA standalone、个别 WebView）系统切换后不派发
 *     change 事件，页面重新可见 / 窗口聚焦时重新应用，免去手动刷新；
 *  4. 启动后按当前偏好再应用一次，覆盖水合（hydration）后的首帧。
 */
export function initTheme() {
  if (!browser || initialized) return;
  initialized = true;

  // 1. 实时跟随操作系统亮/暗切换
  if (darkMQ) {
    darkMQ.addEventListener('change', () => {
      if (readStored() === 'auto') applyTheme('auto');
    });
  }

  // 2. 跨标签页 / 窗口实时同步
  window.addEventListener('storage', (e) => {
    if (e.key === STORAGE_KEY && e.newValue) applyTheme(e.newValue);
  });

  // 3. 兜底：重新可见 / 聚焦时重新应用（应对不派发 change 的环境）
  const recheck = () => {
    if (readStored() === 'auto') applyTheme('auto');
  };
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) recheck();
  });
  window.addEventListener('focus', recheck);

  // 4. 启动后一次性应用，保证首帧即正确
  applyTheme(readStored());
}
