<script>
  // 补全输入框：把 RecordForm 里 4 处手写下拉（城市 / 场馆 / 渠道 / 同行）收成一处。
  //
  // 交互契约（与原先各处逐条对齐，避免迁移时改变手感）：
  //   · 焦点即开下拉；失焦 120ms 后才关，给点击留出时间
  //   · 中文输入法组词期间不响应键盘（isComposing / keyCode 229）
  //   · ↑↓ 移动高亮并滚动跟随、Enter 选中、Esc 关闭；两者都 stopPropagation，
  //     否则会冒泡到表单层的 Enter 拦截，导致"选中建议"失效
  //   · multi=true 时按逗号/顿号分段：以最后一段作为过滤词，选中后替换该段
  //     （原实现把整串当过滤词，字段一有值下拉就空了；且会把未打完的半段也留下）
  //   · keepOpen：选中后是否保持下拉打开（连续添加用）
  let {
    value = $bindable(''),
    suggestions = [],
    multi = false,
    cap = 30,
    placeholder = '',
    label = '',
    keepOpen = false,
    spellcheck = false,
    onpick,
    oncommit
  } = $props();

  let open = $state(false);
  let active = $state(-1);
  let blurTimer = null;
  let listEl = $state(null);

  // 多值模式下，正在输入的是最后一段
  let query = $derived.by(() => {
    const raw = value || '';
    if (!multi) return raw.trim().toLowerCase();
    const segs = raw.split(/[,，、]/);
    return (segs[segs.length - 1] || '').trim().toLowerCase();
  });

  let filtered = $derived.by(() => {
    const q = query;
    const list = q ? suggestions.filter((v) => v.toLowerCase().includes(q)) : suggestions;
    return list.slice(0, cap);
  });

  function openList() {
    if (blurTimer) {
      clearTimeout(blurTimer);
      blurTimer = null;
    }
    open = true;
    active = -1;
  }

  function onBlur() {
    closeSoon();
    // 失焦即视为「输入告一段落」，让调用方按原 onchange 的时机应用
    if (typeof oncommit === 'function') oncommit();
  }

  function closeSoon() {
    if (blurTimer) clearTimeout(blurTimer);
    blurTimer = setTimeout(() => {
      blurTimer = null;
      open = false;
      active = -1;
    }, 120);
  }

  function pick(v) {
    if (multi) {
      const segs = (value || '').split(/[,，、]/).map((s) => s.trim());
      const done = segs.slice(0, -1).filter(Boolean);
      if (!done.includes(v)) done.push(v);
      value = done.join(', ');
    } else {
      value = v;
    }
    active = -1;
    open = keepOpen;
    if (typeof onpick === 'function') onpick(v);
  }

  function onKeydown(e) {
    if (e.isComposing || e.keyCode === 229) return; // 中文输入法组词中不响应
    if (!open) return;
    const items = filtered;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      if (!items.length) return;
      e.preventDefault();
      e.stopPropagation();
      let i = active;
      i = e.key === 'ArrowDown' ? i + 1 : i - 1;
      if (i < -1) i = -1;
      if (i > items.length - 1) i = items.length - 1;
      active = i;
      scrollActiveIntoView(i);
    } else if (e.key === 'Enter') {
      const i = active;
      if (i >= 0 && i < items.length) {
        e.preventDefault();
        e.stopPropagation();
        pick(items[i]);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      open = false;
      active = -1;
    }
  }

  function scrollActiveIntoView(idx) {
    if (idx < 0) return;
    setTimeout(() => {
      const list = listEl;
      const items = list?.querySelectorAll('.ci-item');
      if (items && items[idx]) items[idx].scrollIntoView({ block: 'nearest' });
    }, 0);
  }
</script>

<div class="ci">
  <input
    class="input ci-input"
    type="text"
    {spellcheck}
    {placeholder}
    aria-label={label || undefined}
    autocomplete="off"
    bind:value
    onfocus={openList}
    onblur={onBlur}
    oninput={openList}
    onkeydown={onKeydown}
  />
  {#if open && filtered.length}
    <div class="ci-list" bind:this={listEl} role="listbox" aria-label={label || placeholder}>
      {#each filtered as v, i (v)}
        <button
          type="button"
          class="ci-item"
          class:active={i === active}
          role="option"
          aria-selected={i === active}
          onmousedown={(e) => e.preventDefault()}
          onclick={() => pick(v)}>{v}</button
        >
      {/each}
    </div>
  {/if}
</div>

<style>
  /* 定位与内部滚动；外层尺寸（flex / min-width）由调用方的容器决定 */
  .ci {
    position: relative;
    min-width: 0;
  }
  .ci-input {
    width: 100%;
  }
  .ci-list {
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
  .ci-item {
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
  .ci-item:hover,
  .ci-item.active {
    background: var(--accent-soft);
    color: var(--accent);
  }
</style>
