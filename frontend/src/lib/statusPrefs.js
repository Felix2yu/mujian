// 演出状态定义与「列表显示哪些状态」的本地偏好。
// 与后端 active_status 语义一致：0 正常 / 1 想看 / 2 已取消 / 3 未赴约。
export const STATUS_LABELS = { 0: '正常', 1: '想看', 2: '已取消', 3: '未赴约' };
// 展示顺序：正常、已取消、想看、未赴约
export const ALL_STATUSES = [0, 2, 1, 3];
const KEY = 'mujian:status_filter';

export function loadStatusFilter() {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY));
    if (Array.isArray(raw)) {
      const valid = raw.filter((n) => ALL_STATUSES.includes(n));
      if (valid.length) return valid;
    }
  } catch { /* ignore */ }
  return [...ALL_STATUSES];
}

export function saveStatusFilter(list) {
  try {
    localStorage.setItem(KEY, JSON.stringify([...list]));
  } catch { /* ignore */ }
}
