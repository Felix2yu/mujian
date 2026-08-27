<script>
  // Horizontal ranking list with proportional bars and rank badges.
  let { items = [], unit = '场', hrefFn = null } = $props();

  let max = $derived(Math.max(1, ...items.map((i) => i.count || 0)));
</script>

<ol class="rank">
  {#each items as it, i}
    <li>
      <span class="rk" class:top={i < 3}>{i + 1}</span>
      {#if hrefFn && it.id}
        <a class="nm" href={hrefFn(it)} title={it.name}>{it.name}</a>
      {:else}
        <span class="nm" title={it.name}>{it.name}</span>
      {/if}
      <span class="track"><span class="fill" style="width:{(it.count / max) * 100}%"></span></span>
      <span class="ct">{it.count}<span class="u">{unit}</span></span>
    </li>
  {/each}
  {#if items.length === 0}
    <li class="empty">暂无数据</li>
  {/if}
</ol>

<style>
  .rank { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 7px; }
  .rank li { display: flex; align-items: center; gap: 10px; font-size: 13.5px; }
  .rk {
    width: 20px; height: 20px; flex: 0 0 auto; border-radius: 6px;
    display: grid; place-items: center; font-size: 11px; font-weight: 700;
    background: var(--surface-3); color: var(--text-2); font-variant-numeric: tabular-nums;
  }
  .rk.top { background: var(--gold); color: #fff; }
  .nm {
    flex: 0 0 110px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    color: var(--text-2); text-decoration: none;
  }
  .nm:hover { color: var(--accent); }
  .track { flex: 1; height: 8px; border-radius: 99px; background: var(--surface-3); overflow: hidden; }
  .fill { display: block; height: 100%; border-radius: 99px; background: linear-gradient(90deg, var(--accent), var(--accent-strong)); }
  .ct { width: 52px; text-align: right; font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; }
  .ct .u { font-size: 10px; color: var(--text-muted); font-weight: 500; margin-left: 2px; }
  .empty { color: var(--text-3); justify-content: center; }
</style>
