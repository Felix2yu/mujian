<script>
  import { api, coverUrl } from '$lib/api.js';
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
      lat: '', lng: ''
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
      play: splitList(form.play),
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

<form class="form" on:submit|preventDefault={handleSubmit}>
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
            <button type="button" class="star" class:on={form.rating >= n} on:click={() => setRating(n)} aria-label={`评分 ${n}`}>★</button>
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
    <label>剧目 <span class="hint">逗号分隔</span></label>
    <input class="input" bind:value={form.play} placeholder="蝴蝶梦, 邯郸记" />
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
    <h3>封面与位置</h3>
    <div class="row">
      <div class="grow2">
        <label>封面</label>
        <input class="input" bind:value={form.coverFile} placeholder="covers/xxx.jpg 或上传图片" />
        <div class="upload-row">
          <label class="btn sm upload-btn">
            {uploading ? '上传中…' : '⇪ 上传图片'}
            <input type="file" accept="image/*" on:change={handleUpload} disabled={uploading} hidden />
          </label>
          <button type="button" class="btn sm" on:click={() => (pickerOpen = true)}>▦ 从已有演出引用</button>
          {#if form.coverFile}
            <img class="preview" src={coverUrl(form.coverFile)} alt="封面预览" />
          {/if}
        </div>
      </div>
      <div>
        <label>坐标 <span class="hint">纬度 / 经度</span></label>
        <div class="money">
          <input class="input" type="number" step="0.0001" bind:value={form.lat} placeholder="31.2304" />
          <input class="input" type="number" step="0.0001" bind:value={form.lng} placeholder="121.4737" />
        </div>
      </div>
    </div>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  <div class="actions">
    <button type="submit" class="btn primary lg" disabled={saving}>{saving ? '保存中…' : '保存'}</button>
    <button type="button" class="btn lg" on:click={onCancel}>取消</button>
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
</style>
