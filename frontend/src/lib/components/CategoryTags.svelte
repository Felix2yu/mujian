<script>
  // 多剧种选择：chips + 回车添加；categories 提供建议列表（datalist）
  let { values = $bindable([]), categories = [], placeholder = '添加剧种，回车确认' } = $props();
  let input = $state('');
  const uid = $props.id();

  const suggestions = $derived(categories.map((c) => c.name).filter(Boolean));

  function commit(raw) {
    const parts = String(raw || '')
      .split(/[,，]/)
      .map((s) => s.trim())
      .filter(Boolean);
    for (const p of parts) {
      if (!values.includes(p)) values.push(p);
    }
    input = '';
  }

  function onKeydown(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      commit(input);
    } else if (e.key === 'Backspace' && !input && values.length) {
      values.pop();
    }
  }

  function onSelect(e) {
    commit(e.target.value);
  }
</script>

<div class="ctags">
  {#each values as v, i (v)}
    <span class="chip">
      {v}
      <button type="button" class="x" onclick={() => values.splice(i, 1)} title="移除">×</button>
    </span>
  {/each}
  <input
    class="ctag-input"
    bind:value={input}
    onkeydown={onKeydown}
    onchange={onSelect}
    list={uid}
    {placeholder}
  />
  <datalist id={uid}>
    {#each suggestions as s}<option value={s} />{/each}
  </datalist>
</div>

<style>
  .ctags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius, 8px);
    background: var(--surface-2);
    min-height: 38px;
  }
  .ctags:focus-within { border-color: var(--accent); }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: var(--accent-soft);
    color: var(--accent);
    border-radius: 999px;
    padding: 2px 4px 2px 10px;
    font-size: 13px;
    font-weight: 500;
    line-height: 1.5;
    white-space: nowrap;
  }
  .x {
    border: none;
    background: none;
    color: inherit;
    opacity: 0.65;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .x:hover { opacity: 1; background: rgba(0, 0, 0, 0.12); }
  .ctag-input {
    flex: 1;
    min-width: 110px;
    border: none;
    background: none;
    outline: none;
    color: var(--text);
    font-size: 14px;
    padding: 2px 4px;
  }
</style>
