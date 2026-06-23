import { writable } from 'svelte/store';
import { browser } from '$app/environment';

function createThemeStore() {
  const stored = browser ? localStorage.getItem('theme') || 'auto' : 'auto';
  const { subscribe, set } = writable(stored);

  if (browser) {
    subscribe(value => {
      localStorage.setItem('theme', value);
      applyTheme(value);
    });
    applyTheme(stored);

    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      const current = localStorage.getItem('theme') || 'auto';
      if (current === 'auto') applyTheme('auto');
    });
  }

  return { subscribe, set };
}

function applyTheme(theme) {
  if (!browser) return;
  const root = document.documentElement;

  if (theme === 'auto') {
    const dark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    root.classList.toggle('dark', dark);
    root.classList.toggle('light', !dark);
  } else {
    root.classList.toggle('dark', theme === 'dark');
    root.classList.toggle('light', theme === 'light');
  }
}

export const theme = createThemeStore();
