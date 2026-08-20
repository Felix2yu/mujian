<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api, coverUrl, formatCurrency, formatDate } from '$lib/api.js';

  const id = $page.params.id;
  let rec = $state(null);
  let loading = $state(true);
  let error = $state('');
  let deleting = $state(false);

  const statusLabel = { 0: '正常', 1: '想看', 2: '已取消', 3: '其他' };

  async function load() {
    loading = true;
    error = '';
    try {
      rec = await api.getRecord(id);
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
    <a class="back" href="/">← 返回</a>

    <div class="hero card">
      <div class="cover">
        {#if rec.coverFile}
          <img src={coverUrl(rec.coverFile)} alt={rec.name} />
        {:else if rec.coverThumb}
          <img src={rec.coverThumb} alt={rec.name} />
        {:else}
          <div class="no-cover"><span>{(rec.name || '?').slice(0, 1)}</span></div>
        {/if}
      </div>
      <div class="head">
        <div class="badges">
          {#if rec.categoryName}
            <a class="pill" href={`/?category=${encodeURIComponent(rec.categoryName)}`}>{rec.categoryName}</a>
          {/if}
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
          <button class="btn danger sm" on:click={remove} disabled={deleting}>{deleting ? '删除中…' : '删除'}</button>
        </div>
      </div>
    </div>

    {#if rec.artist_names?.length || rec.play?.length}
      <div class="card section">
        {#if rec.artist_names?.length}
          <h3>演员阵容</h3>
          <div class="tags">
            {#each rec.artist_names as a}
              <a class="tag" href={`/?q=${encodeURIComponent(a)}`}>{a}</a>
            {/each}
          </div>
        {/if}
        {#if rec.play?.length}
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
          {#if rec.categoryName}
            <div class="kv-row"><dt>分类</dt><dd><a class="flink" href={`/?category=${encodeURIComponent(rec.categoryName)}`}>{rec.categoryName}</a></dd></div>
          {/if}
          {#if rec.channel}
            <div class="kv-row"><dt>渠道</dt><dd><a class="flink" href={`/?q=${encodeURIComponent(rec.channel)}`}>{rec.channel}</a></dd></div>
          {/if}
          {#if rec.company}
            <div class="kv-row"><dt>剧团</dt><dd><a class="flink" href={`/?q=${encodeURIComponent(rec.company)}`}>{rec.company}</a></dd></div>
          {/if}
          {#if rec.city}
            <div class="kv-row"><dt>城市</dt><dd><a class="flink" href={`/?city=${encodeURIComponent(rec.city)}`}>{rec.city}</a></dd></div>
          {/if}
          {#if rec.address}
            <div class="kv-row"><dt>场馆</dt><dd><a class="flink" href={`/?q=${encodeURIComponent(rec.address)}`}>{rec.address}</a></dd></div>
          {/if}
          {#if rec.friends}
            <div class="kv-row"><dt>同行</dt><dd><a class="flink" href={`/?q=${encodeURIComponent(rec.friends)}`}>{rec.friends}</a></dd></div>
          {/if}
          {#if rec.seat}
            <div class="kv-row"><dt>座位</dt><dd>{rec.seat}</dd></div>
          {/if}
        </dl>
      </div>

      <div class="card section">
        <h3>费用</h3>
        <dl class="kv">
          <div class="kv-row"><dt>票价</dt><dd class="money">{rec.price ? formatCurrency(rec.price, rec.price_currency) : '—'}</dd></div>
          <div class="kv-row"><dt>实付</dt><dd class="money">{rec.pay_price ? formatCurrency(rec.pay_price, rec.pay_price_currency) : '—'}</dd></div>
          <div class="kv-row"><dt>其他花费</dt><dd class="money">{rec.other_cost ? formatCurrency(rec.other_cost, rec.other_cost_currency) : '—'}</dd></div>
          {#if rec.pay_price || rec.other_cost}
            <div class="kv-row total"><dt>合计</dt><dd class="money">{formatCurrency((rec.pay_price || 0) + (rec.other_cost || 0), rec.pay_price_currency || 'CNY')}</dd></div>
          {/if}
        </dl>
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

  .tags { display: flex; gap: 6px; flex-wrap: wrap; }
  .section .tiny { margin: 14px 0 0; }

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
  .kv-row.total { border-top: 2px solid var(--border); font-weight: 600; margin-top: 4px; }
  dt { color: var(--text-muted); flex: 0 0 auto; }
  dd { margin: 0; text-align: right; }
  .flink { color: var(--text); border-bottom: 1px dashed var(--border-strong); transition: all var(--t-fast) var(--ease); }
  .flink:hover { color: var(--accent); border-color: var(--accent); }
  .money { font-variant-numeric: tabular-nums; }

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
