<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import CategoryTags from '$lib/components/CategoryTags.svelte';

  let { selectedIds, records, onClose, onSaved } = $props();

  let categories = $state([]);
  let dramaTree = $state([]);
  let saving = $state(false);
  let error = $state('');
  // 多币种开关：来自「设置」，OFF 时批量编辑不提供币种字段（金额沿用记录原币种）
  let settings = $state({ multi_currency: true });

  // 每个字段的修改模式：null = 不修改，{enabled: true, value: ...} = 修改
  let fields = $state({
    category: { enabled: false, value: [] },
    rating: { enabled: false, value: 0 },
    activeStatus: { enabled: false, value: 0 },
    city: { enabled: false, value: '' },
    address: { enabled: false, value: '' },
    channel: { enabled: false, value: '' },
    company: { enabled: false, value: '' },
    friends: { enabled: false, value: '' },
    remark: { enabled: false, value: '' },
    seat: { enabled: false, value: '' },
    price: { enabled: false, value: '' },
    priceCurrency: { enabled: false, value: 'CNY' },
    payPrice: { enabled: false, value: '' },
    payPriceCurrency: { enabled: false, value: 'CNY' },
    otherCost: { enabled: false, value: '' },
    otherCostCurrency: { enabled: false, value: 'CNY' },
    drama_ids: { enabled: false, op: 'append', value: [] },
    zhezi_ids: { enabled: false, op: 'append', value: [] },
    play: { enabled: false, op: 'append', value: [] },
    guest: { enabled: false, op: 'append', value: [] },
    artist_names: { enabled: false, op: 'append', value: [] }
  });

  let chosenDramas = $derived(
    dramaTree.filter((d) => fields.drama_ids.value.includes(d.id))
  );

  let chosenZhezis = $derived.by(() => {
    const zs = [];
    for (const d of dramaTree) {
      for (const z of d.zhezis || []) {
        if (fields.zhezi_ids.value.includes(z.id)) {
          zs.push({ ...z, dramaName: d.name });
        }
      }
    }
    return zs;
  });

  function toggleField(key) {
    fields[key].enabled = !fields[key].enabled;
  }

  function toggleDrama(id) {
    const arr = fields.drama_ids.value;
    const i = arr.indexOf(id);
    fields.drama_ids.value = i >= 0 ? arr.filter((x) => x !== id) : [...arr, id];
  }

  function toggleZhezi(id) {
    const arr = fields.zhezi_ids.value;
    const i = arr.indexOf(id);
    fields.zhezi_ids.value = i >= 0 ? arr.filter((x) => x !== id) : [...arr, id];
  }

  function togglePlay(name) {
    const arr = fields.play.value;
    const i = arr.indexOf(name);
    fields.play.value = i >= 0 ? arr.filter((x) => x !== name) : [...arr, name];
  }

  function toggleGuest(name) {
    const arr = fields.guest.value;
    const i = arr.indexOf(name);
    fields.guest.value = i >= 0 ? arr.filter((x) => x !== name) : [...arr, name];
  }

  function toggleArtist(name) {
    const arr = fields.artist_names.value;
    const i = arr.indexOf(name);
    fields.artist_names.value = i >= 0 ? arr.filter((x) => x !== name) : [...arr, name];
  }

  async function loadMeta() {
    try {
      const [cats, tree] = await Promise.all([
        api.listCategories(),
        api.getDramaTree().catch(() => [])
      ]);
      categories = cats;
      dramaTree = tree;
    } catch (e) { /* ignore */ }
  }

  async function save() {
    saving = true;
    error = '';
    try {
      const payload = { ids: selectedIds };

      // 标量字段
      if (fields.category.enabled && fields.category.value.length > 0) {
        payload.category_names = { op: 'set', value: [...fields.category.value] };
      }
      if (fields.rating.enabled) payload.rating = parseInt(fields.rating.value) || 0;
      if (fields.activeStatus.enabled) payload.active_status = parseInt(fields.activeStatus.value) || 0;
      if (fields.city.enabled) payload.city = fields.city.value;
      if (fields.address.enabled) payload.address = fields.address.value;
      if (fields.channel.enabled) payload.channel = fields.channel.value;
      if (fields.company.enabled) payload.company = fields.company.value;
      if (fields.friends.enabled) payload.friends = fields.friends.value;
      if (fields.remark.enabled) payload.remark = fields.remark.value;
      if (fields.seat.enabled) payload.seat = fields.seat.value;
      if (fields.price.enabled) payload.price = parseFloat(fields.price.value) || 0;
      if (fields.priceCurrency.enabled) payload.price_currency = fields.priceCurrency.value;
      if (fields.payPrice.enabled) payload.pay_price = parseFloat(fields.payPrice.value) || 0;
      if (fields.payPriceCurrency.enabled) payload.pay_price_currency = fields.payPriceCurrency.value;
      if (fields.otherCost.enabled) payload.other_cost = parseFloat(fields.otherCost.value) || 0;
      if (fields.otherCostCurrency.enabled) payload.other_cost_currency = fields.otherCostCurrency.value;

      // 数组字段
      if (fields.drama_ids.enabled && fields.drama_ids.value.length > 0) {
        payload.drama_ids = { op: fields.drama_ids.op, value: fields.drama_ids.value };
      }
      if (fields.zhezi_ids.enabled && fields.zhezi_ids.value.length > 0) {
        payload.zhezi_ids = { op: fields.zhezi_ids.op, value: fields.zhezi_ids.value };
      }
      if (fields.play.enabled && fields.play.value.length > 0) {
        payload.play = { op: fields.play.op, value: fields.play.value };
      }
      if (fields.guest.enabled && fields.guest.value.length > 0) {
        payload.guest = { op: fields.guest.op, value: fields.guest.value };
      }
      if (fields.artist_names.enabled && fields.artist_names.value.length > 0) {
        payload.artist_names = { op: fields.artist_names.op, value: fields.artist_names.value };
      }

      const result = await api.batchUpdate(selectedIds, payload);
      onSaved?.(result);
      onClose?.();
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  onMount(loadMeta);

  onMount(async () => {
    try {
      settings = await api.getSettings();
    } catch (e) { /* 读取失败则用默认（多币种开） */ }
  });
</script>

<div class="modal-mask" onclick={onClose}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <div class="modal-head">
      <h2>批量编辑 <span class="count">{selectedIds.length} 条记录</span></h2>
      <button class="close" onclick={onClose}>✕</button>
    </div>

    <div class="modal-body">
      {#if error}<div class="banner error">⚠ {error}</div>{/if}

      <div class="section">
        <div class="section-title">分类与评分</div>
        <label class="field col">
          <span class="cat-head">
            <input type="checkbox" checked={fields.category.enabled} onchange={() => toggleField('category')} />
            <span>剧种（可多个，覆盖设置）</span>
          </span>
          {#if fields.category.enabled}
            <CategoryTags bind:values={fields.category.value} {categories} placeholder="添加剧种，回车确认" />
          {:else}
            <span class="muted small">勾选后设置剧种</span>
          {/if}
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.rating.enabled} onchange={() => toggleField('rating')} />
          <span>评分</span>
          <select bind:value={fields.rating.value} disabled={!fields.rating.enabled}>
            <option value="0">未评</option>
            <option value="1">★</option>
            <option value="2">★★</option>
            <option value="3">★★★</option>
            <option value="4">★★★★</option>
            <option value="5">★★★★★</option>
          </select>
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.activeStatus.enabled} onchange={() => toggleField('activeStatus')} />
          <span>状态</span>
          <select bind:value={fields.activeStatus.value} disabled={!fields.activeStatus.enabled}>
            <option value="0">正常</option>
            <option value="1">想看</option>
          </select>
        </label>
      </div>

      <div class="section">
        <div class="section-title">场地与渠道</div>
        <label class="field">
          <input type="checkbox" checked={fields.city.enabled} onchange={() => toggleField('city')} />
          <span>城市</span>
          <input class="input" bind:value={fields.city.value} placeholder="如：上海" disabled={!fields.city.enabled} />
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.address.enabled} onchange={() => toggleField('address')} />
          <span>地址</span>
          <input class="input" bind:value={fields.address.value} placeholder="剧院地址" disabled={!fields.address.enabled} />
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.channel.enabled} onchange={() => toggleField('channel')} />
          <span>购票渠道</span>
          <input class="input" bind:value={fields.channel.value} placeholder="如：大麦、猫眼" disabled={!fields.channel.enabled} />
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.company.enabled} onchange={() => toggleField('company')} />
          <span>剧团/主办方</span>
          <input class="input" bind:value={fields.company.value} placeholder="逗号分隔" disabled={!fields.company.enabled} />
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.seat.enabled} onchange={() => toggleField('seat')} />
          <span>座位</span>
          <input class="input" bind:value={fields.seat.value} disabled={!fields.seat.enabled} />
        </label>
        <label class="field">
          <input type="checkbox" checked={fields.friends.enabled} onchange={() => toggleField('friends')} />
          <span>同行好友</span>
          <input class="input" bind:value={fields.friends.value} placeholder="逗号分隔" disabled={!fields.friends.enabled} />
        </label>
        <label class="field full">
          <input type="checkbox" checked={fields.remark.enabled} onchange={() => toggleField('remark')} />
          <span>备注</span>
          <textarea class="input" bind:value={fields.remark.value} rows="2" disabled={!fields.remark.enabled}></textarea>
        </label>
      </div>

      <div class="section">
        <div class="section-title">费用</div>
        <div class="cost-row">
          <label class="field">
            <input type="checkbox" checked={fields.price.enabled} onchange={() => toggleField('price')} />
            <span>票价</span>
            <input class="input" type="number" bind:value={fields.price.value} placeholder="金额" disabled={!fields.price.enabled} />
          </label>
          {#if settings.multi_currency}
          <label class="field">
            <input type="checkbox" checked={fields.priceCurrency.enabled} onchange={() => toggleField('priceCurrency')} />
            <span>币种</span>
            <select bind:value={fields.priceCurrency.value} disabled={!fields.priceCurrency.enabled}>
              <option value="CNY">¥ CNY</option>
              <option value="USD">$ USD</option>
            </select>
          </label>
          {/if}
        </div>
        <div class="cost-row">
          <label class="field">
            <input type="checkbox" checked={fields.payPrice.enabled} onchange={() => toggleField('payPrice')} />
            <span>实付</span>
            <input class="input" type="number" bind:value={fields.payPrice.value} placeholder="金额" disabled={!fields.payPrice.enabled} />
          </label>
          {#if settings.multi_currency}
          <label class="field">
            <input type="checkbox" checked={fields.payPriceCurrency.enabled} onchange={() => toggleField('payPriceCurrency')} />
            <span>币种</span>
            <select bind:value={fields.payPriceCurrency.value} disabled={!fields.payPriceCurrency.enabled}>
              <option value="CNY">¥ CNY</option>
              <option value="USD">$ USD</option>
            </select>
          </label>
          {/if}
        </div>
        <div class="cost-row">
          <label class="field">
            <input type="checkbox" checked={fields.otherCost.enabled} onchange={() => toggleField('otherCost')} />
            <span>其他费用</span>
            <input class="input" type="number" bind:value={fields.otherCost.value} placeholder="金额" disabled={!fields.otherCost.enabled} />
          </label>
          {#if settings.multi_currency}
          <label class="field">
            <input type="checkbox" checked={fields.otherCostCurrency.enabled} onchange={() => toggleField('otherCostCurrency')} />
            <span>币种</span>
            <select bind:value={fields.otherCostCurrency.value} disabled={!fields.otherCostCurrency.enabled}>
              <option value="CNY">¥ CNY</option>
              <option value="USD">$ USD</option>
            </select>
          </label>
          {/if}
        </div>
      </div>

      <div class="section">
        <div class="section-title">
          <label class="inline-check">
            <input type="checkbox" checked={fields.drama_ids.enabled} onchange={() => toggleField('drama_ids')} />
            <span>剧目</span>
          </label>
          <select class="op-select" bind:value={fields.drama_ids.op} disabled={!fields.drama_ids.enabled}>
            <option value="append">追加</option>
            <option value="set">替换为</option>
            <option value="remove">移除</option>
          </select>
        </div>
        {#if fields.drama_ids.enabled}
          {#if dramaTree.length === 0}
            <div class="hint">暂无剧目，请先到剧目档案添加</div>
          {:else}
            <div class="chip-grid">
              {#each dramaTree as d (d.id)}
                <label class="chip">
                  <input type="checkbox" checked={fields.drama_ids.value.includes(d.id)} onchange={() => toggleDrama(d.id)} />
                  <span>{d.name}{#if d.categoryNames?.length} <em class="cat">{d.categoryNames.join('/')}</em>{/if}</span>
                </label>
              {/each}
            </div>
          {/if}
        {/if}
      </div>

      <div class="section">
        <div class="section-title">
          <label class="inline-check">
            <input type="checkbox" checked={fields.zhezi_ids.enabled} onchange={() => toggleField('zhezi_ids')} />
            <span>折子</span>
          </label>
          <select class="op-select" bind:value={fields.zhezi_ids.op} disabled={!fields.zhezi_ids.enabled}>
            <option value="append">追加</option>
            <option value="set">替换为</option>
            <option value="remove">移除</option>
          </select>
        </div>
        {#if fields.zhezi_ids.enabled}
          {#if chosenDramas.length === 0}
            <div class="hint">请先选择剧目</div>
          {:else}
            {#each chosenDramas as d (d.id)}
              <div class="zhezi-group">
                <div class="zhezi-title">{d.name}</div>
                <div class="chip-grid">
                  {#each d.zhezis || [] as z (z.id)}
                    <label class="chip small">
                      <input type="checkbox" checked={fields.zhezi_ids.value.includes(z.id)} onchange={() => toggleZhezi(z.id)} />
                      <span>{z.name}</span>
                      {#if z.aliases && z.aliases.length}
                        <em class="alias">{z.aliases.join(' / ')}</em>
                      {/if}
                    </label>
                  {/each}
                </div>
              </div>
            {/each}
          {/if}
        {/if}
      </div>

      <div class="section">
        <div class="section-title">
          <label class="inline-check">
            <input type="checkbox" checked={fields.play.enabled} onchange={() => toggleField('play')} />
            <span>剧名（逗号分隔输入，回车添加）</span>
          </label>
          <select class="op-select" bind:value={fields.play.op} disabled={!fields.play.enabled}>
            <option value="append">追加</option>
            <option value="set">替换为</option>
            <option value="remove">移除</option>
          </select>
        </div>
        {#if fields.play.enabled}
          <input
            class="input"
            placeholder="输入剧名，回车添加"
            onkeydown={(e) => {
              if (e.key === 'Enter' && e.target.value.trim()) {
                togglePlay(e.target.value.trim());
                e.target.value = '';
              }
            }}
          />
          {#if fields.play.value.length}
            <div class="chip-grid">
              {#each fields.play.value as p}
                <span class="chip selected">{p} <button onclick={() => togglePlay(p)}>✕</button></span>
              {/each}
            </div>
          {/if}
        {/if}
      </div>

      <div class="section">
        <div class="section-title">
          <label class="inline-check">
            <input type="checkbox" checked={fields.guest.enabled} onchange={() => toggleField('guest')} />
            <span>嘉宾</span>
          </label>
          <select class="op-select" bind:value={fields.guest.op} disabled={!fields.guest.enabled}>
            <option value="append">追加</option>
            <option value="set">替换为</option>
            <option value="remove">移除</option>
          </select>
        </div>
        {#if fields.guest.enabled}
          <input
            class="input"
            placeholder="输入嘉宾名，回车添加"
            onkeydown={(e) => {
              if (e.key === 'Enter' && e.target.value.trim()) {
                toggleGuest(e.target.value.trim());
                e.target.value = '';
              }
            }}
          />
          {#if fields.guest.value.length}
            <div class="chip-grid">
              {#each fields.guest.value as g}
                <span class="chip selected">{g} <button onclick={() => toggleGuest(g)}>✕</button></span>
              {/each}
            </div>
          {/if}
        {/if}
      </div>

      <div class="section">
        <div class="section-title">
          <label class="inline-check">
            <input type="checkbox" checked={fields.artist_names.enabled} onchange={() => toggleField('artist_names')} />
            <span>演员</span>
          </label>
          <select class="op-select" bind:value={fields.artist_names.op} disabled={!fields.artist_names.enabled}>
            <option value="append">追加</option>
            <option value="set">替换为</option>
            <option value="remove">移除</option>
          </select>
        </div>
        {#if fields.artist_names.enabled}
          <input
            class="input"
            placeholder="输入演员名，回车添加"
            onkeydown={(e) => {
              if (e.key === 'Enter' && e.target.value.trim()) {
                toggleArtist(e.target.value.trim());
                e.target.value = '';
              }
            }}
          />
          {#if fields.artist_names.value.length}
            <div class="chip-grid">
              {#each fields.artist_names.value as a}
                <span class="chip selected">{a} <button onclick={() => toggleArtist(a)}>✕</button></span>
              {/each}
            </div>
          {/if}
        {/if}
      </div>
    </div>

    <div class="modal-foot">
      <button class="btn" onclick={onClose}>取消</button>
      <button class="btn primary" onclick={save} disabled={saving}>
        {saving ? '保存中…' : `保存（${selectedIds.length} 条）`}
      </button>
    </div>
  </div>
</div>

<style>
  .modal-mask {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 20px;
  }
  .modal {
    background: var(--surface);
    border-radius: var(--radius-lg);
    width: 100%;
    max-width: 640px;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-xl);
  }
  .modal-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
  }
  .modal-head h2 {
    margin: 0;
    font-size: 18px;
  }
  .count {
    color: var(--text-muted);
    font-weight: normal;
    font-size: 14px;
  }
  .close {
    border: none;
    background: none;
    color: var(--text-3);
    cursor: pointer;
    font-size: 18px;
    width: 32px;
    height: 32px;
    border-radius: 50%;
  }
  .close:hover { background: var(--surface-3); }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 16px 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .section {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px 14px;
  }
  .section-title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    margin-bottom: 10px;
    font-size: 14px;
  }
  .section-title .inline-check {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: normal;
  }

  .field {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
    flex-wrap: wrap;
  }
  .field:last-child { margin-bottom: 0; }
  .field input[type="checkbox"] { flex-shrink: 0; }
  .field span {
    min-width: 70px;
    font-size: 13px;
    color: var(--text-2);
  }
  .field.full { flex-direction: column; align-items: stretch; }
  .field.full span { margin-bottom: 4px; }
  .field.col { flex-direction: column; align-items: stretch; }
  .field.col .cat-head { display: flex; align-items: center; gap: 8px; }
  .field.col .cat-head span { min-width: 0; }
  .field:has(input[type="checkbox"]:not(:checked)) span {
    color: var(--text-3);
    text-decoration: line-through;
  }

  .input {
    flex: 1;
    min-width: 0;
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text);
    font-size: 13px;
  }
  .input:focus {
    outline: none;
    border-color: var(--accent);
  }
  select.input,
  .field select {
    flex: 1;
    min-width: 120px;
  }

  .cost-row {
    display: flex;
    gap: 8px;
    margin-bottom: 8px;
  }
  .cost-row:last-child { margin-bottom: 0; }
  .cost-row .field { flex: 1; }

  .op-select {
    margin-left: auto;
    padding: 4px 8px;
    font-size: 12px;
  }

  .chip-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 4px 10px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--surface-2);
    font-size: 12px;
    cursor: pointer;
    user-select: none;
  }
  .chip input { display: none; }
  .chip:has(input:checked) {
    border-color: var(--accent);
    background: var(--accent-softer);
    color: var(--accent);
  }
  .chip.selected {
    border-color: var(--accent);
    background: var(--accent-softer);
    color: var(--accent);
  }
  .chip.selected button {
    border: none;
    background: none;
    color: inherit;
    cursor: pointer;
    font-size: 10px;
  }
  .chip .cat {
    font-size: 10px;
    color: var(--text-3);
    font-style: normal;
  }
  .chip:has(input:checked) .cat { color: var(--accent); opacity: 0.7; }
  .chip.small { font-size: 11px; padding: 3px 8px; }
  .chip .alias {
    font-size: 10px;
    color: var(--text-3);
    font-style: normal;
  }

  .zhezi-group { margin-top: 8px; }
  .zhezi-title {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    margin-bottom: 4px;
  }

  .hint {
    font-size: 12px;
    color: var(--text-3);
    margin-top: 8px;
  }

  .modal-foot {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 20px;
    border-top: 1px solid var(--border);
  }
</style>