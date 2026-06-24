<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import TagInput from './TagInput.svelte';

  let { show = null, onsaved, oncancel } = $props();

  let form = $state({
    name: '', venue: '', date: '', duration: 120, status: 'normal',
    category_id: null, poster_url: '', setlist: '', cast: '',
    company: '', friends: '', rating: null, seat: '',
    notes: '', review: '', ticket_cost: null, other_cost: null
  });

  let categories = $state([]);
  let saving = $state(false);
  let error = $state('');
  let posterFile = $state(null);
  let posterPreview = $state('');
  let dragover = $state(false);

  let companyList = $state([]);
  let castList = $state([]);
  let friendsList = $state([]);
  let venueList = $state([]);

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
        duration: show.duration || 120, status: show.status || 'normal',
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

  async function handleSubmit(e) {
    e.preventDefault();
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
      onsaved?.();
    } catch (e) { error = e.message; }
    finally { saving = false; }
  }
</script>

<form onsubmit={handleSubmit} class="show-form">
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
      <div class="form-group form-group-short">
        <label for="duration">时长(分钟)</label>
        <input type="number" id="duration" bind:value={form.duration} min="0" />
      </div>
      <div class="form-group">
        <label>评分</label>
        <div class="star-rating">
          {#each [1,2,3,4,5] as val}
            <button type="button" class="star-btn" class:active={form.rating >= val} onclick={() => setRating(val)}>
              {form.rating >= val ? '★' : '☆'}
            </button>
          {/each}
          {#if form.rating}<span class="rating-text">{form.rating}/5</span>{/if}
        </div>
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label for="status">状态</label>
        <select id="status" bind:value={form.status}>
          <option value="normal">正常</option>
          <option value="cancelled">已取消</option>
          <option value="pending_tickets">待开票</option>
          <option value="no_show">未赴约</option>
        </select>
      </div>
      <div class="form-group">
        <label for="category">分类</label>
        <select id="category" bind:value={form.category_id}>
          <option value={null}>无分类</option>
          {#each categories as cat}<option value={cat.id}>{cat.name}</option>{/each}
        </select>
      </div>
    </div>
    <div class="form-group">
      <label for="notes">备注</label>
      <textarea id="notes" bind:value={form.notes} rows="2" placeholder="其他备注信息"></textarea>
    </div>
  </div>

  <div class="form-section">
    <h3>海报</h3>
    <div class="poster-upload" class:dragover ondrop={handleDrop} ondragover={handleDragOver} ondragleave={handleDragLeave}>
      {#if posterPreview}
        <img src={posterPreview} alt="海报预览" class="poster-preview" />
        <button type="button" class="btn-remove-poster" onclick={() => { posterFile = null; posterPreview = ''; form.poster_url = ''; }}>移除</button>
      {:else}
        <div class="poster-placeholder">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>
          <p>拖拽图片到此处，或</p>
          <label class="btn-select-poster">选择图片<input type="file" accept="image/*" onchange={handleFileSelect} hidden /></label>
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
        <label>剧团</label>
        <TagInput bind:value={form.company} placeholder="输入剧团名按回车" suggestions={companyList} />
      </div>
      <div class="form-group">
        <label>阵容</label>
        <TagInput bind:value={form.cast} placeholder="输入演员名按回车" suggestions={castList} />
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>同行好友</label>
        <TagInput bind:value={form.friends} placeholder="输入好友名按回车" suggestions={friendsList} />
      </div>
      <div class="form-group">
        <label for="seat">座位</label>
        <input type="text" id="seat" bind:value={form.seat} placeholder="如：3排15座" />
      </div>
    </div>
    <div class="form-group">
      <label>剧目</label>
      <TagInput bind:value={form.setlist} placeholder="输入剧目名按回车" suggestions={[]} />
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
    <button type="button" class="btn-cancel" onclick={() => oncancel?.()}>取消</button>
    <button type="submit" class="btn-submit" disabled={saving}>
      {saving ? '保存中...' : show ? '更新' : '添加'}
    </button>
  </div>
</form>

<style>
  .show-form {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 28px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }
  .error {
    background: var(--danger-bg);
    color: var(--danger-text);
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    margin-bottom: 24px;
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .error::before {
    content: '⚠';
  }
  .form-section {
    margin-bottom: 28px;
    padding-bottom: 28px;
    border-bottom: 1px solid var(--border);
  }
  .form-section:last-of-type {
    border-bottom: none;
    margin-bottom: 0;
    padding-bottom: 0;
  }
  .form-section h3 {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 18px;
    color: var(--text-primary);
    letter-spacing: -0.01em;
  }
  .form-row {
    display: flex;
    gap: 16px;
    margin-bottom: 14px;
  }
  .form-group {
    flex: 1;
    margin-bottom: 14px;
  }
  .form-row .form-group {
    margin-bottom: 0;
  }
  .form-group-short {
    flex: 0 0 130px;
  }
  label {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-muted);
    margin-bottom: 8px;
  }
  input, select, textarea {
    width: 100%;
  }
  input[type="number"]::-webkit-inner-spin-button,
  input[type="number"]::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
  input[type="number"] { -moz-appearance: textfield; }
  textarea { resize: vertical; }
  .star-rating { display: flex; align-items: center; gap: 2px; padding: 8px 0; }
  .star-btn { font-size: 24px; color: var(--border); cursor: pointer; padding: 0 2px; transition: color 0.15s, transform 0.15s; background: none; border: none; }
  .star-btn:hover { transform: scale(1.2); }
  .star-btn.active { color: var(--warning); }
  .rating-text { margin-left: 8px; font-size: 13px; color: var(--text-muted); }
  .poster-upload {
    border: 2px dashed var(--border);
    border-radius: var(--radius-md);
    padding: 32px;
    text-align: center;
    transition: all 0.2s ease;
    cursor: pointer;
  }
  .poster-upload:hover { border-color: var(--accent); background: var(--accent-bg); }
  .poster-upload.dragover { border-color: var(--accent); background: var(--accent-bg); }
  .poster-preview { max-width: 200px; max-height: 200px; border-radius: var(--radius-sm); object-fit: cover; }
  .btn-remove-poster {
    margin-top: 12px;
    padding: 6px 16px;
    background: var(--danger-bg);
    color: var(--danger-text);
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    transition: all 0.15s;
  }
  .btn-remove-poster:hover { background: var(--danger-bg-hover); }
  .poster-placeholder { display: flex; flex-direction: column; align-items: center; gap: 10px; }
  .poster-placeholder svg { color: var(--text-muted); opacity: 0.5; }
  .poster-placeholder p { color: var(--text-muted); margin: 0; font-size: 13px; }
  .btn-select-poster {
    display: inline-block;
    padding: 8px 20px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    transition: all 0.2s;
  }
  .btn-select-poster:hover { background: var(--accent-light); }
  .poster-hint { font-size: 12px; color: var(--text-muted); }
  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 12px;
    margin-top: 28px;
  }
  .btn-cancel {
    padding: 10px 24px;
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-weight: 500;
    font-size: 14px;
    transition: all 0.2s;
    border: 1px solid var(--border);
  }
  .btn-cancel:hover { background: var(--bg-surface-hover); }
  .btn-submit {
    padding: 10px 32px;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: #fff;
    font-weight: 500;
    font-size: 14px;
    transition: all 0.2s;
  }
  .btn-submit:hover:not(:disabled) { background: var(--accent-light); transform: translateY(-1px); }
  .btn-submit:disabled { opacity: 0.6; cursor: not-allowed; }
  @media (max-width: 768px) {
    .show-form { padding: 16px; }
    .form-row { flex-direction: column; gap: 0; }
    .form-row .form-group { margin-bottom: 14px; }
    .form-actions { flex-direction: column; gap: 8px; }
    .form-actions button { width: 100%; text-align: center; }
  }
</style>
