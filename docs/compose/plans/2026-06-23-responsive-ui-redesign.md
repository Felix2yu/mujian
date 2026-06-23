# Responsive UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use compose:subagent (recommended) or compose:execute to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign all pages and the navigation to be fully responsive on mobile, tablet, and desktop.

**Architecture:** Mobile-first CSS with breakpoints at 480px (phone), 768px (tablet), 1024px (desktop). Navbar converts to hamburger menu on mobile. Grid layouts stack vertically. Filter bars become collapsible on small screens. No new dependencies needed — pure CSS media queries.

**Tech Stack:** SvelteKit, CSS media queries, vanilla JS for mobile menu toggle.

---

## File Structure

- Modify: `frontend/src/routes/+layout.svelte` — navbar responsive redesign
- Modify: `frontend/src/routes/+page.svelte` — home page responsive
- Modify: `frontend/src/routes/shows/+page.svelte` — shows list filters responsive
- Modify: `frontend/src/routes/dashboard/+page.svelte` — dashboard responsive
- Modify: `frontend/src/routes/settings/+page.svelte` — settings responsive
- Modify: `frontend/src/routes/shows/[id]/+page.svelte` — show detail responsive
- Modify: `frontend/src/routes/shows/import/+page.svelte` — import page responsive
- Modify: `frontend/src/lib/components/ShowCard.svelte` — card responsive
- Modify: `frontend/src/lib/components/Calendar.svelte` — calendar responsive
- Modify: `frontend/src/lib/components/ShowForm.svelte` — form responsive

---

### Task 1: Layout Navbar Responsive Redesign

**Covers:** Mobile navigation, hamburger menu, responsive nav

**Files:**
- Modify: `frontend/src/routes/+layout.svelte`

- [ ] **Step 1: Add mobile menu state and hamburger toggle**

In the `<script>` section, add:
```javascript
let mobileMenuOpen = false;

function toggleMobileMenu() {
  mobileMenuOpen = !mobileMenuOpen;
}

function closeMobileMenu() {
  mobileMenuOpen = false;
}
```

- [ ] **Step 2: Update navbar HTML for mobile**

Replace the navbar content with:
```svelte
<nav class="navbar">
  <div class="nav-brand">
    <a href="/" on:click={closeMobileMenu}>幕间</a>
  </div>

  <button class="hamburger" class:open={mobileMenuOpen} on:click={toggleMobileMenu} aria-label="菜单">
    <span></span>
    <span></span>
    <span></span>
  </button>

  <div class="nav-menu" class:open={mobileMenuOpen}>
    <div class="nav-links">
      <a href="/" class:active={currentPath === '/'} on:click={closeMobileMenu}>日历</a>
      <a href="/shows" class:active={currentPath.startsWith('/shows') && !currentPath.includes('/import') && !currentPath.includes('/new')} on:click={closeMobileMenu}>演出列表</a>
      <a href="/dashboard" class:active={currentPath === '/dashboard'} on:click={closeMobileMenu}>看板</a>
      <a href="/shows/new" class:active={currentPath === '/shows/new'} on:click={closeMobileMenu}>添加演出</a>
    </div>
    <div class="nav-search">
      <form on:submit|preventDefault={handleSearch}>
        <input
          type="text"
          placeholder="搜索演出..."
          bind:value={searchQuery}
          on:blur={() => setTimeout(closeSearch, 200)}
        />
      </form>
      {#if showSearch && searchResults.length > 0}
        <div class="search-results">
          {#each searchResults.slice(0, 5) as show}
            <a href="/shows/{show.id}" class="search-item" on:click={closeSearch}>
              <span class="search-name">{show.name}</span>
              <span class="search-venue">{show.venue}</span>
            </a>
          {/each}
          {#if searchResults.length > 5}
            <a href="/search?q={encodeURIComponent(searchQuery)}" class="search-more" on:click={closeSearch}>
              查看全部 {searchResults.length} 条结果 →
            </a>
          {/if}
        </div>
      {/if}
    </div>
    <div class="nav-right-mobile">
      {#if stats}
        <div class="nav-stats">
          <span>{stats.total_shows} 场演出</span>
          <span>{stats.total_hours.toFixed(0)} 小时</span>
        </div>
      {/if}
      <a href="/settings" class:active={currentPath === '/settings'} on:click={closeMobileMenu}>⚙ 设置</a>
    </div>
  </div>

  <div class="nav-right">
    {#if stats}
      <div class="nav-stats">
        <span>{stats.total_shows} 场演出</span>
        <span>{stats.total_hours.toFixed(0)} 小时</span>
      </div>
    {/if}
    <a href="/settings" class="nav-settings" class:active={currentPath === '/settings'}>⚙</a>
  </div>
</nav>
```

- [ ] **Step 3: Add responsive CSS**

Replace the existing `<style>` section with responsive styles. Key additions:
```css
/* Hamburger button - hidden on desktop */
.hamburger {
  display: none;
  flex-direction: column;
  justify-content: center;
  gap: 5px;
  width: 36px;
  height: 36px;
  padding: 6px;
  z-index: 110;
}

.hamburger span {
  display: block;
  width: 100%;
  height: 2px;
  background: #333;
  border-radius: 2px;
  transition: all 0.3s;
}

:global(.dark) .hamburger span {
  background: #e0e0e0;
}

.hamburger.open span:nth-child(1) {
  transform: rotate(45deg) translate(5px, 5px);
}

.hamburger.open span:nth-child(2) {
  opacity: 0;
}

.hamburger.open span:nth-child(3) {
  transform: rotate(-45deg) translate(5px, -5px);
}

/* Nav right mobile - hidden on desktop */
.nav-right-mobile {
  display: none;
}

/* Mobile breakpoint */
@media (max-width: 768px) {
  .navbar {
    padding: 0 16px;
    gap: 0;
  }

  .hamburger {
    display: flex;
  }

  .nav-menu {
    position: fixed;
    top: 60px;
    left: 0;
    right: 0;
    bottom: 0;
    background: #fff;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    transform: translateX(100%);
    transition: transform 0.3s;
    z-index: 105;
    overflow-y: auto;
  }

  :global(.dark) .nav-menu {
    background: #1a1a1a;
  }

  .nav-menu.open {
    transform: translateX(0);
  }

  .nav-links {
    flex-direction: column;
    gap: 4px;
  }

  .nav-links a {
    padding: 12px 16px;
    font-size: 16px;
  }

  .nav-search {
    width: 100%;
  }

  .nav-search input {
    width: 100%;
    padding: 10px 16px;
    font-size: 16px;
    border-radius: 8px;
  }

  .nav-right {
    display: none;
  }

  .nav-right-mobile {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding-top: 16px;
    border-top: 1px solid #eee;
  }

  :global(.dark) .nav-right-mobile {
    border-top-color: #444;
  }

  .nav-right-mobile a {
    padding: 12px 16px;
    border-radius: 8px;
    background: #f0f0f0;
    text-align: center;
  }

  :global(.dark) .nav-right-mobile a {
    background: #2a2a2a;
  }

  main {
    padding: 16px;
  }
}
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/routes/+layout.svelte
git commit -m "feat: navbar responsive with hamburger menu for mobile"
```

---

### Task 2: Home Page Responsive

**Covers:** Calendar page mobile layout

**Files:**
- Modify: `frontend/src/routes/+page.svelte`

- [ ] **Step 1: Add responsive CSS to home page**

Add media queries to the existing `<style>`:
```css
@media (max-width: 768px) {
  .main-content {
    grid-template-columns: 1fr;
  }

  .stats-bar {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
    padding: 16px;
  }

  .stat-value {
    font-size: 22px;
  }

  .calendar-section {
    padding: 12px;
  }

  .sidebar-section {
    padding: 16px;
  }
}

@media (max-width: 480px) {
  .stats-bar {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
    padding: 12px;
  }

  .stat-value {
    font-size: 20px;
  }

  .stat-label {
    font-size: 11px;
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/routes/+page.svelte
git commit -m "feat: home page responsive layout for mobile"
```

---

### Task 3: Shows List Page Responsive

**Covers:** Shows list filters, batch operations on mobile

**Files:**
- Modify: `frontend/src/routes/shows/+page.svelte`

- [ ] **Step 1: Add filter toggle state for mobile**

In the `<script>` section, add:
```javascript
let filtersExpanded = false;
```

- [ ] **Step 2: Wrap filters in collapsible panel**

Replace the filter section in the template. The desktop view keeps filters visible, mobile gets a toggle button:

```svelte
<div class="header-right">
  <button class="filter-toggle" on:click={() => filtersExpanded = !filtersExpanded}>
    🔍 筛选 {#if hasActiveFilters}<span class="filter-badge"></span>{/if}
  </button>

  <div class="filters-panel" class:expanded={filtersExpanded || !isMobile}>
    <div class="filters">
      <input type="text" class="search-input" placeholder="搜索..." bind:value={searchQuery} />
      <select bind:value={statusFilter}>
        <option value="">全部状态</option>
        <option value="planned">计划中</option>
        <option value="watched">已观看</option>
        <option value="cancelled">已取消</option>
      </select>
      <select bind:value={categoryFilter}>
        <option value="">全部分类</option>
        {#each categories as cat}
          <option value={cat.name}>{cat.name}</option>
        {/each}
      </select>
      <select bind:value={ratingFilter}>
        <option value="">全部评分</option>
        <option value="5">★★★★★</option>
        <option value="4">★★★★</option>
        <option value="3">★★★</option>
        <option value="2">★★</option>
        <option value="1">★</option>
        <option value="0">无评分</option>
      </select>
      <select bind:value={sortBy}>
        <option value="date_desc">日期 ↓</option>
        <option value="date_asc">日期 ↑</option>
        <option value="name">名称</option>
        <option value="rating_desc">评分 ↓</option>
        <option value="rating_asc">评分 ↑</option>
      </select>
      {#if searchQuery || statusFilter || categoryFilter || ratingFilter}
        <button class="clear-btn" on:click={clearFilters}>清除筛选</button>
      {/if}
    </div>
  </div>

  <span class="result-count">{filteredShows.length}/{shows.length}</span>
  <button class="batch-btn" class:active={batchMode} on:click={toggleBatchMode}>
    {batchMode ? '退出' : '批量'}
  </button>
  <a href="/shows/import" class="action-btn">📥</a>
  <a href={api.getExportUrl()} class="action-btn" download>📤</a>
  <a href="/shows/new" class="add-btn">+</a>
</div>
```

- [ ] **Step 3: Add isMobile reactive variable**

In the `<script>` section, add:
```javascript
let isMobile = false;

function checkMobile() {
  isMobile = window.innerWidth <= 768;
}

onMount(() => {
  checkMobile();
  window.addEventListener('resize', checkMobile);
  loadShows();
  categories = await api.listCategories();
});
```

And update the existing `onMount`:
```javascript
$: hasActiveFilters = searchQuery || statusFilter || categoryFilter || ratingFilter;
```

- [ ] **Step 4: Add responsive CSS**

```css
.filter-toggle {
  display: none;
  padding: 8px 16px;
  background: #f0f0f0;
  border-radius: 8px;
  font-weight: 500;
  position: relative;
}

.filter-badge {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 8px;
  height: 8px;
  background: #E74C3C;
  border-radius: 50%;
}

.action-btn {
  padding: 8px 12px;
  background: #f0f0f0;
  border-radius: 8px;
  font-size: 14px;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    gap: 12px;
  }

  .header-right {
    width: 100%;
    flex-wrap: wrap;
    gap: 8px;
  }

  .filter-toggle {
    display: block;
  }

  .filters-panel {
    display: none;
    width: 100%;
  }

  .filters-panel.expanded {
    display: block;
  }

  .filters {
    flex-direction: column;
    gap: 8px;
  }

  .filters select, .search-input {
    width: 100%;
  }

  .result-count {
    font-size: 12px;
  }

  .batch-btn {
    padding: 8px 12px;
    font-size: 13px;
  }

  .add-btn {
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 18px;
    padding: 0;
  }

  .batch-bar {
    flex-wrap: wrap;
    gap: 8px;
  }

  .batch-panel .batch-form {
    flex-direction: column;
  }

  .batch-form .form-group {
    width: 100%;
  }
}
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/shows/+page.svelte
git commit -m "feat: shows list responsive with collapsible filters on mobile"
```

---

### Task 4: Dashboard Responsive

**Covers:** Dashboard charts and stats on mobile

**Files:**
- Modify: `frontend/src/routes/dashboard/+page.svelte`

- [ ] **Step 1: Update responsive CSS**

Add to the existing `<style>`:
```css
@media (max-width: 768px) {
  .dashboard {
    padding: 0;
  }

  h1 {
    font-size: 20px;
    margin-bottom: 16px;
  }

  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
  }

  .stat-card {
    padding: 14px 10px;
  }

  .stat-value {
    font-size: 22px;
  }

  .charts-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .chart-card {
    padding: 16px;
  }

  .chart-container {
    height: 200px;
  }

  .lists-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .list-card {
    padding: 16px;
  }
}

@media (max-width: 480px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
  }

  .stat-card {
    padding: 12px 8px;
  }

  .stat-value {
    font-size: 20px;
  }

  .stat-label {
    font-size: 11px;
  }
}
```

- [ ] **Step 2: Fix doughnut chart legend for mobile**

Update the `drawCategoryChart` function to change legend position on mobile:
```javascript
function drawCategoryChart() {
  const canvas = document.getElementById('categoryChart');
  if (!canvas || !data.by_category?.length) return;
  const ctx = canvas.getContext('2d');
  const isMobile = window.innerWidth <= 768;

  if (categoryChart) categoryChart.destroy();
  categoryChart = new Chart(ctx, {
    type: 'doughnut',
    data: {
      labels: data.by_category.map(c => c.name),
      datasets: [{
        data: data.by_category.map(c => c.count),
        backgroundColor: data.by_category.map(c => c.color)
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: isMobile ? 'bottom' : 'right',
          labels: { padding: isMobile ? 8 : 12, usePointStyle: true, font: { size: isMobile ? 11 : 13 } }
        }
      }
    }
  });
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/dashboard/+page.svelte
git commit -m "feat: dashboard responsive with stacked charts on mobile"
```

---

### Task 5: Settings Page Responsive

**Covers:** Settings form layout on mobile

**Files:**
- Modify: `frontend/src/routes/settings/+page.svelte`

- [ ] **Step 1: Add responsive CSS**

Add to the existing `<style>`:
```css
@media (max-width: 768px) {
  .settings-page {
    padding: 0;
  }

  h1 {
    font-size: 20px;
  }

  .section {
    padding: 16px;
    margin-bottom: 12px;
  }

  .form-group select, .form-group input {
    max-width: 100%;
  }

  .s3-form .form-row {
    flex-direction: column;
    gap: 0;
  }

  .categories-list {
    gap: 0;
  }

  .cat-item {
    padding: 8px 0;
  }

  .add-cat {
    flex-wrap: wrap;
  }

  .add-cat input[type="text"] {
    flex: 1 1 100%;
  }

  .backup-actions {
    flex-direction: column;
    align-items: stretch;
  }

  .btn-backup, .btn-restore {
    text-align: center;
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/routes/settings/+page.svelte
git commit -m "feat: settings page responsive for mobile"
```

---

### Task 6: Show Detail Page Responsive

**Covers:** Show detail layout on mobile

**Files:**
- Modify: `frontend/src/routes/shows/[id]/+page.svelte`

- [ ] **Step 1: Find and add responsive CSS**

The show detail page has `.detail-header`, `.info-grid`, `.header-actions`. Add:
```css
@media (max-width: 768px) {
  .detail-card {
    padding: 20px 16px;
  }

  .detail-header {
    flex-direction: column;
    gap: 16px;
  }

  .header-info h1 {
    font-size: 22px;
  }

  .header-actions {
    width: 100%;
  }

  .header-actions .edit-btn,
  .header-actions .delete-btn {
    flex: 1;
    text-align: center;
  }

  .info-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/routes/shows/[id]/+page.svelte
git commit -m "feat: show detail responsive for mobile"
```

---

### Task 7: Import Page & ShowForm Responsive

**Covers:** Import page and form layouts on mobile

**Files:**
- Modify: `frontend/src/routes/shows/import/+page.svelte`
- Modify: `frontend/src/lib/components/ShowForm.svelte`

- [ ] **Step 1: Import page responsive CSS**

Add to import page `<style>`:
```css
@media (max-width: 768px) {
  .import-page {
    padding: 0;
  }

  .columns-grid {
    grid-template-columns: 1fr;
  }

  .dropzone {
    padding: 30px 16px;
  }

  .result-stats {
    flex-wrap: wrap;
    gap: 16px;
  }

  .result-actions {
    flex-direction: column;
  }
}
```

- [ ] **Step 2: ShowForm responsive CSS**

Add to ShowForm `<style>`:
```css
@media (max-width: 768px) {
  .show-form {
    padding: 16px;
  }

  .form-row {
    flex-direction: column;
    gap: 0;
  }

  .form-row .form-group {
    margin-bottom: 12px;
  }

  .form-actions {
    flex-direction: column;
    gap: 8px;
  }

  .form-actions button {
    width: 100%;
    text-align: center;
  }
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/shows/import/+page.svelte frontend/src/lib/components/ShowForm.svelte
git commit -m "feat: import page and show form responsive for mobile"
```

---

### Task 8: Calendar Component Responsive

**Covers:** Calendar grid on mobile

**Files:**
- Modify: `frontend/src/lib/components/Calendar.svelte`

- [ ] **Step 1: Add responsive CSS**

Add to Calendar `<style>`:
```css
@media (max-width: 768px) {
  .calendar-header {
    flex-wrap: wrap;
    gap: 8px;
  }

  .title {
    font-size: 16px;
  }

  .day-cell {
    min-height: 60px;
    padding: 2px;
  }

  .day-number {
    font-size: 11px;
  }

  .event-dot {
    font-size: 9px;
    padding: 1px 4px;
  }

  .calendar-legend {
    flex-wrap: wrap;
    gap: 8px;
  }
}

@media (max-width: 480px) {
  .day-cell {
    min-height: 48px;
  }

  .event-text {
    display: none;
  }

  .event-dot {
    width: 6px;
    height: 6px;
    padding: 0;
    border-radius: 50%;
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/lib/components/Calendar.svelte
git commit -m "feat: calendar component responsive for mobile"
```

---

### Task 9: Build and Verify

**Covers:** Final build verification

**Files:**
- None (build only)

- [ ] **Step 1: Build frontend**

```bash
cd frontend && npm install && npm run build
```

- [ ] **Step 2: Copy dist and build backend**

```bash
rm -rf backend/dist && cp -r frontend/dist backend/dist
cd backend && go build -o mujian .
```

- [ ] **Step 3: Test mobile viewport**

Start the server and verify with curl that pages load correctly.

- [ ] **Step 4: Final commit**

```bash
cd .. && git add -A && git commit -m "feat: complete responsive UI redesign for mobile, tablet, desktop"
```
