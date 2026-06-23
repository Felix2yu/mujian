<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '$lib/api';

  export let show = null;

  const dispatch = createEventDispatcher();

  let form = {
    name: '', venue: '', date: '', duration: 120, status: 'planned',
    category_id: null, poster_url: '', setlist: '', cast: '',
    company: '', friends: '', rating: null, seat: '',
    notes: '', review: '', ticket_cost: null, other_cost: null
  };

  let categories = [];
  let saving = false, error = '';
  let posterFile = null, posterPreview = '', dragover = false;

  let companyList = [], castList = [], friendsList = [], venueList = [];

  onMount(async () => {
    const [cats, comp, cast, fr, ven] = await Promise.all([
      api.listCategories(),
      api.getAutocomplete('company'),
      api.getAutocomplete('cast'),
      api.getAutocomplete('friends'),
      api.getAutocomplete('venue')
    ]);
    categories = cats || [];
    companyList = comp || [];
    castList = cast || [];
    friendsList = fr || [];
    venueList = ven || [];

    if (show) {
      form = {
        name: show.name || '', venue: show.venue || '',
        date: show.date ? formatDateTimeLocal(show.date) : '',
        duration: show.duration || 120, status: show.status || 'planned',
        category_id: show.category_id || null, poster_url: show.poster_url || '',
        setlist: show.setlist || '', cast: show.cast || '',
        company: show.company || '', friends: show.friends || '',
        rating: show.rating, seat: show.seat || '',
        notes: show.notes || '', review: show.review || '',
        ticket_cost: show.ticket_cost, other_cost: show.other_cost
      };
      if (show.poster_url) posterPreview = show.poster_url;
    } else {
      form.date = formatDateTimeLocal(new Date().toISOString());
    }
  });

  function formatDateTimeLocal(iso) {
    const d = new Date(iso);
    return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}-${String(d.getDate()).padStart(2,'0')}T${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}`;
  }

  function handleFileSelect(e) { if (e.target.files[0]) processFile(e.target.files[0]); }
  function handleDrop(e) { e.preventDefault(); dragover = false; const f = e.dataTransfer.files[0]; if (f && f.type.startsWith('image/')) processFile(f); }
  function handleDragOver(e) { e.preventDefault(); dragover = true; }
  function handleDragLeave() { dragover = false; }

  function processFile(file) {
    posterFile = file;
    const reader = new FileReader();
    reader.onload = (e) => { posterPreview = e.target.result; };
    reader.readAsDataURL(file);
  }

  function setRating(val) { form.rating = form.rating === val ? null : val; }

  async function handleSubmit() {
    if (!form.name.trim()) { error = '请输入演出名称'; return; }
    if (!form.date) { error = '请选择演出时间'; return; }
    saving = true; error = '';
    try {
      if (posterFile) {
        const res = await api.uploadFile(posterFile);
        form.poster_url = res.url;
      }
      if (show) { await api.updateShow(show.id, form); }
      else { await api.createShow(form); }
      dispatch('saved');
    } catch (e) { error = e.message; }
    finally { saving = false; }
  }
</script>

<form on:submit|preventDefault={handleSubmit} class="show-form">
  {#if error}<div class="error">{error}</div>{/if}

  <div class="form-section">
    <h3>基本信息</h3>
    <div class="form-row">
      <div class="form-group">
        <label for="name">演出名称 *</label>
        <input type="text" id="name" bind:value={form.name} placeholder="如：茶馆" required />
      </div>
      <div class="form-group">
        <label for="venue">场地</label>
        <input type="text" id="venue" bind:value={form.venue} list="venue-list" placeholder="如：国家大剧院" />
        <datalist id="venue-list">
          {#each venueList as v}<option value={v} />{/each}
        </datalist>
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label for="date">演出时间 *</label>
        <input type="datetime-local" id="date" bind:value={form.date} required />
      </div>
      <div class="form-group">
        <label for="duration">时长(分钟)</label>
        <input type="number" id="duration" bind:value={form.duration} min="0" />
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label for="status">状态</label>
        <select id="status" bind:value={form.status}>
          <option value="planned">计划中</option>
          <option value="watched">已观看</option>
          <option value="cancelled">已取消</option>
        </select>
      </div>
      <div class="form-group">
        <label for="category">分类</label>
        <select id="category" bind:value={form.category_id}>
          <option value={null}>无分类</option>
          {#each categories as cat}<option value={cat.id}>{cat.name}</option>{/each}
        </select>
      </div>
      <div class="form-group">
        <label>评分</label>
        <div class="star-rating">
          {#each [1,2,3,4,5] as val}
            <button type="button" class="star-btn" class:active={form.rating >= val} on:click={() => setRating(val)}>
              {form.rating >= val ? '★' : '☆'}
            </button>
          {/each}
          {#if form.rating}<span class="rating-text">{form.rating}/5</span>{/if}
        </div>
      </div>
    </div>
    <div class="form-group">
      <label for="notes">备注</label>
      <textarea id="notes" bind:value={form.notes} rows="2" placeholder="其他备注信息"></textarea>
    </div>
  </div>

  <div class="form-section">
    <h3>海报</h3>
    <div class="poster-upload" class:dragover on:drop={handleDrop} on:dragover={handleDragOver} on:dragleave={handleDragLeave}>
      {#if posterPreview}
        <img src={posterPreview} alt="海报预览" class="poster-preview" />
        <button type="button" class="btn-remove-poster" on:click={() => { posterFile = null; posterPreview = ''; form.poster_url = ''; }}>移除</button>
      {:else}
        <div class="poster-placeholder">
          <span class="poster-icon">🖼</span>
          <p>拖拽图片到此处，或</p>
          <label class="btn-select-poster">选择图片<input type="file" accept="image/*" on:change={handleFileSelect} hidden /></label>
          <span class="poster-hint">支持 JPG、PNG、WebP</span>
        </div>
      {/if}
    </div>
    <div class="form-group" style="margin-top:8px">
      <input type="url" bind:value={form.poster_url} placeholder="或输入海报URL" />
    </div>
  </div>

  <div class="form-section">
    <h3>详细信息</h3>
    <div class="form-row">
      <div class="form-group">
        <label for="company">剧团</label>
        <input type="text" id="company" bind:value={form.company} list="company-list" placeholder="如：北京人艺" />
        <datalist id="company-list">{#each companyList as c}<option value={c} />{/each}</datalist>
      </div>
      <div class="form-group">
        <label for="cast">阵容</label>
        <input type="text" id="cast" bind:value={form.cast} list="cast-list" placeholder="如：于是之, 郑榕" />
        <datalist id="cast-list">{#each castList as c}<option value={c} />{/each}</datalist>
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label for="friends">同行好友</label>
        <input type="text" id="friends" bind:value={form.friends} list="friends-list" placeholder="如：小明, 小红" />
        <datalist id="friends-list">{#each friendsList as f}<option value={f} />{/each}</datalist>
      </div>
      <div class="form-group">
        <label for="seat">座位</label>
        <input type="text" id="seat" bind:value={form.seat} placeholder="如：3排15座" />
      </div>
    </div>
    <div class="form-group">
      <label for="setlist">剧目</label>
      <textarea id="setlist" bind:value={form.setlist} rows="3" placeholder="每行一个剧目"></textarea>
    </div>
  </div>

  <div class="form-section">
    <h3>花费</h3>
    <div class="form-row">
      <div class="form-group">
        <label for="ticket_cost">门票费用</label>
        <input type="number" id="ticket_cost" bind:value={form.ticket_cost} step="0.01" min="0" placeholder="元" />
      </div>
      <div class="form-group">
        <label for="other_cost">其他花费</label>
        <input type="number" id="other_cost" bind:value={form.other_cost} step="0.01" min="0" placeholder="元" />
      </div>
    </div>
  </div>

  <div class="form-section">
    <h3>个人感受</h3>
    <div class="form-group">
      <label for="review">剧评</label>
      <textarea id="review" bind:value={form.review} rows="4" placeholder="写下你的感受..."></textarea>
    </div>
  </div>

  <div class="form-actions">
    <button type="button" class="btn-cancel" on:click={() => dispatch('cancel')}>取消</button>
    <button type="submit" class="btn-submit" disabled={saving}>
      {saving ? '保存中...' : show ? '更新' : '添加'}
    </button>
  </div>
</form>

<style>
  .show-form { background: #fff; border-radius: 12px; padding: 24px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
  .error { background: #fee; color: #c00; padding: 12px 16px; border-radius: 8px; margin-bottom: 20px; }
  .form-section { margin-bottom: 24px; padding-bottom: 24px; border-bottom: 1px solid #eee; }
  .form-section:last-of-type { border-bottom: none; }
  .form-section h3 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #333; }
  .form-row { display: flex; gap: 16px; margin-bottom: 12px; }
  .form-group { flex: 1; margin-bottom: 12px; }
  .form-row .form-group { margin-bottom: 0; }
  label { display: block; font-size: 13px; font-weight: 500; color: #666; margin-bottom: 6px; }
  input, select, textarea { width: 100%; }
  textarea { resize: vertical; }
  .star-rating { display: flex; align-items: center; gap: 2px; padding-top: 4px; }
  .star-btn { font-size: 24px; color: #ddd; cursor: pointer; padding: 0 2px; transition: color 0.15s, transform 0.15s; background: none; border: none; }
  .star-btn:hover { transform: scale(1.2); }
  .star-btn.active { color: #f39c12; }
  .rating-text { margin-left: 8px; font-size: 13px; color: #999; }
  .poster-upload { border: 2px dashed #ddd; border-radius: 8px; padding: 24px; text-align: center; transition: border-color 0.2s, background 0.2s; cursor: pointer; }
  .poster-upload.dragover { border-color: #4A90D9; background: #f0f7ff; }
  .poster-preview { max-width: 200px; max-height: 200px; border-radius: 8px; object-fit: cover; }
  .btn-remove-poster { margin-top: 8px; padding: 4px 12px; background: #fee; color: #c00; border-radius: 6px; font-size: 12px; }
  .poster-placeholder { display: flex; flex-direction: column; align-items: center; gap: 8px; }
  .poster-icon { font-size: 32px; }
  .poster-placeholder p { color: #666; margin: 0; font-size: 13px; }
  .btn-select-poster { display: inline-block; padding: 6px 16px; background: #4A90D9; color: #fff; border-radius: 6px; cursor: pointer; font-size: 13px; }
  .btn-select-poster:hover { background: #3a7bc8; }
  .poster-hint { font-size: 11px; color: #999; }
  .form-actions { display: flex; justify-content: flex-end; gap: 12px; margin-top: 24px; }
  .btn-cancel { padding: 10px 24px; border-radius: 8px; background: #f0f0f0; color: #666; font-weight: 500; transition: background 0.2s; }
  .btn-cancel:hover { background: #e0e0e0; }
  .btn-submit { padding: 10px 32px; border-radius: 8px; background: #4A90D9; color: #fff; font-weight: 500; transition: background 0.2s; }
  .btn-submit:hover:not(:disabled) { background: #3a7bc8; }
  .btn-submit:disabled { opacity: 0.6; cursor: not-allowed; }
  @media (max-width: 768px) {
    .show-form { padding: 16px; }
    .form-row { flex-direction: column; gap: 0; }
    .form-row .form-group { margin-bottom: 12px; }
    .form-actions { flex-direction: column; gap: 8px; }
    .form-actions button { width: 100%; text-align: center; }
  }
</style>
