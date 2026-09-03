<script>
  import { page } from '$app/stores';
  import { api } from '$lib/api.js';
  import BackLink from '$lib/components/BackLink.svelte';
  import RecordCard from '$lib/components/RecordCard.svelte';
  import CategoryTags from '$lib/components/CategoryTags.svelte';

  let id = $derived($page.params.id);
  let drama = $state(null);
  let zhezis = $state([]);
  let records = $state([]);
  let loading = $state(true);
  let error = $state('');
  let categories = $state([]);

  // inline drama editing
  let editingInfo = $state(false);
  let info = $state({ name: '', aliases: '', categoryNames: [], remark: '' });

  // zhezi creation / editing
  let form = $state({ name: '', aliases: '', remark: '' });
  let editingId = $state(null);
  let saving = $state(false);
  let deleting = $state(false);

  const splitList = (s) => (s || '').split(/[,，]/).map((x) => x.trim()).filter(Boolean);

  async function load() {
    loading = true;
    error = '';
    try {
      const d = await api.getDrama(id);
      drama = d;
      zhezis = d.zhezis || [];
      records = d.records || [];
      info = { name: d.name, aliases: (d.aliases || []).join(', '), categoryNames: [], remark: d.remark };
      api.listCategories().then((cs) => (categories = cs)).catch(() => {});
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function startEdit() {
    // 编辑框显示当前生效剧种；清空保存则回到「按演出自动聚合」
    info = { name: drama.name, aliases: (drama.aliases || []).join(', '), categoryNames: (drama.categoryNames || []).slice(), remark: drama.remark };
    editingInfo = true;
  }

  async function saveInfo() {
    if (!info.name.trim() || saving) return;
    saving = true;
    error = '';
    try {
      const aliases = splitList(info.aliases);
      drama = await api.updateDrama(id, { name: info.name.trim(), aliases, categoryNames: info.categoryNames.slice(), remark: info.remark.trim() });
      editingInfo = false;
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  async function removeDrama() {
    if (!confirm(`删除剧目「${drama.name}」？其下所有折子也会一并删除，演出记录不受影响。`)) return;
    deleting = true;
    try {
      await api.deleteDrama(id);
      location.href = '/dramas';
    } catch (e) {
      error = e.message;
      deleting = false;
    }
  }

  function resetForm() {
    form = { name: '', aliases: '', remark: '' };
    editingId = null;
  }

  function editZhezi(z) {
    editingId = z.id;
    form = { name: z.name, aliases: (z.aliases || []).join(', '), remark: z.remark };
  }

  async function saveZhezi() {
    if (!form.name.trim() || saving) return;
    saving = true;
    error = '';
    try {
      const payload = { name: form.name.trim(), aliases: splitList(form.aliases), remark: form.remark.trim() };
      if (editingId) {
        await api.updateZhezi(editingId, payload);
      } else {
        await api.createZhezi(id, payload);
      }
      resetForm();
      await load();
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  async function removeZhezi(z) {
    if (!confirm(`删除折子「${z.name}」？已关联该折子的演出记录不受影响。`)) return;
    error = '';
    try {
      await api.deleteZhezi(z.id);
      await load();
    } catch (e) {
      error = e.message;
    }
  }

  // 拖拽排序状态
  let dragIdx = $state(-1);
  let overIdx = $state(-1);
  let overBefore = $state(true);

  function onDragStart(i) {
    dragIdx = i;
  }

  function onDragOver(e, i) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    overIdx = i;
    overBefore = e.clientY < rect.top + rect.height / 2;
  }

  function onDrop(targetIdx) {
    if (dragIdx < 0 || dragIdx === targetIdx) {
      resetDrag();
      return;
    }
    const next = zhezis.slice();
    const [moved] = next.splice(dragIdx, 1);
    next.splice(targetIdx, 0, moved);
    resetDrag();
    error = '';
    api.reorderZhezis(id, next.map((x) => x.id))
      .then(() => {
        zhezis = next;
        if (drama) drama.zheziCount = next.length;
      })
      .catch((e) => (error = e.message));
  }

  function resetDrag() {
    dragIdx = -1;
    overIdx = -1;
  }

  $effect(() => { if (id) load(); });
</script>
<svelte:head><title>{drama ? `${drama.name} - 幕间` : "剧目 - 幕间"}</title></svelte:head>


<div class="fade-up">
  <BackLink fallback="/dramas" label="← 剧目" />

  {#if loading}
    <div class="loading">
      <div class="skeleton" style="height: 22px; width: 40%;"></div>
      <div class="skeleton" style="height: 14px; width: 60%;"></div>
      <div class="skeleton" style="height: 200px;"></div>
    </div>
  {:else if error}
    <div class="banner error">⚠ {error}</div>
    <BackLink fallback="/dramas" label="← 返回剧目列表" />
  {:else if drama}
    {#if error}<div class="banner error">⚠ {error}</div>{/if}

    <div class="card head-card">
      {#if editingInfo}
        <input class="input" placeholder="剧目名称" bind:value={info.name} />
        <input class="input" placeholder="别名（逗号分隔），如：三国志, 桃园三结义" bind:value={info.aliases} />
        <div class="row">
          <CategoryTags bind:values={info.categoryNames} {categories} placeholder="剧种（可多个）；清空则按演出自动统计" />
        </div>
        <div class="row"><span class="muted small">留空剧种 = 按该剧目「单独演出」时的剧种自动统计（拼盘演出已自动排除）；手动填写可覆盖</span></div>
        <textarea class="input" rows="2" placeholder="备注" bind:value={info.remark}></textarea>
        <div class="actions">
          <button class="btn primary sm" onclick={saveInfo} disabled={saving || !info.name.trim()}>{saving ? '保存中…' : '保存'}</button>
          <button class="btn ghost sm" onclick={() => { editingInfo = false; }}>取消</button>
        </div>
      {:else}
        <div class="head-main">
          <div>
            <h1>{drama.name}</h1>
            <div class="sub">
              {#if drama.aliases && drama.aliases.length}
                <span class="aliases">别名：{drama.aliases.join(' / ')}</span>
              {/if}
              {#each drama.categoryNames || [] as cn}<span class="pill" title="由关联演出自动统计">{cn}</span>{/each}
              <span class="muted">{drama.zheziCount} 折 · {drama.recordCount} 场演出</span>
            </div>
            {#if drama.remark}<p class="remark">{drama.remark}</p>{/if}
          </div>
          <div class="head-actions">
            <button class="btn sm" onclick={startEdit}>编辑</button>
            <button class="btn danger sm" onclick={removeDrama} disabled={deleting}>{deleting ? '删除中…' : '删除'}</button>
          </div>
        </div>
      {/if}
    </div>

    <div class="card section">
      <div class="sec-head">
        <h3>折子</h3>
        <span class="muted small">一次演出可能只演其中部分折子；按住拖把上下拖动即可调整顺序，不同剧种/剧团称呼不同时可添加别名</span>
      </div>

      <div class="zhezis">
        {#each zhezis as z, i (z.id)}
          <div class="z-row card"
            draggable={editingId === z.id ? 'false' : 'true'}
            class:editing={editingId === z.id}
            class:dragging={dragIdx === i}
            class:drop-before={overIdx === i && dragIdx !== i && overBefore}
            class:drop-after={overIdx === i && dragIdx !== i && !overBefore}
            ondragstart={(e) => { onDragStart(i); e.dataTransfer.effectAllowed = 'move'; }}
            ondragover={(e) => onDragOver(e, i)}
            ondragleave={() => { if (overIdx === i) overIdx = -1; }}
            ondrop={(e) => { e.preventDefault(); onDrop(i); }}
            ondragend={() => resetDrag()}
          >
            <span class="grip" title="拖动排序">⠿</span>
            {#if editingId === z.id}
              <div class="z-edit grow">
                <input class="input" placeholder="折子名称" bind:value={form.name} onkeydown={(e) => e.key === 'Enter' && saveZhezi()} />
                <input class="input" placeholder="别名（逗号分隔），如：拾画 · 叫画" bind:value={form.aliases} onkeydown={(e) => e.key === 'Enter' && saveZhezi()} />
                <input class="input" placeholder="备注（可选）" bind:value={form.remark} onkeydown={(e) => e.key === 'Enter' && saveZhezi()} />
                <div class="actions">
                  <button class="btn primary sm" onclick={saveZhezi} disabled={saving || !form.name.trim()}>{saving ? '保存中…' : '保存'}</button>
                  <button class="btn ghost sm" onclick={resetForm}>取消</button>
                </div>
              </div>
            {:else}
              <div class="z-info grow">
                <div class="z-name">
                  {z.name}
                  {#if z.aliases && z.aliases.length}
                    <span class="aliases">别名：{z.aliases.join(' / ')}</span>
                  {/if}
                </div>
                {#if z.remark}<div class="z-remark">{z.remark}</div>{/if}
              </div>
              <div class="z-ops">
                <button class="op" onclick={() => editZhezi(z)} title="编辑">✎</button>
                <button class="op danger" onclick={() => removeZhezi(z)} title="删除">✕</button>
              </div>
            {/if}
          </div>
        {/each}
      </div>

      {#if zhezis.length === 0}
        <div class="empty small card"><div class="h">还没有折子，在下方添加第一个</div></div>
      {/if}

      {#if editingId === null}
        <div class="add-z card">
          <div class="add-title">＋ 添加折子</div>
          <div class="row">
            <input class="input" placeholder="折子名称，如：游园" bind:value={form.name} onkeydown={(e) => e.key === 'Enter' && saveZhezi()} />
            <input class="input" placeholder="别名（逗号分隔，可空）" bind:value={form.aliases} onkeydown={(e) => e.key === 'Enter' && saveZhezi()} />
          </div>
          <div class="row">
            <input class="input" placeholder="备注（可选）" bind:value={form.remark} onkeydown={(e) => e.key === 'Enter' && saveZhezi()} />
            <button class="btn primary" onclick={saveZhezi} disabled={saving || !form.name.trim()}>{saving ? '保存中…' : '添加'}</button>
          </div>
        </div>
      {/if}
    </div>

    <div class="card section">
      <div class="sec-head"><h3>演出记录 <span class="num">{records.length}</span></h3></div>
      {#if records.length === 0}
        <div class="empty small card"><div class="h">还没有演出记录关联该剧目</div></div>
      {:else}
        <div class="grid">
          {#each records as r (r.id)}
            <RecordCard record={r} />
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .back { display: inline-flex; color: var(--text-muted); font-size: 13.5px; margin-bottom: 12px; }
  .back:hover { color: var(--accent); }
  .loading { display: flex; flex-direction: column; gap: 12px; }

  .head-card { padding: 18px 20px; margin-bottom: 14px; }
  .head-main { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; }
  h1 { margin: 0 0 8px; font-size: 26px; line-height: 1.25; }
  .sub { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .pill { background: var(--accent-soft); color: var(--accent); padding: 2px 10px; border-radius: 999px; font-size: 12.5px; }
  .muted { color: var(--text-muted); font-size: 13px; }
  .small { font-size: 12.5px; }
  .remark { margin: 10px 0 0; color: var(--text-2); white-space: pre-wrap; line-height: 1.6; }
  .head-actions { display: flex; gap: 8px; flex: 0 0 auto; }
  .row { display: flex; gap: 10px; margin-top: 10px; }
  .row .input { flex: 1; }
  .actions { display: flex; gap: 8px; margin-top: 10px; }

  .section { padding: 18px 20px; margin-top: 14px; }
  .section h3 { margin: 0; font-size: 16px; display: flex; align-items: center; gap: 8px; }
  .num { color: var(--accent); font-family: var(--font-sans); font-weight: 700; font-size: 15px; }
  .sec-head { margin-bottom: 14px; }
  .sec-head .muted { display: block; margin-top: 4px; }

  .zhezis { display: flex; flex-direction: column; gap: 8px; }
  .z-row { position: relative; display: flex; align-items: center; gap: 8px; padding: 12px 14px; margin-bottom: 0; cursor: grab; }
  .z-row .grow { flex: 1 1 auto; min-width: 0; }
  .z-row:active { cursor: grabbing; }
  .z-row.editing { cursor: default; }
  .z-row.dragging { opacity: 0.4; cursor: grabbing; }
  /* 拖拽插入指示 */
  .z-row.drop-before::before,
  .z-row.drop-after::after {
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
  .z-row.drop-before::before { top: -6px; }
  .z-row.drop-after::after { bottom: -6px; }
  .grip {
    flex: 0 0 auto; color: var(--text-3); font-size: 14px; line-height: 1; cursor: grab;
    padding: 4px 2px; user-select: none;
  }
  .grip:hover { color: var(--accent); }
  .z-info { min-width: 0; }
  .z-name { font-size: 15px; font-weight: 600; display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; }
  .aliases { font-size: 12px; font-weight: 400; color: var(--text-muted); }
  .z-remark { font-size: 12.5px; color: var(--text-2); margin-top: 3px; }
  .z-ops { display: flex; gap: 4px; flex: 0 0 auto; }
  .op { border: none; background: none; color: var(--text-3); width: 26px; height: 26px; border-radius: 6px; cursor: pointer; font-size: 13px; }
  .op:hover { background: var(--surface-3); color: var(--text); }
  .op.danger:hover { background: var(--danger-soft); color: var(--danger); }
  .z-edit { display: flex; flex-direction: column; gap: 8px; }

  .add-z { padding: 14px; margin-top: 10px; }
  .add-title { font-weight: 600; margin-bottom: 6px; color: var(--text-2); }

  .empty { padding: 14px; text-align: center; }
  .empty.small .h { margin: 0; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(172px, 1fr)); gap: 14px; margin-top: 4px; }

  @media (max-width: 640px) {
    h1 { font-size: 22px; }
    .head-main { flex-direction: column; }
    .grid { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
  }
</style>