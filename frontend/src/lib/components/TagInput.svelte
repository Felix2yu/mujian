<script>
  import { createEventDispatcher } from 'svelte';

  export let value = '';
  export let placeholder = '';
  export let suggestions = [];

  const dispatch = createEventDispatcher();

  let inputValue = '';
  let inputEl;

  $: tags = value ? value.split(/[,，]/).map(s => s.trim()).filter(Boolean) : [];

  function addTag() {
    const v = inputValue.trim();
    if (!v || tags.includes(v)) {
      inputValue = '';
      return;
    }
    const newTags = [...tags, v];
    value = newTags.join(', ');
    inputValue = '';
    dispatch('change', value);
  }

  function removeTag(index) {
    const newTags = tags.filter((_, i) => i !== index);
    value = newTags.join(', ');
    dispatch('change', value);
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
      dispatch('change', value);
    }
    inputValue = '';
    inputEl?.focus();
  }

  $: filteredSuggestions = inputValue.length > 0
    ? suggestions.filter(s => s.toLowerCase().includes(inputValue.toLowerCase()) && !tags.includes(s))
    : [];
</script>

<div class="tag-input-wrapper">
  <div class="tag-input" on:click={() => inputEl?.focus()}>
    {#each tags as tag, i}
      <span class="tag">
        <span class="tag-text">{tag}</span>
        <button type="button" class="tag-remove" on:click|stopPropagation={() => removeTag(i)}>×</button>
      </span>
    {/each}
    <input
      bind:this={inputEl}
      bind:value={inputValue}
      on:keydown={handleKeydown}
      placeholder={tags.length === 0 ? placeholder : ''}
      class="tag-field"
    />
  </div>
  {#if filteredSuggestions.length > 0}
    <div class="suggestions">
      {#each filteredSuggestions.slice(0, 8) as s}
        <button type="button" class="suggestion-item" on:click={() => selectSuggestion(s)}>
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
    gap: 4px;
    padding: 6px 10px;
    border: 1px solid #ddd;
    border-radius: 6px;
    min-height: 38px;
    align-items: center;
    cursor: text;
    transition: border-color 0.2s;
    background: #fff;
  }
  .tag-input:focus-within { border-color: #4A90D9; }
  :global(.dark) .tag-input { background: #2a2a2a; border-color: #444; }
  :global(.dark) .tag-input:focus-within { border-color: #4A90D9; }
  .tag {
    display: flex;
    align-items: center;
    gap: 4px;
    background: #e8f0fe;
    color: #333;
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 13px;
    white-space: nowrap;
  }
  .tag-remove {
    font-size: 14px;
    color: #999;
    padding: 0 2px;
    line-height: 1;
  }
  .tag-remove:hover { color: #c00; }
  .tag-field {
    border: none;
    outline: none;
    background: transparent;
    font-size: 14px;
    flex: 1;
    min-width: 60px;
    padding: 2px 0;
  }
  .suggestions {
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    background: #fff;
    border: 1px solid #ddd;
    border-radius: 6px;
    margin-top: 4px;
    max-height: 160px;
    overflow-y: auto;
    z-index: 50;
    box-shadow: 0 4px 8px rgba(0,0,0,0.1);
  }
  :global(.dark) .suggestions { background: #2a2a2a; border-color: #444; }
  .suggestion-item {
    display: block;
    width: 100%;
    text-align: left;
    padding: 8px 12px;
    font-size: 13px;
    background: transparent;
    border: none;
    cursor: pointer;
  }
  .suggestion-item:hover { background: #f0f0f0; }
  :global(.dark) .suggestion-item:hover { background: #333; }
</style>
