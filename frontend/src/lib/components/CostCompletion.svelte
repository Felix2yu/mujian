<script>
  import { onMount } from 'svelte';
  import { api, coverUrl } from '$lib/api.js';

  // ===== 费用补全 =====
  //
  // 语义（与后端 internal/db/cost_review.go 一致）：
  //   · 字段值 > 0                  → 已填写，永不进入待确认清单，也永不被改写；
  //   · 值 NULL（未填写）或 0       → 进入待确认清单；
  //   · 用户对某字段做出确认后，服务端记一条 cost_reviews，该条目随即离开清单。
  //
  // 关键点：历史版本把「未填写」默认存成了 0，所以清单里的 0 元无法与真实
  // 免费区分。本模块不猜测，而是把区分动作交给用户逐项确认 —— 确认记录可撤销，
  // 补全操作只写当前为 NULL/0 的字段，因此不可能误改有效金额。
  const COST_FIELDS = [
    { key: 'price', label: '票价', desc: '票面价格', zeroLabel: '免费', zeroAction: '标记为免费' },
    { key: 'pay_price', label: '实付', desc: '实际支付金额', zeroLabel: '无实付', zeroAction: '标记为无实付' },
    { key: 'other_cost', label: '其他花费', desc: '交通、餐饮等额外支出', zeroLabel: '无支出', zeroAction: '标记为无支出' }
  ];

  let costLoading = $state(false);
  let costError = $state('');
  let costNotice = $state('');
  let costBusy = $state(false);
  let costRecords = $state([]);   // 仍待确认的记录（服务端已排除已标记项）
  let costSummary = $state(null); // 全量概览计数
  let costFieldFilter = $state('all'); // 'all' | 字段 key
  let costValueFilter = $state('pending'); // 'pending' | 'empty' | 'zero'
  let costVenue = $state('');
  let costQuery = $state('');
  // 默认只展开第一个分组：三个字段全部展开会让卡片长达上千像素。其余分组点击
  // 标题展开，保持多开能力。
  let costExpanded = $state({ price: true, pay_price: false, other_cost: false });
  let costSel = $state({ price: [], pay_price: [], other_cost: [] });
  let costRowAmount = $state({}); // `${field}:${id}` -> 行内输入金额

  // 演出预览浮窗：列表里很多演出名称相近（同一剧目的不同场次、连台本戏等），
  // 仅凭名称无法判断该给哪一场补费用。点名称即弹出封面 / 演员 / 剧团 / 备注等
  // 辨识信息。详情按需懒加载（复用 GET /api/records/{id}），并在内存缓存，
  // 避免为几百条待确认记录一次性拉全量详情。
  let costPreview = $state(null); // { id, loading, error, rec, anchor } | null
  let costPreviewCache = {};      // recordID -> Record（同一记录二次点击免请求）
  let costPreviewReq = 0;         // 竞态令牌：只接受最后一次请求的结果

  function costRowKey(field, id) { return `${field}:${id}`; }

  // 费用补全列表里的日期：只到天，避免时区把 19:30 的演出显示成第二天。
  function fmtCostDate(ts) {
    if (!ts) return '—';
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  }

  // 场馆候选（按待确认条数降序）
  let costVenueOptions = $derived.by(() => {
    const map = new Map();
    for (const r of costRecords) {
      if (r.address) map.set(r.address, (map.get(r.address) || 0) + 1);
    }
    return [...map.entries()].sort((a, b) => b[1] - a[1]).map(([address, count]) => ({ address, count }));
  });

  // 场馆 / 关键词筛选
  let costFiltered = $derived.by(() => {
    const q = costQuery.trim().toLowerCase();
    return costRecords.filter((r) => {
      if (costVenue && r.address !== costVenue) return false;
      if (q) {
        const hay = `${r.name || ''} ${r.address || ''} ${r.city || ''}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  });

  // 把记录「按字段展开」成行：一行 = 一条记录的一个待确认字段。
  // kind 用于区分两类待确认：empty = 列为 NULL（确定未填写）；
  // zero = 值为 0（可能是真实免费，也可能是历史默认的脏数据）。
  let costRows = $derived.by(() => {
    const out = {};
    for (const f of COST_FIELDS) out[f.key] = [];
    for (const r of costFiltered) {
      const pending = r.pending_fields || [];
      for (const f of COST_FIELDS) {
        if (!pending.includes(f.key)) continue;
        const raw = r[f.key];
        const kind = raw === null || raw === undefined ? 'empty' : 'zero';
        if (costValueFilter !== 'pending' && costValueFilter !== kind) continue;
        out[f.key].push({ rec: r, kind });
      }
    }
    return out;
  });

  let costVisibleFields = $derived(
    costFieldFilter === 'all' ? COST_FIELDS : COST_FIELDS.filter((f) => f.key === costFieldFilter)
  );

  let costVisibleCount = $derived(
    COST_FIELDS.reduce((n, f) => n + (costRows[f.key]?.length || 0), 0)
  );

  let costPendingCounts = $derived(costSummary?.pending || {});
  let costReviewedCounts = $derived(costSummary?.reviewed || {});
  let costReviewedTotal = $derived(costSummary?.reviewed_total || 0);

  // 生效目标：优先取勾选项，否则作用于当前筛选出的全部行。始终以「当前可见」
  // 取交集，避免筛选切换后残留的选择被误伤。
  function costTargets(field) {
    const visible = costRows[field].map((row) => row.rec.id);
    const sel = (costSel[field] || []).filter((id) => visible.includes(id));
    return sel.length > 0 ? sel : visible;
  }

  function costToggleSel(field, id) {
    const cur = costSel[field] || [];
    costSel = { ...costSel, [field]: cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id] };
  }

  function costSelectAll(field) {
    const visible = costRows[field].map((row) => row.rec.id);
    const cur = (costSel[field] || []).filter((id) => visible.includes(id));
    costSel = { ...costSel, [field]: cur.length === visible.length ? [] : visible };
  }

  // ---- 演出预览浮窗 ----

  // 预览里的日期带星期与时刻：相近名称的场次常靠「哪天几点」区分。
  function fmtPreviewTime(ts) {
    if (!ts) return '—';
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} 周${'日一二三四五六'[d.getDay()]} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  // 浮窗里的金额：整数不带小数（票价 180 而非 180.00），与列表的观感一致。
  function fmtMoney(v, cur) {
    if (v === null || v === undefined) return '未填写';
    const sym = !cur || cur === 'CNY' ? '¥' : cur + ' ';
    const n = Number(v);
    return sym + (Number.isInteger(n) ? String(n) : n.toFixed(2));
  }

  const COST_PREVIEW_W = 340;
  // 浮窗渲染在模板根层级（与本组件挂载点同级，不受 .fade-up 的 transform 影响），
  // 用 fixed 定位，坐标取点击时的视口矩形。因此任何滚动都会让坐标失准 —— 见下面的
  // 滚动即关闭。
  let costPreviewPos = $derived.by(() => {
    const a = costPreview?.anchor;
    if (!a || typeof window === 'undefined') return { left: 12, top: 12, maxH: 420 };
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const left = Math.max(12, Math.min(a.left, vw - COST_PREVIEW_W - 12));
    const below = a.bottom + 8;
    // 下方放不下（按 340px 估高）就翻到上方，保证浮窗不被视口下缘截断。
    const top = vh - below >= 340 ? below : Math.max(12, a.top - 8 - Math.min(340, vh - 24));
    return { left, top, maxH: Math.max(180, vh - top - 12) };
  });

  function costClosePreview() {
    costPreview = null;
  }

  async function costTogglePreview(rec, event) {
    // 点同一条 = 收起（再点一次关闭，避免只能靠点空白处退出）
    if (costPreview?.id === rec.id) {
      costPreview = null;
      return;
    }
    const r = event?.currentTarget?.getBoundingClientRect?.();
    const anchor = r ? { left: r.left, top: r.top, bottom: r.bottom } : null;

    const cached = costPreviewCache[rec.id];
    if (cached) {
      costPreview = { id: rec.id, loading: false, error: '', rec: cached, anchor };
      return;
    }
    const token = ++costPreviewReq;
    costPreview = { id: rec.id, loading: true, error: '', rec: null, anchor };
    try {
      const full = await api.getRecord(rec.id);
      if (token !== costPreviewReq) return; // 期间又点了别的记录
      costPreviewCache[rec.id] = full;
      costPreview = { id: rec.id, loading: false, error: '', rec: full, anchor };
    } catch (e) {
      if (token !== costPreviewReq) return;
      costPreview = { id: rec.id, loading: false, error: e.message || '加载失败', rec: null, anchor };
    }
  }

  // 浮窗是 fixed 定位，任何滚动都会让锚点坐标失准；捕获阶段监听可一并
  // 捕获列表内部（.cost-rows）的滚动。浮窗自身滚动不应触发关闭。
  function onGlobalScroll(e) {
    if (!costPreview) return;
    const t = e.target;
    if (t && t !== document && typeof t.closest === 'function' && t.closest('.cost-preview')) return;
    costClosePreview();
  }

  function onGlobalKeydown(e) {
    if (e.key === 'Escape' && costPreview) costClosePreview();
  }

  // 点浮窗与触发按钮之外的任何位置都关闭。放在 document 冒泡阶段：触发按钮
  // 自己的 click（打开/切换）先执行，这里再按 closest 判断，不会误关。
  function onDocClick(e) {
    if (!costPreview) return;
    const t = e.target;
    if (t && typeof t.closest === 'function' && (t.closest('.cost-preview') || t.closest('.cost-row-name'))) return;
    costClosePreview();
  }

  async function loadCosts() {
    costLoading = true;
    costError = '';
    try {
      const res = await api.costPending();
      costRecords = res.records || [];
      costSummary = res.summary || null;
      // 丢掉已不在清单中的选择（例如刚被标记掉的记录）
      const alive = new Set(costRecords.map((r) => r.id));
      costSel = Object.fromEntries(
        COST_FIELDS.map((f) => [f.key, (costSel[f.key] || []).filter((id) => alive.has(id))])
      );
    } catch (e) {
      costError = '加载失败：' + e.message;
    } finally {
      costLoading = false;
    }
  }

  // 提交一次确认操作并刷新清单。所有入口都汇总到这里，保证「操作 → 重新拉取」
  // 的单一真相：清单永远来自服务端，前端不做乐观推断，避免本地状态与服务端
  // 的「已标记」判定漂移。
  async function costSubmit(payload, notice) {
    costBusy = true;
    costError = '';
    costNotice = '';
    try {
      await api.costReview(payload);
      costNotice = notice;
      costSel = { price: [], pay_price: [], other_cost: [] };
      await loadCosts();
    } catch (e) {
      costError = '操作失败：' + e.message;
    } finally {
      costBusy = false;
    }
  }

  // 分组批量：标记为 0 元（免费 / 无实付 / 无支出）
  function costMarkZero(field) {
    const ids = costTargets(field);
    if (ids.length === 0) return;
    const f = COST_FIELDS.find((x) => x.key === field);
    costSubmit({ ids, field, action: 'zero' }, `已将 ${ids.length} 条记录的${f.label}标记为${f.zeroLabel}`);
  }

  // 分组批量：跳过（暂不处理，列值不变，仅移出清单）
  function costMarkSkip(field) {
    const ids = costTargets(field);
    if (ids.length === 0) return;
    const f = COST_FIELDS.find((x) => x.key === field);
    costSubmit({ ids, field, action: 'skip' }, `已跳过 ${ids.length} 条${f.label}（列值保持不变）`);
  }

  // 分组批量：填入统一金额
  function costSetAmount(field) {
    const raw = String(costRowAmount[costRowKey(field, '__batch__')] ?? '').trim();
    const amount = Number(raw);
    if (raw === '' || !Number.isFinite(amount) || amount < 0) {
      costError = '请先填写一个不小于 0 的金额';
      return;
    }
    const ids = costTargets(field);
    if (ids.length === 0) return;
    const f = COST_FIELDS.find((x) => x.key === field);
    costSubmit({ ids, field, action: 'amount', amount }, `已为 ${ids.length} 条${f.label}填入 ${amount} 元`);
  }

  // 单行：填入该行输入的金额
  function costSetRowAmount(row) {
    const field = row.field;
    const k = costRowKey(field, row.rec.id);
    const raw = String(costRowAmount[k] ?? '').trim();
    const amount = Number(raw);
    if (raw === '' || !Number.isFinite(amount) || amount < 0) {
      costError = '请填写一个不小于 0 的金额';
      return;
    }
    costSubmit({ ids: [row.rec.id], field, action: 'amount', amount }, `已填入 ${amount} 元`);
  }

  // 单行：标记为 0 元
  function costRowZero(row) {
    const f = COST_FIELDS.find((x) => x.key === row.field);
    costSubmit({ ids: [row.rec.id], field: row.field, action: 'zero' },
      `已标记为${f.zeroLabel}`);
  }

  // 单行：跳过
  function costRowSkip(row) {
    costSubmit({ ids: [row.rec.id], field: row.field, action: 'skip' }, '已跳过该条（列值保持不变）');
  }

  // 撤销：单个字段全部标记
  async function costUndoField(field) {
    const f = COST_FIELDS.find((x) => x.key === field);
    costBusy = true;
    costError = '';
    costNotice = '';
    try {
      const res = await api.costUnreview({ all: true, field });
      costNotice = `已撤销 ${res.restored} 条${f.label}标记，它们重新回到待确认清单`;
      await loadCosts();
    } catch (e) {
      costError = '撤销失败：' + e.message;
    } finally {
      costBusy = false;
    }
  }

  // 撤销：全部字段的所有标记
  async function costUndoAll() {
    costBusy = true;
    costError = '';
    costNotice = '';
    try {
      const res = await api.costUnreview({ all: true });
      costNotice = `已撤销全部 ${res.restored} 条标记`;
      await loadCosts();
    } catch (e) {
      costError = '撤销失败：' + e.message;
    } finally {
      costBusy = false;
    }
  }

  onMount(() => {
    loadCosts();

    // 费用预览浮窗的全局退出方式：点空白处 / 滚动 / Esc / 改窗口尺寸。
    window.addEventListener('scroll', onGlobalScroll, { capture: true, passive: true });
    window.addEventListener('resize', costClosePreview);
    document.addEventListener('keydown', onGlobalKeydown);
    document.addEventListener('click', onDocClick);
    return () => {
      window.removeEventListener('scroll', onGlobalScroll, { capture: true });
      window.removeEventListener('resize', costClosePreview);
      document.removeEventListener('keydown', onGlobalKeydown);
      document.removeEventListener('click', onDocClick);
    };
  });
</script>

<section class="fade-up cost-section" aria-labelledby="cost-section-title">
  <div class="cost-section-head">
    <h2 id="cost-section-title">费用补全</h2>
    <p class="tiny muted">集中处理「未填写」与「0 元」的费用字段。</p>
  </div>

  <div class="card cost-wizard">
    <h3>待确认字段</h3>
    <p class="hint cost-intro">
      0 元可能是真实免费 / 无支出，也可能是历史默认写入的脏数据；操作仅作用于当前为 NULL 或 0 的字段，不会改写已有金额。确认后可撤销。
    </p>

    {#if costError}<div class="banner error">⚠ {costError}</div>{/if}
    {#if costNotice}<div class="banner success">✓ {costNotice}</div>{/if}

    <!-- 概览：服务端全量计数，不受本地筛选与列表截断影响 -->
    <div class="cost-overview">
      <span class="cost-stat">
        <span class="k">待确认记录</span>
        <span class="v">{costSummary?.pending_records ?? '—'}</span>
      </span>
      {#each COST_FIELDS as f (f.key)}
        <span class="cost-stat">
          <span class="k">{f.label}</span>
          <span class="v accent">{costPendingCounts[f.key] ?? 0}</span>
        </span>
      {/each}
      <span class="cost-stat">
        <span class="k">已标记</span>
        <span class="v">{costReviewedTotal}</span>
      </span>
      <button class="btn sm ghost cost-refresh" onclick={loadCosts} disabled={costLoading || costBusy}>
        {costLoading ? '加载中…' : '刷新'}
      </button>
    </div>

    <!-- 筛选：「未填写」(NULL) 与「值为 0」(疑似历史脏数据) 分开看，是查漏补缺的关键 -->
    <div class="cost-filters">
      <div class="cost-seg" role="group" aria-label="字段筛选">
        <button class="cost-seg-btn" class:on={costFieldFilter === 'all'} onclick={() => (costFieldFilter = 'all')}>全部字段</button>
        {#each COST_FIELDS as f (f.key)}
          <button class="cost-seg-btn" class:on={costFieldFilter === f.key} onclick={() => (costFieldFilter = f.key)}>{f.label}</button>
        {/each}
      </div>
      <div class="cost-seg" role="group" aria-label="取值筛选" title="区分「确实是 0 元」与「历史默认写成了 0」">
        <button class="cost-seg-btn" class:on={costValueFilter === 'pending'} onclick={() => (costValueFilter = 'pending')}>全部</button>
        <button class="cost-seg-btn" class:on={costValueFilter === 'empty'} onclick={() => (costValueFilter = 'empty')} title="列为 NULL，确定未填写">未填写</button>
        <button class="cost-seg-btn" class:on={costValueFilter === 'zero'} onclick={() => (costValueFilter = 'zero')} title="值为 0：可能是真实免费，也可能是历史遗留的脏数据">值为 0</button>
      </div>
      <select class="input cost-venue-select" bind:value={costVenue} disabled={costBusy} aria-label="场馆筛选">
        <option value="">全部场馆</option>
        {#each costVenueOptions as v (v.address)}
          <option value={v.address}>{v.address}（{v.count}）</option>
        {/each}
      </select>
      <input class="input cost-search" type="search" placeholder="搜索剧目 / 场馆 / 城市" bind:value={costQuery} disabled={costBusy} />
      <span class="tiny muted cost-visible-count">当前筛选 {costVisibleCount} 条</span>
    </div>

    {#if costLoading && !costSummary}
      <div class="banner info" style="margin-top: 12px;">加载中…</div>
    {:else if (costSummary?.pending_records ?? 0) === 0}
      <div class="banner success" style="margin-top: 12px;">✓ 所有费用字段均已补全</div>
    {/if}

    {#each costVisibleFields as f (f.key)}
      {@const rows = costRows[f.key] ?? []}
      {@const sel = (costSel[f.key] || []).filter((id) => rows.some((r) => r.rec.id === id))}
      {@const targets = sel.length > 0 ? sel : rows.map((r) => r.rec.id)}
      <div class="cost-group">
        <div class="cost-group-head">
          <button
            class="cost-group-toggle"
            onclick={() => (costExpanded = { ...costExpanded, [f.key]: !costExpanded[f.key] })}
            disabled={costBusy}
          >
            <span class="chevron" class:open={costExpanded[f.key]}>▶</span>
            <span class="cost-group-title">{f.label}</span>
            <span class="tiny muted">{f.desc}</span>
          </button>
          <span class="badge">{costPendingCounts[f.key] ?? 0} 待确认</span>
        </div>

        {#if costExpanded[f.key]}
          <div class="cost-group-body">
            <div class="cost-bulk">
              <label class="cost-selectall">
                <input
                  type="checkbox"
                  checked={rows.length > 0 && sel.length === rows.length}
                  onchange={() => costSelectAll(f.key)}
                  disabled={costBusy || rows.length === 0}
                />
                <span>{sel.length > 0 ? `已选 ${sel.length} 条` : `共 ${rows.length} 条`}</span>
              </label>
              <button class="btn sm primary" disabled={costBusy || targets.length === 0} onclick={() => costMarkZero(f.key)}>
                {f.zeroAction}（{targets.length}）
              </button>
              <button class="btn sm" disabled={costBusy || targets.length === 0} onclick={() => costMarkSkip(f.key)}>
                跳过（{targets.length}）
              </button>
              <span class="cost-amount-group">
                <input
                  class="input cost-mini-input"
                  type="number" min="0" step="0.01" placeholder="金额"
                  bind:value={costRowAmount[costRowKey(f.key, '__batch__')]}
                  disabled={costBusy}
                />
                <button class="btn sm" disabled={costBusy || targets.length === 0} onclick={() => costSetAmount(f.key)}>填入</button>
              </span>
              {#if (costReviewedCounts[f.key] ?? 0) > 0}
                <button class="btn sm ghost" disabled={costBusy} onclick={() => costUndoField(f.key)}>
                  撤销已标记 {costReviewedCounts[f.key]}
                </button>
              {/if}
            </div>

            {#if rows.length === 0}
              <div class="tiny muted" style="padding: 6px 0;">当前筛选条件下该字段无待确认条目</div>
            {:else}
              <div class="cost-rows">
                {#each rows as row (row.rec.id)}
                  <div class="cost-row">
                    <input
                      type="checkbox"
                      checked={sel.includes(row.rec.id)}
                      onchange={() => costToggleSel(f.key, row.rec.id)}
                      disabled={costBusy}
                    />
                    <span class="cost-row-date">{fmtCostDate(row.rec.date)}</span>
                    <button
                      type="button"
                      class="cost-row-name"
                      class:open={costPreview?.id === row.rec.id}
                      title={`${row.rec.name || '无名称'}（点击预览封面 / 演员 / 备注）`}
                      onclick={(e) => costTogglePreview(row.rec, e)}
                    >{row.rec.name || '（无名称）'}</button>
                    {#if row.rec.address}
                      <span class="cost-row-venue" title={row.rec.address}>{row.rec.address}</span>
                    {/if}
                    <span
                      class="cost-kind"
                      class:zero={row.kind === 'zero'}
                      title={row.kind === 'empty' ? '未填写（列为空）' : '值为 0：需确认是真实免费还是历史脏数据'}
                    >{row.kind === 'empty' ? '未填写' : '0 元'}</span>
                    <span class="cost-row-ops">
                      <input
                        class="input cost-mini-input"
                        type="number" min="0" step="0.01" placeholder="金额"
                        bind:value={costRowAmount[costRowKey(f.key, row.rec.id)]}
                        disabled={costBusy}
                      />
                      <button class="btn sm" disabled={costBusy} onclick={() => costSetRowAmount({ ...row, field: f.key })}>填入</button>
                      <button class="btn sm" disabled={costBusy} onclick={() => costRowZero({ ...row, field: f.key })}>{f.zeroLabel}</button>
                      <button class="btn sm ghost" disabled={costBusy} onclick={() => costRowSkip({ ...row, field: f.key })}>跳过</button>
                    </span>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>
</section>

<!-- 演出预览浮窗：fixed 定位到点击时的视口坐标，因此必须与 .fade-up 同级
     —— fadeUp 动画（fill-mode: both）会残留 transform: translateY(0)，
     使 .fade-up 成为 fixed 元素的包含块，导致定位基准不再是视口。 -->
{#if costPreview}
  <div
    class="cost-preview"
    style="left: {costPreviewPos.left}px; top: {costPreviewPos.top}px; max-height: {costPreviewPos.maxH}px;"
    role="dialog"
    aria-label="演出预览"
  >
    {#if costPreview.loading}
      <div class="cost-preview-state">加载中…</div>
    {:else if costPreview.error}
      <div class="cost-preview-state">⚠ 无法加载预览：{costPreview.error}</div>
    {:else if costPreview.rec}
      {@const rec = costPreview.rec}
      <!-- 库里存在极少数「空壳记录」（名称/日期/场馆/剧目/备注全空），
           浮窗据此给出空状态提示，而不是留一片空白让人困惑 -->
      {@const hasInfo = !!(rec.address || rec.city || rec.categoryNames?.length || rec.artist_names?.length || rec.company || rec.play?.length || rec.seat || rec.friends || rec.rating > 0)}
      <div class="cost-preview-main">
        <div class="cost-preview-cover">
          {#if rec.coverThumb || rec.coverFile}
            <!-- 加载失败（S3 异常 / 文件丢失）时移除 img，占位文案靠
                 img + span 的相邻选择器自动出现，避免露出破图与 alt 文本 -->
            <img
              src={coverUrl(rec.coverThumb || rec.coverFile)}
              alt=""
              loading="lazy"
              onerror={(e) => e.currentTarget.remove()}
            />
          {/if}
          <span class="cost-preview-noimg">无封面</span>
        </div>
        <div class="cost-preview-info">
          <div class="cost-preview-title">{rec.name || '（无名称）'}</div>
          <div class="cost-preview-when">{fmtPreviewTime(rec.date)}</div>
          {#if rec.address || rec.city}
            <div class="cost-preview-line"><span class="k">场馆</span><span class="v">{rec.address || rec.city}</span></div>
          {/if}
          {#if rec.categoryNames?.length}
            <div class="cost-preview-line"><span class="k">剧种</span><span class="v">{rec.categoryNames.join('、')}</span></div>
          {/if}
          {#if rec.artist_names?.length}
            <div class="cost-preview-line"><span class="k">演员</span><span class="v">{rec.artist_names.join('、')}</span></div>
          {/if}
          {#if rec.company}
            <div class="cost-preview-line"><span class="k">剧团</span><span class="v">{rec.company}</span></div>
          {/if}
          {#if rec.play?.length}
            <div class="cost-preview-line"><span class="k">剧目</span><span class="v">{rec.play.join('、')}</span></div>
          {/if}
          {#if rec.seat || rec.friends || rec.rating > 0}
            <div class="cost-preview-line">
              <span class="k">其他</span>
              <span class="v">{[rec.seat && `座位 ${rec.seat}`, rec.friends && `同行 ${rec.friends}`, rec.rating > 0 && `评分 ${rec.rating}`].filter(Boolean).join(' · ')}</span>
            </div>
          {/if}
          {#if !hasInfo && !rec.remark}
            <div class="cost-preview-empty">该记录暂无场馆、演员、剧目与备注信息，可到详情页补充。</div>
          {/if}
        </div>
      </div>
      <!-- 当前费用状态：补费用时最需要确认「这场到底有没有花钱」。
           已填金额是本场确有支出的直接反证，可避免把真实花费误标为免费。 -->
      <div class="cost-preview-costs">
        {#each [['票价', rec.price, rec.price_currency], ['实付', rec.pay_price, rec.pay_price_currency], ['其他', rec.other_cost, rec.other_cost_currency]] as it}
          <span class="cost-preview-cost">
            <span class="k">{it[0]}</span>
            {#if it[1] === null || it[1] === undefined}
              <span class="v none">未填写</span>
            {:else}
              <span class="v" class:zero={it[1] === 0}>{fmtMoney(it[1], it[2])}</span>
            {/if}
          </span>
        {/each}
      </div>
      {#if rec.remark}
        <div class="cost-preview-remark">
          <span class="k">备注</span>
          <p>{rec.remark}</p>
        </div>
      {/if}
      <div class="cost-preview-foot">
        <a class="cost-preview-link" href={`/records/${rec.id}`}>打开完整详情 ↗</a>
        <button type="button" class="btn sm ghost" onclick={costClosePreview}>关闭</button>
      </div>
    {/if}
  </div>
{/if}

<style>
  /* ---------- 费用补全 ----------
     本组件从设置页迁到「数据」页；.sec / .hint 是页面级样式（不跨组件生效），
     因此这里自带卡片内边距与 hint 定义。 */
  .cost-section {
    margin-top: 28px;
    padding-top: 18px;
    border-top: 1px solid var(--border);
  }
  .cost-section-head { margin-bottom: 14px; }
  .cost-section-head h2 {
    margin: 0 0 4px;
    font-size: 16px;
    font-weight: 600;
    color: var(--text);
  }
  .cost-section-head p { margin: 0; }

  .cost-wizard { padding: 18px 20px; }
  .cost-wizard h3 { margin-bottom: 6px; font-size: 14.5px; }
  .cost-intro { margin: 0 0 10px; }

  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; display: block; margin-top: 4px; }

  .badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--accent-softer);
    color: var(--accent);
    font-size: 12px;
    font-weight: 600;
    line-height: 1.4;
  }
  .chevron {
    display: inline-block;
    font-size: 10px;
    transition: transform var(--t-fast) var(--ease);
    color: var(--text-3);
  }
  .chevron.open { transform: rotate(90deg); }

  .cost-overview {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 16px;
    margin-bottom: 10px;
    font-size: 12.5px;
  }
  .cost-stat {
    display: inline-flex;
    align-items: baseline;
    gap: 4px;
    color: var(--text-3);
  }
  .cost-stat .k { white-space: nowrap; }
  .cost-stat .v {
    font-weight: 600;
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }
  .cost-stat .v.accent { color: var(--accent); }
  .cost-refresh { margin-left: auto; padding: 4px 10px; font-size: 12px; }

  .cost-filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    margin-bottom: 10px;
  }
  .cost-seg {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
  }
  .cost-seg-btn {
    padding: 4px 9px;
    font-size: 12px;
    background: var(--surface);
    color: var(--text-3);
    border: none;
    border-right: 1px solid var(--border);
    cursor: pointer;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .cost-seg-btn:last-child { border-right: none; }
  .cost-seg-btn:hover { color: var(--accent); }
  .cost-seg-btn.on { background: var(--accent-softer); color: var(--accent); font-weight: 600; }
  .cost-venue-select {
    flex: 0 1 200px;
    min-width: 140px;
    height: 28px;
    font-size: 12px;
    padding: 0 8px;
  }
  .cost-search {
    flex: 1 1 160px;
    min-width: 130px;
    height: 28px;
    font-size: 12px;
    padding: 0 10px;
  }
  .cost-visible-count { font-size: 12px; white-space: nowrap; color: var(--text-3); }

  .cost-group {
    margin-top: 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
  }
  .cost-group-head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 10px;
    background: none;
    border-bottom: 1px solid var(--border);
  }
  .cost-group-toggle {
    display: flex;
    align-items: center;
    gap: 7px;
    flex: 1;
    min-width: 0;
    padding: 0;
    background: none;
    border: none;
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    text-align: left;
  }
  .cost-group-toggle:disabled { cursor: default; opacity: 0.7; }
  .cost-group-toggle .chevron { color: var(--text-3); font-size: 9px; }
  .cost-group-title { flex: none; color: var(--text-2); }
  .cost-group-head .badge { padding: 1px 6px; font-size: 11px; }
  .cost-group-body { padding: 9px 10px 10px; }

  .cost-bulk {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
  }
  .cost-bulk .btn { padding: 4px 10px; font-size: 12px; }
  .cost-selectall {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 12px;
    color: var(--text-3);
    cursor: pointer;
    user-select: none;
  }
  .cost-selectall input { accent-color: var(--accent); cursor: pointer; }
  .cost-amount-group { display: inline-flex; align-items: center; gap: 5px; }

  .cost-rows {
    border: 1px solid var(--border);
    border-radius: 6px;
    max-height: 320px;
    overflow-y: auto;
  }
  .cost-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    padding: 5px 9px;
    font-size: 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .cost-row:last-child { border-bottom: none; }
  .cost-row input[type='checkbox'] { accent-color: var(--accent); cursor: pointer; flex: none; }
  .cost-row-date { color: var(--text-3); flex: none; font-variant-numeric: tabular-nums; }
  /* 名称是预览浮窗的触发器：reset 掉按钮默认外观，保留原来的省略号布局，
     用虚线下划线给出「可点击」的暗示。 */
  .cost-row-name {
    flex: 1 1 120px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: inherit;
    text-align: left;
    color: var(--text-2);
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    border-bottom: 1px dashed transparent;
    transition: color var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease);
  }
  .cost-row-name:hover { color: var(--accent); border-bottom-color: currentColor; }
  .cost-row-name.open { color: var(--accent); border-bottom-color: currentColor; font-weight: 600; }
  .cost-row-venue {
    color: var(--text-3);
    font-size: 11px;
    flex: 0 1 120px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* 区分两类待确认：未填写（NULL，确定缺失，用强调色）与值为 0（疑似历史
     脏数据，数量极大，用中性色以免满屏红点制造噪音）。 */
  .cost-kind {
    flex: none;
    padding: 1px 6px;
    border-radius: 999px;
    font-size: 10.5px;
    font-weight: 600;
    background: var(--accent-softer);
    color: var(--accent);
    white-space: nowrap;
  }
  .cost-kind.zero { background: var(--surface-2); color: var(--text-3); }
  /* 行内操作区可换行：窄屏（≤380px）时若固定不折行，会把整行顶出容器。 */
  .cost-row-ops {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    flex: 0 1 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
    margin-left: auto;
  }
  .cost-mini-input { width: 76px; height: 26px; font-size: 12px; padding: 0 7px; }
  @media (max-width: 420px) {
    /* 极窄屏下让操作区独占一行，避免与剧目/场馆挤在同一行。 */
    .cost-row-ops { flex-basis: 100%; margin-left: 0; justify-content: flex-start; }
  }

  /* ---------- 演出预览浮窗 ----------
     与 .fade-up 同级渲染（见模板注释），用 fixed 定位到点击时的视口坐标；
     z-index 高于导航（50），低于抽屉 / 灯箱。 */
  .cost-preview {
    position: fixed;
    z-index: 70;
    width: 340px;
    max-width: calc(100vw - 24px);
    overflow: auto;
    padding: 12px;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: 12px;
    box-shadow: var(--shadow-md);
    animation: costPop 0.14s var(--ease);
  }
  @keyframes costPop {
    from { opacity: 0; transform: translateY(-4px); }
    to { opacity: 1; transform: none; }
  }
  .cost-preview-state { font-size: 12.5px; color: var(--text-3); padding: 2px 0; }
  .cost-preview-main { display: flex; gap: 12px; }
  .cost-preview-cover {
    flex: none;
    width: 88px;
    height: 118px;
    border-radius: 8px;
    overflow: hidden;
    background: var(--surface-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .cost-preview-cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
  /* img 仍在（加载成功）时隐藏占位；img 被 onerror 移除后选择器失配，
     占位自动显示。无封面记录（从未渲染 img）同理显示占位。 */
  .cost-preview-cover img + .cost-preview-noimg { display: none; }
  .cost-preview-noimg { font-size: 11px; color: var(--text-3); }
  .cost-preview-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .cost-preview-title { font-size: 13.5px; font-weight: 600; color: var(--text); line-height: 1.4; }
  .cost-preview-when { font-size: 12px; color: var(--text-3); font-variant-numeric: tabular-nums; }
  .cost-preview-line { display: flex; gap: 6px; font-size: 12px; line-height: 1.5; color: var(--text-2); }
  .cost-preview-line .k { flex: none; width: 28px; color: var(--text-3); }
  .cost-preview-line .v { min-width: 0; word-break: break-word; }
  .cost-preview-empty { font-size: 11.5px; line-height: 1.55; color: var(--text-3); }
  /* 费用现状：三项内联，未填写 / 0 元弱化，避免与「已填金额」混淆 */
  .cost-preview-costs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    margin-top: 10px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
    font-size: 12px;
  }
  .cost-preview-cost { display: inline-flex; align-items: baseline; gap: 4px; }
  .cost-preview-cost .k { color: var(--text-3); }
  .cost-preview-cost .v { color: var(--text-2); font-weight: 600; font-variant-numeric: tabular-nums; }
  .cost-preview-cost .v.none,
  .cost-preview-cost .v.zero { color: var(--text-3); font-weight: 400; }
  /* 费用行已在上方给出分隔线，备注紧贴其后即可，不再重复画线 */
  .cost-preview-remark { margin-top: 8px; }
  .cost-preview-remark .k { font-size: 11.5px; color: var(--text-3); }
  /* 备注限 4 行并加省略号：固定 max-height 会在行高非整数倍处切出
     「半截字」，line-clamp 则是整行截断，浮窗高度也更稳定。
     全文看「打开完整详情」。 */
  .cost-preview-remark p {
    margin: 3px 0 0;
    font-size: 12px;
    line-height: 1.6;
    color: var(--text-2);
    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .cost-preview-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: 10px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
  }
  .cost-preview-link { font-size: 12px; color: var(--accent); text-decoration: none; }
  .cost-preview-link:hover { text-decoration: underline; }
</style>
