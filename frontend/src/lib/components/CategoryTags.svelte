<script>
  // 多剧种选择：chips 可拖拽排序；回车/逗号添加；categories 提供自定义下拉建议。
  // 不使用原生 datalist：在 Safari / 移动端基本不弹建议，补全不可靠（与 channel / company
  // / city 字段一致改自定义下拉）。自定义下拉在所有浏览器 / 设备下都能稳定补全。
  let { values = $bindable([]), categories = [], placeholder = '添加剧种，回车确认' } = $props();
  let input = $state('');
  let showList = $state(false);
  let activeIndex = $state(-1);
  let composing = $state(false); // 中文输入法组词中，避免回车误提交
  let listEl = $state(null);

  // 下拉建议：排除已选，按实时演出数（recordCount）由大到小排序；仅影响建议顺序，
  // 不影响已选 chips（values）的顺序。分类管理页不使用本组件，故不受其排序影响。
  const filteredSuggestions = $derived.by(() => {
    const q = input.trim().toLowerCase();
    const chosen = new Set(values);
    let list = categories
      .filter((c) => !chosen.has(c.name))
      .sort((a, b) => (b.recordCount || 0) - (a.recordCount || 0))
      .map((c) => c.name)
      .filter(Boolean);
    if (q) list = list.filter((n) => n.toLowerCase().includes(q));
    return list.slice(0, 60);
  });

  function commit(raw) {
    const parts = String(raw || '')
      .split(/[,，、]\s*/)
      .map((s) => s.trim())
      .filter(Boolean);
    for (const p of parts) {
      if (!values.includes(p)) values.push(p);
    }
    input = '';
    activeIndex = -1;
  }

  // 从下拉点选已有剧种：加入 chips 并清空输入，下拉保持打开以便继续添加
  function pick(s) {
    if (!values.includes(s)) values.push(s);
    input = '';
    activeIndex = -1;
  }

  function onKeydown(e) {
    if (e.key === 'Enter') {
      if (composing) return; // 输入法组词回车，不提交
      e.preventDefault();
      if (showList && activeIndex >= 0 && activeIndex < filteredSuggestions.length) {
        pick(filteredSuggestions[activeIndex]);
      } else {
        commit(input);
      }
    } else if (e.key === 'ArrowDown') {
      if (!showList || !filteredSuggestions.length) return;
      e.preventDefault();
      showList = true;
      activeIndex = Math.min(activeIndex + 1, filteredSuggestions.length - 1);
    } else if (e.key === 'ArrowUp') {
      if (!showList || !filteredSuggestions.length) return;
      e.preventDefault();
      if (activeIndex <= 0) activeIndex = -1;
      else activeIndex -= 1;
    } else if (e.key === 'Escape') {
      showList = false;
      activeIndex = -1;
    } else if (e.key === 'Backspace' && !input && values.length) {
      values.pop();
    }
  }

  // 高亮项滚动进可视区
  $effect(() => {
    if (activeIndex < 0 || !listEl) return;
    const el = listEl.querySelector('.ctags-suggest-item.active');
    el?.scrollIntoView({ block: 'nearest' });
  });

  let blurTimer = null;

  function onFocus() {
    if (blurTimer) { clearTimeout(blurTimer); blurTimer = null; }
    showList = true;
    activeIndex = -1;
  }

  // 失焦时延时关闭；若输入了完整的已有剧种则自动提交（沿用原 onchange 语义）。
  // 关闭定时器在重新聚焦时取消，避免焦点快速切换（或瞬时 blur）误关下拉。
  function onBlur() {
    if (blurTimer) clearTimeout(blurTimer);
    blurTimer = setTimeout(() => {
      blurTimer = null;
      showList = false;
      const v = input.trim();
      if (v && categories.some((c) => c.name === v) && !values.includes(v)) {
        commit(v);
      }
    }, 150);
  }

  function onCompositionStart() {
    composing = true;
  }
  function onCompositionEnd() {
    composing = false;
    showList = true;
  }

  function onDragOver(e, i) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    overIdx = i;
    overBefore = e.clientX < rect.left + rect.width / 2;
  }

  function onDropAt(i, before) {
    if (dragIdx < 0 || dragIdx === i) {
      resetDrag();
      return;
    }
    const next = values.slice();
    const [moved] = next.splice(dragIdx, 1);
    // 移除拖动项后，位于其后的目标索引左移一位；再按指示的插入侧落位
    const at = dragIdx < i ? i - 1 : i;
    next.splice(before ? at : at + 1, 0, moved);
    values.splice(0, values.length, ...next);
    resetDrag();
  }

  function resetDrag() {
    dragIdx = -1;
    overIdx = -1;
  }

  // chips 拖拽状态
  let dragIdx = $state(-1);
  let overIdx = $state(-1);
  let overBefore = $state(true);
</script>

<div class="ctags">
  {#each values as v, i (v)}
    <span
      class="chip"
      draggable="true"
      class:dragging={dragIdx === i}
      class:drop-before={overIdx === i && dragIdx !== i && overBefore}
      class:drop-after={overIdx === i && dragIdx !== i && !overBefore}
      title="拖动调整顺序"
      ondragstart={(e) => { dragIdx = i; e.dataTransfer.effectAllowed = 'move'; }}
      ondragover={(e) => onDragOver(e, i)}
      ondragleave={() => { if (overIdx === i) overIdx = -1; }}
      ondrop={(e) => { e.preventDefault(); onDropAt(i, overBefore); }}
      ondragend={resetDrag}
    >
      <span class="grip" aria-hidden="true">⠿</span>
      {v}
      <button type="button" class="x" onclick={() => values.splice(i, 1)} title="移除">×</button>
    </span>
  {/each}
  <input
    class="ctag-input"
    bind:value={input}
    onfocus={onFocus}
    onblur={onBlur}
    onkeydown={onKeydown}
    oncompositionstart={onCompositionStart}
    oncompositionend={onCompositionEnd}
    {placeholder}
    autocomplete="off"
    spellcheck="false"
  />
  {#if showList && filteredSuggestions.length}
    <div class="ctags-suggest" bind:this={listEl}>
      {#each filteredSuggestions as s, i (s)}
        <button
          type="button"
          class="ctags-suggest-item"
          class:active={i === activeIndex}
          onmousedown={(e) => e.preventDefault()}
          onclick={() => pick(s)}
        >{s}</button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .ctags {
    position: relative;
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
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: var(--accent-soft);
    color: var(--accent);
    border-radius: 999px;
    padding: 2px 4px 2px 6px;
    font-size: 13px;
    font-weight: 500;
    line-height: 1.5;
    white-space: nowrap;
    cursor: grab;
    transition: opacity var(--t-fast, 0.15s) ease, outline-color var(--t-fast, 0.15s) ease;
  }
  .chip:active { cursor: grabbing; }
  .chip.dragging { opacity: 0.35; }
  /* 拖拽插入指示：目标左/右侧显示竖线光标，表示松手后的插入位置 */
  .chip.drop-before::before,
  .chip.drop-after::after {
    content: '';
    position: absolute;
    top: -4px;
    bottom: -4px;
    width: 3px;
    border-radius: 2px;
    background: var(--accent);
    pointer-events: none;
  }
  .chip.drop-before::before { left: -5px; }
  .chip.drop-after::after { right: -5px; }
  .grip {
    color: currentColor;
    opacity: 0.5;
    font-size: 12px;
    line-height: 1;
    user-select: none;
    cursor: grab;
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
  /* 自定义下拉建议（替代原生 datalist，兼容 Safari / 移动端） */
  .ctags-suggest {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 40;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius, 8px);
    box-shadow: 0 10px 28px rgba(0, 0, 0, 0.22);
    max-height: 240px;
    overflow-y: auto;
    padding: 4px;
  }
  .ctags-suggest-item {
    display: block;
    width: 100%;
    text-align: left;
    border: none;
    background: none;
    padding: 9px 12px;
    border-radius: var(--radius-sm, 6px);
    font-size: 14px;
    color: var(--text-2);
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ctags-suggest-item:hover,
  .ctags-suggest-item.active {
    background: var(--accent-soft);
    color: var(--accent);
  }
</style>
