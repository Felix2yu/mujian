<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api.js';

  // 自动备份：从「设置」页迁到「数据」页（备份属于数据管理，与导入 / 导出同源）。
  //
  // 与在设置页时的一个关键差别：设置页有页面级「保存设置」按钮，本页没有。
  // 因此这里改为**改动即存** —— 任一控件变化后立即 PUT 局部更新。
  // PUT /api/settings 的入参是 config.SettingsUpdate（全部指针字段），只提交
  // 下面这四个键，其余设置（S3 凭据、AI、货殖等）不受影响。
  const BACKUP_INTERVALS = [
    { v: 0, label: '关闭' },
    { v: 24, label: '每天' },
    { v: 72, label: '每 3 天' },
    { v: 168, label: '每周' },
    { v: 336, label: '每 2 周' },
    { v: 720, label: '每月' },
    { v: 2160, label: '每季度' }
  ];
  const BACKUP_FORMATS = [
    { v: 'db', label: '数据库快照（.db）', hint: '单个 SQLite 库文件，停机后直接换回文件即可恢复' },
    { v: 'json', label: '纯数据（data.json）', hint: '体积小，可从上方导入区还原，不含封面' },
    { v: 'zip', label: '数据 + 封面（.zip）', hint: 'data.json 加全部封面文件，可从上方导入区完整还原' }
  ];

  let format = $state('db');
  let interval = $state(0);
  let keep = $state(10);
  let remote = $state(false);
  let lastAt = $state(0);
  // S3 是否已配置齐（Bucket + Access Key）——决定「备份后上传到 S3」能否勾选。
  let s3Ready = $state(false);
  let loading = $state(true);
  let loadError = $state('');

  let running = $state(false);
  let msg = $state('');
  // '' | 'saving' | 'saved' | 'error'
  let saveState = $state('');
  let saveError = $state('');
  let backups = $state([]);
  let restoring = $state('');

  // 存量值不在预设档位时（如旧配置的 6 小时）动态补一个选项，避免下拉显示空白
  const intervalOptions = $derived(
    BACKUP_INTERVALS.some((i) => i.v === interval)
      ? BACKUP_INTERVALS
      : [...BACKUP_INTERVALS, { v: interval, label: `每 ${interval} 小时` }]
  );

  let saveTimer = null;
  let msgTimer = null;

  function fmtSize(n) {
    if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB';
    if (n >= 1 << 10) return (n / (1 << 10)).toFixed(0) + ' KB';
    return n + ' B';
  }

  function fmtTime(ts) {
    if (!ts) return '从未备份';
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  async function refreshBackups() {
    try {
      const res = await api.backupList();
      backups = res.backups || [];
    } catch (e) { /* 列表加载失败不打断页面 */ }
  }

  async function persist() {
    saveState = 'saving';
    saveError = '';
    try {
      const fresh = await api.updateSettings({
        backup_format: format,
        backup_interval_hours: interval,
        backup_keep: Math.max(1, Number(keep) || 10),
        backup_remote: remote
      });
      if (fresh && typeof fresh.last_backup_at === 'number') lastAt = fresh.last_backup_at;
      saveState = 'saved';
      clearTimeout(saveTimer);
      saveTimer = setTimeout(() => {
        if (saveState === 'saved') saveState = '';
      }, 2400);
    } catch (e) {
      saveState = 'error';
      saveError = e.message || '保存失败';
    }
  }

  async function runNow() {
    running = true;
    msg = '';
    try {
      const res = await api.backupRun();
      msg = `已生成备份 ${res.file}`;
      lastAt = Math.floor(Date.now() / 1000);
      refreshBackups();
    } catch (e) {
      msg = '备份失败：' + e.message;
    } finally {
      running = false;
      clearTimeout(msgTimer);
      msgTimer = setTimeout(() => (msg = ''), 6000);
    }
  }

  async function restore(file) {
    if (!confirm(`用 ${file} 恢复数据？现有同 ID 记录会被覆盖。`)) return;
    restoring = file;
    try {
      await api.backupRestoreFrom(file);
      msg = '已从 ' + file + ' 恢复';
    } catch (e) {
      msg = '恢复失败：' + e.message;
    } finally {
      restoring = '';
      clearTimeout(msgTimer);
      msgTimer = setTimeout(() => (msg = ''), 6000);
    }
  }

  async function remove(file) {
    if (!confirm(`删除备份 ${file}？此操作不可恢复。`)) return;
    await api.backupDelete(file).catch((e) => (msg = '删除失败：' + e.message));
    refreshBackups();
  }

  onMount(async () => {
    try {
      const s = await api.getSettings();
      format = ['db', 'json', 'zip'].includes(s.backup_format) ? s.backup_format : 'db';
      interval = typeof s.backup_interval_hours === 'number' ? s.backup_interval_hours : 0;
      keep = typeof s.backup_keep === 'number' ? s.backup_keep : 10;
      remote = s.backup_remote === true;
      lastAt = typeof s.last_backup_at === 'number' ? s.last_backup_at : 0;
      s3Ready = !!(s.s3_bucket?.trim() && s.s3_access_key?.trim());
    } catch (e) {
      loadError = e.message || '读取备份配置失败';
    } finally {
      loading = false;
    }
    refreshBackups();
  });
</script>

<div class="card sec">
  <div class="card-head">
    <h3>自动备份</h3>
    <span
      class="save-state"
      class:ok={saveState === 'saved'}
      class:err={saveState === 'error'}
      role="status"
      aria-live="polite"
    >{#if loading}读取配置…{:else if saveState === 'saving'}保存中…{:else if saveState === 'saved'}已保存 ✓{:else if saveState === 'error'}⚠ {saveError}{:else}改动即存{/if}</span>
  </div>

  {#if loadError}<div class="banner error">⚠ {loadError}</div>{/if}

  <div class="s3-grid">
    <label class="field">
      <span>备份格式</span>
      <select class="input" bind:value={format} onchange={persist} disabled={loading} style="max-width: 260px;">
        {#each BACKUP_FORMATS as f (f.v)}
          <option value={f.v}>{f.label}</option>
        {/each}
      </select>
      <span class="hint">{BACKUP_FORMATS.find((f) => f.v === format)?.hint}</span>
    </label>
    <label class="field">
      <span>备份间隔</span>
      <select class="input" bind:value={interval} onchange={persist} disabled={loading} style="max-width: 200px;">
        {#each intervalOptions as it (it.v)}
          <option value={it.v}>{it.label}</option>
        {/each}
      </select>
      <span class="hint">按间隔自动把所选格式的备份写入服务端 backups/ 目录</span>
    </label>
    <label class="field">
      <span>保留份数</span>
      <input class="input" type="number" min="1" max="100" bind:value={keep} onchange={persist} disabled={loading} style="max-width: 120px;" />
      <span class="hint">超出后自动删除最旧的快照</span>
    </label>
    <label class="field">
      <span>上次备份</span>
      <input class="input" readonly value={fmtTime(lastAt)} />
    </label>
  </div>

  <!-- 开关单独一行：它带一句较长的说明，塞进上面的网格会在窄屏把单元格撑得很难看 -->
  <label class="field s3-toggle">
    <span class="check-row">
      <input type="checkbox" bind:checked={remote} onchange={persist} disabled={loading || !s3Ready} />
      <span>备份后上传到 S3</span>
    </span>
    <span class="hint">
      {#if s3Ready}
        每次备份成功后把该文件推送到 S3 桶内的 backups/ 目录（本机备份仍保留）
      {:else}
        需要先在「设置 → S3 对象存储」填写完整的 Bucket 与 Access Key
      {/if}
    </span>
  </label>

  <div class="convert-actions">
    <button class="btn" disabled={running || loading} onclick={runNow}>
      {running ? '备份中…' : '立即备份'}
    </button>
    {#if msg}<span class="hint" style="align-self: center;">{msg}</span>{/if}
  </div>

  {#if backups.length}
    <div class="backup-list">
      {#each backups as b (b.file)}
        <div class="backup-row">
          <span class="backup-name" title={b.file}>{b.file}</span>
          <span class="backup-size">{fmtSize(b.size)}</span>
          <span class="backup-time">{fmtTime(b.modified)}</span>
          <span class="backup-ops">
            <a class="btn sm" href={api.backupDownloadUrl(b.file)} download>下载</a>
            {#if b.file.endsWith('.json') || b.file.endsWith('.zip')}
              <button type="button" class="btn sm" disabled={restoring === b.file} onclick={() => restore(b.file)}>
                {restoring === b.file ? '恢复中…' : '恢复'}
              </button>
            {:else}
              <button type="button" class="btn sm" disabled title=".db 快照需停机后替换数据库文件恢复">恢复</button>
            {/if}
            <button type="button" class="btn sm danger" onclick={() => remove(b.file)}>删除</button>
          </span>
        </div>
      {/each}
    </div>
  {:else}
    <p class="hint" style="margin-top: 10px;">还没有备份文件：点「立即备份」生成第一份，或把备份间隔调成非「关闭」。</p>
  {/if}
  <p class="hint" style="margin-top: 8px;">以上改动立即生效，无需另行保存。重启服务不会重置备份节奏（按最近一份快照的时间续算）。</p>
</div>

<style>
  /* 组件自带页面级样式：父页面的 <style> 是 scoped 的，不会作用到子组件内部，
     `.sec` / `.field` / `.hint` 这类必须在这里重新声明。 */
  .sec { padding: 18px 20px; }
  .sec h3 { margin: 0; font-size: 15.5px; }
  .card-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }
  .save-state { font-size: 12px; color: var(--text-3); }
  .save-state.ok { color: var(--success); }
  .save-state.err { color: var(--danger); }

  .s3-grid {
    display: grid;
    /* 用 min() 兜底：容器窄于 240px 时轨道下限降为容器宽度，
       避免 auto-fit 的硬性 240px 下限在窄屏把页面撑破 */
    grid-template-columns: repeat(auto-fit, minmax(min(240px, 100%), 1fr));
    gap: 12px 16px;
  }
  .field { display: flex; flex-direction: column; gap: 4px; font-size: 13.5px; color: var(--text-2); }
  .hint { font-weight: 400; color: var(--text-3); font-size: 12px; display: block; margin-top: 4px; }
  .s3-toggle { margin-top: 14px; }
  .check-row { display: flex; align-items: center; gap: 8px; }
  .check-row input[type='checkbox'] {
    width: 18px;
    height: 18px;
    flex: none;
    accent-color: var(--accent);
    cursor: pointer;
  }
  .check-row input[type='checkbox']:disabled { cursor: not-allowed; }

  .convert-actions {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin-top: 14px;
    padding-top: 14px;
    border-top: 1px dashed var(--border);
  }

  .backup-list { margin-top: 12px; border: 1px solid var(--border); border-radius: var(--radius, 10px); overflow: hidden; }
  .backup-row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .backup-row:last-child { border-bottom: none; }
  .backup-name { flex: 1 1 180px; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
  .backup-size { color: var(--text-3); font-size: 12px; flex: none; }
  .backup-time { color: var(--text-3); font-size: 12px; flex: none; }
  .backup-ops { display: flex; gap: 6px; flex: none; margin-left: auto; }
</style>
