<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { fade, scale } from 'svelte/transition';
  import { api, coverUrl, formatCurrency, formatDate } from '$lib/api.js';
  import { STATUS_LABELS, ALL_STATUSES } from '$lib/statusPrefs.js';
  import BackLink from '$lib/components/BackLink.svelte';

  const id = $page.params.id;
  const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
  function weekday(ts) {
    if (!ts) return '';
    return WEEKDAYS[new Date(ts * 1000).getDay()];
  }
  // 演出时长（分钟）→ 友好文本；0 视为未知
  function durationText(mins) {
    if (!mins || mins <= 0) return '';
    if (mins < 60) return `${mins} 分钟`;
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    return m ? `${h} 小时 ${m} 分钟` : `${h} 小时`;
  }
  let rec = $state(null);
  let loading = $state(true);
  let error = $state('');
  let deleting = $state(false);
  let dramaMap = $state(new Map());
  let zheziMap = $state(new Map());

  // 详情页快速打分：点击星标即时保存到后端，无需进入编辑页
  let hoverRating = $state(0);
  let ratingBusy = $state(false);
  let ratingSaved = $state(false);
  let ratingError = $state('');

  // 详情页快捷切换演出状态：点击状态即即时保存到后端，无需进入编辑页
  let statusBusy = $state(false);
  let statusSaved = $state(false);
  let statusError = $state('');

  // 封面灯箱：点击放大查看
  // 列表/详情显示缩略图优先，灯箱使用原图
  const thumbSrc = $derived(rec?.coverThumb || rec?.coverFile || '');
  const fullSrc = $derived(rec?.coverFile || rec?.coverThumb || '');

  // 灯箱打开时锁定背景滚动，Esc 关闭
  $effect(() => {
    if (!lightbox && !lightboxSrc) return;
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; };
  });

  function onWindowKeydown(e) {
    if (e.key === 'Escape') { lightbox = false; lightboxSrc = ''; }
  }

  function openLightbox() {
    if (fullSrc) { lightbox = true; lightboxSrc = fullSrc; }
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

  // 票根/现场照
  let photos = $state([]);
  let photoBusy = $state('');
  let photoFile = $state(null);
  let photoFileList = $state(null);
  let lightboxSrc = $state(''); // 空字符串 = 关闭；支持封面与多张照片
  let lightbox = $state(false);

  async function loadPhotos() {
    try {
      const res = await api.listRecordPhotos(id);
      photos = res.photos || [];
    } catch (e) { photos = []; }
  }
  async function addPhoto() {
    if (!photoFile) return;
    photoBusy = 'add';
    try {
      const up = await api.uploadFile(photoFile);
      await api.addRecordPhoto(id, up.key);
      photoFile = null;
      await loadPhotos();
    } catch (e) {
      error = e.message;
    } finally {
      photoBusy = '';
    }
  }
  async function removePhoto(p) {
    if (!confirm('移除这张照片？图片文件保留在服务器，可由封面清理统一回收。')) return;
    photoBusy = p.id;
    try {
      await api.deleteRecordPhoto(id, p.id);
      if (lightboxSrc === coverUrl(p.file_name)) { lightbox = false; lightboxSrc = ''; }
      await loadPhotos();
    } finally {
      photoBusy = '';
    }
  }
  async function movePhoto(p, dir) {
    const ids = photos.map((x) => x.id);
    const i = ids.indexOf(p.id);
    const j = i + dir;
    if (j < 0 || j >= ids.length) return;
    [ids[i], ids[j]] = [ids[j], ids[i]];
    photos = ids.map((pid) => photos.find((x) => x.id === pid)); // 乐观更新
    await api.reorderRecordPhotos(id, ids).catch(loadPhotos);
  }

  // 相关演出：按关联程度统一打分排序（同剧目 > 同演员 > 同场馆），取最相关若干，带封面缩略图
  let related = $state([]);

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
      loadRelated(r);
      loadPhotos();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  // 相关演出：跨「同剧目 / 同演员 / 同场馆」三个维度累计关联度，合并去重后按
  // 总分降序、日期降序取最相关的若干条。一条记录可同时命中多个维度（分数累加）。
  const REL_WEIGHTS = { drama: 3, artist: 2, venue: 1.5 };
  async function loadRelated(r) {
    related = [];
    const byId = new Map();
    const rollup = (list, reason, weight) => {
      for (const x of list || []) {
        if (!x?.id || x.id === r.id) continue;
        const cur = byId.get(x.id);
        if (cur) {
          cur.score += weight;
          if (!cur.reasons.includes(reason)) cur.reasons.push(reason);
        } else {
          byId.set(x.id, { ...x, score: weight, reasons: [reason] });
        }
      }
    };
    const tasks = [];
    // 同剧目：该演出关联的全部剧目（权重最高）
    for (const did of r.drama_ids || []) {
      if (did) tasks.push(api.listRecords({ drama: did, limit: 50 }).then((res) => rollup(res?.records, '同剧目', REL_WEIGHTS.drama)).catch(() => {}));
    }
    // 同演员：该演出关联的前若干位演员（权重次之；封顶避免一条记录演员过多时请求爆炸）
    for (const aid of (r.artist_ids || []).slice(0, 6)) {
      if (aid) tasks.push(api.listRecords({ artist: aid, limit: 50 }).then((res) => rollup(res?.records, '同演员', REL_WEIGHTS.artist)).catch(() => {}));
    }
    // 同场馆：地址模糊匹配（权重最低）
    if (r.address) {
      tasks.push(api.getByField('address', r.address).then((res) => rollup(res, '同场馆', REL_WEIGHTS.venue)).catch(() => {}));
    }
    await Promise.all(tasks);
    related = [...byId.values()]
      .sort((a, b) => b.score - a.score || (b.date || 0) - (a.date || 0))
      .slice(0, 8);
  }

  async function remove() {
    if (!confirm('确定删除这条演出？删除后进入回收站，30 天内可在「数据」页恢复。')) return;
    deleting = true;
    try {
      await api.deleteRecord(id);
      location.href = '/';
    } catch (e) {
      error = e.message;
      deleting = false;
    }
  }

  // 快速打分：乐观更新 + 失败回滚。直接把当前完整记录回写（Record 响应字段与
  // RecordRequest 同构），仅改 rating，避免部分字段更新把其他列清零。
  async function setRating(n) {
    if (!rec) return;
    const prev = rec.rating;
    const next = prev === n ? 0 : n; // 再次点击同一星 → 清除评分
    rec.rating = next;               // 乐观更新：星标与文字即时反映
    hoverRating = 0;
    ratingBusy = true;
    ratingSaved = false;
    ratingError = '';
    try {
      const updated = await api.updateRecord(id, { ...rec, rating: next });
      rec = updated;                 // 以服务端返回为准
      if (next > 0) {
        ratingSaved = true;
        setTimeout(() => { ratingSaved = false; }, 1600);
      }
    } catch (e) {
      rec.rating = prev;             // 失败回滚
      ratingError = e.message || '评分保存失败';
    } finally {
      ratingBusy = false;
    }
  }

  // 快捷切换演出状态：与快速打分同理，乐观更新 + 失败回滚。回写完整记录
  // （Record 响应字段与 RecordRequest 同构），仅改 active_status，避免把其他列清零。
  async function setStatus(s) {
    if (!rec || rec.active_status === s) return; // 已是该状态则跳过
    const prev = rec.active_status;
    rec.active_status = s;          // 乐观更新：状态药丸即时切换
    statusBusy = true;
    statusSaved = false;
    statusError = '';
    try {
      const updated = await api.updateRecord(id, { ...rec, active_status: s });
      rec = updated;                // 以服务端返回为准
      statusSaved = true;
      setTimeout(() => { statusSaved = false; }, 1600);
    } catch (e) {
      rec.active_status = prev;     // 失败回滚
      statusError = e.message || '状态保存失败';
    } finally {
      statusBusy = false;
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
  <BackLink fallback="/" label="← 返回列表" />
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
        </div>
        <h1>{rec.name}</h1>
        <div class="sub">
          <span class="date">{rec.dateText || formatDate(rec.date)} {weekday(rec.date)}</span>
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
        <div class="rate-inline" aria-busy={ratingBusy}>
          <span class="rate-label">我的评分</span>
          <div class="star-row" role="radiogroup" aria-label="为这场演出评分">
            {#each [1, 2, 3, 4, 5] as n}
              <button
                type="button"
                class="star"
                class:on={Math.max(rec.rating, hoverRating) >= n}
                onclick={() => setRating(n)}
                onmouseenter={() => (hoverRating = n)}
                onmouseleave={() => (hoverRating = 0)}
                onfocus={() => (hoverRating = n)}
                onblur={() => (hoverRating = 0)}
                aria-label={`评 ${n} 星`}
                aria-pressed={rec.rating >= n}
                disabled={ratingBusy}
              >★</button>
            {/each}
            <span class="rate-text tiny">{rec.rating ? `${rec.rating} 分` : '未评分'}</span>
            {#if ratingSaved}<span class="rate-saved tiny">✓ 已保存</span>{/if}
            {#if ratingError}<span class="rate-err tiny">⚠ {ratingError}</span>{/if}
          </div>
        </div>
        <div class="status-inline" aria-busy={statusBusy}>
          <span class="status-label">演出状态</span>
          <div class="status-row" role="radiogroup" aria-label="切换演出状态">
            {#each ALL_STATUSES as s}
              <button
                type="button"
                class="status-chip s{s}"
                class:active={rec.active_status === s}
                onclick={() => setStatus(s)}
                disabled={statusBusy}
                title={statusLabel[s]}
                aria-pressed={rec.active_status === s}
              >{statusLabel[s]}</button>
            {/each}
            {#if statusSaved}<span class="status-saved tiny">✓ 已保存</span>{/if}
            {#if statusError}<span class="status-err tiny">⚠ {statusError}</span>{/if}
          </div>
        </div>
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
          {#if rec.duration}
            <div class="kv-row"><dt>时长</dt><dd>{durationText(rec.duration)}</dd></div>
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
        {#if rec.total_cost > 0}
          <div class="fee-total">
            <span class="fee-total-label">合计</span>
            <span class="fee-total-amount">{formatCurrency(rec.total_cost, rec.pay_price_currency || 'CNY')}</span>
          </div>
        {/if}
        {#if rec.channel}
          <div class="fee-channel"><span class="fee-channel-key">渠道</span><a class="flink" href={`/?q=${encodeURIComponent(rec.channel)}`}>{rec.channel}</a></div>
        {/if}
      </div>
    </div>

    <div class="card section">
      <h3>照片（票根 / 现场照）</h3>
        <div class="photo-actions">
          <label class="btn sm" class:disabled={photoBusy === 'add'}>
            {photoBusy === 'add' ? '上传中…' : '⇪ 添加照片'}
            <input type="file" accept="image/*" hidden bind:files={photoFileList} onchange={(e) => { photoFile = e.target.files?.[0] || null; addPhoto(); e.target.value = ''; }} />
          </label>
          {#if !photos.length}<span class="hint">还没有照片。上传后点击可看大图。</span>{/if}
        </div>
        {#if photos.length}
          <div class="photo-grid">
            {#each photos as p, i (p.id)}
              <div class="photo-cell">
                <img src={coverUrl(p.file_name)} alt="照片 {i + 1}" loading="lazy" onclick={() => { lightbox = true; lightboxSrc = p.file_name; }} />
                <div class="photo-ops">
                  <button type="button" onclick={() => movePhoto(p, -1)} disabled={i === 0} aria-label="前移">◀</button>
                  <button type="button" onclick={() => removePhoto(p)} disabled={photoBusy === p.id} aria-label="删除">✕</button>
                  <button type="button" onclick={() => movePhoto(p, 1)} disabled={i === photos.length - 1} aria-label="后移">▶</button>
                </div>
              </div>
            {/each}
          </div>
        {/if}

    </div>

    {#if rec.remark}
      <div class="card section">
        <h3>备注</h3>
        <p class="remark">{rec.remark}</p>
      </div>
    {/if}

    {#if related.length}
      <div class="card section">
        <h3>相关演出</h3>
        <p class="rel-hint muted tiny">按关联程度排序：同剧目 / 同演员 / 同场馆，命中越多越靠前</p>
        <div class="rel-grid">
          {#each related as x (x.id)}
            <a class="rel-card" href={`/records/${x.id}`}>
              <div class="rel-cover">
                {#if x.coverThumb || x.coverFile}
                  <img src={coverUrl(x.coverThumb || x.coverFile)} alt={x.name} loading="lazy" />
                {:else}
                  <div class="rel-no-cover"><span>{(x.name || '?').slice(0, 1)}</span></div>
                {/if}
              </div>
              <div class="rel-info">
                <span class="rel-name">{x.name}</span>
                <span class="rel-meta">{x.dateText ? x.dateText.slice(0, 10) : ''}{x.city ? ' · ' + x.city : ''}</span>
                <span class="rel-reasons">
                  {#each x.reasons as reason (reason)}<span class="rel-reason">{reason}</span>{/each}
                </span>
              </div>
            </a>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}

{#if lightbox && lightboxSrc}
  <button type="button" class="lightbox" onclick={() => { lightbox = false; lightboxSrc = ''; }} aria-label="关闭大图" transition:fade={{ duration: 150 }}>
    <img src={coverUrl(lightboxSrc)} alt={rec.name} transition:scale={{ start: 0.96, duration: 180 }} />
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
  h1 { font-size: 26px; margin: 0 0 8px; line-height: 1.25; }
  .sub { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 14px; color: var(--text-2); margin-bottom: 4px; }
  .sub .date { color: var(--text-muted); }
  .dot { opacity: 0.4; }
  .loc { color: var(--text-2); border-bottom: 1px dashed var(--border-strong); transition: color var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease); }
  .loc:hover { color: var(--accent); border-color: var(--accent); }
  .actions { display: flex; gap: 8px; margin-top: auto; padding-top: 16px; }

  /* 详情页快速打分：星标控件 + 即时保存反馈 */
  .rate-inline { display: flex; align-items: center; gap: 10px; margin-top: 12px; flex-wrap: wrap; }
  .rate-label { font-size: 13px; color: var(--text-muted); flex: 0 0 auto; }
  .star-row { display: flex; align-items: center; gap: 2px; }
  .star {
    border: none;
    background: none;
    font-size: 22px;
    line-height: 1;
    color: var(--border-strong);
    cursor: pointer;
    padding: 0 1px;
    transition: transform var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .star:hover { transform: scale(1.18); }
  .star.on { color: var(--gold); }
  .star:disabled { cursor: default; opacity: 0.7; }
  .rate-text { margin-left: 6px; font-size: 12.5px; color: var(--text-muted); }
  .rate-saved { margin-left: 4px; font-size: 12.5px; color: var(--accent); }
  .rate-err { margin-left: 4px; font-size: 12.5px; color: var(--danger); }

  /* 详情页快捷切换演出状态：状态药丸一键切换，当前状态高亮 */
  .status-inline { display: flex; align-items: center; gap: 10px; margin-top: 12px; flex-wrap: wrap; }
  .status-label { font-size: 13px; color: var(--text-muted); flex: 0 0 auto; }
  .status-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .status-chip {
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text-2);
    border-radius: 999px;
    padding: 4px 13px;
    font-size: 13px;
    line-height: 1.5;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .status-chip:hover { border-color: var(--border-strong); color: var(--text); }
  .status-chip:active { transform: scale(0.97); }
  .status-chip.active { color: #fff; border-color: transparent; font-weight: 600; }
  .status-chip.s0.active { background: var(--accent); }
  .status-chip.s1.active { background: #926a18; }
  .status-chip.s2.active { background: #5a5a60; text-decoration: line-through; }
  .status-chip.s3.active { background: #5d3e6e; }
  .status-chip:disabled { cursor: default; opacity: 0.7; }
  .status-saved { margin-left: 4px; font-size: 12.5px; color: var(--accent); }
  .status-err { margin-left: 4px; font-size: 12.5px; color: var(--danger); }

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
  .flink { color: var(--text); border-bottom: 1px dashed var(--border-strong); transition: color var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease); }
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
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }
  .fee-total-label { font-size: 14px; font-weight: 600; color: var(--text-2); }
  .fee-total-amount {
    font-size: 20px;
    font-weight: 700;
    color: var(--text);
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

  .rel-hint { margin: 0 0 12px; }
  .rel-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; }
  .rel-card {
    display: flex; flex-direction: column; gap: 8px;
    border: 1px solid var(--border); border-radius: var(--radius);
    padding: 8px; color: inherit; text-decoration: none;
    transition: border-color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .rel-card:hover { border-color: var(--accent); background: var(--accent-softer); transform: translateY(-2px); }
  .rel-cover {
    width: 100%; aspect-ratio: 3 / 4; border-radius: var(--radius-sm);
    overflow: hidden; background: var(--surface-3); flex: 0 0 auto;
  }
  .rel-cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .rel-no-cover {
    width: 100%; height: 100%; display: flex; align-items: center; justify-content: center;
    background: linear-gradient(160deg, var(--surface-3), var(--surface-2));
  }
  .rel-no-cover span { font-family: var(--font-serif); font-size: 36px; color: var(--text-3); opacity: 0.6; }
  .rel-info { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .rel-name {
    font-weight: 600; font-size: 13.5px; line-height: 1.3; color: var(--text);
    display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical;
    overflow: hidden; text-overflow: ellipsis;
  }
  .rel-meta { color: var(--text-3); font-size: 12px; }
  .rel-reasons { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 2px; }
  .rel-reason {
    font-size: 11px; color: var(--accent); background: var(--accent-soft);
    border-radius: 999px; padding: 1px 7px; white-space: nowrap;
  }

  .photo-actions { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
  .photo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 10px; }
  .photo-cell { position: relative; border: 1px solid var(--border); border-radius: var(--radius-sm, 8px); overflow: hidden; background: var(--surface-3); }
  .photo-cell img { width: 100%; aspect-ratio: 3 / 4; object-fit: cover; display: block; cursor: zoom-in; }
  .photo-ops {
    position: absolute; inset: auto 0 0 0; display: flex; justify-content: center; gap: 6px;
    padding: 4px; background: rgb(0 0 0 / 0.45); opacity: 0; transition: opacity 0.15s;
  }
  .photo-cell:hover .photo-ops { opacity: 1; }
  .photo-ops button {
    border: none; background: rgb(255 255 255 / 0.85); color: #111; border-radius: 5px;
    width: 24px; height: 22px; font-size: 11px; cursor: pointer; line-height: 1;
  }
  .photo-ops button:disabled { opacity: 0.4; cursor: default; }
</style>
