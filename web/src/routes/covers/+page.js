import { redirect } from '@sveltejs/kit';

// 封面管理已并入「数据」页（数据页的「封面维护」分区），本路由不再是页面。
// 保留 308 跳转，避免旧书签、已安装 PWA 缓存的入口、以及历史链接落到 404。
export function load() {
  redirect(308, '/data');
}
