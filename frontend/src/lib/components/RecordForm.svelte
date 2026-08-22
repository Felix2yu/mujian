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
      play: '', guest: '',
      active_status: 0,
      date_local: '', coverFile: '', coverThumb: '',
      lat: '', lng: '',
      drama_ids: [], zhezi_ids: [], artist_ids: []
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
    f.artist_ids = (r.artist_ids || []).slice();
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
  let fileInput = $state(null);

  // 演员：自由文本胶囊（未建档案的姓名）。编辑已有记录时，
  // 初始为 record.artist_names 中未关联到档案的部分（等演员列表载入后计算）。
  let freeNames = $state([]);

  // 字段显隐由「设置」控制（同行人 / 实付 / 其他花费），默认全部显示
  let settings = $state({ show_friends: true, show_pay_price: true, show_other_cost: true });

  // 坐标默认折叠在「定位」图标后，点击图标才展开经纬度输入
  let showCoord = $state(false);

  // 常用字段自动补全（来自历史记录 /api/autocomplete/{field}）
  let ac = $state({ city: [], address: [], channel: [], company: [], seat: [], friends: [] });

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
  let dramaQuery = $state('');
  let showDramaList = $state(false);
  let newDrama = $state({ name: '', categoryName: '' });
  let creatingDrama = $state(false);

  const chosenDramas = $derived(
    dramaTree.filter((d) => form.drama_ids.includes(d.id))
  );

  const addableDramas = $derived(
    dramaTree.filter((d) => !form.drama_ids.includes(d.id))
  );

  // 关联剧目下拉的可搜索过滤结果（按名称 / 剧种）
  const filteredDramas = $derived(
    dramaQuery.trim()
      ? addableDramas.filter(
          (d) =>
            (d.name || '').toLowerCase().includes(dramaQuery.trim().toLowerCase()) ||
            (d.categoryName || '').toLowerCase().includes(dramaQuery.trim().toLowerCase())
        )
      : addableDramas
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
    dramaQuery = '';
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

  // 演员 picker state（胶囊 tag 输入）
  let artistList = $state([]);
  let artistQuery = $state('');
  let showArtistList = $state(false);
  let creatingArtist = $state(false);

  const chosenArtists = $derived(
    artistList.filter((a) => form.artist_ids.includes(a.id))
  );
  const addableArtists = $derived(
    artistList.filter((a) => !form.artist_ids.includes(a.id))
  );
  const filteredArtists = $derived(
    artistQuery.trim()
      ? addableArtists.filter((a) => (a.name || '').toLowerCase().includes(artistQuery.trim().toLowerCase()))
      : addableArtists
  );

  function addArtist(aid) {
    if (!aid || form.artist_ids.includes(aid)) return;
    form.artist_ids = [...form.artist_ids, aid];
    artistQuery = '';
  }
  function removeArtist(aid) {
    form.artist_ids = form.artist_ids.filter((x) => x !== aid);
  }
  function removeFreeName(n) {
    freeNames = freeNames.filter((x) => x !== n);
  }

  // 把输入框文本（可含中英文逗号分隔的多个名字）解析为胶囊：
  // 与档案精确同名 → 关联档案；否则 → 自由文本胶囊
  function commitArtistInput() {
    const names = (artistQuery || '').split(/[,，]/).map((x) => x.trim()).filter(Boolean);
    if (!names.length) { artistQuery = ''; return; }
    for (const n of names) {
      const hit = addableArtists.find((a) => a.name === n);
      if (hit) {
        form.artist_ids = [...form.artist_ids, hit.id];
      } else if (
        !freeNames.includes(n) &&
        !chosenArtists.some((a) => a.name === n)
      ) {
        freeNames = [...freeNames, n];
      }
    }
    artistQuery = '';
  }

  function onArtistKeydown(e) {
    if (e.isComposing || e.keyCode === 229) return; // 中文输入法组词中不响应
    if (e.key === 'Enter') {
      e.preventDefault();
      commitArtistInput();
    } else if (e.key === 'Backspace' && !artistQuery) {
      // 输入框为空时退格：删除最后一个胶囊（优先自由文本，其次已关联）
      if (freeNames.length) freeNames = freeNames.slice(0, -1);
      else if (form.artist_ids.length) form.artist_ids = form.artist_ids.slice(0, -1);
    }
  }

  function onArtistInput(e) {
    if (e.isComposing) return; // 中文输入法组词中不解析
    artistQuery = e.currentTarget.value;
    // 输入（含粘贴）中出现逗号：立即批量解析为胶囊
    if (/[,，]/.test(artistQuery)) commitArtistInput();
  }

  // 剧团：一场演出可能隶属多个演出团体，支持逗号分隔一次添加多个
  let companyQuery = $state('');
  const companyTags = $derived(
    (form.company || '').split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  );
  function addCompany(name) {
    name = (name || '').trim();
    if (!name) return;
    const cur = (form.company || '').split(/[,，]/).map((s) => s.trim()).filter(Boolean);
    if (!cur.includes(name)) cur.push(name);
    form.company = cur.join(', ');
  }
  function removeCompany(name) {
    const cur = (form.company || '')
      .split(/[,，]/)
      .map((s) => s.trim())
      .filter(Boolean)
      .filter((n) => n !== name);
    form.company = cur.join(', ');
  }
  function commitCompanyInput() {
    const names = (companyQuery || '').split(/[,，]/).map((x) => x.trim()).filter(Boolean);
    if (!names.length) {
      companyQuery = '';
      return;
    }
    for (const n of names) addCompany(n);
    companyQuery = '';
  }
  function onCompanyKeydown(e) {
    if (e.isComposing || e.keyCode === 229) return; // 中文输入法组词中不响应
    if (e.key === 'Enter') {
      e.preventDefault();
      commitCompanyInput();
    } else if (e.key === 'Backspace' && !companyQuery) {
      const cur = (form.company || '').split(/[,，]/).map((s) => s.trim()).filter(Boolean);
      if (cur.length) form.company = cur.slice(0, -1).join(', ');
    }
  }
  function onCompanyInput(e) {
    if (e.isComposing) return;
    companyQuery = e.currentTarget.value;
    if (/[,，]/.test(companyQuery)) commitCompanyInput();
  }

  async function createNewArtist(name) {
    name = (name || '').trim();
    if (!name || creatingArtist) return;
    creatingArtist = true;
    error = '';
    try {
      const a = await api.createArtist({ name });
      await loadArtistList();
      form.artist_ids = [...form.artist_ids, a.id];
      artistQuery = '';
    } catch (e) {
      error = e.message;
    } finally {
      creatingArtist = false;
    }
  }
  async function loadArtistList() {
    try {
      artistList = await api.listArtists();
    } catch (e) { /* 非关键，忽略 */ }
  }

  onMount(loadDramaTree);

  onMount(async () => {
    // 读取「设置」中的字段显隐偏好，决定编辑界面是否渲染对应字段
    try {
      settings = await api.getSettings();
    } catch (e) {
      /* 读取失败则用默认（全部显示） */
    }
  });

  onMount(async () => {
    await loadArtistList();
    // 编辑场景：档案外的演员姓名进入自由文本胶囊
    const initial = (record && record.artist_names) || [];
    freeNames = initial.filter((n) => !chosenArtists.some((a) => a.name === n));
  });

  onMount(async () => {
    const fields = ['city', 'address', 'channel', 'company', 'seat', 'friends'];
    const results = await Promise.all(fields.map((f) => api.getAutocomplete(f).catch(() => [])));
    const next = {};
    fields.forEach((f, i) => (next[f] = results[i] || []));
    ac = next;
  });

  function triggerUpload() {
    fileInput?.click();
  }

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
    commitArtistInput(); // 输入框里未回车的残留名字也一并收进胶囊
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
      // 演员以关联实体为准：已选演员的名称 + 自由文本胶囊中未匹配档案的名字
      artist_names: [
        ...chosenArtists.map((a) => a.name),
        ...freeNames.filter(
          (n) => !chosenArtists.some((a) => a.name === n) && !artistList.some((a) => a.name === n)
        )
      ],
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
  <!-- ============ 基本信息 ============ -->
  <div class="card section">
    <h3>基本信息</h3>
    <div class="row">
      <div class="f-lg">
        <label>名称 <span class="req">*</span></label>
        <input class="input" bind:value={form.name} placeholder="演出名称" />
      </div>
      <div class="f-sm">
        <label>分类</label>
        <input class="input" bind:value={form.categoryName} list="cat-list" placeholder="如：昆剧" />
        <datalist id="cat-list">
          {#each categories as c}<option value={c.name} />{/each}
        </datalist>
      </div>
    </div>
    <div class="row">
      <div class="f-sm">
        <label>城市</label>
        <input class="input" bind:value={form.city} list="city-list" placeholder="如：上海" />
        <datalist id="city-list">
          {#each ac.city as v}<option value={v} />{/each}
        </datalist>
      </div>
      <div class="f-lg">
        <label>场馆 / 地址</label>
        <div class="addr-row">
          <input class="input" bind:value={form.address} list="addr-list" placeholder="如：上海大剧院" />
          <button
            type="button"
            class="loc-btn"
            class:active={geoStatus === 'ok' && !!form.lat && !!form.lng}
            class:open={showCoord}
            onclick={() => (showCoord = !showCoord)}
            title={showCoord ? '收起坐标' : '定位坐标（点击展开经纬度）'}
            aria-label="定位坐标"
            aria-expanded={showCoord}
          ><svg class="loc-ico" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M12 21c4.5-4.6 7-8.1 7-12a7 7 0 1 0-14 0c0 3.9 2.5 7.4 7 12z" />
              <circle cx="12" cy="9" r="2.4" fill="currentColor" stroke="none" />
            </svg></button>
        </div>
        <datalist id="addr-list">
          {#each ac.address as v}<option value={v} />{/each}
        </datalist>
        {#if showCoord}
          <!-- 坐标：点击定位图标后才展开，平时不显示经纬度 -->
          <div class="sub-field">
            <div class="sub-head">
              <span class="sub-title">坐标</span>
              <span class="hint">地址自动定位，同场馆演出将同步该坐标 · 再次点击定位图标可收起</span>
            </div>
            {#if geoStatus === 'ok' && form.lat && form.lng}
              <div class="geo-line"><svg class="geo-pin" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 21c4.5-4.6 7-8.1 7-12a7 7 0 1 0-14 0c0 3.9 2.5 7.4 7 12z" /><circle cx="12" cy="9" r="2.4" fill="currentColor" stroke="none" /></svg> {form.lat}, {form.lng}</div>
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
              <div class="row" style="margin-top:6px;">
                <input class="input" type="number" step="0.000001" bind:value={form.lat} placeholder="纬度 31.230416" oninput={() => { manualOverride = true; geoStatus = 'ok'; }} />
                <input class="input" type="number" step="0.000001" bind:value={form.lng} placeholder="经度 121.473700" oninput={() => { manualOverride = true; geoStatus = 'ok'; }} />
              </div>
            </details>
          </div>
        {/if}
      </div>
    </div>
  </div>

  <!-- ============ 观演信息 ============ -->
  <div class="card section">
    <h3>观演信息</h3>
    <div class="row">
      <div class="f-md">
        <label>演出时间</label>
        <input class="input" type="datetime-local" bind:value={form.date_local} />
      </div>
      <div class="f-xs">
        <label>状态</label>
        <select class="input" bind:value={form.active_status}>
          <option value={0}>正常</option>
          <option value={1}>想看</option>
          <option value={2}>已取消</option>
          <option value={3}>其他</option>
        </select>
      </div>
      <div class="f-md">
        <label>评分</label>
        <div class="star-row">
          {#each [1, 2, 3, 4, 5] as n}
            <button type="button" class="star" class:on={form.rating >= n} onclick={() => setRating(n)} aria-label={`评分 ${n}`}>★</button>
          {/each}
          <span class="tiny rate-text">{form.rating ? `${form.rating} 分` : '未评分'}</span>
        </div>
      </div>
    </div>
    <div class="row">
      <div class="f-sm">
        <label>座位</label>
        <input class="input" bind:value={form.seat} list="seat-list" placeholder="如：3排15座" />
        <datalist id="seat-list">
          {#each ac.seat as v}<option value={v} />{/each}
        </datalist>
      </div>
      {#if settings.show_friends}
      <div class="f-md">
        <label>同行</label>
        <input class="input" bind:value={form.friends} list="friends-list" placeholder="同行人，多个用逗号分隔" />
        <datalist id="friends-list">
          {#each ac.friends as v}<option value={v} />{/each}
        </datalist>
      </div>
      {/if}
    </div>
  </div>

  <!-- ============ 费用与渠道 ============ -->
  <div class="card section">
    <h3>费用与渠道</h3>
    <div class="row">
      <div class="f-sm">
        <label>购买渠道</label>
        <input class="input" bind:value={form.channel} list="channel-list" placeholder="如：大麦" />
        <datalist id="channel-list">
          {#each ac.channel as v}<option value={v} />{/each}
        </datalist>
      </div>
      <div class="f-sm">
        <label>票价</label>
        <div class="money"><input class="input" type="number" step="0.01" min="0" bind:value={form.price} /><input class="input cur" bind:value={form.price_currency} /></div>
      </div>
    </div>
    {#if settings.show_pay_price}
      <div class="row">
        <div class="f-sm">
          <label>实付</label>
          <div class="money"><input class="input" type="number" step="0.01" min="0" bind:value={form.pay_price} /><input class="input cur" bind:value={form.pay_price_currency} /></div>
        </div>
      </div>
    {/if}
    {#if settings.show_other_cost}
      <div class="row">
        <div class="f-sm">
          <label>其他花费</label>
          <div class="money"><input class="input" type="number" step="0.01" min="0" bind:value={form.other_cost} /><input class="input cur" bind:value={form.other_cost_currency} /></div>
        </div>
      </div>
    {/if}
  </div>

  <!-- ============ 阵容 ============ -->
  <div class="card section">
    <h3>阵容</h3>
    <label>演员 <span class="hint">回车生成胶囊；逗号分隔可一次添加多个；与档案同名自动关联</span></label>
    <div class="combo">
      <div class="tagbox" onclick={(e) => e.currentTarget.querySelector('input')?.focus()}>
        {#each chosenArtists as a (a.id)}
          <span class="capsule linked">
            {a.name}
            <button type="button" class="cap-x" onclick={() => removeArtist(a.id)} title="移除该演员" aria-label={`移除 ${a.name}`}>✕</button>
          </span>
        {/each}
        {#each freeNames as n (n)}
          <span class="capsule free">
            {n}
            <button type="button" class="cap-x" onclick={() => removeFreeName(n)} title="移除" aria-label={`移除 ${n}`}>✕</button>
          </span>
        {/each}
        <input
          placeholder={chosenArtists.length || freeNames.length ? '' : '输入演员姓名，回车确认…'}
          bind:value={artistQuery}
          onfocus={() => (showArtistList = true)}
          onblur={() => setTimeout(() => (showArtistList = false), 120)}
          onkeydown={onArtistKeydown}
          oninput={onArtistInput}
        />
      </div>
      {#if showArtistList && (filteredArtists.length || artistQuery.trim())}
        <div class="combo-list">
          {#each filteredArtists as a (a.id)}
            <button type="button" class="combo-item" onclick={() => addArtist(a.id)}>{a.name}</button>
          {/each}
          {#if artistQuery.trim() && !artistList.some((a) => a.name === artistQuery.trim())}
            <button type="button" class="combo-item create" disabled={creatingArtist} onclick={() => createNewArtist(artistQuery)}>
              {creatingArtist ? '创建中…' : `＋ 新建演员档案「${artistQuery.trim()}」`}
            </button>
          {:else if !filteredArtists.length}
            <div class="combo-empty">无匹配演员</div>
          {/if}
        </div>
      {/if}
    </div>
    <div class="row">
      <div class="f-sm">
        <label>剧团 <span class="hint">回车生成胶囊；逗号分隔可一次添加多个</span></label>
        <div class="tagbox" onclick={(e) => e.currentTarget.querySelector('input')?.focus()}>
          {#each companyTags as t (t)}
            <span class="capsule free">
              {t}
              <button type="button" class="cap-x" onclick={() => removeCompany(t)} title="移除该团体" aria-label={`移除 ${t}`}>✕</button>
            </span>
          {/each}
          <input
            placeholder={companyTags.length ? '' : '如：上海昆剧团'}
            bind:value={companyQuery}
            list="company-list"
            onkeydown={onCompanyKeydown}
            oninput={onCompanyInput}
          />
        </div>
        <datalist id="company-list">
          {#each ac.company as v}<option value={v} />{/each}
        </datalist>
      </div>
    </div>
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
      <div class="combo">
        <input
          class="input"
          placeholder="🔍 搜索并关联已有剧目…"
          bind:value={dramaQuery}
          onfocus={() => (showDramaList = true)}
          onblur={() => setTimeout(() => (showDramaList = false), 120)}
          onkeydown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              const first = filteredDramas[0];
              if (first) addDrama(first.id);
            }
          }}
        />
        {#if showDramaList}
          <div class="combo-list">
            {#each filteredDramas as d (d.id)}
              <button type="button" class="combo-item" onclick={() => addDrama(d.id)}>{d.name}{d.categoryName ? `（${d.categoryName}）` : ''}</button>
            {:else}
              <div class="combo-empty">无匹配剧目，可改用右侧新建</div>
            {/each}
          </div>
        {/if}
      </div>
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
  </div>

  <!-- ============ 备注 ============ -->
  <div class="card section">
    <h3>备注</h3>
    <textarea class="input" rows="8" bind:value={form.remark} placeholder="剧评、观感、备忘…"></textarea>
  </div>

  <!-- ============ 封面 ============ -->
  <div class="card section">
    <h3>封面</h3>
    <div class="cover-layout">
      {#if form.coverFile}
        <img class="preview" src={coverUrl(form.coverFile)} alt="封面预览" />
      {/if}
      <div class="cover-main">
        <div class="upload-row">
          <button type="button" class="btn sm" onclick={triggerUpload} disabled={uploading}>
            {uploading ? '上传中…' : '⇪ 上传图片'}
          </button>
          <input type="file" accept="image/*" onchange={handleUpload} disabled={uploading} hidden bind:this={fileInput} />
          <button type="button" class="btn sm" onclick={() => (pickerOpen = true)}>▦ 从已有演出引用</button>
        </div>
        <div class="cover-url">
          <label>封面 URL</label>
          <input class="input" bind:value={form.coverFile} placeholder="covers/xxx.jpg 或上传图片" />
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
  .form { display: flex; flex-direction: column; gap: 14px; max-width: 860px; margin: 0 auto; }
  .section { padding: 18px 20px; }
  .section h3 { margin: 0 0 6px; font-size: 15.5px; color: var(--text-2); }
  .req { color: var(--accent); }
  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; }

  /* 字段宽度档位：短字段不独占整行（需压过全局 .row > * 的 flex:1） */
  .row > .f-xs { flex: 0 1 130px; min-width: 110px; }
  .row > .f-sm { flex: 0 1 200px; min-width: 150px; }
  .row > .f-md { flex: 1 1 240px; min-width: 180px; }
  .row > .f-lg { flex: 2.4 1 320px; min-width: 220px; }

  .money { display: flex; gap: 6px; }
  .money .cur { max-width: 76px; }

  /* 坐标：场馆的子字段（缩进 + 左侧竖线表示从属） */
  .sub-field {
    margin-top: 10px;
    padding: 10px 12px 12px;
    border-left: 2px solid var(--border-strong);
    border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
    background: var(--surface-2);
  }
  .sub-head { display: flex; align-items: baseline; gap: 8px; margin-bottom: 6px; }
  .sub-title { font-size: 13px; font-weight: 500; color: var(--text-2); }
  .geo-line { padding: 7px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); font-size: 13.5px; }
  .geo-manual { margin-top: 8px; }
  .geo-manual summary { cursor: pointer; color: var(--accent); }

  .star-row { display: flex; align-items: center; gap: 2px; min-height: 40px; }
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

  /* 场馆地址行：输入框 + 内联定位图标 */
  .addr-row { display: flex; align-items: stretch; gap: 8px; }
  .addr-row .input { flex: 1; min-width: 0; }
  .loc-btn {
    flex: 0 0 auto;
    width: 40px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    cursor: pointer;
    color: var(--text-2);
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: all var(--t-fast) var(--ease);
  }
  .loc-btn:hover { border-color: var(--accent); background: var(--accent-soft); color: var(--accent); }
  .loc-btn.active { border-color: var(--accent); background: var(--accent-soft); color: var(--accent); }
  .loc-btn.open { border-color: var(--accent); background: var(--accent-soft); color: var(--accent); }
  .geo-pin { vertical-align: -2px; }

  /* 演员胶囊 tag 输入 */
  .tagbox {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    min-height: 40px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    cursor: text;
    transition: border-color var(--t-fast) var(--ease), box-shadow var(--t-fast) var(--ease),
      background var(--t-fast) var(--ease);
  }
  .tagbox:hover { border-color: var(--border-strong); }
  .tagbox:focus-within {
    border-color: var(--accent);
    background: var(--surface);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }
  .tagbox input {
    flex: 1;
    min-width: 140px;
    height: 28px;
    border: none;
    background: none;
    outline: none;
    padding: 0 4px;
    font-size: 14px;
    font-family: inherit;
    color: var(--text);
  }
  .tagbox input::placeholder { color: var(--text-3); }
  .capsule {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 6px 3px 11px;
    border-radius: 999px;
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
    animation: fadeIn var(--t-fast) var(--ease);
  }
  .capsule.linked { background: var(--accent-soft); color: var(--accent); }
  .capsule.free { background: var(--surface-3); color: var(--text-2); border: 1px solid var(--border); }
  .cap-x {
    border: none;
    background: none;
    color: inherit;
    opacity: 0.55;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    cursor: pointer;
    font-size: 10px;
    line-height: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
  }
  .cap-x:hover { opacity: 1; background: rgba(0, 0, 0, 0.12); }

  .cover-layout { display: flex; gap: 18px; align-items: flex-start; flex-wrap: wrap; }
  .cover-main { display: flex; flex-direction: column; gap: 14px; flex: 1; min-width: 220px; }
  .upload-row { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; margin-top: 12px; }
  .cover-url label { display: block; font-size: 13px; font-weight: 500; color: var(--text-2); margin: 0 0 6px; }
  .preview {
    width: 180px;
    height: 240px;
    object-fit: cover;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    flex-shrink: 0;
  }

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

  /* 可搜索下拉（演员 / 剧目共用） */
  .combo { position: relative; flex: 2; min-width: 200px; }
  .combo-list {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 30;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: 0 10px 28px rgba(0, 0, 0, 0.22);
    max-height: 230px;
    overflow-y: auto;
    padding: 4px;
  }
  .combo-item {
    display: block;
    width: 100%;
    text-align: left;
    border: none;
    background: none;
    padding: 9px 12px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    color: var(--text-2);
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .combo-item:hover { background: var(--accent-soft); color: var(--accent); }
  .combo-item.create { color: var(--accent); font-weight: 500; }
  .combo-empty { padding: 10px 12px; color: var(--text-3); font-size: 13px; }
  .ply-new { border: 1px solid var(--border); border-radius: var(--radius); padding: 10px 14px; flex: 1; }
  .ply-new summary { cursor: pointer; color: var(--accent); }
  .ply-new-body { margin-top: 10px; display: flex; flex-direction: column; gap: 10px; }
</style>
