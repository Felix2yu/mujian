<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { api } from '$lib/api';

  export let show = null;

  const dispatch = createEventDispatcher();

  let form = {
    name: '',
    venue: '',
    date: '',
    duration: 120,
    status: 'planned',
    category_id: null,
    poster_url: '',
    setlist: '',
    cast: '',
    company: '',
    friends: '',
    rating: null,
    seat: '',
    notes: '',
    review: '',
    ticket_cost: null,
    other_cost: null
  };

  let categories = [];
  let saving = false;
  let error = '';

  onMount(async () => {
    categories = await api.listCategories();
    if (show) {
      form = {
        name: show.name || '',
        venue: show.venue || '',
        date: show.date ? formatDateTimeLocal(show.date) : '',
        duration: show.duration || 120,
        status: show.status || 'planned',
        category_id: show.category_id || null,
        poster_url: show.poster_url || '',
        setlist: show.setlist || '',
        cast: show.cast || '',
        company: show.company || '',
        friends: show.friends || '',
        rating: show.rating,
        seat: show.seat || '',
        notes: show.notes || '',
        review: show.review || '',
        ticket_cost: show.ticket_cost,
        other_cost: show.other_cost
      };
    } else {
      const now = new Date();
      form.date = formatDateTimeLocal(now.toISOString());
    }
  });

  function formatDateTimeLocal(iso) {
    const d = new Date(iso);
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const mins = String(d.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day}T${hours}:${mins}`;
  }

  async function handleSubmit() {
    if (!form.name.trim()) {
      error = '请输入演出名称';
      return;
    }
    if (!form.date) {
      error = '请选择演出时间';
      return;
    }

    saving = true;
    error = '';

    try {
      if (show) {
        await api.updateShow(show.id, form);
      } else {
        await api.createShow(form);
      }
      dispatch('saved');
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<form on:submit|preventDefault={handleSubmit} class="show-form">
  {#if error}
    <div class="error">{error}</div>
  {/if}

  <div class="form-section">
    <h3>基本信息</h3>

    <div class="form-row">
      <div class="form-group">
        <label for="name">演出名称 *</label>
        <input type="text" id="name" bind:value={form.name} placeholder="如：茶馆" required />
      </div>

      <div class="form-group">
        <label for="venue">场地</label>
        <input type="text" id="venue" bind:value={form.venue} placeholder="如：国家大剧院" />
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
          {#each categories as cat}
            <option value={cat.id}>{cat.name}</option>
          {/each}
        </select>
      </div>

      <div class="form-group">
        <label for="rating">评分</label>
        <select id="rating" bind:value={form.rating}>
          <option value={null}>无评分</option>
          <option value={5}>★★★★★ 5</option>
          <option value={4}>★★★★ 4</option>
          <option value={3}>★★★ 3</option>
          <option value={2}>★★ 2</option>
          <option value={1}>★ 1</option>
        </select>
      </div>
    </div>
  </div>

  <div class="form-section">
    <h3>详细信息</h3>

    <div class="form-row">
      <div class="form-group">
        <label for="company">剧团</label>
        <input type="text" id="company" bind:value={form.company} placeholder="如：北京人艺" />
      </div>

      <div class="form-group">
        <label for="cast">阵容</label>
        <input type="text" id="cast" bind:value={form.cast} placeholder="如：于是之, 郑榕" />
      </div>
    </div>

    <div class="form-row">
      <div class="form-group">
        <label for="friends">同行好友</label>
        <input type="text" id="friends" bind:value={form.friends} placeholder="如：小明, 小红" />
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

    <div class="form-group">
      <label for="poster_url">海报URL</label>
      <input type="url" id="poster_url" bind:value={form.poster_url} placeholder="https://..." />
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

    <div class="form-group">
      <label for="notes">备注</label>
      <textarea id="notes" bind:value={form.notes} rows="2" placeholder="其他备注信息"></textarea>
    </div>
  </div>

  <div class="form-actions">
    <button type="button" class="btn-cancel" on:click={handleCancel}>取消</button>
    <button type="submit" class="btn-submit" disabled={saving}>
      {saving ? '保存中...' : show ? '更新' : '添加'}
    </button>
  </div>
</form>

<style>
  .show-form {
    background: #fff;
    border-radius: 12px;
    padding: 24px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .error {
    background: #fee;
    color: #c00;
    padding: 12px 16px;
    border-radius: 8px;
    margin-bottom: 20px;
  }

  .form-section {
    margin-bottom: 24px;
    padding-bottom: 24px;
    border-bottom: 1px solid #eee;
  }

  .form-section:last-of-type {
    border-bottom: none;
  }

  .form-section h3 {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 16px;
    color: #333;
  }

  .form-row {
    display: flex;
    gap: 16px;
    margin-bottom: 12px;
  }

  .form-group {
    flex: 1;
    margin-bottom: 12px;
  }

  .form-row .form-group {
    margin-bottom: 0;
  }

  label {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: #666;
    margin-bottom: 6px;
  }

  input, select, textarea {
    width: 100%;
  }

  textarea {
    resize: vertical;
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 12px;
    margin-top: 24px;
  }

  .btn-cancel {
    padding: 10px 24px;
    border-radius: 8px;
    background: #f0f0f0;
    color: #666;
    font-weight: 500;
    transition: background 0.2s;
  }

  .btn-cancel:hover {
    background: #e0e0e0;
  }

  .btn-submit {
    padding: 10px 32px;
    border-radius: 8px;
    background: #4A90D9;
    color: #fff;
    font-weight: 500;
    transition: background 0.2s;
  }

  .btn-submit:hover:not(:disabled) {
    background: #3a7bc8;
  }

  .btn-submit:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  @media (max-width: 768px) {
    .show-form {
      padding: 16px;
    }

    .form-row {
      flex-direction: column;
      gap: 0;
    }

    .form-row .form-group {
      margin-bottom: 12px;
    }

    .form-actions {
      flex-direction: column;
      gap: 8px;
    }

    .form-actions button {
      width: 100%;
      text-align: center;
    }
  }
</style>
