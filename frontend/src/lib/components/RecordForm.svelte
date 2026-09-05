<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { api, coverUrl } from '$lib/api.js';
  import { geocodeAddress } from '$lib/geocode.js';
  import { STATUS_LABELS } from '$lib/statusPrefs.js';
  import CoverPicker from '$lib/components/CoverPicker.svelte';
  import CategoryTags from '$lib/components/CategoryTags.svelte';

  let { record = null, categories = [], initialDate = '', onSubmit, onCancel } = $props();

  function emptyForm() {
    return {
      name: '', channel: '', city: '', address: '', categoryNames: [],
      rating: 0, seat: '', friends: '', company: '', remark: '',
      duration: 120,
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
    if (!r) {
      if (initialDate) f.date_local = `${initialDate}T${settings.default_start_time}`;
      return f;
    }
    f.name = r.name || '';
    f.channel = r.channel || '';
    f.city = r.city || '';
    f.address = r.address || '';
    f.categoryNames = (r.categoryNames && r.categoryNames.length ? r.categoryNames : r.categoryName ? [r.categoryName] : []).slice();
    f.rating = r.rating || 0;
    f.duration = r.duration || 0;
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

  // AI 填写：粘贴演出信息后调用后端模型提取字段并批量填入表单
  let aiEnabled = $state(false);
  let aiOpen = $state(false);
  let aiText = $state('');
  let aiBusy = $state(false);
  let aiErr = $state('');
  let aiDone = $state(''); // 从链接解析成功时的来源提示（面板保持展开以展示）

  onMount(async () => {
    try {
      const s = await api.getSettings();
      aiEnabled = !!s.ai_enabled;
    } catch (e) {
      aiEnabled = false;
    }
  });

  // 演员：自由文本胶囊（未建档案的姓名）。编辑已有记录时，
  // 初始为 record.artist_names 中未关联到档案的部分（等演员列表载入后计算）。
  let freeNames = $state([]);

  // 字段显隐由「设置」控制（同行人 / 实付 / 其他花费），默认全部显示
  let settings = $state({ show_friends: true, show_pay_price: true, show_other_cost: true, multi_currency: true, default_start_time: '19:30' });

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
    // 组件销毁时取消未触发的地理编码定时器（$effect 清理函数）。
    return () => clearTimeout(geoTimer);
  });

  // 剧目 / 折子 picker state
  let dramaTree = $state([]);
  let dramaQuery = $state('');
  let dramaComposing = $state(false);
  let showDramaList = $state(false);
  let newDrama = $state({ name: '' });
  let creatingDrama = $state(false);

  const chosenDramas = $derived(
    dramaTree.filter((d) => form.drama_ids.includes(d.id))
  );

  // 已选剧目按 form.drama_ids 顺序渲染（拖拽排序即重排该数组），
  // 提交时 play 数组也按此顺序生成，保证与详情页展示一致。
  const orderedChosenDramas = $derived(
    form.drama_ids.map((id) => dramaTree.find((d) => d.id === id)).filter(Boolean)
  );

  let dramaDragIdx = $state(-1);
  let dramaOverIdx = $state(-1);
  let dramaOverBefore = $state(true);

  function onDramaDragOver(e, i) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    dramaOverIdx = i;
    dramaOverBefore = e.clientY < rect.top + rect.height / 2;
  }

  // 已关联剧目的内联编辑：不离开表单即可改名。剧种交由后端按「单独演出」自动聚合，
  // 这里不再暴露剧种编辑，避免改一场演出就污染该剧目的全部剧种展示（footgun）。
  let editingDramaId = $state(null);
  let dramaEdit = $state({ name: '' });
  let savingDramaEdit = $state(false);

  function startDramaEdit(d) {
    editingDramaId = d.id;
    dramaEdit = { name: d.name };
  }

  function cancelDramaEdit() {
    editingDramaId = null;
    dramaEdit = { name: '' };
  }

  async function saveDramaEdit() {
    if (!dramaEdit.name.trim() || savingDramaEdit) return;
    savingDramaEdit = true;
    error = '';
    try {
      await api.updateDrama(editingDramaId, {
        name: dramaEdit.name.trim()
      });
      await loadDramaTree();
      cancelDramaEdit();
    } catch (e) {
      error = e.message;
    } finally {
      savingDramaEdit = false;
    }
  }

  // 内联新增折子：无需跳到剧目详情页。在当前剧目下直接创建并刷新 tree，
  // 创建后自动选中该折子。
  let addZheziFor = $state('');
  let newZheziName = $state('');
  let savingZhezi = $state(false);

  async function createZheziFor(did) {
    const name = newZheziName.trim();
    if (!name || !did || savingZhezi) return;
    savingZhezi = true;
    error = '';
    try {
      const z = await api.createZhezi(did, { name: name });
      await loadDramaTree();
      if (z && z.id) form.zhezi_ids = [...form.zhezi_ids, z.id];
      newZheziName = '';
      addZheziFor = '';
    } catch (e) {
      error = e.message;
    } finally {
      savingZhezi = false;
    }
  }

  function onDramaDropAt(targetIdx) {
    if (dramaDragIdx < 0 || dramaDragIdx === targetIdx) {
      dramaDragIdx = -1;
      dramaOverIdx = -1;
      return;
    }
    const next = form.drama_ids.slice();
    const [moved] = next.splice(dramaDragIdx, 1);
    next.splice(targetIdx, 0, moved);
    form.drama_ids = next;
    dramaDragIdx = -1;
    dramaOverIdx = -1;
  }

  const addableDramas = $derived(
    dramaTree.filter((d) => !form.drama_ids.includes(d.id))
  );

  // 关联剧目下拉的可搜索过滤结果（按名称 / 剧种）；组词中暂停过滤
  const filteredDramas = $derived(
    dramaQuery.trim() && !dramaComposing
      ? addableDramas.filter(
          (d) =>
            (d.name || '').toLowerCase().includes(dramaQuery.trim().toLowerCase()) ||
            (d.categoryName || '').toLowerCase().includes(dramaQuery.trim().toLowerCase()) ||
            (d.categoryNames || []).some((c) => c.toLowerCase().includes(dramaQuery.trim().toLowerCase()))
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
    showDramaList = false;
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
      const d = await api.createDrama({ name });
      await loadDramaTree();
      form.drama_ids = [...form.drama_ids, d.id];
      newDrama = { name: '' };
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

  // 演员胶囊统一视图：档案演员 + 自由文本按展示顺序合并，支持拖拽排序。
  // 重排后写回 form.artist_ids 与 freeNames，最终 artist_names 顺序即此顺序。
  let artistDragIdx = $state(-1);
  let artistOverIdx = $state(-1);
  const artistItems = $derived([
    ...chosenArtists.map((a) => ({ kind: 'linked', key: a.id, label: a.name, id: a.id })),
    ...freeNames.map((n) => ({ kind: 'free', key: 'free:' + n, label: n }))
  ]);

  let artistOverBefore = $state(true);
  function onArtistDragOver(e, i) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    artistOverIdx = i;
    artistOverBefore = e.clientX < rect.left + rect.width / 2;
  }
  function onArtistDropAt(targetIdx, before) {
    if (artistDragIdx < 0 || artistDragIdx === targetIdx) {
      artistDragIdx = -1;
      artistOverIdx = -1;
      return;
    }
    const next = artistItems.slice();
    const [moved] = next.splice(artistDragIdx, 1);
    // 移除拖动项后，位于其后的目标索引左移一位；再按指示的插入侧落位
    const at = artistDragIdx < targetIdx ? targetIdx - 1 : targetIdx;
    next.splice(before ? at : at + 1, 0, moved);
    form.artist_ids = next.filter((x) => x.kind === 'linked').map((x) => x.id);
    freeNames = next.filter((x) => x.kind === 'free').map((x) => x.label);
    artistDragIdx = -1;
    artistOverIdx = -1;
  }
  let artistQuery = $state('');
  // 输入法组词中暂停本地过滤，避免拼音中间态把下拉列表过滤得闪烁
  let artistComposing = $state(false);
  let showArtistList = $state(false);
  let creatingArtist = $state(false);

  const chosenArtists = $derived(
    artistList.filter((a) => form.artist_ids.includes(a.id))
  );
  const addableArtists = $derived(
    artistList.filter((a) => !form.artist_ids.includes(a.id))
  );
  const filteredArtists = $derived(
    artistQuery.trim() && !artistComposing
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

  // 失焦时：输入内容与演员档案精确同名才自动提交；新名字留在输入框，需回车显式确认
  function onArtistBlur() {
    const n = artistQuery.trim();
    if (n && artistList.some((a) => a.name === n)) commitArtistInput();
    setTimeout(() => (showArtistList = false), 120);
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

  // 剧团胶囊拖拽排序：重排后写回逗号分隔的 company 字段
  let companyDragIdx = $state(-1);
  let companyOverIdx = $state(-1);
  let companyOverBefore = $state(true);
  function onCompanyDragOver(e, ti) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    companyOverIdx = ti;
    companyOverBefore = e.clientX < rect.left + rect.width / 2;
  }
  function onCompanyDropAt(targetIdx, before) {
    if (companyDragIdx < 0 || companyDragIdx === targetIdx) {
      companyDragIdx = -1;
      companyOverIdx = -1;
      return;
    }
    const next = companyTags.slice();
    const [moved] = next.splice(companyDragIdx, 1);
    // 移除拖动项后，位于其后的目标索引左移一位；再按指示的插入侧落位
    const at = companyDragIdx < targetIdx ? targetIdx - 1 : targetIdx;
    next.splice(before ? at : at + 1, 0, moved);
    form.company = next.join(', ');
    companyDragIdx = -1;
    companyOverIdx = -1;
  }
  function onCompanyInput(e) {
    if (e.isComposing) return;
    companyQuery = e.currentTarget.value;
    if (/[,，]/.test(companyQuery)) commitCompanyInput();
  }

  // 失焦时：输入内容在历史团体中才自动提交；新名称留在输入框，需回车生成胶囊
  function onCompanyBlur() {
    const n = companyQuery.trim();
    if (n && ac.company.includes(n)) commitCompanyInput();
    setTimeout(() => (showCompanyList = false), 120);
  }

  // 剧团 / 渠道输入建议：子串匹配历史值，点击即补充（替代原生 datalist 的不可控匹配）
  let showCompanyList = $state(false);
  let showChannelList = $state(false);
  const filteredCompanies = $derived.by(() => {
    const q = companyQuery.trim().toLowerCase();
    const base = ac.company.filter((v) => !companyTags.includes(v));
    if (!q) return base;
    return base.filter((v) => v.toLowerCase().includes(q));
  });
  const filteredChannels = $derived.by(() => {
    const q = (form.channel || '').trim().toLowerCase();
    if (!q) return ac.channel;
    return ac.channel.filter((v) => v.toLowerCase().includes(q));
  });
  function pickCompany(v) {
    addCompany(v);
    companyQuery = '';
  }
  function pickChannel(v) {
    form.channel = v;
    showChannelList = false;
  }

  // 城市 / 场馆输入建议：子串匹配历史值，点击即补充
  // （iOS Safari 对原生 datalist 支持差，基本不弹建议，故与渠道一致用自定义下拉）
  let showCityList = $state(false);
  let showAddrList = $state(false);
  const filteredCities = $derived.by(() => {
    const q = form.city.trim().toLowerCase();
    if (!q) return ac.city.slice(0, 30);
    return ac.city.filter((v) => v.toLowerCase().includes(q)).slice(0, 30);
  });
  const filteredAddresses = $derived.by(() => {
    const q = form.address.trim().toLowerCase();
    if (!q) return ac.address.slice(0, 30);
    return ac.address.filter((v) => v.toLowerCase().includes(q)).slice(0, 30);
  });
  function pickCity(v) {
    form.city = v;
    showCityList = false;
  }
  function pickAddress(v) {
    form.address = v; // 触发既有 effect 自动地理定位
    showAddrList = false;
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
      const s = await api.getSettings();
      settings = { ...settings, ...s };
      // 新建记录时，根据设置中的默认开始时间更新date_local
      if (!record && initialDate && form.date_local) {
        const time = s.default_start_time || '19:30';
        form.date_local = `${initialDate}T${time}`;
      }
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
    // 选择器已解析出缩略图 key：带上它，详情页/列表不必加载 2000px 原图
    form.coverThumb = c.thumb || '';
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
      categoryName: form.categoryNames[0] || '',
      categoryNames: form.categoryNames.slice(),
      rating: Number(form.rating) || 0,
      duration: Number(form.duration) || 0,
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
      // 剧目字段由所关联的剧目档案推导，保证与档案一致；顺序跟随 drama_ids
      play: orderedChosenDramas.map((d) => d.name),
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

  // AI 填写：把模型返回的结构化字段批量映射到表单。与「从既往演出复制」同口径——
  // 只覆盖 AI 给出的字段，未提及的保持原样；演员因无法解析实体 ID，全部作为
  // 自由文本胶囊补充（entity_ids 清空）。
  // 收起 AI 面板并清空状态提示（保留 aiText，方便再次展开继续编辑）
  function closeAi() {
    aiOpen = false;
    aiErr = '';
    aiDone = '';
  }

  async function applyAi() {
    const text = aiText.trim();
    if (!text) {
      aiErr = '请先粘贴演出信息';
      return;
    }
    aiBusy = true;
    aiErr = '';
    aiDone = '';
    try {
      const data = await api.aiParse(text);
      const asStr = (v) => (typeof v === 'string' ? v.trim() : v == null ? '' : String(v).trim());
      const asArr = (v) => (Array.isArray(v) ? v.map((x) => asStr(x)).filter(Boolean) : []);
      if (data.name) form.name = asStr(data.name);
      if (data.city) form.city = asStr(data.city);
      if (data.address) form.address = asStr(data.address);
      if (data.channel) form.channel = asStr(data.channel);
      if (data.company) form.company = asStr(data.company);
      if (data.seat) form.seat = asStr(data.seat);
      if (data.friends) form.friends = asStr(data.friends);
      if (data.remark) form.remark = asStr(data.remark);
      const cats = asArr(data.categoryNames);
      if (cats.length) form.categoryNames = cats;
      if (typeof data.rating === 'number' && data.rating > 0) {
        form.rating = Math.max(0, Math.min(5, Math.round(data.rating)));
      }
      if (typeof data.duration === 'number' && data.duration > 0) {
        form.duration = Math.round(data.duration);
      }
      if (typeof data.price === 'number' && data.price > 0) form.price = data.price;
      if (typeof data.pay_price === 'number' && data.pay_price > 0) form.pay_price = data.pay_price;
      if (typeof data.other_cost === 'number' && data.other_cost > 0) form.other_cost = data.other_cost;
      // 剧目/折子：AI 只给名字，best-effort 在剧目树上按名称（含别名）匹配并
      // 勾选档案——提交时 play 由 drama_ids 推导，必须挂上档案才真正生效。
      // 匹配不上的名字静默跳过，由用户在剧目选择器里手动补。
      const stripMarks = (s) => s.replace(/[《》〈〉「」]/g, '').trim();
      const plays = asArr(data.play).map(stripMarks).filter(Boolean);
      for (const p of plays) {
        const d = dramaTree.find((dd) => {
          const dn = stripMarks(dd.name || '');
          return (
            (dn && (dn === p || (dn.length >= 2 && (dn.includes(p) || p.includes(dn))))) ||
            (dd.aliases || []).some((a) => stripMarks(a) === p)
          );
        });
        if (d && !form.drama_ids.includes(d.id)) form.drama_ids = [...form.drama_ids, d.id];
      }
      const zheziNames = asArr(data.zhezi_names).map(stripMarks).filter(Boolean);
      if (zheziNames.length) {
        // 已选剧目时只在这些剧目的折子里找，避免跨剧重名折子误挂
        const pool = form.drama_ids.length
          ? dramaTree.filter((d) => form.drama_ids.includes(d.id))
          : dramaTree;
        for (const d of pool) {
          for (const z of d.zhezis || []) {
            const zn = stripMarks(z.name || '');
            const hit =
              (zn && zheziNames.includes(zn)) ||
              (z.aliases || []).some((a) => zheziNames.includes(stripMarks(a)));
            if (hit && !form.zhezi_ids.includes(z.id)) {
              form.zhezi_ids = [...form.zhezi_ids, z.id];
            }
          }
        }
      }
      const guests = asArr(data.guest);
      if (guests.length) form.guest = guests.join(', ');
      if (typeof data.active_status === 'number') {
        form.active_status = Math.max(0, Math.min(3, Math.round(data.active_status)));
      }
      // 时间：优先 date_local "YYYY-MM-DDTHH:MM"
      if (typeof data.date_local === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(asStr(data.date_local))) {
        form.date_local = asStr(data.date_local);
      }
      if (typeof data.lat === 'number') form.lat = String(data.lat);
      if (typeof data.lng === 'number') form.lng = String(data.lng);
      // 演员：全部作为自由文本胶囊补充
      const names = asArr(data.artist_names);
      if (names.length) {
        form.artist_ids = [];
        freeNames = [
          ...freeNames,
          ...names.filter((n) => !artistList.some((a) => a.name === n) && !freeNames.includes(n))
        ];
      }
      aiErr = '';
      const src = data._source || {};
      if (src.url) {
        // 链接场景保留面板，让用户确认抓取来源；纯文本场景仍自动收起
        aiDone = src.title ? `已读取网页《${src.title}》并填充识别出的字段` : '已读取链接网页并填充识别出的字段';
      } else {
        aiOpen = false; // 填充完成后收起面板
      }
    } catch (e) {
      aiErr = e.message || 'AI 解析失败';
    } finally {
      aiBusy = false;
    }
  }

  // 币种：下拉选择，避免自由文本输入币种缩写（如 cny/CNY/Cny）导致的数据不一致。
  const commonCurrencies = ['CNY', 'USD', 'HKD', 'TWD', 'JPY', 'EUR', 'GBP', 'KRW', 'AUD', 'SGD'];
  function currencyOptions(v) {
    const out = [];
    if (v) out.push(v);
    for (const c of commonCurrencies) if (c !== v) out.push(c);
    return out;
  }


  // ---------- 从既往演出复制（仅新建模式显示） ----------
  // 搜索历史演出 → 勾选要复制的字段 → 应用到当前表单。日期/封面/状态默认
  // 不勾：新演出通常有自己的时间与海报。应用演员时，未建档的名字进自由胶囊。
  const COPY_FIELD_DEFS = [
    { key: 'name', label: '演出名称' },
    { key: 'categoryNames', label: '剧种分类' },
    { key: 'city', label: '城市' },
    { key: 'address', label: '场馆地址' },
    { key: 'coord', label: '场馆坐标' },
    { key: 'company', label: '剧团' },
    { key: 'channel', label: '渠道' },
    { key: 'artists', label: '演员' },
    { key: 'dramas', label: '剧目 / 折子' },
    { key: 'price', label: '票价' },
    { key: 'pay_price', label: '实付' },
    { key: 'other_cost', label: '其他花费' },
    { key: 'seat', label: '座位' },
    { key: 'duration', label: '时长' },
    { key: 'friends', label: '同行' },
    { key: 'remark', label: '备注' },
    { key: 'rating', label: '评分' },
    { key: 'date', label: '演出时间' },
    { key: 'cover', label: '封面' },
    { key: 'active_status', label: '演出状态' }
  ];
  // 座位/同行与时间/封面/状态/评分同属「每场大概率不同」的个人信息，默认不勾。
  const COPY_DEFAULT_OFF = new Set(['date', 'cover', 'active_status', 'rating', 'seat', 'duration', 'friends']);

  const COPY_SEARCH_PAGE = 20; // 每页搜索结果数
  let copySearch = $state('');
  let copySearching = $state(false);
  let copyResults = $state([]);
  let copyTotal = $state(0);
  let copyLimit = $state(COPY_SEARCH_PAGE);
  let copySource = $state(null);
  let copyApplied = $state(false);
  let copyFields = $state(
    Object.fromEntries(COPY_FIELD_DEFS.map((f) => [f.key, !COPY_DEFAULT_OFF.has(f.key)]))
  );
  let copySearchTimer = null;

  function defaultCopyFields() {
    return Object.fromEntries(COPY_FIELD_DEFS.map((f) => [f.key, !COPY_DEFAULT_OFF.has(f.key)]));
  }

  // 源记录对应字段的取值预览；空值返回 ''，对应勾选项整行隐藏。
  function copyFieldPreview(key, src) {
    const money = (v, c) => (v ? `${v} ${c || 'CNY'}` : '');
    switch (key) {
      case 'name': return src.name || '';
      case 'categoryNames':
        return (src.categoryNames && src.categoryNames.length
          ? src.categoryNames
          : src.categoryName
            ? [src.categoryName]
            : []
        ).join('、');
      case 'city': return src.city || '';
      case 'address': return src.address || '';
      case 'coord':
        return src.coordinate ? `${src.coordinate.latitude}, ${src.coordinate.longitude}` : '';
      case 'company': return src.company || '';
      case 'channel': return src.channel || '';
      case 'artists': return (src.artist_names || []).join('、');
      case 'dramas': return (src.play || []).filter(Boolean).join('、');
      case 'price': return money(src.price, src.price_currency);
      case 'pay_price': return money(src.pay_price, src.pay_price_currency);
      case 'other_cost': return money(src.other_cost, src.other_cost_currency);
      case 'seat': return src.seat || '';
      case 'duration': return src.duration ? `${src.duration} 分钟` : '';
      case 'friends': return src.friends || '';
      case 'remark': return src.remark || '';
      case 'rating': return src.rating ? `${src.rating} 分` : '';
      case 'date':
        return src.dateText
          ? src.dateText.slice(0, 16)
          : src.date
            ? fmtDateLocal(src.date).replace('T', ' ')
            : '';
      case 'cover': return src.coverFile ? '有封面' : '';
      case 'active_status': return src.active_status ? STATUS_LABELS[src.active_status] || '' : '';
      default: return '';
    }
  }

  const truncPreview = (s, n = 22) => (s.length > n ? s.slice(0, n) + '…' : s);

  // 只展示源记录里有值的字段，每项带取值预览，方便决定是否勾选。
  let copyFieldRows = $derived(
    copySource
      ? COPY_FIELD_DEFS.map((f) => ({ ...f, preview: copyFieldPreview(f.key, copySource) }))
          .filter((r) => r.preview)
      : []
  );

  function onCopySearchInput() {
    clearTimeout(copySearchTimer);
    copyLimit = COPY_SEARCH_PAGE; // 新关键词从头翻页
    copySearchTimer = setTimeout(doCopySearch, 260);
  }

  async function doCopySearch() {
    const q = copySearch.trim();
    if (!q) {
      copyResults = [];
      copyTotal = 0;
      return;
    }
    copySearching = true;
    try {
      const res = await api.listRecords({ q, limit: copyLimit });
      copyResults = res.records || [];
      copyTotal = res.total || 0;
    } catch (e) {
      copyResults = [];
      copyTotal = 0;
    } finally {
      copySearching = false;
    }
  }

  async function loadMoreCopyResults() {
    copyLimit += COPY_SEARCH_PAGE;
    await doCopySearch();
  }

  function pickCopySource(r) {
    copySource = r;
    copyApplied = false;
    copyResults = [];
    copyTotal = 0;
    copyLimit = COPY_SEARCH_PAGE;
    copySearch = '';
    copyFields = defaultCopyFields();
  }

  function resetCopySource() {
    copySource = null;
    copyApplied = false;
    copySearch = '';
    copyResults = [];
    copyTotal = 0;
    copyLimit = COPY_SEARCH_PAGE;
  }

  // 下拉候选里显示前 3 位演员，超出显示总人数
  function artistPreview(r) {
    const names = r.artist_names || [];
    if (!names.length) return '';
    const head = names.slice(0, 3).join('、');
    return names.length > 3 ? `${head} 等${names.length}人` : head;
  }

  function fmtDateLocal(ts) {
    if (!ts) return '';
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function applyCopy() {
    const src = copySource;
    if (!src) return;
    const on = (k) => !!copyFields[k];

    if (on('name')) form.name = src.name || '';
    if (on('categoryNames')) {
      form.categoryNames = (src.categoryNames && src.categoryNames.length
        ? src.categoryNames
        : src.categoryName
          ? [src.categoryName]
          : []
      ).slice();
    }
    if (on('city')) form.city = src.city || '';
    if (on('address')) form.address = src.address || '';
    if (on('coord')) {
      form.lat = src.coordinate ? (src.coordinate.latitude ?? '') : '';
      form.lng = src.coordinate ? (src.coordinate.longitude ?? '') : '';
    }
    // 地址变了而坐标没复制：清掉"已解析"标记让自动定位接管；
    // 地址和坐标都复制了：标记已解析，避免地理编码把复制的坐标覆盖掉。
    if (on('address')) {
      lastGeocoded = on('coord') && src.coordinate ? form.address : '';
    }
    if (on('company')) form.company = src.company || '';
    if (on('channel')) form.channel = src.channel || '';
    if (on('artists')) {
      form.artist_ids = (src.artist_ids || []).slice();
      const names = src.artist_names || [];
      freeNames = names.filter(
        (n) => !artistList.some((a) => a.name === n) && !freeNames.includes(n)
      );
    }
    if (on('dramas')) {
      form.drama_ids = (src.drama_ids || []).slice();
      form.zhezi_ids = (src.zhezi_ids || []).slice();
    }
    if (on('price')) {
      form.price = src.price || 0;
      form.price_currency = src.price_currency || 'CNY';
    }
    if (on('pay_price')) {
      form.pay_price = src.pay_price || 0;
      form.pay_price_currency = src.pay_price_currency || 'CNY';
    }
    if (on('other_cost')) {
      form.other_cost = src.other_cost || 0;
      form.other_cost_currency = src.other_cost_currency || 'CNY';
    }
    if (on('seat')) form.seat = src.seat || '';
    if (on('duration')) form.duration = src.duration || 0;
    if (on('friends')) form.friends = src.friends || '';
    if (on('remark')) form.remark = src.remark || '';
    if (on('rating')) form.rating = src.rating || 0;
    if (on('date')) form.date_local = fmtDateLocal(src.date);
    if (on('cover')) {
      form.coverFile = src.coverFile || '';
      form.coverThumb = src.coverThumb || '';
    }
    if (on('active_status')) form.active_status = src.active_status || 0;
    copyApplied = true;
  }

  onDestroy(() => clearTimeout(copySearchTimer));

  // 封面灯箱：点击预览放大查看原图
  let lightbox = $state(false);
  let lightboxSrc = $state('');
  function openLightbox() {
    const src = form.coverFile ? coverUrl(form.coverFile) : '';
    if (src) { lightboxSrc = src; lightbox = true; }
  }
  function closeLightbox() { lightbox = false; lightboxSrc = ''; }
  function onFormKeydown(e) {
    if (e.key === 'Escape') closeLightbox();
  }
</script>
<svelte:window onkeydown={onFormKeydown} />

<form class="form" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} onkeydown={(e) => { if (e.key === 'Enter' && e.target.tagName === 'INPUT') e.preventDefault(); }}>
<div class="two-col">
  <div class="col-left">
  {#if !record}
  <!-- ============ AI 填写 ============ -->
  {#if aiEnabled}
  <div class="card section ai-card">
    <div class="ai-head">
      <h3>AI 填写</h3>
      {#if !aiOpen}
        <button type="button" class="btn sm" onclick={() => (aiOpen = true)}>粘贴信息并解析</button>
      {/if}
    </div>
    {#if aiOpen}
      <p class="ai-hint muted">把购票短信 / 宣传文案 / 观演记录等文本粘贴到下方，也可以直接粘贴演出推文链接（微信公众号文章等），AI 会先抓取网页正文，再从内容中提取时间、演员、剧目、折子等字段批量填入（只覆盖识别出的字段，未提及的保持不变）。</p>
      <textarea
        class="input ai-text"
        rows="5"
        bind:value={aiText}
        placeholder="支持购票短信、演出推文链接（如 https://mp.weixin.qq.com/s/…）或宣传文案，例如：大麦网 您已购票 昆剧《牡丹亭》 2026-05-01 19:30 上海大剧院 票价 580 实付 580 主演 张军、沈昳丽 江苏昆山当代昆剧院…"
        spellcheck="false"
      ></textarea>
      <div class="ai-actions">
        <button type="button" class="btn" disabled={aiBusy} onclick={applyAi}>
          {aiBusy ? '解析中…' : '用 AI 解析并填充'}
        </button>
        <button type="button" class="btn ghost" disabled={aiBusy} onclick={closeAi}>收起</button>
      </div>
      {#if aiErr}<div class="banner error">⚠ {aiErr}</div>{/if}
      {#if aiDone}<div class="banner success">✓ {aiDone}</div>{/if}
    {/if}
  </div>
  {/if}
  <!-- ============ 从既往演出复制 ============ -->
  <div class="card section copy-card">
    <h3>从既往演出复制</h3>
    {#if !copySource}
      <input
        class="input"
        spellcheck="false"
        bind:value={copySearch}
        oninput={onCopySearchInput}
        placeholder="搜索演出名称、演员、城市、剧团…"
      />
      <p class="copy-hint muted">支持空格分隔多关键词，如「牡丹亭 上海」（每个词命中任意字段即可，需同时满足）；每次显示 20 条，超出可点「显示更多」。勾选项只列出这条演出有值的字段。</p>
      {#if copySearching && copyResults.length === 0}
        <div class="copy-hint muted">搜索中…</div>
      {:else if copySearch.trim() && !copySearching && copyResults.length === 0}
        <div class="copy-hint muted">没有匹配的演出</div>
      {:else if copyResults.length}
        <div class="copy-results">
          {#each copyResults as r (r.id)}
            <button type="button" class="copy-item" onclick={() => pickCopySource(r)}>
              <span class="copy-name">{r.name}</span>
              <span class="copy-meta">
                {r.dateText ? r.dateText.slice(0, 10) : '无日期'}{r.city ? ' · ' + r.city : ''}{r.address ? ' · ' + r.address : ''}{artistPreview(r) ? ' · ' + artistPreview(r) : ''}
              </span>
            </button>
          {/each}
        </div>
        {#if copyResults.length < copyTotal}
          <button type="button" class="btn ghost copy-more" disabled={copySearching} onclick={loadMoreCopyResults}>
            {copySearching ? '加载中…' : `显示更多（已显示 ${copyResults.length} / 共 ${copyTotal} 场）`}
          </button>
        {/if}
      {/if}
    {:else}
      <div class="copy-source">
        <div class="copy-src-info">
          <span class="copy-name">{copySource.name}</span>
          <span class="copy-meta">
            {copySource.dateText ? copySource.dateText.slice(0, 10) : '无日期'}{copySource.city ? ' · ' + copySource.city : ''}
          </span>
        </div>
        <button type="button" class="btn ghost" onclick={resetCopySource}>重选</button>
      </div>
      <div class="copy-fields">
        {#each copyFieldRows as f (f.key)}
          <label class="copy-field" title={f.preview}>
            <input type="checkbox" bind:checked={copyFields[f.key]} />
            <span class="copy-field-body">
              <span class="copy-field-label">{f.label}</span>
              <span class="copy-field-val">
                {#if f.key === 'cover'}
                  <img class="copy-cover-thumb" src={coverUrl(copySource.coverThumb || copySource.coverFile)} alt="" />
                {/if}
                {truncPreview(f.preview)}
              </span>
            </span>
          </label>
        {/each}
        {#if copyFieldRows.length === 0}
          <div class="copy-hint muted">这条演出没有可复制的内容</div>
        {/if}
      </div>
      <div class="copy-actions">
        <button type="button" class="btn" onclick={applyCopy}>复制所选字段</button>
        {#if copyApplied}<span class="copy-ok">已复制 ✓ 可继续修改</span>{/if}
      </div>
    {/if}
  </div>
  {/if}
  <!-- ============ 基本信息 ============ -->
  <div class="card section">
    <h3>基本信息</h3>
    <div class="row two-flex">
      <div>
        <label>名称 <span class="req">*</span></label>
        <input class="input" spellcheck="false" bind:value={form.name} placeholder="演出名称" />
      </div>
      <div>
        <label>剧种（可多个）</label>
        <CategoryTags bind:values={form.categoryNames} {categories} placeholder="如：昆剧，回车添加" />
      </div>
    </div>
    <div class="row two-flex-reverse">
      <div>
        <label>城市</label>
        <div class="combo">
          <input
            class="input"
            spellcheck="false"
            bind:value={form.city}
            placeholder="如：上海"
            onfocus={() => (showCityList = true)}
            onblur={() => setTimeout(() => (showCityList = false), 120)}
          />
          {#if showCityList && filteredCities.length}
            <div class="combo-list">
              {#each filteredCities as v (v)}
                <button type="button" class="combo-item" onmousedown={(e) => e.preventDefault()} onclick={() => pickCity(v)}>{v}</button>
              {/each}
            </div>
          {/if}
        </div>
      </div>
      <div>
        <label>场馆 / 地址</label>
        <div class="addr-row">
          <div class="combo addr-combo">
            <input
              class="input"
              spellcheck="false"
              bind:value={form.address}
              placeholder="如：上海大剧院"
              onfocus={() => (showAddrList = true)}
              onblur={() => setTimeout(() => (showAddrList = false), 120)}
            />
            {#if showAddrList && filteredAddresses.length}
              <div class="combo-list">
                {#each filteredAddresses as v (v)}
                  <button type="button" class="combo-item" onmousedown={(e) => e.preventDefault()} onclick={() => pickAddress(v)}>{v}</button>
                {/each}
              </div>
            {/if}
          </div>
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
                <input class="input" type="number" inputmode="decimal" step="0.000001" bind:value={form.lat} placeholder="纬度 31.230416" oninput={() => { manualOverride = true; geoStatus = 'ok'; }} />
                <input class="input" type="number" inputmode="decimal" step="0.000001" bind:value={form.lng} placeholder="经度 121.473700" oninput={() => { manualOverride = true; geoStatus = 'ok'; }} />
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
    <div class="row four-equal">
      <div>
        <label>演出时间</label>
        <input class="input" type="datetime-local" bind:value={form.date_local} autocomplete="off" />
      </div>
      <div>
        <label>状态</label>
        <select class="input" bind:value={form.active_status}>
          <option value={0}>正常</option>
          <option value={2}>已取消</option>
          <option value={1}>想看</option>
          <option value={3}>未赴约</option>
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
      <div>
        <label>时长</label>
        <div class="money"><input class="input" type="number" inputmode="numeric" min="0" step="1" bind:value={form.duration} placeholder="分钟" /><span class="unit">分钟</span></div>
      </div>
    </div>
    <div class="row two-equal">
      <div>
        <label>座位</label>
        <input class="input" spellcheck="false" bind:value={form.seat} list="seat-list" placeholder="如：3排15座" />
        <datalist id="seat-list">
          {#each ac.seat as v}<option value={v} />{/each}
        </datalist>
      </div>
      {#if settings.show_friends}
      <div>
        <label>同行</label>
        <input class="input" spellcheck="false" bind:value={form.friends} list="friends-list" placeholder="同行人，多个用逗号分隔" />
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
    <div class="row" style="grid-template-columns: 2fr 1fr 1fr 1fr;">
      <div>
        <label>购买渠道</label>
        <div class="combo">
          <input
            class="input"
            spellcheck="false"
            bind:value={form.channel}
            placeholder="如：大麦"
            onfocus={() => (showChannelList = true)}
            onblur={() => setTimeout(() => (showChannelList = false), 120)}
          />
          {#if showChannelList && filteredChannels.length}
            <div class="combo-list">
              {#each filteredChannels as v (v)}
                <button type="button" class="combo-item" onmousedown={(e) => e.preventDefault()} onclick={() => pickChannel(v)}>{v}</button>
              {/each}
            </div>
          {/if}
        </div>
      </div>
      <div>
        <label>票价</label>
        <div class="money"><input class="input" type="number" inputmode="decimal" step="0.01" min="0" bind:value={form.price} />{#if settings.multi_currency}<select class="input cur" bind:value={form.price_currency}>{#each currencyOptions(form.price_currency) as c}<option value={c}>{c}</option>{/each}</select>{/if}</div>
      </div>
      {#if settings.show_pay_price}
        <div>
          <label>实付</label>
          <div class="money"><input class="input" type="number" inputmode="decimal" step="0.01" min="0" bind:value={form.pay_price} />{#if settings.multi_currency}<select class="input cur" bind:value={form.pay_price_currency}>{#each currencyOptions(form.pay_price_currency) as c}<option value={c}>{c}</option>{/each}</select>{/if}</div>
        </div>
      {/if}
      {#if settings.show_other_cost}
        <div>
          <label>其他花费</label>
          <div class="money"><input class="input" type="number" inputmode="decimal" step="0.01" min="0" bind:value={form.other_cost} />{#if settings.multi_currency}<select class="input cur" bind:value={form.other_cost_currency}>{#each currencyOptions(form.other_cost_currency) as c}<option value={c}>{c}</option>{/each}</select>{/if}</div>
        </div>
      {/if}
    </div>
  </div>

  <!-- ============ 阵容 ============ -->
  <div class="card section">
    <h3>阵容</h3>
    <label>演员 <span class="hint">回车生成胶囊；逗号分隔可一次添加多个；与档案同名自动关联</span></label>
    <div class="combo">
      <div class="tagbox" onclick={(e) => e.currentTarget.querySelector('input')?.focus()}>
        {#each artistItems as item, i (item.key)}
          <span
                class="capsule"
                class:linked={item.kind === 'linked'}
                class:free={item.kind === 'free'}
                class:cap-dragging={artistDragIdx === i}
                class:drop-before={artistOverIdx === i && artistDragIdx !== i && artistOverBefore}
                class:drop-after={artistOverIdx === i && artistDragIdx !== i && !artistOverBefore}
                draggable="true"
                title="拖动调整顺序"
                ondragstart={(e) => { artistDragIdx = i; e.dataTransfer.effectAllowed = 'move'; }}
                ondragover={(e) => onArtistDragOver(e, i)}
                ondragleave={() => { if (artistOverIdx === i) artistOverIdx = -1; }}
                ondrop={(e) => { e.preventDefault(); onArtistDropAt(i, artistOverBefore); }}
                ondragend={() => { artistDragIdx = -1; artistOverIdx = -1; }}
          >
            <span class="cap-grip" aria-hidden="true">⠿</span>
            <span class="cap-text">{item.label}</span>
            <button type="button" class="cap-x" onclick={item.kind === 'linked' ? () => removeArtist(item.id) : () => removeFreeName(item.label)} title={item.kind === 'linked' ? '移除该演员' : '移除'} aria-label={`移除 ${item.label}`}>✕</button>
          </span>
        {/each}
        <input
          spellcheck="false"
          placeholder={chosenArtists.length || freeNames.length ? '' : '输入演员姓名，回车确认…'}
          bind:value={artistQuery}
          onfocus={() => (showArtistList = true)}
          onblur={onArtistBlur}
          onkeydown={onArtistKeydown}
          oninput={onArtistInput}
          oncompositionstart={() => (artistComposing = true)}
          oncompositionend={() => (artistComposing = false)}
        />
      </div>
      {#if showArtistList && (filteredArtists.length || artistQuery.trim())}
        <div class="combo-list">
          {#each filteredArtists as a (a.id)}
            <button type="button" class="combo-item" onmousedown={(e) => e.preventDefault()} onclick={() => addArtist(a.id)}>{a.name}</button>
          {/each}
          {#if artistQuery.trim() && !artistList.some((a) => a.name === artistQuery.trim())}
            <button type="button" class="combo-item create" disabled={creatingArtist} onmousedown={(e) => e.preventDefault()} onclick={() => createNewArtist(artistQuery)}>
              {creatingArtist ? '创建中…' : `＋ 新建演员档案「${artistQuery.trim()}」`}
            </button>
          {:else if !filteredArtists.length}
            <div class="combo-empty">无匹配演员</div>
          {/if}
        </div>
      {/if}
    </div>
    <label>剧团 <span class="hint">已有团体失焦自动添加；新名称回车生成胶囊，逗号分隔可一次添加多个</span></label>
    <div class="combo">
      <div class="tagbox" onclick={(e) => e.currentTarget.querySelector('input')?.focus()}>
        {#each companyTags as t, ti (t)}
          <span
            class="capsule free"
            class:cap-dragging={companyDragIdx === ti}
            class:drop-before={companyOverIdx === ti && companyDragIdx !== ti && companyOverBefore}
            class:drop-after={companyOverIdx === ti && companyDragIdx !== ti && !companyOverBefore}
            draggable="true"
            title="拖动调整顺序"
            ondragstart={(e) => { companyDragIdx = ti; e.dataTransfer.effectAllowed = 'move'; }}
            ondragover={(e) => onCompanyDragOver(e, ti)}
            ondragleave={() => { if (companyOverIdx === ti) companyOverIdx = -1; }}
            ondrop={(e) => { e.preventDefault(); onCompanyDropAt(ti, companyOverBefore); }}
            ondragend={() => { companyDragIdx = -1; companyOverIdx = -1; }}
          >
            <span class="cap-grip" aria-hidden="true">⠿</span>
            <span class="cap-text">{t}</span>
            <button type="button" class="cap-x" onclick={() => removeCompany(t)} title="移除该团体" aria-label={`移除 ${t}`}>✕</button>
          </span>
        {/each}
        <input
          spellcheck="false"
          placeholder={companyTags.length ? '' : '如：上海昆剧团'}
          bind:value={companyQuery}
          onfocus={() => (showCompanyList = true)}
          onblur={onCompanyBlur}
          onkeydown={onCompanyKeydown}
          oninput={onCompanyInput}
        />
      </div>
      {#if showCompanyList && (filteredCompanies.length || companyQuery.trim())}
        <div class="combo-list">
          {#each filteredCompanies as v (v)}
            <button type="button" class="combo-item" onmousedown={(e) => e.preventDefault()} onclick={() => pickCompany(v)}>{v}</button>
          {/each}
          {#if companyQuery.trim() && !filteredCompanies.length}
            <div class="combo-empty">无匹配团体，回车添加新团体</div>
          {/if}
        </div>
      {/if}
    </div>
    <label>剧目</label>
    <div class="ply">
      {#if chosenDramas.length === 0}
        <div class="ply-empty muted tiny">尚未关联剧目。从下方选择或新建一个剧目。</div>
      {/if}
      {#each orderedChosenDramas as d, di (d.id)}
        <div
          class="ply-item"
          draggable={editingDramaId !== d.id ? 'true' : 'false'}
          class:dragging={dramaDragIdx === di}
          class:drop-before={dramaOverIdx === di && dramaDragIdx !== di && dramaOverBefore}
          class:drop-after={dramaOverIdx === di && dramaDragIdx !== di && !dramaOverBefore}
          ondragstart={(e) => { dramaDragIdx = di; e.dataTransfer.effectAllowed = 'move'; }}
          ondragover={(e) => onDramaDragOver(e, di)}
          ondragleave={() => { if (dramaOverIdx === di) dramaOverIdx = -1; }}
          ondrop={(e) => { e.preventDefault(); onDramaDropAt(di); }}
          ondragend={() => { dramaDragIdx = -1; dramaOverIdx = -1; }}
        >
          {#if editingDramaId === d.id}
            <div class="ply-edit">
              <input class="input" spellcheck="false" placeholder="剧目名称" bind:value={dramaEdit.name} />
              <div class="ply-edit-actions">
                <button type="button" class="btn primary sm" onclick={saveDramaEdit} disabled={savingDramaEdit || !dramaEdit.name.trim()}>
                  {savingDramaEdit ? '保存中…' : '保存'}
                </button>
                <button type="button" class="btn ghost sm" onclick={cancelDramaEdit}>取消</button>
              </div>
            </div>
          {:else}
          <div class="ply-head">
            <span class="ply-name" title="拖动调整顺序">
              <span class="cap-grip" aria-hidden="true">⠿</span>
              {d.name}{#if d.categoryNames?.length}<em class="ply-cat">{d.categoryNames.join(' / ')}</em>{/if}
            </span>
            <span class="ply-ops">
              <button type="button" class="ply-edit-btn" onclick={() => startDramaEdit(d)} title="编辑该剧目名称">✎</button>
              <button type="button" class="ply-x" onclick={() => removeDrama(d.id)} title="移除该剧目">✕</button>
            </span>
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
            <div class="small muted">该剧目暂无折子</div>
          {/if}
          <!-- 内联新增折子：无需跳到剧目详情页 -->
          {#if addZheziFor === d.id}
            <div class="zhezi-add">
              <input spellcheck="false" bind:this={zheziInput} class="input" placeholder="新折子名称" bind:value={newZheziName} onkeydown={(e) => e.key === 'Enter' && createZheziFor(d.id)} />
              <button type="button" class="btn primary sm" onclick={() => createZheziFor(d.id)} disabled={savingZhezi || !newZheziName.trim()}>{savingZhezi ? '添加中…' : '添加'}</button>
              <button type="button" class="btn ghost sm" onclick={() => { addZheziFor = ''; newZheziName = ''; }}>取消</button>
            </div>
          {:else}
            <button type="button" class="zhezi-add-btn" onclick={() => { addZheziFor = d.id; newZheziName = ''; tick().then(() => zheziInput?.focus()); }}>＋ 添加折子</button>
          {/if}
          {/if}
        </div>
      {/each}
    </div>
    <div class="ply-add">
      <div class="combo">
        <input
          class="input"
          spellcheck="false"
          placeholder="🔍 搜索并关联已有剧目…"
          bind:value={dramaQuery}
          onfocus={() => (showDramaList = true)}
          onblur={() => setTimeout(() => (showDramaList = false), 120)}
          oncompositionstart={() => (dramaComposing = true)}
          oncompositionend={() => (dramaComposing = false)}
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
              <button type="button" class="combo-item" onmousedown={(e) => e.preventDefault()} onclick={() => addDrama(d.id)}>{d.name}{d.categoryNames?.length ? `（${d.categoryNames.join('/')}）` : ''}</button>
            {:else}
              <div class="combo-empty">无匹配剧目，可改用右侧新建</div>
            {/each}
          </div>
        {/if}
      </div>
      <details class="ply-new">
        <summary class="small">＋ 新建剧目</summary>
        <div class="ply-new-body">
          <div class="row">
            <input class="input" spellcheck="false" placeholder="剧目，如：牡丹亭" bind:value={newDrama.name} onkeydown={(e) => e.key === 'Enter' && createNewDrama()} />
            <button type="button" class="btn sm" onclick={createNewDrama} disabled={creatingDrama || !newDrama.name.trim()}>{creatingDrama ? '创建中…' : '创建并关联'}</button>
          </div>
        </div>
      </details>
    </div>
  </div>

  </div><!-- .col-left -->
  <div class="col-right">
  <!-- ============ 封面 ============ -->
  <div class="card section">
    <h3>封面</h3>
    <div class="cover-layout">
      {#if form.coverFile}
        <img class="preview zoomable" src={coverUrl(form.coverThumb || form.coverFile)} alt="封面预览" onclick={openLightbox} role="button" tabindex={0} aria-label="点击放大查看封面" onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openLightbox(); } }} />
      {/if}
      <div class="cover-main">
        <div class="upload-row">
          <button type="button" class="btn sm" onclick={triggerUpload} disabled={uploading}>
            {uploading ? '上传中…' : '⇪ 上传图片'}
          </button>
          <input type="file" accept="image/*" onchange={handleUpload} disabled={uploading} hidden bind:this={fileInput} />
          <button type="button" class="btn sm" onclick={() => (pickerOpen = true)}>▦ 从已有演出引用</button>
        </div>
      </div>
    </div>
  </div>

  <!-- ============ 备注（sticky 浮动） ============ -->
  <div class="remark-sticky">
    <div class="card section remark-card">
      <h3>备注</h3>
      <textarea class="input" spellcheck="false" rows="8" bind:value={form.remark} placeholder="剧评、观感、备忘…"></textarea>
    </div>
  </div>
  </div><!-- .col-right -->
</div><!-- .two-col -->

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  <div class="actions">
    <button type="submit" class="btn primary lg" disabled={saving}>{saving ? '保存中…' : '保存'}</button>
    <button type="button" class="btn lg" onclick={onCancel}>取消</button>
  </div>
</form>

<CoverPicker open={pickerOpen} onSelect={pickCover} onClose={() => (pickerOpen = false)} />

{#if lightbox && lightboxSrc}
  <button type="button" class="lightbox" onclick={closeLightbox} aria-label="关闭大图">
    <img src={lightboxSrc} alt="" />
  </button>
{/if}

<style>
  .form { display: flex; flex-direction: column; gap: 14px; max-width: 1200px; margin: 0 auto; }
  /* 桌面端双列：左列放表单主体，右列放封面+备注 */
  .two-col { display: flex; flex-direction: column; gap: 14px; }
  @media (min-width: 860px) {
    .two-col { display: grid; grid-template-columns: 1fr 400px; gap: 16px; }
    .col-left { min-width: 0; display: flex; flex-direction: column; gap: 14px; }
    .col-right { min-width: 300px; display: flex; flex-direction: column; gap: 14px; }
    .remark-sticky { flex: 1; display: flex; flex-direction: column; }
    .remark-card { flex: 1; display: flex; flex-direction: column; }
    .remark-card textarea { flex: 1; resize: none; min-height: 80px; }
  }
  .section { padding: 18px 20px; }
  .section h3 { margin: 0 0 6px; font-size: 15.5px; color: var(--text-2); }
  .req { color: var(--accent); }
  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; }

  .money { display: flex; gap: 6px; }
  .money .unit { align-self: center; color: var(--text-3); font-size: 13px; white-space: nowrap; }
  /* 货币框收窄并让文字居中：默认左对齐 + 34px 箭头预留位会让 CNY 右侧
     剩下一截空白。select 右侧仍留箭头位，只是收窄并居中文字。 */
  /* 货币框宽度自适应最宽选项（币种都是 3 字母代码，约 30px），文字之后
     只剩紧贴箭头的一点间隙。不依赖 select 的文字居中：Chromium 渲染
     <select> 闭合文本既不理 text-align 也不理 text-align-last。 */
  .money .cur { width: auto; max-width: none; min-width: 0; flex: 0 0 auto; }
  .money select.cur { padding-left: 9px; padding-right: 24px; }

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
  .addr-combo { flex: 1; min-width: 0; }
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
    transition: border-color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
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
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 6px 3px 6px;
    border-radius: 999px;
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
    max-width: 100%;
    min-width: 0;
    animation: fadeIn var(--t-fast) var(--ease);
    cursor: grab;
  }
  .capsule:active { cursor: grabbing; }
  .cap-grip { flex: 0 0 auto; color: currentColor; opacity: 0.45; font-size: 11px; line-height: 1; user-select: none; cursor: grab; }
  /* 超长名称：文本省略号收束，手柄与删除按钮保持可见，胶囊不再撑破容器 */
  .cap-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .capsule.cap-dragging { opacity: 0.35; }
  /* 拖拽插入指示：目标左/右侧显示竖线光标，表示松手后的插入位置 */
  .capsule.drop-before::before,
  .capsule.drop-after::after {
    content: '';
    position: absolute;
    top: -4px;
    bottom: -4px;
    width: 3px;
    border-radius: 2px;
    background: var(--accent);
    pointer-events: none;
  }
  .capsule.drop-before::before { left: -5px; }
  .capsule.drop-after::after { right: -5px; }
  .capsule.linked { background: var(--accent-soft); color: var(--accent); }
  .capsule.free { background: var(--surface-3); color: var(--text-2); border: 1px solid var(--border); }
  .cap-x {
    flex: 0 0 auto;
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
  .upload-row { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
  .preview {
    width: 180px;
    aspect-ratio: 3 / 4;
    object-fit: cover;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    flex-shrink: 0;
  }
  .preview.zoomable { cursor: zoom-in; transition: box-shadow var(--t-fast) var(--ease); }
  .preview.zoomable:hover { box-shadow: 0 0 0 2px var(--accent-soft); }

  .actions { display: flex; gap: 10px; margin-top: 4px; }

  /* 剧目 / 折子 picker */
  .ply { display: flex; flex-direction: column; gap: 10px; }
  .ply-empty { padding: 8px 2px; }
  .ply-item { position: relative; border: 1px solid var(--border); border-radius: var(--radius); padding: 12px 14px; background: var(--surface); cursor: grab; }
  .ply-item:active { cursor: grabbing; }
  .ply-item.dragging { opacity: 0.4; }
  /* 拖拽插入指示 */
  .ply-item.drop-before::before,
  .ply-item.drop-after::after {
    content: '';
    position: absolute;
    left: 8px;
    right: 8px;
    height: 3px;
    border-radius: 2px;
    background: var(--accent);
    pointer-events: none;
    z-index: 1;
  }
  .ply-item.drop-before::before { top: -6px; }
  .ply-item.drop-after::after { bottom: -6px; }
  .ply-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .ply-name { font-weight: 600; font-size: 14.5px; }
  .ply-ops { display: inline-flex; align-items: center; gap: 2px; flex: 0 0 auto; }
  .ply-edit-btn {
    border: none; background: none; color: var(--text-3); width: 24px; height: 24px;
    border-radius: 50%; cursor: pointer; font-size: 13px;
  }
  .ply-edit-btn:hover { background: var(--surface-3); color: var(--accent); }
  .ply-edit { display: flex; flex-direction: column; gap: 10px; }
  .ply-edit-actions { display: flex; gap: 8px; }
  .ply-cat { font-style: normal; font-size: 12px; color: var(--text-muted); background: var(--surface-3); border-radius: 999px; padding: 2px 9px; margin-left: 8px; }
  .ply-x { border: none; background: none; color: var(--text-3); width: 24px; height: 24px; border-radius: 50%; cursor: pointer; font-size: 12px; }
  .ply-x:hover { background: var(--danger-soft); color: var(--danger); }
  .ply-zhezis { margin-top: 10px; }
  .ply-zhezis .small { display: block; margin-bottom: 6px; }
  .zhezi-grid { display: flex; flex-wrap: wrap; gap: 8px; }
  .zhezi {
    display: inline-flex; align-items: center; gap: 6px; cursor: pointer;
    border: 1px solid var(--border); border-radius: 999px; padding: 5px 12px;
    font-size: 13px; color: var(--text-2); transition: background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
    user-select: none;
  }
  .zhezi:has(input:checked) { background: var(--accent-soft); border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .zhezi input { accent-color: var(--accent); }
  .zhezi-add {
    display: flex;
    gap: 6px;
    align-items: center;
    margin-top: 10px;
    flex-wrap: wrap;
  }
  .zhezi-add .input { flex: 1 1 160px; min-width: 140px; }
  .zhezi-add-btn {
    margin-top: 10px;
    border: 1px dashed var(--border-strong);
    background: none;
    color: var(--accent);
    border-radius: 999px;
    padding: 4px 12px;
    font-size: 12.5px;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease);
  }
  .zhezi-add-btn:hover { background: var(--accent-soft); border-color: var(--accent); }
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
  /* 收起时收缩到「＋ 新建剧目」文字宽度（此前固定占 1/3 宽，文字右侧
     全是空白边框）；展开后独占整行，新建表单有足够空间。 */
  .ply-new {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 14px;
    flex: 0 0 auto;
    align-self: stretch;
    display: flex;
    flex-direction: column;
    justify-content: center;
  }
  .ply-new[open] { flex: 1 1 100%; }
  .ply-new summary { cursor: pointer; color: var(--accent); }
  .ply-new-body { margin-top: 10px; display: flex; flex-direction: column; gap: 10px; }

  /* ---------- AI 填写 ---------- */
  .ai-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .ai-hint { margin: 0 0 10px; font-size: 12.5px; color: var(--text-3); }
  .ai-text {
    width: 100%;
    resize: vertical;
    font-size: 16px;
    line-height: 1.6;
    font-family: var(--font-sans, inherit);
    padding: 10px 12px;
    margin-bottom: 10px;
  }
  .ai-actions { display: flex; gap: 8px; align-items: center; }

  /* ---------- 从既往演出复制 ---------- */
  .copy-hint { padding: 6px 2px 0; font-size: 13px; }
  .copy-results {
    margin-top: 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius, 10px);
    overflow: hidden;
    max-height: 264px;
    overflow-y: auto;
  }
  .copy-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    text-align: left;
    padding: 8px 12px;
    background: var(--surface);
    border: none;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    font: inherit;
    color: inherit;
  }
  .copy-item:last-child { border-bottom: none; }
  .copy-more { margin-top: 8px; width: 100%; }
  .copy-item:hover { background: var(--accent-softer); }
  .copy-name { font-weight: 600; }
  .copy-meta { font-size: 12px; color: var(--text-3); }
  .copy-source { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .copy-src-info { display: flex; flex-direction: column; gap: 2px; }
  .copy-fields {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
    gap: 8px 14px;
    margin: 14px 0;
  }
  .copy-field { display: flex; align-items: flex-start; gap: 6px; font-size: 13px; cursor: pointer; min-width: 0; }
  .copy-field-body { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
  .copy-field-label { font-weight: 500; }
  .copy-field-val { color: var(--text-3); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 190px; display: flex; align-items: center; gap: 4px; }
  .copy-cover-thumb { width: 22px; height: 30px; object-fit: cover; border-radius: 3px; flex: none; }
  .copy-actions { display: flex; align-items: center; gap: 12px; }
  .copy-ok { color: var(--success); font-size: 13px; }

  /* 封面灯箱 */
  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 9999;
    border: none;
    padding: 12px;
    margin: 0;
    background: rgba(0, 0, 0, 0.86);
    cursor: zoom-out;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
  }
  .lightbox img {
    max-width: min(90vw, 700px);
    max-height: calc(100vh - 24px);
    width: auto;
    height: auto;
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
    object-fit: contain;
  }
</style>
