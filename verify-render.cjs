const { chromium } = require('playwright-core');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const consoleErrors = [];
  const pageErrors = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });
  page.on('pageerror', (err) => pageErrors.push(err.message + '\n' + (err.stack || '')));

  const base = 'http://127.0.0.1:8080';
  await page.goto(base + '/', { waitUntil: 'networkidle' });

  // 等待卡片或空状态出现
  await page.waitForTimeout(1500);

  // 抓取记录数量标题
  const countText = await page.locator('h2 .num').first().textContent().catch(() => 'N/A');
  const cardCount = await page.locator('.record-card-wrapper').count();
  const emptyVisible = await page.locator('.empty').isVisible().catch(() => false);
  const loadingVisible = await page.locator('.skel-card').first().isVisible().catch(() => false);

  // 读取前端实际拿到的 records 数组长度（通过注入 fetch 拦截不可行，改为读取 DOM）
  console.log('=== 渲染结果 ===');
  console.log('记录数标题:', countText);
  console.log('卡片 DOM 数量:', cardCount);
  console.log('空状态可见:', emptyVisible);
  console.log('骨架屏可见(仍在加载):', loadingVisible);
  console.log('=== Console 错误 ===');
  console.log(consoleErrors.join('\n') || '(无)');
  console.log('=== 页面级错误 (pageerror) ===');
  console.log(pageErrors.join('\n---\n') || '(无)');

  // 测试搜索：输入"黎安"
  await page.fill('.search', '黎安');
  await page.waitForTimeout(800);
  const countAfterSearch = await page.locator('h2 .num').first().textContent().catch(() => 'N/A');
  const cardAfterSearch = await page.locator('.record-card-wrapper').count();
  console.log('=== 搜索"黎安"后 ===');
  console.log('记录数标题:', countAfterSearch);
  console.log('卡片 DOM 数量:', cardAfterSearch);

  await browser.close();
})().catch((e) => {
  console.error('SCRIPT_ERROR:', e);
  process.exit(1);
});
