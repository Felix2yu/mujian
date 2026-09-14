<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';
  import { askConfirm } from '$lib/confirm.js';

  // ===== 场馆治理 =====
  //
  // 场馆是 records.address 的去重覆盖层（详情见 backend/internal/db/venues.go）：
  //   · venues 表把同一物理场馆的多种写法归并到一个规范名；
  //   · 合并时把对应记录的 address 改写为规范名，并合并别名、删除 source 行；
  //   · records 不持有 venue_id —— address 始终自包含，避免污染写入热路径。
  //
  // 本面板：列出全部场馆（含实时记录数与「疑似重复」分组），支持多选后并入一个
  // 规范名；合并前先预览影响面（改写记录数 / 并入别名），确认后才落库。

  let venues = $state([]);
  let loading = $state(false);
  let error = $state('');
  let selectedIds = $state([]); // 选中的参与者（含规范名）
  let targetID = $state('');    // 合并目标（规范名）
  let preview = $state(null);   // 预览（dry_run=true 的返回）
  let mergeBusy = $state(false);

  function isSel(id) {
    return selectedIds.includes(id);
  }
  function targetName() {
    return venues.find((v) => v.id === targetID)?.name || '';
  }

  async function loadVenues() {
    loading = true;
    error = '';
    try {
      venues = await api.listVenues();
    } catch (e) {
      error = '加载场馆失败：' + e.message;
    } finally {
      loading = false;
    }
  }

  // 重新同步场馆层与当前记录（纳入新地址、清理零记录行），再刷新列表。
  async function refresh() {
    selectedIds = [];
    targetID = '';
    preview = null;
    try {
      await api.rescanVenues();
    } catch (e) {
      error = '重新同步失败：' + e.message;
    }
    await loadVenues();
  }

  function toggle(id) {
    if (isSel(id)) {
      selectedIds = selectedIds.filter((x) => x !== id);
    } else {
      selectedIds = [...selectedIds, id];
    }
    // 目标必须仍在选中集合内；若仅剩一个选中项则默认它为目标。
    if (!selectedIds.includes(targetID)) targetID = '';
    if (selectedIds.length === 1 && !targetID) targetID = selectedIds[0];
  }

  function setTarget(id) {
    if (isSel(id)) targetID = id;
  }

  // 合并要求：至少 2 个参与者，且目标在参与者内。
  let canMerge = $derived(selectedIds.length >= 2 && !!targetID && selectedIds.includes(targetID));
  let sourceIDs = $derived(selectedIds.filter((id) => id !== targetID));

  async function showPreview() {
    if (!canMerge) return;
    preview = null;
    error = '';
    try {
      preview = await api.mergeVenues(targetID, sourceIDs, { dryRun: true });
    } catch (e) {
      error = '预览失败：' + e.message;
    }
  }

  async function doMerge() {
    if (!canMerge) return;
    const ok = await askConfirm({
      title: '合并场馆',
      message: `将把 ${sourceIDs.length} 个场馆并入「${targetName()}」，改写其对应记录的地址并合并别名。`,
      confirmLabel: '确认合并',
      danger: true,
      impact: preview?.records_repoint ?? 0,
      impactLabel: '条记录地址将被改写'
    });
    if (!ok) return;
    mergeBusy = true;
    error = '';
    try {
      await api.mergeVenues(targetID, sourceIDs, { dryRun: false });
      selectedIds = [];
      targetID = '';
      preview = null;
      await loadVenues();
    } catch (e) {
      error = '合并失败：' + e.message;
    } finally {
      mergeBusy = false;
    }
  }

  function cancelPreview() {
    preview = null;
  }

  onMount(loadVenues);
</script>

<!-- 挂载在 .fade-up 之外（与费用补全 / 封面维护同）：本组件虽无 fixed 灯箱，
     但为与数据治理类工具保持一致的安装位置与节奏，统一放在页面分区之外。 -->
<section class="venue-section">
  <div class="venue-head">
    <h2>场馆治理</h2>
    <p class="tiny muted">同一物理场馆的多种写法（如「开明大戏院」与「开明大戏院（观前街店）」）会归并到一个规范名；合并会改写对应记录的地址并合并别名。</p>
  </div>

  <div class="btn-row">
    <button class="btn" onclick={refresh} disabled={loading}>
      {loading ? '加载中…' : '重新同步 / 刷新'}
    </button>
    <span class="tiny muted">已识别 {venues.length} 个场馆，{venues.filter((v) => v.dup_group).length} 个疑似重复</span>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  <div class="card sec">
    {#if venues.length === 0}
      <p class="tiny muted">暂无场馆数据。场馆会在服务首次启动时从记录地址自动生成。</p>
    {:else}
      <div class="vlist">
        {#each venues as v (v.id)}
          <div class="vrow" class:dup={v.dup_group}>
            <input type="checkbox" checked={isSel(v.id)} onchange={() => toggle(v.id)} aria-label="选择 {v.name}" />
            <span class="vname">{v.name}</span>
            <span class="vcity">{v.city || '—'}</span>
            <span class="vcnt">{v.record_count} 条</span>
            {#if v.dup_group}<span class="vtag">疑似重复</span>{/if}
            <label class="vtarget" class:on={targetID === v.id}>
              <input type="radio" name="vtarget" checked={targetID === v.id} onchange={() => setTarget(v.id)} disabled={!isSel(v.id)} />
              规范名
            </label>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if selectedIds.length > 0}
    <div class="card sec merge-bar">
      <div class="tiny">
        已选 <b>{selectedIds.length}</b> 个场馆
        {#if targetID}，规范名为 <b>{targetName()}</b>{/if}
        {#if !canMerge}<span class="warn"> · 需选择 ≥2 个且指定一个规范名</span>{/if}
      </div>
      <div class="btn-row" style="margin-top: 10px;">
        <button class="btn primary" onclick={showPreview} disabled={!canMerge || mergeBusy}>
          预览合并
        </button>
        <button class="btn" onclick={() => { selectedIds = []; targetID = ''; preview = null; }} disabled={mergeBusy}>
          清除选择
        </button>
      </div>

      {#if preview}
        <div class="preview">
          <h3>合并预览 <span class="tiny muted">试运行 · 尚未写入</span></h3>
          <div class="pstats">
            <span class="pstat"><b>{preview.records_repoint ?? 0}</b> 记录地址改写</span>
            <span class="pstat"><b>{preview.target?.name ?? ''}</b> 规范名</span>
            {#if (preview.sources?.length ?? 0) > 0}
              <span class="pstat"><b>{preview.sources.length}</b> 个源场馆</span>
            {/if}
          </div>
          {#if preview.sources?.length}
            <ul class="plist">
              {#each preview.sources as s}<li>{s.name}（{s.record_count} 条）</li>{/each}
            </ul>
          {/if}
          {#if preview.aliases_added?.length}
            <ul class="plist">
              <li class="tiny">并入别名：{preview.aliases_added.join('、')}</li>
            </ul>
          {/if}
          <div class="btn-row">
            <button class="btn primary danger" onclick={doMerge} disabled={mergeBusy}>
              {mergeBusy ? '合并中…' : '确认合并'}
            </button>
            <button class="btn" onclick={cancelPreview} disabled={mergeBusy}>取消</button>
          </div>
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .venue-section {
    margin-top: 28px;
    padding-top: 18px;
    border-top: 1px solid var(--border);
  }
  .venue-head { margin-bottom: 14px; }
  .venue-head h2 {
    margin: 0 0 4px;
    font-size: 16px;
    font-weight: 600;
    color: var(--text);
  }
  .venue-head p { margin: 0; }
  .btn-row { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 14px; align-items: center; }
  .sec { padding: 18px 20px; margin-top: 16px; }
  .banner.error { margin-top: 12px; padding: 10px 14px; border-radius: var(--radius, 10px); background: rgba(220, 38, 38, 0.08); color: #b91c1c; font-size: 13px; }

  .vlist { display: flex; flex-direction: column; gap: 2px; }
  .vrow {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius, 10px);
    background: var(--surface);
    font-size: 13.5px;
  }
  .vrow.dup { border-left: 3px solid var(--accent); background: var(--accent-softer, rgba(99, 102, 241, 0.06)); }
  .vname { font-weight: 500; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .vcity { color: var(--text-3); font-size: 12px; flex: none; }
  .vcnt { color: var(--text-2); font-size: 12px; flex: none; }
  .vtag {
    font-size: 11px;
    color: var(--accent);
    border: 1px solid var(--accent);
    border-radius: 999px;
    padding: 1px 8px;
    flex: none;
  }
  .vtarget { display: inline-flex; align-items: center; gap: 5px; flex: none; font-size: 12px; color: var(--text-2); cursor: pointer; }
  .vtarget.on { color: var(--accent); font-weight: 600; }
  .vtarget input:disabled { opacity: 0.4; }

  .merge-bar { background: var(--surface-2); }
  .warn { color: var(--warn, #b45309); }
  .preview { margin-top: 14px; padding-top: 14px; border-top: 1px dashed var(--border); }
  .preview h3 { margin: 0 0 10px; font-size: 15px; display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
  .pstats { display: flex; flex-wrap: wrap; gap: 8px; }
  .pstat {
    display: inline-flex;
    align-items: baseline;
    gap: 5px;
    padding: 4px 10px;
    border-radius: 999px;
    background: var(--surface-3);
    font-size: 13px;
    color: var(--text-2);
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pstat b { font-size: 14px; color: var(--text); }
  .plist { margin: 10px 0 0; padding-left: 18px; display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: var(--text-2); }
  .btn.primary.danger { background: var(--danger, #b91c1c); border-color: var(--danger, #b91c1c); color: #fff; }
</style>
