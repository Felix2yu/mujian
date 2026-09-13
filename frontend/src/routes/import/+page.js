import { redirect } from '@sveltejs/kit';

// 兼容旧路径：本页原名「数据」前的 URL 是 /import（当时页面只做导入 / 导出），
// 现已改名 /data。保留 308 跳转，避免旧书签、已安装 PWA 缓存的入口、以及
// 服务端日志里的历史链接落到 404。
export function load() {
  redirect(308, '/data');
}
