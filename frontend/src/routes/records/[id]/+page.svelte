<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { fade, scale } from 'svelte/transition';
  import { api, coverUrl, formatCurrency, formatDate } from '$lib/api.js';
  import { STATUS_LABELS } from '$lib/statusPrefs.js';
  import BackLink from '$lib/components/BackLink.svelte';

  const id = $page.params.id;
  let rec = $state(null);
  let loading = $state(true);
  let error = $state('');
  let deleting = $state(false);
  let dramaMap = $state(new Map());
  let zheziMap = $state(new Map());

  // 封面灯箱：点击放大查看
  let lightbox = $state(false);
  // 列表/详情显示缩略图优先，灯箱使用原图
  const thumbSrc = $derived(rec?.coverThumb || rec?.coverFile || '');
  const fullSrc = $derived(rec?.coverFile || rec?.coverThumb || '');

  // 灯箱打开时锁定背景滚动，Esc 关闭
  $effect(() => {
    if (!lightbox) return;
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; };
  });

  function onWindowKeydown(e) {
    if (e.key === 'Escape') lightbox = false;
  }

  function openLightbox() {
    if (fullSrc) lightbox = true;
  }

  function dramaName(id) {
    return dramaMap.get(id)?.name || '';
  }

  // 按剧名分组折子：[{ dramaName, dramaId, zhezis: [{id, name}] }]
  // 同时包含有折子和无折子的剧目（从 drama_ids 补齐）
  const groupedZhezis = $derived.by(() => {
    const groups = [];
    const groupMap = new Map();

    // 先从 drama_ids 建立所有剧目的空组
    for (const did of rec?.drama_ids || []) {
      const d = dramaMap.get(did);
      if (!d) continue;
      const g = { dramaName: d.name, dramaId: d.id, zhezis: [] };
      groups.push(g);
      groupMap.set(d.id, g);
    }

    // 将折子归入对应剧目组
    for (const zid of rec?.zhezi_ids || []) {
      const z = zheziMap.get(zid);
      if (!z) continue;
      let g = groupMap.get(z.dramaId);
      if (!g) {
        // 折子所属剧目不在 drama_ids 中（数据不一致），兜底新建组
        g = { dramaName: z.dramaName, dramaId: z.dramaId, zhezis: [] };
        groups.push(g);
        groupMap.set(z.dramaId, g);
      }
      g.zhezis.push({ id: z.id, name: z.name });
    }

    return groups;
  });

  const statusLabel = STATUS_LABELS;

  async function load() {
    loading = true;
    error = '';
    try {
      const [r, tree] = await Promise.all([api.getRecord(id), api.getDramaTree().catch(() => [])]);
      rec = r;
      dramaMap = new Map(tree.map((d) => [d.id, d]));
      zheziMap = new Map();
      for (const d of tree) {
        for (const z of d.zhezis || []) zheziMap.set(z.id, { ...z, dramaId: d.id, dramaName: d.name });
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function remove() {
    if (!confirm('确定删除这条记录？此操作不可恢复。')) return;
    deleting = true;
    try {
      await api.deleteRecord(id);
      location.href = '/';
    } catch (e) {
      error = e.message;
      deleting = false;
    }
  }

  onMount(load);
</script>
<svelte:head><title>{rec ? `${rec.name} - 幕间` : "演出 - 幕间"}</title></svelte:head>
<svelte:window onkeydown={onWindowKeydown} />


{#if loading}
  <div class="detail-loading">
    <div class="skeleton skel-cover"></div>
    <div class="skel-col">
      <div class="skeleton skel-title"></div>
      <div class="skeleton skel-line"></div>
      <div class="skeleton skel-line short"></div>
    </div>
  </div>
{:else if error}
  <div class="banner error">⚠ {error}</div>
  <a class="btn ghost" href="/">← 返回列表</a>
{:else if rec}
  <div class="detail fade-up">
    <BackLink />

    <div class="hero card">
      <div
        class="cover"
        class:zoomable={!!fullSrc}
        role={fullSrc ? 'button' : undefined}
        tabindex={fullSrc ? 0 : undefined}
        aria-label={fullSrc ? '放大查看封面' : undefined}
        onclick={openLightbox}
        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openLightbox(); } }}
      >
        {#if rec.coverThumb}
          <img src={coverUrl(rec.coverThumb)} alt={rec.name} />
        {:else if rec.coverFile}
          <img src={coverUrl(rec.coverFile)} alt={rec.name} />
        {:else}
          <div class="no-cover"><span>{(rec.name || '?').slice(0, 1)}</span></div>
        {/if}
      </div>
      <div class="head">
        <div class="badges">
          {#each rec.categoryNames && rec.categoryNames.length ? rec.categoryNames : (rec.categoryName ? [rec.categoryName] : []) as catName (catName)}
            <a class="pill" href={`/?category=${encodeURIComponent(catName)}`}>{catName}</a>
          {/each}
          {#if rec.channel}
            <a class="pill" href={`/?q=${encodeURIComponent(rec.channel)}`}>{rec.channel}</a>
          {/if}
          {#if rec.rating}<span class="pill gold">★ {rec.rating}</span>{/if}
          <span class="pill status">{statusLabel[rec.active_status] ?? rec.active_status}</span>
        </div>
        <h1>{rec.name}</h1>
        <div class="sub">
          <span class="date">{rec.dateText || formatDate(rec.date)}</span>
        </div>
        <div class="sub">
          {#if rec.city}
            <a class="loc" href={`/?city=${encodeURIComponent(rec.city)}`}>{rec.city}</a>
          {/if}
          {#if rec.city && rec.address}<span class="dot">·</span>{/if}
          {#if rec.address}<span class="addr">{rec.address}</span>{/if}
        </div>
        {#if rec.coordinate}
          <div class="sub tiny">📍 {rec.coordinate.latitude}, {rec.coordinate.longitude}</div>
        {/if}
        <div class="actions">
          <a class="btn primary sm" href={`/records/${rec.id}/edit`}>编辑</a>
          <button class="btn danger sm" onclick={remove} disabled={deleting}>{deleting ? '删除中…' : '删除'}</button>
        </div>
      </div>
    </div>

    {#if rec.artist_names?.length || rec.drama_ids?.length || rec.play?.length || rec.zhezi_ids?.length}
      <div class="card section">
        {#if rec.artist_names?.length}
          <h3>演员阵容</h3>
          <div class="tags">
            {#each rec.artist_names as a, i}
              {@const aid = rec.artist_ids?.[i]}
              {#if aid}
                <a class="tag" href={`/artists/${aid}`}>{a}</a>
              {:else}
                <span class="tag">{a}</span>
              {/if}
            {/each}
          </div>
        {/if}

        {#if groupedZhezis.length}
          <h3>剧目与折子</h3>
          <div class="zhezi-clusters">
            {#each groupedZhezis as g (g.dramaId || g.dramaName)}
              <div class="zhezi-cluster">
                {#if g.dramaId}
                  <a class="cluster-drama" href={`/dramas/${g.dramaId}`} title={`查看《${g.dramaName}》详情`}>《{g.dramaName}》</a>
                {:else}
                  <span class="cluster-drama">《{g.dramaName}》</span>
                {/if}
                {#if g.zhezis.length}
                  <div class="cluster-zhezis">
                    {#each g.zhezis as z (z.id)}
                      <a class="ztag" href={`/?zhezi=${encodeURIComponent(z.id)}`} title={`「${z.name}」演出列表`}>{z.name}</a>
                    {/each}
                  </div>
                {:else}
                  <span class="cluster-full">整本</span>
                {/if}
              </div>
            {/each}
          </div>
        {:else if rec.play?.length}
          <h3>剧目</h3>
          <div class="tags">
            {#each rec.play as p}
              <a class="tag" href={`/?q=${encodeURIComponent(p)}`}>{p}</a>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    <div class="cards-row">
      <div class="card section">
        <h3>演出信息</h3>
        <dl class="kv">
          {#if rec.categoryNames && rec.categoryNames.length}
            <div class="kv-row"><dt>分类</dt><dd>{#each rec.categoryNames as cn, i}{i > 0 ? '、' : ''}<a class="flink" href={`/?category=${encodeURIComponent(cn)}`}>{cn}</a>{/each}</dd></div>
          {:else if rec.categoryName}
            <div class="kv-row"><dt>分类</dt><dd><a class="flink" href={`/?category=${encodeURIComponent(rec.categoryName)}`}>{rec.categoryName}</a></dd></div>
          {/if}
          {#if rec.company}
            <div class="kv-row"><dt>剧团</dt><dd>{#each rec.company.split(/[,，]/).map((s) => s.trim()).filter(Boolean) as t, i}{i > 0 ? '、' : ''}<a class="flink" href={`/?q=${encodeURIComponent(t)}`}>{t}</a>{/each}</dd></div>
          {/if}
          {#if rec.city}
            <div class="kv-row"><dt>城市</dt><dd><a class="flink" href={`/?city=${encodeURIComponent(rec.city)}`}>{rec.city}</a></dd></div>
          {/if}
          {#if rec.address}
            <div class="kv-row"><dt>场馆</dt><dd><a class="flink" href={`/?q=${encodeURIComponent(rec.address)}`}>{rec.address}</a></dd></div>
          {/if}
          {#if rec.friends}
            <div class="kv-row"><dt>同行</dt><dd>{#each rec.friends.split(/[,，]/).map((s) => s.trim()).filter(Boolean) as t, i}{i > 0 ? '、' : ''}<a class="flink" href={`/?q=${encodeURIComponent(t)}`}>{t}</a>{/each}</dd></div>
          {/if}
          {#if rec.seat}
            <div class="kv-row"><dt>座位</dt><dd>{rec.seat}</dd></div>
          {/if}
        </dl>
      </div>

      <div class="card section">
        <h3>费用</h3>
        <div class="fee-cards">
          <div class="fee-card">
            <span class="fee-label">票价</span>
            <span class="fee-amount" class:is-empty={!rec.price}>{rec.price ? formatCurrency(rec.price, rec.price_currency) : '—'}</span>
          </div>
          <div class="fee-card">
            <span class="fee-label">实付</span>
            <span class="fee-amount" class:is-empty={!rec.pay_price}>{rec.pay_price ? formatCurrency(rec.pay_price, rec.pay_price_currency) : '—'}</span>
          </div>
          <div class="fee-card">
            <span class="fee-label">其他花费</span>
            <span class="fee-amount" class:is-empty={!rec.other_cost}>{rec.other_cost ? formatCurrency(rec.other_cost, rec.other_cost_currency) : '—'}</span>
          </div>
        </div>
        {#if rec.pay_price || rec.other_cost || rec.price}
          <div class="fee-total">
            <span class="fee-total-label">合计</span>
            <span class="fee-total-amount">{formatCurrency((rec.pay_price > 0 ? rec.pay_price : (rec.price || 0)) + (rec.other_cost || 0), rec.pay_price_currency || 'CNY')}</span>
          </div>
        {/if}
        {#if rec.channel}
          <div class="fee-channel"><span class="fee-channel-key">渠道</span><a class="flink" href={`/?q=${encodeURIComponent(rec.channel)}`}>{rec.channel}</a></div>
        {/if}
      </div>
    </div>

    {#if rec.remark}
      <div class="card section">
        <h3>备注</h3>
        <p class="remark">{rec.remark}</p>
      </div>
    {/if}
  </div>
{/if}

{#if lightbox && fullSrc}
  <button type="button" class="lightbox" onclick={() => (lightbox = false)} aria-label="关闭大图" transition:fade={{ duration: 150 }}>
    <img src={coverUrl(fullSrc)} alt={rec.name} transition:scale={{ start: 0.96, duration: 180 }} />
  </button>
{/if}

<style>
  .back {
    display: inline-flex;
    align-items: center;
    color: var(--text-muted);
    font-size: 13.5px;
    margin-bottom: 12px;
    transition: color var(--t-fast) var(--ease);
  }
  .back:hover { color: var(--accent); }

  .hero { display: flex; gap: 24px; padding: 20px; }
  .cover {
    width: 168px;
    aspect-ratio: 3 / 4;
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--surface-3);
    flex: 0 0 auto;
    box-shadow: var(--shadow-md);
  }
  .cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .cover.zoomable { cursor: zoom-in; }
  .cover.zoomable:active { transform: scale(0.99); }

  /* 封面灯箱 */
  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 100;
    border: none;
    padding: 24px;
    margin: 0;
    background: rgba(0, 0, 0, 0.86);
    cursor: zoom-out;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .lightbox img {
    max-width: min(92vw, 720px);
    max-height: 88vh;
    width: auto;
    height: auto;
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
  }
  .no-cover {
    width: 100%; height: 100%;
    display: flex; align-items: center; justify-content: center;
    background: linear-gradient(160deg, var(--surface-3), var(--surface-2));
  }
  .no-cover span { font-family: var(--font-serif); font-size: 64px; color: var(--text-3); opacity: 0.6; }

  .head { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .badges { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 10px; }
  .pill.gold { background: var(--gold-soft); color: var(--gold); }
  .pill.status { background: var(--surface-3); color: var(--text-2); }
  h1 { font-size: 26px; margin: 0 0 8px; line-height: 1.25; }
  .sub { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 14px; color: var(--text-2); margin-bottom: 4px; }
  .sub .date { color: var(--text-muted); }
  .dot { opacity: 0.4; }
  .loc { color: var(--text-2); border-bottom: 1px dashed var(--border-strong); transition: all var(--t-fast) var(--ease); }
  .loc:hover { color: var(--accent); border-color: var(--accent); }
  .actions { display: flex; gap: 8px; margin-top: auto; padding-top: 16px; }

  .section { padding: 18px 20px; margin-top: 14px; }
  .section h3 { margin: 0 0 12px; font-size: 16px; }
  .section h3:not(:first-child) { margin-top: 18px; }

  .tags { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
  .section .tiny { margin: 14px 0 0; }

  /* 折子按剧目聚合成簇：剧名（书名号）为簇头，折子为独立标签 */
  .zhezi-clusters { display: flex; flex-direction: column; gap: 16px; }
  .zhezi-cluster { display: flex; flex-direction: column; gap: 8px; }
  .cluster-drama {
    font-family: var(--font-serif);
    font-size: 15.5px;
    font-weight: 600;
    color: var(--accent);
    text-decoration: none;
    width: fit-content;
    transition: color var(--t-fast) var(--ease);
  }
  a.cluster-drama:hover { color: var(--accent-strong); text-decoration: underline; text-underline-offset: 3px; }
  .cluster-zhezis { display: flex; gap: 6px; flex-wrap: wrap; }
  .ztag {
    display: inline-flex;
    align-items: center;
    background: var(--surface-2);
    border: 1px solid var(--border);
    color: var(--text-2);
    border-radius: var(--radius-sm);
    padding: 4px 11px;
    font-size: 13px;
    text-decoration: none;
    transition: background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .ztag:hover {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--accent);
  }
  .cluster-full {
    font-size: 12px;
    color: var(--text-3);
    background: var(--surface-2);
    border: 1px dashed var(--border);
    border-radius: var(--radius-sm);
    padding: 2px 9px;
    width: fit-content;
  }

  .cards-row { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .cards-row .section { margin-top: 14px; }

  .kv { margin: 0; display: flex; flex-direction: column; }
  .kv-row {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    padding: 8px 0;
    border-bottom: 1px solid var(--border);
    font-size: 14px;
  }
  .kv-row:last-child { border-bottom: none; }
  dt { color: var(--text-muted); flex: 0 0 auto; }
  dd { margin: 0; text-align: right; }
  .flink { color: var(--text); border-bottom: 1px dashed var(--border-strong); transition: all var(--t-fast) var(--ease); }
  .flink:hover { color: var(--accent); border-color: var(--accent); }
  .money { font-variant-numeric: tabular-nums; }

  /* 费用分卡：票价/实付/其他花费各自成卡，合计加粗强调 */
  .fee-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
  .fee-card {
    display: flex;
    flex-direction: column;
    gap: 6px;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px 13px;
  }
  .fee-label { font-size: 12px; color: var(--text-muted); }
  .fee-amount {
    font-size: 18px;
    font-weight: 600;
    color: var(--text);
    font-variant-numeric: tabular-nums;
    line-height: 1.1;
  }
  .fee-amount.is-empty { color: var(--text-3); font-weight: 500; }
  .fee-total {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    margin-top: 12px;
    padding: 12px 14px;
    background: var(--accent-soft);
    border: 1px solid var(--accent);
    border-radius: var(--radius);
  }
  .fee-total-label { font-size: 14px; font-weight: 700; color: var(--accent); }
  .fee-total-amount {
    font-size: 20px;
    font-weight: 700;
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .fee-channel {
    display: flex;
    gap: 8px;
    align-items: baseline;
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
    font-size: 13px;
    color: var(--text-muted);
  }
  .fee-channel-key { flex: 0 0 auto; }

  .remark { margin: 0; white-space: pre-wrap; line-height: 1.75; color: var(--text-2); }

  /* loading skeleton */
  .detail-loading { display: flex; gap: 24px; }
  .skel-cover { width: 168px; aspect-ratio: 3/4; }
  .skel-col { flex: 1; display: flex; flex-direction: column; gap: 12px; }
  .skel-title { height: 30px; width: 50%; }
  .skel-line { height: 14px; width: 80%; }
  .skel-line.short { width: 40%; }

  @media (max-width: 640px) {
    .hero { flex-direction: column; gap: 16px; }
    .cover { width: 100%; max-width: 220px; margin: 0 auto; }
    .cards-row { grid-template-columns: 1fr; }
    h1 { font-size: 22px; }
  }
</style>
