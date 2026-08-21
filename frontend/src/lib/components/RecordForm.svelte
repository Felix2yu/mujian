<script>
  import { onMount } from 'svelte';
  import { api, coverUrl } from '$lib/api.js';
  import { geocodeAddress } from '$lib/geocode.js';
  import CoverPicker from '$lib/components/CoverPicker.svelte';

  let { record = null, categories = [], onSubmit, onCancel } = $props();

  function emptyForm() {
    return {
      name: '', channel: '', city: '', address: '', categoryName: '',
      rating: 0, seat: '', friends: '', company: '', remark: '',
      price: 0, price_currency: 'CNY',
      pay_price: 0, pay_price_currency: 'CNY',
      other_cost: 0, other_cost_currency: 'CNY',
      artist_names: '', play: '', guest: '',
      active_status: 0,
      date_local: '', coverFile: '', coverThumb: '',
      lat: '', lng: '',
      drama_ids: [], zhezi_ids: []
    };
  }

  function fromRecord(r) {
    const f = emptyForm();
    if (!r) return f;
    f.name = r.name || '';
    f.channel = r.channel || '';
    f.city = r.city || '';
    f.address = r.address || '';
    f.categoryName = r.categoryName || '';
    f.rating = r.rating || 0;
    f.seat = r.seat || '';
    f.friends = r.friends || '';
    f.company = r.company || '';
    f.remark = r.remark || '';
    f.price = r.price || 0;
    f.price_currency = r.price_currency || 'CNY';
    f.pay_price = r.pay_price || 0;
    f.pay_price_currency = r.pay_price_currency || 'CNY';
    f.other_cost = r.other_cost || 0;
    f.other_cost_currency = r.other_cost_currency || 'CNY';
    f.artist_names = (r.artist_names || []).join(', ');
    f.play = (r.play || []).join(', ');
    f.guest = (r.guest || []).join(', ');
    f.drama_ids = (r.drama_ids || []).slice();
    f.zhezi_ids = (r.zhezi_ids || []).slice();
    f.active_status = r.active_status || 0;
    f.coverFile = r.coverFile || '';
    f.coverThumb = r.coverThumb || '';
    if (r.date) {
      const d = new Date(r.date * 1000);
      const pad = (n) => String(n).padStart(2, '0');
      f.date_local = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    }
    if (r.coordinate) {
      f.lat = r.coordinate.latitude ?? '';
      f.lng = r.coordinate.longitude ?? '';
    }
    return f;
  }

  let form = $state(fromRecord(record));
  let error = $state('');
  let uploading = $state(false);
  let saving = $state(false);
  let pickerOpen = $state(false);

  // 地址自动定位（高德地理编码）状态
  let geoStatus = $state('idle'); // idle | loading | ok | nokey | notfound | error
  let lastGeocoded = $state('');
  let manualOverride = $state(false);
  let geoTimer = null;

  // 已有记录载入后，不立即重新解析其原始地址
  lastGeocoded = form.address || '';
  if (form.lat && form.lng) geoStatus = 'ok';

  $effect(() => {
    const addr = form.address;
    const city = form.city;
    if (!addr || addr.trim() === '') {
      lastGeocoded = '';
      geoStatus = 'idle';
      return;
    }
    if (addr === lastGeocoded) return; // 地址未变：保留现有（含手动校正）值
    manualOverride = false;
    clearTimeout(geoTimer);
    geoStatus = 'loading';
    geoTimer = setTimeout(async () => {
      try {
        const c = await geocodeAddress(addr, city || '');
        form.lat = String(c.lat);
        form.lng = String(c.lng);
        geoStatus = 'ok';
        lastGeocoded = addr;
      } catch (e) {
        geoStatus = (e && e.code) || 'error';
        lastGeocoded = addr; // 标记已尝试，避免对同地址反复请求
      }
    }, 600);
  });

  // 剧目 / 折子 picker state
  let dramaTree = $state([]);
  let pickAdd = $state('');
  let newDrama = $state({ name: '', categoryName: '' });
  let creatingDrama = $state(false);

  const chosenDramas = $derived(
    dramaTree.filter((d) => form.drama_ids.includes(d.id))
  );

  const addableDramas = $derived(
    dramaTree.filter((d) => !form.drama_ids.includes(d.id))
  );

  function isZheziSelected(zid) {
    return form.zhezi_ids.includes(zid);
  }

  function toggleZhezi(zid) {
    form.zhezi_ids = isZheziSelected(zid)
      ? form.zhezi_ids.filter((x) => x !== zid)
      : [...form.zhezi_ids, zid];
  }

  function addDrama(did) {
    if (!did || form.drama_ids.includes(did)) return;
    form.drama_ids = [...form.drama_ids, did];
    pickAdd = '';
  }

  function removeDrama(did) {
    form.drama_ids = form.drama_ids.filter((x) => x !== did);
    // 摘除该剧目下已选择的折子
    const drama = dramaTree.find((d) => d.id === did);
    const dramaZhezis = new Set((drama?.zhezis || []).map((z) => z.id));
    form.zhezi_ids = form.zhezi_ids.filter((z) => !dramaZhezis.has(z));
  }

  async function createNewDrama() {
    const name = newDrama.name.trim();
    if (!name || creatingDrama) return;
    creatingDrama = true;
    error = '';
    try {
      const d = await api.createDrama({ name, categoryName: newDrama.categoryName.trim() });
      await loadDramaTree();
      form.drama_ids = [...form.drama_ids, d.id];
      newDrama = { name: '', categoryName: '' };
    } catch (e) {
      error = e.message;
    } finally {
      creatingDrama = false;
    }
  }

  async function loadDramaTree() {
    try {
      dramaTree = await api.getDramaTree();
    } catch (e) { /* 非关键，忽略 */ }
  }

  onMount(loadDramaTree);

  async function handleUpload(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const res = await api.uploadFile(file);
      form.coverFile = res.key;
      if (res.thumb) form.coverThumb = res.thumb;
    } catch (err) {
      error = err.message;
    } finally {
      uploading = false;
    }
  }

  function pickCover(c) {
    form.coverFile = c.file_name;
    form.coverThumb = '';
  }

  function splitList(s) {
    return (s || '').split(/[,，]/).map((x) => x.trim()).filter(Boolean);
  }

  async function handleSubmit() {
    error = '';
    if (!form.name.trim()) {
      error = '名称为必填项';
      return;
    }
    const payload = {
      name: form.name.trim(),
      channel: form.channel.trim(),
      city: form.city.trim(),
      address: form.address.trim(),
      categoryName: form.categoryName.trim(),
      rating: Number(form.rating) || 0,
      seat: form.seat.trim(),
      friends: form.friends.trim(),
      company: form.company.trim(),
      remark: form.remark.trim(),
      price: Number(form.price) || 0,
      price_currency: form.price_currency || 'CNY',
      pay_price: Number(form.pay_price) || 0,
      pay_price_currency: form.pay_price_currency || 'CNY',
      other_cost: Number(form.other_cost) || 0,
      other_cost_currency: form.other_cost_currency || 'CNY',
      artist_names: splitList(form.artist_names),
      // 剧目字段由所关联的剧目档案推导，保证与档案一致
      play: chosenDramas.map((d) => d.name),
      drama_ids: form.drama_ids,
      zhezi_ids: form.zhezi_ids,
      guest: splitList(form.guest),
      active_status: Number(form.active_status) || 0,
      coverFile: form.coverFile.trim(),
      coverThumb: form.coverThumb.trim()
    };
    if (form.date_local) {
      const t = new Date(form.date_local);
      if (!isNaN(t)) {
        payload.date = Math.floor(t.getTime() / 1000);
        const pad = (n) => String(n).padStart(2, '0');
        payload.dateText = `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())} ${pad(t.getHours())}:${pad(t.getMinutes())}`;
      }
    }
    if (form.lat !== '' || form.lng !== '') {
      payload.coordinate = { latitude: parseFloat(form.lat) || 0, longitude: parseFloat(form.lng) || 0 };
    }
    saving = true;
    try {
      await onSubmit(payload);
    } finally {
      saving = false;
    }
  }

  function setRating(n) {
    form.rating = form.rating === n ? 0 : n;
  }
</script>

<form class="form" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
  <div class="card section">
    <h3>基本信息</h3>
    <div class="row">
      <div class="grow2">
        <label>名称 <span class="req">*</span></label>
        <input class="input" bind:value={form.name} placeholder="演出名称" />
      </div>
      <div>
        <label>分类</label>
        <input class="input" bind:value={form.categoryName} list="cat-list" placeholder="如：昆剧" />
        <datalist id="cat-list">
          {#each categories as c}<option value={c.name} />{/each}
        </datalist>
      </div>
    </div>
    <div class="row">
      <div><label>渠道 / 平台</label><input class="input" bind:value={form.channel} placeholder="如：大麦" /></div>
      <div><label>城市</label><input class="input" bind:value={form.city} placeholder="如：上海" /></div>
      <div class="grow2"><label>场馆 / 地址</label><input class="input" bind:value={form.address} placeholder="如：上海大剧院" /></div>
    </div>
    <div class="row">
      <div class="grow2">
        <label>坐标 <span class="hint">地址自动定位，同场馆演出将同步该坐标</span></label>
        {#if geoStatus === 'ok' && form.lat && form.lng}
          <div class="geo-line">📍 {form.lat}, {form.lng}</div>
        {:else if geoStatus === 'loading'}
          <div class="geo-line muted">定位中…</div>
        {:else if geoStatus === 'nokey'}
          <div class="geo-line muted">未配置高德 Key，请在「设置」填写后自动定位</div>
        {:else if geoStatus === 'notfound'}
          <div class="geo-line muted">未找到该地址，可手动校正</div>
        {:else if geoStatus === 'error'}
          <div class="geo-line muted">定位失败（网络/密钥），可手动校正</div>
        {:else}
          <div class="geo-line muted">填写地址后自动定位</div>
        {/if}
        <details class="geo-manual" open={geoStatus === 'nokey' || geoStatus === 'notfound' || geoStatus === 'error' || manualOverride}>
          <summary class="small">手动校正</summary>
          <div class="money" style="margin-top:6px;">
            <input class="input" type="number" step="0.000001" bind:value={form.lat} placeholder="纬度 31.230416" oninput={() => { manualOverride = true; geoStatus = 'ok'; }} />
            <input class="input" type="number" step="0.000001" bind:value={form.lng} placeholder="经度 121.473700" oninput={() => { manualOverride = true; geoStatus = 'ok'; }} />
          </div>
        </details>
      </div>
    </div>
  </div>

  <div class="card section">
    <h3>时间与状态</h3>
    <div class="row">
      <div>
        <label>演出时间</label>
        <input class="input" type="datetime-local" bind:value={form.date_local} />
      </div>
      <div>
        <label>状态</label>
        <select class="input" bind:value={form.active_status}>
          <option value={0}>正常</option>
          <option value={1}>想看</option>
          <option value={2}>已取消</option>
          <option value={3}>其他</option>
        </select>
      </div>
      <div>
        <label>评分</label>
        <div class="star-row">
          {#each [1, 2, 3, 4, 5] as n}
            <button type="button" class="star" class:on={form.rating >= n} onclick={() => setRating(n)} aria-label={`评分 ${n}`}>★</button>
          {/each}
          <span class="tiny rate-text">{form.rating ? `${form.rating} 分` : '未评分'}</span>
        </div>
      </div>
    </div>
  </div>

  <div class="card section">
    <h3>费用</h3>
    <div class="row">
      <div>
        <label>票价</label>
        <div class="money"><input class="input" type="number" step="0.01" min="0" bind:value={form.price} /><input class="input cur" bind:value={form.price_currency} /></div>
      </div>
      <div>
        <label>实付</label>
        <div class="money"><input class="input" type="number" step="0.01" min="0" bind:value={form.pay_price} /><input class="input cur" bind:value={form.pay_price_currency} /></div>
      </div>
      <div>
        <label>其他花费</label>
        <div class="money"><input class="input" type="number" step="0.01" min="0" bind:value={form.other_cost} /><input class="input cur" bind:value={form.other_cost_currency} /></div>
      </div>
    </div>
  </div>

  <div class="card section">
    <h3>阵容与同行</h3>
    <label>演员 <span class="hint">逗号分隔</span></label>
    <input class="input" bind:value={form.artist_names} placeholder="沈昳丽, 张伟伟, 胡刚" />
    <label>剧目</label>
    <div class="ply">
      {#if chosenDramas.length === 0}
        <div class="ply-empty muted tiny">尚未关联剧目。从下方选择或新建一个剧目档案。</div>
      {/if}
      {#each chosenDramas as d (d.id)}
        <div class="ply-item">
          <div class="ply-head">
            <span class="ply-name">
              {d.name}{#if d.categoryName}<em class="ply-cat">{d.categoryName}</em>{/if}
            </span>
            <button type="button" class="ply-x" onclick={() => removeDrama(d.id)} title="移除该剧目">✕</button>
          </div>
          {#if d.zhezis && d.zhezis.length}
            <div class="ply-zhezis">
              <span class="small muted">折子（选中表示本次演出了该折戏）</span>
              <div class="zhezi-grid">
                {#each d.zhezis as z (z.id)}
                  <label class="zhezi">
                    <input type="checkbox" checked={isZheziSelected(z.id)} onchange={() => toggleZhezi(z.id)} />
                    <span>{z.name}</span>
                  </label>
                {/each}
              </div>
            </div>
          {:else}
            <div class="small muted">该剧目暂无折子（可在剧目详情页添加）</div>
          {/if}
        </div>
      {/each}
    </div>
    <div class="ply-add">
      <select class="input" bind:value={pickAdd} onchange={(e) => addDrama(e.currentTarget.value)}>
        <option value="">＋ 关联已有剧目…</option>
        {#each addableDramas as d}
          <option value={d.id}>{d.name}{d.categoryName ? `（${d.categoryName}）` : ''}</option>
        {/each}
      </select>
      <details class="ply-new">
        <summary class="small">＋ 新建剧目档案</summary>
        <div class="ply-new-body">
          <div class="row">
            <input class="input" placeholder="剧目，如：牡丹亭" bind:value={newDrama.name} onkeydown={(e) => e.key === 'Enter' && createNewDrama()} />
            <input class="input" placeholder="剧种，如：昆曲" bind:value={newDrama.categoryName} onkeydown={(e) => e.key === 'Enter' && createNewDrama()} />
          </div>
          <button type="button" class="btn sm" onclick={createNewDrama} disabled={creatingDrama || !newDrama.name.trim()}>{creatingDrama ? '创建中…' : '创建并关联'}</button>
        </div>
      </details>
    </div>
    <div class="row">
      <div><label>同行</label><input class="input" bind:value={form.friends} /></div>
      <div><label>剧团</label><input class="input" bind:value={form.company} /></div>
      <div><label>座位</label><input class="input" bind:value={form.seat} placeholder="如：3排15座" /></div>
    </div>
  </div>

  <div class="card section">
    <h3>备注</h3>
    <textarea class="input" rows="4" bind:value={form.remark} placeholder="剧评、观感、备忘…"></textarea>
  </div>

  <div class="card section">
    <h3>封面</h3>
    <div class="row">
      <div class="grow2">
        <label>封面</label>
        <input class="input" bind:value={form.coverFile} placeholder="covers/xxx.jpg 或上传图片" />
        <div class="upload-row">
          <label class="btn sm upload-btn">
            {uploading ? '上传中…' : '⇪ 上传图片'}
            <input type="file" accept="image/*" onchange={handleUpload} disabled={uploading} hidden />
          </label>
          <button type="button" class="btn sm" onclick={() => (pickerOpen = true)}>▦ 从已有演出引用</button>
          {#if form.coverFile}
            <img class="preview" src={coverUrl(form.coverFile)} alt="封面预览" />
          {/if}
        </div>
      </div>
    </div>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  <div class="actions">
    <button type="submit" class="btn primary lg" disabled={saving}>{saving ? '保存中…' : '保存'}</button>
    <button type="button" class="btn lg" onclick={onCancel}>取消</button>
  </div>
</form>

<CoverPicker open={pickerOpen} onSelect={pickCover} onClose={() => (pickerOpen = false)} />

<style>
  .form { display: flex; flex-direction: column; gap: 14px; max-width: 860px; }
  .section { padding: 18px 20px; }
  .section h3 { margin: 0 0 6px; font-size: 15.5px; color: var(--text-2); }
  .req { color: var(--accent); }
  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; }
  .grow2 { flex: 2 !important; }
  .money { display: flex; gap: 6px; }
  .money .cur { max-width: 76px; }
  .geo-line { padding: 8px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); font-size: 13.5px; }
  .geo-manual { margin-top: 8px; }
  .geo-manual summary { cursor: pointer; color: var(--accent); }

  .star-row { display: flex; align-items: center; gap: 2px; padding-top: 2px; }
  .star {
    border: none;
    background: none;
    font-size: 24px;
    color: var(--border-strong);
    cursor: pointer;
    padding: 0 2px;
    transition: transform var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .star:hover { transform: scale(1.2); }
  .star.on { color: var(--gold); }
  .rate-text { margin-left: 8px; }

  .upload-row { display: flex; align-items: center; gap: 12px; margin-top: 8px; }
  .upload-btn { cursor: pointer; }
  .preview { width: 60px; height: 84px; object-fit: cover; border-radius: var(--radius-sm); border: 1px solid var(--border); }

  .actions { display: flex; gap: 10px; margin-top: 4px; }

  /* 剧目 / 折子 picker */
  .ply { display: flex; flex-direction: column; gap: 10px; }
  .ply-empty { padding: 8px 2px; }
  .ply-item { border: 1px solid var(--border); border-radius: var(--radius); padding: 12px 14px; background: var(--surface); }
  .ply-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .ply-name { font-weight: 600; font-size: 14.5px; }
  .ply-cat { font-style: normal; font-size: 12px; color: var(--text-muted); background: var(--surface-3); border-radius: 999px; padding: 2px 9px; margin-left: 8px; }
  .ply-x { border: none; background: none; color: var(--text-3); width: 24px; height: 24px; border-radius: 50%; cursor: pointer; font-size: 12px; }
  .ply-x:hover { background: var(--danger-soft); color: var(--danger); }
  .ply-zhezis { margin-top: 10px; }
  .ply-zhezis .small { display: block; margin-bottom: 6px; }
  .zhezi-grid { display: flex; flex-wrap: wrap; gap: 8px; }
  .zhezi {
    display: inline-flex; align-items: center; gap: 6px; cursor: pointer;
    border: 1px solid var(--border); border-radius: 999px; padding: 5px 12px;
    font-size: 13px; color: var(--text-2); transition: all var(--t-fast) var(--ease);
    user-select: none;
  }
  .zhezi:has(input:checked) { background: var(--accent-soft); border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .zhezi input { accent-color: var(--accent); }
  .ply-add { display: flex; gap: 10px; align-items: stretch; margin-top: 4px; flex-wrap: wrap; }
  .ply-add .input { flex: 1; }
  .ply-new { border: 1px solid var(--border); border-radius: var(--radius); padding: 10px 14px; flex: 1; }
  .ply-new summary { cursor: pointer; color: var(--accent); }
  .ply-new-body { margin-top: 10px; display: flex; flex-direction: column; gap: 10px; }
</style>
