<script>
  import Banner from '#lib/ui/Banner.svelte';
  import { resolve } from '$app/paths';
  import { onMount, tick, untrack } from 'svelte';
  import { api, coverUrl } from '#lib/api.js';
  import { formatEventTitle } from '#lib/eventTitle.js';
  import { loadPref, savePref } from '#lib/prefs.js';
  import Modal from '#lib/ui/Modal.svelte';
  import EmptyState from '#lib/ui/EmptyState.svelte';

  // 「今天」必须随时间推进：此前是组件创建时冻结的 Date，页面长期停留跨天后
  // 今日高亮与「今天」按钮都会停在旧日期。
  const boot = new Date();
  let now = $state(boot);

  // 从 URL 初始化年月（?y=&m=），便于刷新与分享
  const params = new URLSearchParams(location.search);
  const qy = parseInt(params.get('y'), 10);
  const qm = parseInt(params.get('m'), 10);

  let year = $state(qy > 2000 ? qy : boot.getFullYear());
  let month = $state(qm >= 1 && qm <= 12 ? qm : boot.getMonth() + 1);
  let events = $state([]);
  let loading = $state(true);
  let error = $state('');
  let modalDay = $state(null);
  let showYearPicker = $state(false);
  let pickerYear = $state(null);

  // 中国节假日（含调休补班）：按年缓存，键为完整日期 "YYYY-MM-DD"。
  // 属于锦上添花信息：加载失败静默降级，不影响日历本身的展示。
  let holidays = $state({});

  async function ensureHolidays(y) {
    if (holidays[y]) return;
    try {
      const data = await api.getHolidays(y);
      holidays = { ...holidays, [y]: data?.days || {} };
    } catch (e) {
      // 静默降级：拿不到节假日就只不显示角标
    }
  }

  // 年份选择器的上下限：shiftYear 原本可以一路点到公元几十万年。有数据的年份
  // 未知，夹到一个合理区间比不设限更不容易误操作。
  const MIN_YEAR = 1900;
  const maxYear = () => now.getFullYear() + 5;

  // 「只看订阅范围」：ICS 订阅（/api/calendar.ics）不导出 已取消(2)/未赴约(3)，
  // App 日历默认显示全部，两者肉眼看来不一致且无任何说明。这里给出显式开关 +
  // 页脚计数说明，让用户自己选择要不要对齐。
  const SUBSCRIBED_STATUSES = [0, 1]; // 正常 / 想看
  let onlySubscribed = $state(loadPref('mujian:calendar_only_subscribed', false));
  const isSubscribed = (e) => SUBSCRIBED_STATUSES.includes(e.active_status ?? 0);

  function toggleOnlySubscribed() {
    onlySubscribed = !onlySubscribed;
    savePref('mujian:calendar_only_subscribed', onlySubscribed);
  }

  function openYearPicker() {
    pickerYear = year;
    showYearPicker = true;
  }

  function shiftYear(dy) {
    // 同上：夹到合法区间，并把禁用状态交给 aria-disabled 表达。
    pickerYear = Math.min(maxYear(), Math.max(MIN_YEAR, (pickerYear ?? year) + dy));
  }

  function selectMonth(m) {
    year = pickerYear;
    month = m;
    showYearPicker = false;
    load(true);
  }

  // 请求序号：连续翻月时两个 getCalendar 乱序返回会把旧月份的活动渲染到
  // 当前视图，用序号丢弃过期响应。
  let calReqSeq = 0;

  // 主动翻月写入 history，浏览器前进/后退即可在月份之间穿梭；首次进入与
  // popstate 回填用 replaceState，避免凭空多出历史项。保留 SvelteKit 的
  // 历史状态（sveltekit:index 等），否则返回按钮会因历史索引被清空而回退到
  // 主页而非日历。
  function writeURL(push) {
    const u = new URL(location.href);
    u.searchParams.set('y', String(year));
    u.searchParams.set('m', String(month));
    if (push) history.pushState(window.history.state ?? {}, '', u);
    else history.replaceState(window.history.state ?? {}, '', u);
  }

  async function load(push = false) {
    const seq = ++calReqSeq;
    loading = true;
    error = '';
    ensureHolidays(year); // 与日历数据并行拉取，互不影响
    try {
      const ev = await api.getCalendar(year, month);
      if (seq !== calReqSeq) return; // 已翻到别的月份，丢弃本次响应
      events = ev;
      writeURL(push);
    } catch (e) {
      if (seq === calReqSeq) {
        error = e.message;
        // 月份标题已经切换，保留上个月的 events 会让旧数据顶着新月份展示
        //（计数、统计条也随之失真）；失败时清空，让错误横幅单独说明。
        events = [];
      }
    } finally {
      if (seq === calReqSeq) loading = false;
    }
  }

  function shift(dm) {
    let y = year;
    let m = month + dm;
    while (m < 1) {
      m += 12;
      y -= 1;
    }
    while (m > 12) {
      m -= 12;
      y += 1;
    }
    year = y;
    month = m;
    load(true);
  }

  function goToday() {
    year = now.getFullYear();
    month = now.getMonth() + 1;
    load(true);
  }

  // 关闭当日弹窗并把焦点还给触发它的日期格：关闭按钮 / 遮罩 / Esc 都会
  // 让焦点随弹窗销毁落回 body，此后方向键全部失灵（macOS Safari 默认
  // Tab 又不移动焦点，用户无法自行回到网格）。
  async function closeDayModal() {
    const d = modalDay;
    modalDay = null;
    if (d !== null) await focusDayCell(d);
  }

  // PageUp / PageDown 全局翻月：此前只绑在网格 keydown 上，焦点不在日期
  // 格时按了无反应（默认行为变成滚动页面）。方向键仍只在网格内生效，
  // 避免劫持页面滚动。
  // Esc 由 ui/Modal 统一处理（document 捕获阶段 + stopPropagation，
  // 不会和这里的 window 监听叠加成双重关闭）。
  function onWindowKey(e) {
    if (e.key !== 'PageUp' && e.key !== 'PageDown') return;
    if (showYearPicker || modalDay !== null) return; // 弹窗期间不翻月
    const t = e.target;
    const tag = t?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || t?.isContentEditable) return;
    e.preventDefault(); // 阻止浏览器把 PageUp/Down 解释为页面滚动
    if (e.key === 'PageUp') shift(-1);
    else shift(1);
  }

  // 浏览器前进/后退：pushState 不触发 popstate，所以这只可能是真正的历史导航。
  function onPopState() {
    const p = new URLSearchParams(location.search);
    const y = parseInt(p.get('y'), 10);
    const m = parseInt(p.get('m'), 10);
    if (!y || !m || (y === year && m === month)) return;
    year = y;
    month = m;
    load(false);
  }

  const DOW = ['一', '二', '三', '四', '五', '六', '日'];
  const MAX_SHOW = 4;

  // 根据当天演出数量决定海报网格布局
  // 返回 { cols, rows }，保证尽量用最少格子填满
  function posterLayout(n) {
    if (n <= 1) return { cols: 1, rows: 1 };
    if (n === 2) return { cols: 2, rows: 1 };
    if (n === 3) return { cols: 2, rows: 2, spans: [1, 0, 0, 0] }; // 左列跨两行
    if (n <= 4) return { cols: 2, rows: 2 };
    return { cols: 2, rows: 2 }; // 4+ 显示 4 张 + 溢出角标
  }

  // 后端返回的本月全部演出（含 已取消/未赴约）。
  // visibleEvents 是「只看订阅范围」开关过滤后的结果，UI 计数与按日分组一律用它。
  const visibleEvents = $derived(onlySubscribed ? events.filter(isSubscribed) : events);
  const hiddenCount = $derived(events.length - visibleEvents.length);

  // 月度小结里的金额：整数不带小数，非整数保留两位；符号规则与 formatCurrency 一致。
  function moneyText(amount, currency) {
    const n = Math.round((Number(amount) || 0) * 100) / 100;
    const s = n.toLocaleString('zh-CN', {
      minimumFractionDigits: Number.isInteger(n) ? 0 : 2,
      maximumFractionDigits: 2
    });
    return (currency || 'CNY') === 'CNY' ? `¥${s}` : `${currency} ${s}`;
  }

  // 月度统计：跟随「只看订阅范围」后的可见集合，与下方日历逐项对应。
  // 费用口径：已关联货殖账单的演出取货殖金额（优先于记录内费用），其余
  // 取记录内 TotalCost；同一张账单可能关联多场，按账单 ID 只计一次。
  // 多币种时不做汇率换算，展示金额最大的币种并在提示里列全。
  const monthStats = $derived.by(() => {
    const costs = {}; // currency -> { total, count }
    const cities = {};
    const venues = {};
    const status = { 0: 0, 1: 0, 2: 0, 3: 0 };
    const seenBills = {};
    let rated = 0;
    let ratingSum = 0;
    let huozhiUsed = false;
    let huozhiIncomplete = 0;

    const addCost = (amount, currency) => {
      const n = Number(amount) || 0;
      if (n <= 0) return false;
      const cur = currency || 'CNY';
      (costs[cur] ??= { total: 0, count: 0 }).total += n;
      return true;
    };

    for (const e of visibleEvents) {
      // 本场实际计入的币种；均价按「有费用的场次」计数，而非按账单张数。
      const contributed = {};
      const bills = Array.isArray(e.huozhiBills) ? e.huozhiBills : [];
      if (bills.length) {
        huozhiUsed = true;
        for (const b of bills) {
          const key = String(b.id);
          if (seenBills[key]) continue; // 跨演出共享的账单只计一次
          seenBills[key] = true;
          if (addCost(b.amount, b.currency)) contributed[b.currency || 'CNY'] = true;
        }
      } else if (addCost(e.totalCost, e.costCurrency)) {
        contributed[e.costCurrency || 'CNY'] = true;
      }
      for (const cur of Object.keys(contributed)) costs[cur].count += 1;
      // 关联了账单却只取到一部分（或全部没取到）：金额按已取到的计，并提示。
      if ((e.huozhiLinked ?? 0) > bills.length) huozhiIncomplete += 1;

      if (e.rating > 0) {
        ratingSum += e.rating;
        rated += 1;
      }
      if (e.city) cities[e.city] = true;
      if (e.address) venues[e.address] = true;
      const st = e.active_status ?? 0;
      if (status[st] !== undefined) status[st] += 1;
    }

    const ranked = Object.entries(costs).sort((a, b) => b[1].total - a[1].total);
    const primary = ranked[0] ?? null;
    const statusBits = [
      status[0] ? `已看 ${status[0]}` : '',
      status[1] ? `想看 ${status[1]}` : '',
      status[2] ? `已取消 ${status[2]}` : '',
      status[3] ? `未赴约 ${status[3]}` : ''
    ].filter(Boolean);
    const costTitle = [
      huozhiUsed
        ? '已关联货殖的演出取货殖账单合计（同一账单只计一次），其余取记录内费用'
        : '按记录内填写的实付/票价 + 其他费用合计'
    ];
    if (huozhiIncomplete) {
      costTitle.push(`⚠ ${huozhiIncomplete} 场关联账单未完整取到，按已取到的金额计`);
    }
    costTitle.push(
      ...(ranked.length
        ? ranked.map(([cur, v]) => `${cur} ${v.total.toFixed(2)}`)
        : ['本月记录尚未填写费用'])
    );

    return {
      count: visibleEvents.length,
      statusText: statusBits.join(' · '),
      costText: primary ? moneyText(primary[1].total, primary[0]) : '—',
      costSub: primary
        ? `均价 ${moneyText(primary[1].total / primary[1].count, primary[0])}${
            ranked.length > 1 ? ` · ${ranked.length} 种币种` : ''
          }`
        : '未填费用',
      costTitle: costTitle.join('\n'),
      huozhiUsed,
      huozhiIncomplete,
      cityCount: Object.keys(cities).length,
      venueCount: Object.keys(venues).length,
      avgRating: rated ? (ratingSum / rated).toFixed(1) : '—',
      rated
    };
  });

  // 月历网格：周一起始，前后补空，行数固定为 7 的倍数
  const cells = $derived.by(() => {
    const offset = (new Date(year, month - 1, 1).getDay() + 6) % 7;
    const days = new Date(year, month, 0).getDate();
    const total = Math.ceil((offset + days) / 7) * 7;
    const arr = [];
    for (let i = 0; i < total; i++) {
      const d = i - offset + 1;
      arr.push(d >= 1 && d <= days ? d : null);
    }
    return arr;
  });

  // 按周切分：合法的 role=grid 必须有 row 层（grid → row → gridcell），
  // 此前 42 个 gridcell 直接挂在 grid 下，读屏无法识别行列。
  const weeks = $derived.by(() => {
    const out = [];
    for (let i = 0; i < cells.length; i += 7) out.push(cells.slice(i, i + 7));
    return out;
  });

  // roving tabindex：整张网格只有一个可 Tab 到 的格子，格间用方向键移动，
  // 否则键盘用户要按 28~42 次 Tab 才能走完一个月。
  let focusDay = $state(null);
  let gridEl = $state(null);

  // roving tab 的落点：优先今天，否则本月 1 号。
  // 依赖只有 year/month —— 其余状态读如果进了 effect 的依赖表会在每次双向更新里反复触发
  // （Svelte 5 的 effect 依赖是在首个 await 之前同步读到的所有 $state）。
  function recalcFocus() {
    if (focusDay !== null && cells.includes(focusDay)) return;
    if (
      month === now.getMonth() + 1 &&
      year === now.getFullYear() &&
      cells.includes(now.getDate())
    ) {
      focusDay = now.getDate();
      return;
    }
    focusDay = cells.find((d) => d !== null) ?? null;
  }

  $effect(() => {
    year;
    month;
    untrack(recalcFocus);
  });

  async function focusDayCell(d) {
    focusDay = d;
    await tick();
    gridEl?.querySelector(`[data-day="${d}"]`)?.focus({ preventScroll: false });
  }

  // 方向键在网格内移动；跨出本月上下边界时连带翻月（标准日期选择器的手感）。
  // PageUp / PageDown 已上移到 onWindowKey 全局处理，这里不再接管，避免双重翻月。
  async function onGridKey(e) {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home'].includes(e.key)) return;
    const days = cells.filter((d) => d !== null);
    if (days.length === 0) return;
    e.preventDefault();
    if (e.key === 'Home') {
      goToday();
      return;
    }
    const cur = focusDay ?? days[0];
    const idx = days.indexOf(cur);
    if (idx === -1) {
      focusDayCell(days[0]);
      return;
    }
    const step =
      e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : e.key === 'ArrowUp' ? -7 : 7;
    const next = idx + step;
    if (next >= 0 && next < days.length) {
      focusDayCell(days[next]);
      return;
    }
    // 越界翻月：把「第几天」带到新月份（夹到新月份的最后一天）
    const monthLen = new Date(year, month, 0).getDate();
    const targetDay = Math.min(days[idx] + step, monthLen);
    shift(step < 0 ? -1 : 1);
    await new Promise((r) => setTimeout(r, 0));
    const newLen = new Date(year, month, 0).getDate();
    if (targetDay >= 1 && targetDay <= newLen) await focusDayCell(targetDay);
  }

  // 按日分组（后端已按时间升序返回）
  const byDay = $derived.by(() => {
    const map = {};
    for (const e of visibleEvents) {
      const d = new Date(e.date * 1000).getDate();
      (map[d] ??= []).push(e);
    }
    return map;
  });

  function isToday(d) {
    return d === now.getDate() && month === now.getMonth() + 1 && year === now.getFullYear();
  }

  function pad(n) {
    return String(n).padStart(2, '0');
  }

  function dateStr(d) {
    return `${year}-${pad(month)}-${pad(d)}`;
  }

  function openNew(d) {
    modalDay = null;
    // 本应用 SPA（ssr=false + adapter-static fallback）下，程序化 goto() 导航
    // 在此环境不可用；改用整页跳转。location.href 会写入新的浏览器历史项，
    // 新增页的返回按钮仍能回到日历。这与首页「＋ 新建记录」的 <a href> 一致可靠。
    window.location.href = `/records/new?date=${dateStr(d)}`;
  }

  function posterSrc(e) {
    return coverUrl(e.coverThumb || e.coverFile || '');
  }

  const STATUS_LABEL = { 1: '想看', 2: '已取消', 3: '未赴约' };
  const STATUS_CLASS = { 1: 'status-wish', 2: 'status-cancel', 3: 'status-miss' };
  const statusLabel = (s) => STATUS_LABEL[s] || '';
  const statusClass = (s) => STATUS_CLASS[s] || '';

  // 点击日期：空白日期直接新增演出（与底部「点击空白日期可新增」提示一致）；
  // 有演出的日期弹出当日弹窗，查看演出并可再点「+ 新增演出」。
  function onCellClick(d) {
    if ((byDay[d] ?? []).length === 0) {
      openNew(d);
    } else {
      modalDay = d;
    }
  }

  function timeStr(ts) {
    const d = new Date(ts * 1000);
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  // 当日弹窗头部显示的节假日信息（休/班）
  const modalHoliday = $derived(modalDay !== null ? holidays[year]?.[dateStr(modalDay)] : null);

  function categoryColor(name) {
    // 简单哈希生成稳定的柔和色彩
    if (!name) return 'cat-default';
    let h = 0;
    for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
    return `cat-${h % 12}`;
  }

  // 演出显示名（「剧种《剧名》」）的规则下沉到 #lib/eventTitle.js：
  // 此前只有日历页用这套拼法，列表页显示裸名，同一场演出两个叫法。

  onMount(() => {
    load();
    // 「今天」跟随系统时间推进：分钟级轮询 + 页面重新可见时立即校正，
    // 保证长期停留的后台标签页跨天后高亮不会错。
    const timer = setInterval(() => (now = new Date()), 60_000);
    const onVisible = () => {
      if (!document.hidden) now = new Date();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('popstate', onPopState);
    return () => {
      clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('popstate', onPopState);
    };
  });
</script>

<svelte:window onkeydown={onWindowKey} />
<svelte:head><title>日历 - 幕间</title></svelte:head>

<div class="fade-up">
  {#if error}<Banner tone="error">{error}</Banner>{/if}

  <section class="calendar-card card" aria-label={`${year}年${month}月日历`}>
    <!-- 工具栏：← 今天  YYYY 年 M 月  →   场次 -->
    <header class="cal-header">
      <button class="nav-btn" onclick={() => shift(-1)} aria-label="上个月" type="button">
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M15 18l-6-6 6-6" />
        </svg>
      </button>
      <button class="today-btn" onclick={goToday} type="button">今天</button>
      <button
        class="ym-display"
        type="button"
        onclick={openYearPicker}
        aria-label={`选择年份与月份，当前 ${year} 年 ${month} 月`}
        aria-haspopup="dialog"
      >
        <span class="y">{year}</span>
        <span class="sep">年</span>
        <span class="m">{month}</span>
        <span class="sep2">月</span>
        <svg
          class="ym-caret"
          viewBox="0 0 24 24"
          width="14"
          height="14"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
      <button class="nav-btn" onclick={() => shift(1)} aria-label="下个月" type="button">
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M9 18l6-6-6-6" />
        </svg>
      </button>
    </header>

    <!-- 月度小结：场次 / 开销 / 城市 / 评分。加载时用等高骨架占位，避免切月跳动 -->
    {#if loading}
      <div class="month-stats" aria-hidden="true">
        {#each [0, 1, 2, 3] as i (i)}
          <div class="ms-item">
            <span class="skeleton ms-skel ms-skel-label"></span>
            <span class="skeleton ms-skel ms-skel-value"></span>
            <span class="skeleton ms-skel ms-skel-sub"></span>
          </div>
        {/each}
      </div>
    {:else if !error}
      <div class="month-stats" role="group" aria-label="本月统计">
        <div class="ms-item">
          <span class="ms-label">场次</span>
          <span class="ms-value">{monthStats.count}<span class="ms-unit">场</span></span>
          <span class="ms-sub" title={monthStats.statusText}
            >{monthStats.statusText || '暂无演出'}</span
          >
        </div>
        <div class="ms-item ms-cost" title={monthStats.costTitle}>
          <span class="ms-label"
            >开销{#if monthStats.huozhiUsed || monthStats.huozhiIncomplete}<span
                class="ms-src"
                class:partial={monthStats.huozhiIncomplete}
                >{monthStats.huozhiIncomplete ? '货殖未全' : '含货殖'}</span
              >{/if}</span
          >
          <span class="ms-value">{monthStats.costText}</span>
          <span class="ms-sub">{monthStats.costSub}</span>
        </div>
        <div class="ms-item">
          <span class="ms-label">城市</span>
          <span class="ms-value"
            >{monthStats.cityCount || '—'}{#if monthStats.cityCount}<span class="ms-unit">城</span
              >{/if}</span
          >
          <span class="ms-sub"
            >{monthStats.venueCount ? `${monthStats.venueCount} 座场馆` : '暂无场地'}</span
          >
        </div>
        <div class="ms-item">
          <span class="ms-label">平均评分</span>
          <span class="ms-value"
            >{#if monthStats.rated}<span class="ms-star">★</span
              >{monthStats.avgRating}{:else}—{/if}</span
          >
          <span class="ms-sub">{monthStats.rated ? `${monthStats.rated} 场已评` : '暂无评分'}</span>
        </div>
      </div>
    {/if}

    <!-- ICS 订阅只包含 正常/想看，App 日历默认显示全部：默认给出的解释 + 一键对齐开关 -->
    <div class="cal-notice">
      <label class="sub-toggle">
        <input type="checkbox" checked={onlySubscribed} onchange={toggleOnlySubscribed} />
        <span>只看订阅范围（不含已取消 / 未赴约）</span>
      </label>
      {#if hiddenCount > 0}
        <span class="sub-note">本月另有 {hiddenCount} 场未进入日历订阅</span>
      {/if}
    </div>

    {#if loading}
      <!-- 骨架行数必须等于真实行数，否则切月时网格会高度跳动（28/35/42 三种）。
           星期行同样保留，切月前后高度完全一致。 -->
      <div class="grid-wrap">
        <div class="dow" aria-hidden="true">
          {#each DOW as w, i (i)}
            <div class="dow-cell" class:weekend={i >= 5}>{w}</div>
          {/each}
        </div>
        {#each weeks as week, wi (wi)}
          <div class="week">
            {#each week as _, i (i)}
              <div class="cell blank-skel"></div>
            {/each}
          </div>
        {/each}
      </div>
    {:else}
      <!-- 合法的 grid → row → gridcell 三层结构 + roving tabindex -->
      <div
        class="grid-wrap"
        bind:this={gridEl}
        role="grid"
        tabindex="-1"
        aria-label={`${year}年${month}月`}
        onkeydown={onGridKey}
      >
        <div class="dow" role="row">
          {#each DOW as w, i (i)}
            <div
              class="dow-cell"
              class:weekend={i >= 5}
              role="columnheader"
              aria-label={`星期${w}`}
            >
              {w}
            </div>
          {/each}
        </div>
        {#each weeks as week, wi (wi)}
          <div class="week" role="row">
            {#each week as d, i (i)}
              <div class="cell-slot" role="gridcell">
                {#if d === null}
                  <div class="cell empty" aria-hidden="true"></div>
                {:else}
                  {@const evs = byDay[d] ?? []}
                  {@const hd = holidays[year]?.[dateStr(d)]}
                  {@const hasToday = isToday(d)}
                  {@const isWeekend = i >= 5}
                  {@const showCount = Math.min(evs.length, MAX_SHOW)}
                  {@const overflow = evs.length > MAX_SHOW ? evs.length - MAX_SHOW : 0}
                  {@const layout = posterLayout(showCount)}
                  <button
                    type="button"
                    class="cell"
                    data-day={d}
                    tabindex={focusDay === d ? 0 : -1}
                    class:today={hasToday}
                    class:weekend={isWeekend}
                    class:has-event={evs.length > 0}
                    aria-label={`${year}年${month}月${d}日${hd ? '，' + (hd.off ? '休假' : '调休上班') + (hd.name ? '，' + hd.name : '') : ''}${evs.length ? '，' + evs.length + '场演出' : ''}`}
                    title={hd?.name ? `${hd.name}${hd.off ? '（休）' : '（班）'}` : undefined}
                    onclick={() => onCellClick(d)}
                    onfocus={() => (focusDay = d)}
                  >
                    <!-- 背景海报层：填满整个格子 -->
                    {#if evs.length}
                      <div
                        class="poster-grid"
                        style="grid-template-columns: repeat({layout.cols}, 1fr); grid-template-rows: repeat({layout.rows}, 1fr);"
                      >
                        {#each evs.slice(0, MAX_SHOW) as e, idx (e.id)}
                          <span
                            class="p-slot"
                            class:dimmed={e.active_status === 2 || e.active_status === 3}
                            class:wish={e.active_status === 1}
                            class:span-full={(layout.spans?.[idx] ?? 0) === 1}
                            title={formatEventTitle(e.name, e.categoryName, e.categoryNames)}
                          >
                            {#if posterSrc(e)}
                              <img src={posterSrc(e)} alt="" loading="lazy" />
                            {:else}
                              <span class="p-ph">{e.name?.[0] ?? '?'}</span>
                            {/if}
                          </span>
                        {/each}
                      </div>
                      {#if overflow}
                        <span class="more-badge">+{overflow}</span>
                      {/if}
                    {/if}

                    <!-- 日期徽章：始终覆盖在格子左上角 -->
                    <span
                      class="d-badge"
                      class:today-badge={hasToday}
                      class:weekend-badge={isWeekend && !evs.length}
                    >
                      {d}
                      {#if hasToday && !evs.length}<span class="today-dot"></span>{/if}
                    </span>
                    <!-- 节假日 休/班 角标：独立放在格子右上角，实底白字，
                         海报与暗色背景上都清晰，也不与左上日期徽章挤在一起 -->
                    {#if hd}
                      <span class="h-badge" class:h-off={hd.off} class:h-work={!hd.off}
                        >{hd.off ? '休' : '班'}</span
                      >
                    {/if}
                    <!-- 节日名：仅无海报的格子有空间展示，避免与演出海报抢位 -->
                    {#if hd?.name && !evs.length}
                      <span class="h-name">{hd.name}</span>
                    {/if}
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/each}
      </div>
      {#if visibleEvents.length === 0}
        <div class="month-empty">本月没有演出记录</div>
      {/if}
    {/if}
  </section>

  <p class="hint">
    点击日期查看当天演出 · 点击空白日期可新增 · PageUp / PageDown 随时翻月 ·
    点开日期关闭后可用方向键移动<br />
    <span class="hint-holiday"
      ><i class="hb-off">休</i> 法定节假日 · <i class="hb-work">班</i> 调休补班日</span
    >
  </p>
</div>

{#if modalDay !== null}
  {@const evs = byDay[modalDay] ?? []}
  <!-- 弹窗外壳（遮罩/滚动锁/Esc/焦点陷阱/焦点回位）统一走 ui/Modal；
       关闭时再由 closeDayModal 把焦点还给触发它的日期格。 -->
  <Modal
    open
    onclose={closeDayModal}
    padded={false}
    width="min(440px, 100%)"
    maxHeight="min(86vh, 720px)"
    label={`${month}月${modalDay}日`}
  >
    <header class="modal-head">
      <div class="modal-date">
        <span class="modal-month">{month}月</span>
        <span class="modal-day">{modalDay}</span>
      </div>
      <div class="modal-sub">
        {#if modalHoliday}
          <span class="modal-holiday" class:mh-work={!modalHoliday.off} title={modalHoliday.name}>
            {modalHoliday.name}{modalHoliday.off ? ' · 休' : ' · 班（调休）'}
          </span>
        {/if}
        {#if evs.length}
          <span class="modal-sub-text">共 {evs.length} 场演出</span>
        {:else}
          <span class="modal-sub-text">暂无演出</span>
        {/if}
      </div>
    </header>

    {#if evs.length}
      <ul class="day-list" role="list">
        {#each evs as e (e.id)}
          <li class="day-item" role="listitem">
            <a class="day-link" href={resolve(`/records/${e.id}`)}>
              {#if posterSrc(e)}
                <!-- 不带 .coverable：layout 中全局灯箱是捕获阶段监听，会 preventDefault
                     把「点海报进详情」改写成看大图，且 Esc 会同时关掉灯箱和当日弹窗。
                     整行已经是详情链接，这里保留纯展示语义。 -->
                <img src={posterSrc(e)} alt="" width="52" height="70" loading="lazy" />
              {:else}
                <span class="diph">{e.name?.[0] ?? '?'}</span>
              {/if}
              <div class="di-body">
                <div class="di-top">
                  <span class="di-name"
                    >{formatEventTitle(e.name, e.categoryName, e.categoryNames)}</span
                  >
                  {#if statusLabel(e.active_status)}
                    <span class="di-status {statusClass(e.active_status)}"
                      >{statusLabel(e.active_status)}</span
                    >
                  {/if}
                </div>
                <div class="di-meta">
                  <span class="di-time">{timeStr(e.date)}</span>
                  {#if e.categoryName}
                    <span class="di-tag {categoryColor(e.categoryName)}">{e.categoryName}</span>
                  {/if}
                  {#if e.city}<span class="di-city">{e.city}</span>{/if}
                </div>
                {#if e.address}<div class="di-addr">{e.address}</div>{/if}
              </div>
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <EmptyState bare title="这一天还没有演出记录">
        {#snippet icon()}
          <svg
            viewBox="0 0 24 24"
            width="40"
            height="40"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <rect x="3" y="4" width="18" height="18" rx="2" />
            <path d="M3 9h18M8 2v4M16 2v4" />
          </svg>
        {/snippet}
      </EmptyState>
    {/if}

    {#snippet footer()}
      <button class="btn ghost sm" onclick={closeDayModal} type="button">关闭</button>
      <button class="btn primary sm" onclick={() => openNew(modalDay)} type="button"
        >+ 新增演出</button
      >
    {/snippet}
  </Modal>
{/if}

{#if showYearPicker}
  <Modal bind:open={showYearPicker} padded={false} width="min(360px, 100%)" label="选择年份与月份">
    <header class="ym-head">
      <button
        class="nav-btn"
        type="button"
        onclick={() => shiftYear(-1)}
        aria-label="上一年"
        disabled={pickerYear <= MIN_YEAR}
      >
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M15 18l-6-6 6-6" />
        </svg>
      </button>
      <span class="ym-title">{pickerYear} 年</span>
      <button
        class="nav-btn"
        type="button"
        onclick={() => shiftYear(1)}
        aria-label="下一年"
        disabled={pickerYear >= maxYear()}
      >
        <svg
          viewBox="0 0 24 24"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M9 18l6-6-6-6" />
        </svg>
      </button>
    </header>
    <div class="month-grid">
      {#each Array(12) as _, i (i)}
        <button
          class="month-cell"
          class:current={pickerYear === year && i + 1 === month}
          type="button"
          onclick={() => selectMonth(i + 1)}
          aria-label={`${pickerYear} 年 ${i + 1} 月`}
          aria-current={pickerYear === year && i + 1 === month ? 'true' : undefined}
          >{i + 1} 月</button
        >
      {/each}
    </div>
  </Modal>
{/if}

<style>
  /* ============ 外层卡片 ============ */
  .calendar-card {
    padding: 0;
    overflow: hidden;
    border-radius: var(--radius-xl);
  }

  /* ============ 头部导航 ============ */
  .cal-header {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 14px 16px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, var(--surface-2) 0%, var(--surface) 100%);
  }

  .nav-btn {
    width: 32px;
    height: 32px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-3);
    cursor: pointer;
    flex-shrink: 0;
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease);
  }
  .nav-btn:hover {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .nav-btn:active {
    transform: scale(0.94);
  }
  .nav-btn:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .nav-btn:disabled:hover {
    background: transparent;
    color: var(--text-3);
  }

  .today-btn {
    height: 28px;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text-2);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    flex-shrink: 0;
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease);
  }
  .today-btn:hover {
    background: var(--accent-soft);
    color: var(--accent);
    border-color: var(--accent);
  }

  .ym-display {
    display: inline-flex;
    align-items: baseline;
    gap: 2px;
    background: transparent;
    border: none;
    margin: 0;
    padding: 4px 8px;
    border-radius: var(--radius-sm);
    color: inherit;
    font-family: inherit;
    cursor: pointer;
    transition:
      background var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease);
    white-space: nowrap;
  }
  .ym-display:hover {
    background: var(--accent-soft);
  }
  .ym-display:active {
    transform: scale(0.97);
  }
  .ym-caret {
    align-self: center;
    margin-left: 2px;
    color: var(--text-3);
    transition:
      transform var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease);
  }
  .ym-display:hover .ym-caret {
    color: var(--accent);
    transform: translateY(1px);
  }
  .ym-display .y {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-3);
    letter-spacing: 0.06em;
  }
  .ym-display .sep {
    font-size: 13px;
    color: var(--text-3);
    margin-right: 2px;
  }
  .ym-display .sep2 {
    font-size: 13px;
    color: var(--text-3);
    margin-left: 1px;
  }
  .ym-display .m {
    font-size: 16px;
    font-weight: 700;
    color: var(--text);
  }

  /* ============ 月度小结 ============ */
  /* 四栏等宽：场次 / 开销 / 城市 / 评分。加载骨架与真实内容同高，切月不跳动。 */
  .month-stats {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    padding: 10px 14px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .ms-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    padding: 0 14px;
  }
  .ms-item:first-child {
    padding-left: 2px;
  }
  .ms-item + .ms-item {
    border-left: 1px solid var(--border);
  }
  .ms-label {
    font-size: 11px;
    color: var(--text-3);
    letter-spacing: 0.08em;
    line-height: 1.2;
    white-space: nowrap;
  }
  /* 开销来源角标：有货殖账单参与时才出现，未取全时改为警示色 */
  .ms-src {
    display: inline-block;
    margin-left: 5px;
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--success-soft);
    color: var(--success);
    font-size: 9.5px;
    font-weight: 600;
    letter-spacing: 0;
    vertical-align: 1px;
  }
  .ms-src.partial {
    background: var(--gold-soft);
    color: var(--gold);
  }
  .ms-value {
    display: flex;
    align-items: baseline;
    gap: 1px;
    font-family: var(--font-serif);
    font-size: 20px;
    font-weight: 700;
    color: var(--text);
    line-height: 1.15;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ms-item:first-child .ms-value {
    color: var(--accent);
  }
  .ms-unit {
    margin-left: 3px;
    font-family: var(--font-sans);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3);
    letter-spacing: 0.04em;
  }
  .ms-sub {
    font-size: 11px;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ms-star {
    margin-right: 2px;
    font-size: 13px;
    color: var(--gold);
  }
  .ms-cost {
    cursor: help;
  }

  .ms-skel {
    display: block;
    border-radius: 4px;
  }
  .ms-skel-label {
    width: 34px;
    height: 10px;
  }
  .ms-skel-value {
    width: 62%;
    height: 20px;
    margin: 2px 0;
  }
  .ms-skel-sub {
    width: 78%;
    height: 10px;
  }

  /* ============ 年历选择弹窗 ============ */
  .ym-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 18px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, var(--surface-2) 0%, var(--surface) 100%);
  }
  .ym-title {
    font-family: var(--font-serif);
    font-size: 18px;
    font-weight: 700;
    color: var(--text);
    letter-spacing: 0.04em;
  }
  .month-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
    padding: 18px;
  }
  .month-cell {
    padding: 15px 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text-2);
    font-family: var(--font-serif);
    font-size: 15px;
    font-weight: 600;
    line-height: 1;
    cursor: pointer;
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease);
  }
  .month-cell:hover {
    border-color: var(--accent);
    color: var(--accent);
    background: var(--accent-softer);
    box-shadow: 0 2px 8px -2px var(--accent-soft);
    transform: translateY(-1px);
  }
  .month-cell:active {
    transform: translateY(0) scale(0.97);
  }
  .month-cell.current {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
    box-shadow: 0 2px 8px -2px var(--accent);
  }
  .month-cell.current:hover {
    background: var(--accent);
    color: #fff;
  }

  /* ============ 星期标题 ============ */
  /* 星期行现在是 role=grid 的直接子行（此前它是 grid 的兄弟节点，
     columnheader 没有 grid 祖先，读屏无法把表头和日期对应起来）。 */
  .dow {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 5px;
    padding: 4px 0 2px;
  }
  .dow-cell {
    text-align: center;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-3);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  .dow-cell.weekend {
    color: var(--gold);
  }

  /* ============ 日期网格 ============ */
  /* grid-wrap → week(row) → cell-slot(gridcell) → button(cell) */
  .grid-wrap {
    display: flex;
    flex-direction: column;
    gap: 5px;
    padding: 6px 12px 14px;
  }
  .week {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 5px;
  }
  .cell-slot {
    min-width: 0;
  }

  .cell {
    aspect-ratio: 1 / 1;
    width: 100%;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    display: block;
    text-align: left;
    cursor: pointer;
    position: relative;
    overflow: hidden;
    font: inherit;
    padding: 0;
    transition:
      border-color var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease);
  }
  .cell:hover:not(.empty) {
    border-color: var(--accent);
    box-shadow: 0 2px 10px -2px var(--accent-soft);
    transform: translateY(-1px);
  }
  .cell:active:not(.empty) {
    transform: translateY(0) scale(0.98);
  }

  .cell.empty {
    background: transparent;
    border-color: transparent;
    cursor: default;
  }

  .cell.blank-skel {
    aspect-ratio: 1 / 1;
    border: none;
    border-radius: var(--radius);
    background-image: linear-gradient(
      90deg,
      var(--surface-3) 25%,
      var(--surface-2) 50%,
      var(--surface-3) 75%
    );
    background-size: 800px 100%;
    animation: shimmer 1.4s ease infinite;
  }

  /* ============ 海报填充层 ============ */
  .poster-grid {
    position: absolute;
    inset: 0;
    display: grid;
    gap: 2px;
    background: var(--surface-2);
  }
  .p-slot {
    position: relative;
    display: block;
    overflow: hidden;
    background: var(--surface-3);
  }
  .p-slot.span-full {
    grid-row: 1 / span 2;
  }
  .p-slot img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.3s var(--ease);
  }
  .cell:hover .p-slot img {
    transform: scale(1.04);
  }
  /* 已取消 / 未赴约 才压暗；「想看」是未来计划，不该与取消同权，只加一圈金色边框 */
  .p-slot.dimmed img {
    filter: grayscale(0.65) opacity(0.55);
  }
  .p-slot.wish::after {
    content: '';
    position: absolute;
    inset: 0;
    box-shadow: inset 0 0 0 2px var(--gold);
    pointer-events: none;
  }
  .p-ph {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: var(--font-serif);
    font-size: clamp(14px, 3.5vw, 22px);
    font-weight: 700;
    color: var(--text-3);
    background: linear-gradient(135deg, var(--surface-3), var(--surface-2));
  }

  /* 海报间微分隔线 */
  .p-slot + .p-slot {
    box-shadow: inset 1px 0 0 var(--surface);
  }
  .poster-grid[style*='grid-template-rows'] .p-slot {
    box-shadow: none;
  }

  /* ============ 日期徽章（叠层左上角） ============ */
  .d-badge {
    position: absolute;
    top: 5px;
    left: 5px;
    z-index: 2;
    font-family: var(--font-serif);
    font-size: 13px;
    font-weight: 700;
    line-height: 1;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 5px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 3px;
    background: var(--surface);
    color: var(--text-2);
    border: 1px solid var(--border);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
    letter-spacing: 0;
  }
  /* 有海报时日期徽章更紧凑、更透明 */
  .has-event .d-badge {
    font-size: 11px;
    min-width: 15px;
    height: 15px;
    padding: 0 3px;
    border-radius: 4px;
    background: rgba(255, 255, 255, 0.9);
    backdrop-filter: blur(2px);
    border: none;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.18);
  }
  :global(.dark) .has-event .d-badge {
    background: rgba(28, 25, 23, 0.88);
    color: var(--text);
  }

  /* 今日徽章 */
  .d-badge.today-badge {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
    box-shadow: 0 2px 6px -1px var(--accent);
  }
  .has-event .d-badge.today-badge {
    background: var(--accent);
    color: #fff;
    border: none;
    box-shadow: 0 2px 6px -1px var(--accent);
  }

  /* 周末纯文本日期 */
  .d-badge.weekend-badge {
    color: var(--gold);
  }

  .today-dot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--accent);
    margin-left: 1px;
  }

  /* ============ 节假日 休/班 角标（格子右上角，实底白字） ============ */
  /* 实底色而非淡底彩字：日期格可能铺满演出海报或处于暗色主题，
     只有白字+饱和底色在两种背景上都清晰。 */
  .h-badge {
    position: absolute;
    top: 4px;
    right: 4px;
    z-index: 2;
    font-size: 10.5px;
    font-weight: 700;
    line-height: 1;
    padding: 3px 5px;
    border-radius: 5px;
    letter-spacing: 0;
    color: #fff;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
    pointer-events: none;
  }
  .h-badge.h-off {
    background: var(--danger);
  }
  .h-badge.h-work {
    background: #15803d;
  }

  /* 节日名：只在无海报的格子里展示，日期徽章正下方 */
  .h-name {
    position: absolute;
    top: 27px;
    left: 6px;
    z-index: 2;
    max-width: calc(100% - 12px);
    font-size: 10px;
    line-height: 1.1;
    font-weight: 600;
    color: var(--danger);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    pointer-events: none;
  }

  /* 当日弹窗头部的节假日说明 */
  .modal-holiday {
    display: inline-block;
    font-size: 12px;
    font-weight: 600;
    color: var(--danger);
    margin-bottom: 3px;
  }
  .modal-holiday.mh-work {
    color: var(--gold);
  }

  /* 页脚图例 */
  .hint-holiday {
    font-size: 11.5px;
    color: var(--text-3);
  }
  .hint-holiday .hb-off,
  .hint-holiday .hb-work {
    display: inline-block;
    font-style: normal;
    font-size: 10px;
    font-weight: 700;
    line-height: 1;
    padding: 2px 4px;
    border-radius: 4px;
    color: #fff;
    vertical-align: 1px;
  }
  .hint-holiday .hb-off {
    background: var(--danger);
  }
  .hint-holiday .hb-work {
    background: #15803d;
  }

  /* 有海报时格子的整体暗化遮罩，让日期徽章更清晰 */
  .has-event::after {
    content: '';
    position: absolute;
    inset: 0;
    pointer-events: none;
    background: linear-gradient(180deg, rgba(0, 0, 0, 0.12) 0%, transparent 30%);
    z-index: 1;
  }
  /* 但 hover 时稍微去掉遮罩让海报更亮 */
  .cell:hover.has-event::after {
    background: linear-gradient(180deg, rgba(0, 0, 0, 0.06) 0%, transparent 25%);
  }

  /* 溢出角标：超过 MAX_SHOW 的场次 */
  .more-badge {
    position: absolute;
    z-index: 3;
    bottom: 4px;
    right: 4px;
    padding: 2px 6px;
    border-radius: 999px;
    background: rgba(28, 25, 23, 0.82);
    backdrop-filter: blur(4px);
    color: #fff;
    font-size: 10px;
    font-weight: 600;
    line-height: 1;
    letter-spacing: 0.02em;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.25);
  }

  /* ============ 「只看订阅范围」说明条 ============ */
  .cal-notice {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    padding: 8px 14px;
    border-bottom: 1px solid var(--border);
    background: var(--surface-2);
  }
  .sub-toggle {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: 12.5px;
    color: var(--text-2);
    cursor: pointer;
    user-select: none;
    white-space: nowrap;
  }
  .sub-toggle input[type='checkbox'] {
    width: 14px;
    height: 14px;
    margin: 0;
    accent-color: var(--accent);
    flex-shrink: 0;
  }
  .sub-note {
    font-size: 12px;
    color: var(--text-3);
  }

  .month-empty {
    padding: 6px 14px 18px;
    text-align: center;
    font-size: 13px;
    color: var(--text-3);
  }

  /* ============ 提示文字 ============ */
  .hint {
    margin-top: 14px;
    text-align: center;
    font-size: 12.5px;
    color: var(--text-3);
  }

  /* ============ Modal ============ */
  /* 遮罩 / 面板外壳 / 底部操作条已由 ui/Modal 提供，这里只留内容样式 */
  .modal-head {
    padding: 22px 24px 16px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, var(--accent-softer) 0%, var(--surface) 100%);
    text-align: center;
  }
  .modal-date {
    display: inline-flex;
    align-items: baseline;
    gap: 4px;
    font-family: var(--font-serif);
  }
  .modal-month {
    font-size: 14px;
    color: var(--text-3);
    font-weight: 500;
    letter-spacing: 0.04em;
  }
  .modal-day {
    font-size: 36px;
    font-weight: 700;
    color: var(--accent);
    line-height: 1;
  }
  .modal-sub {
    margin-top: 6px;
  }
  .modal-sub-text {
    font-size: 13px;
    color: var(--text-3);
  }

  .day-list {
    list-style: none;
    margin: 0;
    padding: 12px 16px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-height: min(50vh, 380px);
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .day-item {
    margin: 0;
    padding: 0;
  }
  .day-link {
    display: flex;
    align-items: stretch;
    gap: 12px;
    padding: 10px;
    border-radius: var(--radius);
    background: var(--surface-2);
    transition:
      background var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease);
    border: 1px solid transparent;
  }
  .day-link:hover {
    background: var(--accent-softer);
    border-color: var(--accent-soft);
    transform: translateX(3px);
  }
  .day-link img,
  .diph {
    width: 44px;
    aspect-ratio: 3 / 4;
    object-fit: cover;
    border-radius: 6px;
    flex-shrink: 0;
  }
  .diph {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--surface-3);
    color: var(--text-3);
    font-size: 18px;
    font-weight: 600;
  }

  .di-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .di-top {
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }
  .di-name {
    font-size: 14.5px;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .di-status {
    font-size: 11px;
    font-weight: 500;
    padding: 2px 7px;
    border-radius: 999px;
    line-height: 1.2;
    flex-shrink: 0;
  }
  .di-status.status-wish {
    background: var(--gold-soft);
    color: var(--gold);
  }
  .di-status.status-cancel {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .di-status.status-miss {
    background: var(--surface-3);
    color: var(--text-muted);
  }

  .di-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-3);
    flex-wrap: wrap;
  }
  .di-time {
    font-weight: 500;
    color: var(--text-2);
  }
  .di-tag {
    font-size: 10.5px;
    padding: 1px 6px;
    border-radius: 4px;
    font-weight: 500;
  }
  .di-addr {
    font-size: 11.5px;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* 分类颜色变体（柔和色调） */
  .cat-default {
    background: var(--surface-3);
    color: var(--text-muted);
  }
  .cat-0 {
    background: rgba(180, 35, 24, 0.08);
    color: var(--accent);
  }
  .cat-1 {
    background: rgba(217, 119, 6, 0.1);
    color: var(--gold);
  }
  .cat-2 {
    background: rgba(21, 128, 61, 0.08);
    color: #15803d;
  }
  .cat-3 {
    background: rgba(59, 130, 246, 0.1);
    color: #3b82f6;
  }
  .cat-4 {
    background: rgba(147, 51, 234, 0.08);
    color: #9333ea;
  }
  .cat-5 {
    background: rgba(236, 72, 153, 0.1);
    color: #ec4899;
  }
  .cat-6 {
    background: rgba(249, 115, 22, 0.1);
    color: #f97316;
  }
  .cat-7 {
    background: rgba(14, 165, 233, 0.1);
    color: #0ea5e9;
  }
  .cat-8 {
    background: rgba(168, 85, 247, 0.1);
    color: #a855f7;
  }
  .cat-9 {
    background: rgba(34, 197, 94, 0.1);
    color: #22c55e;
  }
  .cat-10 {
    background: rgba(244, 63, 94, 0.1);
    color: #f43f5e;
  }
  .cat-11 {
    background: rgba(20, 184, 166, 0.1);
    color: #14b8a6;
  }

  /* 当日弹窗空态已改用 ui/EmptyState（bare），此处不再自带样式 */

  /* ============ 响应式 ============ */
  @media (max-width: 640px) {
    .cal-header {
      padding: 10px 10px;
      gap: 4px;
    }
    .nav-btn {
      width: 30px;
      height: 30px;
    }
    .today-btn {
      height: 26px;
      padding: 0 8px;
      font-size: 11px;
      border-radius: 5px;
    }
    .ym-display {
      gap: 0;
      padding: 3px 6px;
    }
    .ym-display .y {
      font-size: 12px;
      letter-spacing: 0.04em;
    }
    .ym-display .sep,
    .ym-display .sep2 {
      font-size: 11px;
    }
    .ym-display .m {
      font-size: 15px;
    }
    .ym-caret {
      width: 12px;
      height: 12px;
    }

    /* 手机上四栏会挤成两行：2×2 网格，去掉中缝处的分隔线 */
    .month-stats {
      grid-template-columns: repeat(2, minmax(0, 1fr));
      row-gap: 10px;
      padding: 10px 12px 12px;
    }
    .ms-item {
      padding: 0 10px;
    }
    .ms-item:first-child,
    .month-stats .ms-item:nth-child(3) {
      padding-left: 2px;
      border-left: none;
    }
    .ms-value {
      font-size: 17px;
    }

    .dow {
      padding: 2px 0 0;
      gap: 3px;
    }
    .dow-cell {
      font-size: 11px;
      letter-spacing: 0.08em;
    }

    .grid-wrap {
      gap: 3px;
      padding: 4px 8px 12px;
    }
    .week {
      gap: 3px;
    }
    .cell {
      border-radius: 8px;
    }
    .d-badge {
      top: 3px;
      left: 3px;
      font-size: 11px;
      min-width: 15px;
      height: 15px;
      padding: 0 3px;
      gap: 2px;
    }
    .h-badge {
      top: 3px;
      right: 3px;
      font-size: 9.5px;
      padding: 2px 4px;
      border-radius: 4px;
    }
    .h-name {
      top: 22px;
      left: 4px;
      font-size: 9.5px;
    }
    .has-event .d-badge {
      font-size: 10px;
      min-width: 13px;
      height: 13px;
      padding: 0 2px;
    }
    .more-badge {
      font-size: 9px;
      padding: 1px 5px;
      bottom: 3px;
      right: 3px;
    }

    .modal-head {
      padding: 18px 18px 14px;
    }
    .modal-day {
      font-size: 30px;
    }
    .day-list {
      padding: 10px 12px;
    }
    .day-link img,
    .diph {
      width: 38px;
    }
    .di-name {
      font-size: 14px;
    }
  }

  @media (max-width: 420px) {
    .ym-display .y {
      display: none;
    }
    .ym-display .sep {
      display: none;
    }
    .ym-display .sep2 {
      display: none;
    }
    .ym-display .m {
      font-size: 18px;
    }
    .month-grid {
      gap: 6px;
      padding: 14px;
    }
    .month-cell {
      padding: 13px 0;
      font-size: 14px;
    }
  }
</style>
