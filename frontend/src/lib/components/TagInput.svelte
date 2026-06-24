<script>
  let { value = $bindable(''), placeholder = '', suggestions = [], onchange, onflush } = $props();

  let inputValue = $state('');
  let inputEl;

  let tags = $derived(value ? value.split(/[,，]/).map(s => s.trim()).filter(Boolean) : []);

  function addTag() {
    const v = inputValue.trim();
    if (!v || tags.includes(v)) {
      inputValue = '';
      return;
    }
    const newTags = [...tags, v];
    value = newTags.join(', ');
    inputValue = '';
    onchange?.(value);
  }

  function removeTag(index) {
    const newTags = tags.filter((_, i) => i !== index);
    value = newTags.join(', ');
    onchange?.(value);
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addTag();
    } else if (e.key === 'Backspace' && inputValue === '' && tags.length > 0) {
      removeTag(tags.length - 1);
    }
  }

  function selectSuggestion(s) {
    if (!tags.includes(s)) {
      const newTags = [...tags, s];
      value = newTags.join(', ');
      onchange?.(value);
    }
    inputValue = '';
    inputEl?.focus();
  }

  let filteredSuggestions = $derived(inputValue.length > 0
    ? suggestions.filter(s => s.toLowerCase().includes(inputValue.toLowerCase()) && !tags.includes(s))
    : []);

  onflush?.(() => addTag());
</script>

<div class="tag-input-wrapper">
  <div class="tag-input" onclick={() => inputEl?.focus()}>
    {#each tags as tag, i}
      <span class="tag">
        <span class="tag-text">{tag}</span>
        <button type="button" class="tag-remove" onclick={(e) => { e.stopPropagation(); removeTag(i); }}>×</button>
      </span>
    {/each}
    <input
      bind:this={inputEl}
      bind:value={inputValue}
      onkeydown={handleKeydown}
      placeholder={tags.length === 0 ? placeholder : ''}
      class="tag-field"
    />
  </div>
  {#if filteredSuggestions.length > 0}
    <div class="suggestions">
      {#each filteredSuggestions.slice(0, 8) as s}
        <button type="button" class="suggestion-item" onclick={() => selectSuggestion(s)}>
          {s}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .tag-input-wrapper { position: relative; }
  .tag-input {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    min-height: 42px;
    align-items: center;
    cursor: text;
    transition: all 0.2s ease;
    background: var(--bg-input);
    border: 1.5px solid var(--border);
  }
  .tag-input:hover {
    border-color: var(--border-hover);
  }
  .tag-input:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
  }
  .tag {
    display: flex;
    align-items: center;
    gap: 4px;
    background: var(--accent-bg);
    color: var(--accent);
    padding: 3px 8px;
    border-radius: 6px;
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
  }
  .tag-remove {
    font-size: 14px;
    color: var(--text-muted);
    padding: 0 2px;
    line-height: 1;
    transition: color 0.15s;
  }
  .tag-remove:hover { color: var(--danger-text); }
  .tag-field {
    border: none;
    outline: none;
    background: transparent;
    font-size: 14px;
    flex: 1;
    min-width: 60px;
    padding: 4px 0;
    color: var(--text-primary);
  }
  .suggestions {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    max-height: 180px;
    overflow-y: auto;
    z-index: 50;
    box-shadow: var(--shadow-md);
  }
  .suggestion-item {
    display: block;
    width: 100%;
    text-align: left;
    padding: 10px 14px;
    font-size: 13px;
    background: transparent;
    border: none;
    cursor: pointer;
    transition: background 0.15s;
    color: var(--text-primary);
  }
  .suggestion-item:hover { background: var(--bg-surface); }
</style>
