<script lang="ts">
  import { onMount } from 'svelte';
  import { currentView, navigate } from '../stores/index';
  import { listViews, runView, runQuery, weeklyReview, updateTask, archiveTask, trashTask, restoreTask } from '../api/client';
  import type { SavedView, ViewTask, WeeklyReviewData } from '../api/types';
  import TaskRow from '../components/TaskRow.svelte';
  import TaskDetail from '../components/TaskDetail.svelte';

  let views: SavedView[] = [];
  let activeView: SavedView | null = null;
  let viewResults: ViewTask[] = [];
  let reviewData: WeeklyReviewData | null = null;
  let loading = true;
  let resultsLoading = false;
  let error = '';
  let selectedId: string | null = null;
  let detailTask: { task: ViewTask } | null = null;

  // Ad-hoc query
  let showQueryInput = false;
  let queryText = '';

  onMount(async () => {
    try {
      const data = await listViews();
      views = data.views;
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  });

  async function openView(view: SavedView) {
    activeView = view;
    resultsLoading = true;
    selectedId = null;
    navigate('views');
    try {
      const data = await runView(view.name);
      viewResults = data.tasks;
    } catch (e) { error = String(e); }
    finally { resultsLoading = false; }
  }

  async function doQuery() {
    const q = queryText.trim();
    if (!q) return;
    activeView = { name: `Query: ${q}`, query: q, include_archived: false };
    showQueryInput = false;
    resultsLoading = true;
    selectedId = null;
    try {
      const data = await runQuery(q);
      viewResults = data.tasks;
    } catch (e) { error = String(e); }
    finally { resultsLoading = false; }
  }

  async function openWeeklyReview() {
    navigate('weekly-review');
    activeView = null;
    resultsLoading = true;
    try {
      reviewData = await weeklyReview();
    } catch (e) { error = String(e); }
    finally { resultsLoading = false; }
  }

  async function refreshResults() {
    if (!activeView) return;
    resultsLoading = true;
    try {
      const data = activeView.query.startsWith('Query:')
        ? await runQuery(activeView.query.replace(/^Query:\s*/, ''))
        : await runView(activeView.name);
      viewResults = data.tasks;
    } catch (e) { error = String(e); }
    finally { resultsLoading = false; }
  }

  async function markDone(vt: ViewTask) {
    try { await updateTask(vt.task.id, { state: 'done' }); await refreshResults(); }
    catch (e) { error = String(e); }
  }

  async function archive(vt: ViewTask) {
    try { await archiveTask(vt.task.id); await refreshResults(); }
    catch (e) { error = String(e); }
  }

  async function restore(vt: ViewTask) {
    try { await restoreTask(vt.task.id); await refreshResults(); }
    catch (e) { error = String(e); }
  }

  async function trash(vt: ViewTask) {
    try { await trashTask(vt.task.id); await refreshResults(); }
    catch (e) { error = String(e); }
  }

  function handleKeydown(e: KeyboardEvent) {
    if ($currentView !== 'views' || !activeView) return;
    if (detailTask) return;
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
    if (e.metaKey || e.ctrlKey) return;
    const idx = viewResults.findIndex(vt => vt.task.id === selectedId);
    switch (e.key) {
      case 'j': case 'ArrowDown':
        e.preventDefault();
        selectedId = viewResults[Math.min(idx + 1, viewResults.length - 1)]?.task.id ?? null;
        break;
      case 'k': case 'ArrowUp':
        e.preventDefault();
        selectedId = viewResults[Math.max(idx - 1, 0)]?.task.id ?? null;
        break;
      case 'e': case 'Enter':
        if (idx >= 0) { e.preventDefault(); detailTask = { task: viewResults[idx] }; }
        break;
      case 'R':
        e.preventDefault();
        refreshResults();
        break;
      case 'Escape':
        activeView = null; viewResults = [];
        break;
    }
  }

  // Weekly review section navigation
  type ReviewSection = 'missing-next' | 'aging-waiting' | 'someday' | 'archived';
  let reviewSection: ReviewSection = 'missing-next';

  const reviewSections: { id: ReviewSection; label: string }[] = [
    { id: 'missing-next',  label: 'Missing Next Action' },
    { id: 'aging-waiting', label: 'Aging Waiting For'   },
    { id: 'someday',       label: 'Someday / Maybe'     },
    { id: 'archived',      label: 'Recently Archived'   },
  ];

  $: reviewItems = !reviewData ? [] :
    reviewSection === 'missing-next'  ? [] :  // projects, not tasks
    reviewSection === 'aging-waiting' ? reviewData.aging_waiting_for :
    reviewSection === 'someday'       ? reviewData.someday_maybe :
    reviewSection === 'archived'      ? reviewData.recent_archived : [];

  $: reviewProjects = reviewData?.projects_without_next_action ?? [];
</script>

<svelte:window on:keydown={handleKeydown} />

{#if detailTask}
  <TaskDetail
    task={detailTask.task.task}
    on:close={() => { detailTask = null; refreshResults(); }}
    on:saved={() => { detailTask = null; refreshResults(); }}
    on:trashed={() => { detailTask = null; refreshResults(); }}
  />
{/if}

<div class="flex flex-col h-full">

  {#if $currentView === 'views' && !activeView}
    <!-- ── View list ──────────────────────────────────────────────────── -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <h1 class="text-lg font-semibold text-[#e8e8e8]">Views</h1>
      <button
        class="ml-auto px-3 py-1 text-xs border border-[#333] text-[#666] hover:text-[#999] rounded"
        on:click={() => { showQueryInput = !showQueryInput; }}
      >/ Query</button>
    </header>

    {#if showQueryInput}
      <div class="px-6 py-3 border-b border-[#2a2a2a]">
        <form on:submit|preventDefault={doQuery} class="flex gap-2">
          <input
            type="text"
            bind:value={queryText}
            placeholder="e.g. deadline:<7d @work state:next-action"
            autofocus
            class="flex-1 bg-[#242424] border border-[#333] rounded px-3 py-1.5 text-sm
                   text-[#e8e8e8] font-mono placeholder-[#444] focus:outline-none focus:border-[#7c5cbf]"
          />
          <button type="submit" class="px-3 py-1.5 bg-[#7c5cbf] text-white text-sm rounded">Search</button>
          <button type="button" class="px-3 py-1.5 text-[#666] text-sm"
            on:click={() => { showQueryInput = false; queryText = ''; }}>Cancel</button>
        </form>
      </div>
    {/if}

    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
      {:else if error}
        <div class="px-6 py-4 text-red-400 text-sm">{error}</div>
      {:else}
        <!-- Weekly Review -->
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div
          class="px-6 py-3 border-b border-[#222] hover:bg-[#1e1e1e] cursor-pointer flex items-center gap-3"
          on:click={openWeeklyReview}
        >
          <span class="text-base">📋</span>
          <span class="text-sm text-[#e8e8e8]">Weekly Review</span>
          <span class="ml-auto text-xs text-[#555]">W</span>
        </div>

        {#each views as view}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <div
            class="px-6 py-3 border-b border-[#222] hover:bg-[#1e1e1e] cursor-pointer"
            on:click={() => openView(view)}
          >
            <div class="text-sm text-[#e8e8e8]">{view.name}</div>
            {#if view.query}
              <div class="text-xs text-[#555] mt-0.5 font-mono">{view.query}</div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>

  {:else if $currentView === 'views' && activeView}
    <!-- ── View results ───────────────────────────────────────────────── -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <button
        class="text-[#7c5cbf] hover:text-[#9575d4] text-sm flex-shrink-0"
        on:click={() => { activeView = null; viewResults = []; }}
      >← Views</button>
      <h1 class="text-lg font-semibold text-[#e8e8e8] flex-1 truncate">{activeView.name}</h1>
      <span class="text-[#666] text-sm flex-shrink-0">{viewResults.length} result{viewResults.length !== 1 ? 's' : ''}</span>
      <button
        class="text-xs text-[#555] hover:text-[#999] border border-[#333] rounded px-2 py-1"
        on:click={refreshResults}
        title="Refresh (R)"
      >↺</button>
    </header>

    <div class="flex-1 overflow-y-auto">
      {#if resultsLoading}
        <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
      {:else if viewResults.length === 0}
        <div class="px-6 py-8 text-[#555] text-sm">No results.</div>
      {:else}
        {#each viewResults as vt (vt.task.id)}
          <TaskRow
            task={vt.task}
            selected={selectedId === vt.task.id}
            showSource={true}
            on:select={() => selectedId = vt.task.id}
            on:open={() => detailTask = { task: vt }}
            on:done={() => markDone(vt)}
            on:archive={() => vt.is_archived ? restore(vt) : archive(vt)}
            on:trash={() => trash(vt)}
          />
        {/each}
      {/if}
    </div>

    <div class="px-6 py-2 border-t border-[#2a2a2a] text-xs text-[#555]">
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">j/k</kbd> navigate</span>
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">e</kbd> edit</span>
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">R</kbd> refresh</span>
      <span><kbd class="bg-[#242424] px-1 rounded">Esc</kbd> back</span>
    </div>

  {:else if $currentView === 'weekly-review'}
    <!-- ── Weekly Review ──────────────────────────────────────────────── -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <button
        class="text-[#7c5cbf] hover:text-[#9575d4] text-sm"
        on:click={() => { reviewData = null; navigate('views'); }}
      >← Views</button>
      <h1 class="text-lg font-semibold text-[#e8e8e8]">Weekly Review</h1>
    </header>

    <!-- Section tabs -->
    <div class="flex border-b border-[#2a2a2a] overflow-x-auto">
      {#each reviewSections as sec}
        {@const count =
          sec.id === 'missing-next'  ? (reviewData?.projects_without_next_action.length ?? 0) :
          sec.id === 'aging-waiting' ? (reviewData?.aging_waiting_for.length ?? 0) :
          sec.id === 'someday'       ? (reviewData?.someday_maybe.length ?? 0) :
                                       (reviewData?.recent_archived.length ?? 0)}
        <button
          class="px-4 py-3 text-sm flex-shrink-0 border-b-2 transition-colors
                 {reviewSection === sec.id
                   ? 'border-[#7c5cbf] text-[#9575d4]'
                   : 'border-transparent text-[#666] hover:text-[#999]'}"
          on:click={() => reviewSection = sec.id}
        >
          {sec.label}
          {#if count > 0}
            <span class="ml-1 text-xs bg-[#2a2a2a] px-1.5 py-0.5 rounded-full">{count}</span>
          {/if}
        </button>
      {/each}
    </div>

    <div class="flex-1 overflow-y-auto">
      {#if resultsLoading}
        <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
      {:else if reviewSection === 'missing-next'}
        {#if reviewProjects.length === 0}
          <div class="px-6 py-8 text-[#555] text-sm">All projects have next actions ✓</div>
        {:else}
          {#each reviewProjects as proj}
            <div class="px-6 py-3 border-b border-[#222]">
              <div class="text-sm text-[#e8e8e8]">{proj.title}</div>
              <div class="text-xs text-[#666] mt-0.5">{proj.task_count} task{proj.task_count !== 1 ? 's' : ''}</div>
            </div>
          {/each}
        {/if}
      {:else}
        {#if reviewItems.length === 0}
          <div class="px-6 py-8 text-[#555] text-sm">Nothing here ✓</div>
        {:else}
          {#each reviewItems as vt (vt.task.id)}
            <TaskRow
              task={vt.task}
              selected={selectedId === vt.task.id}
              showSource={true}
              on:select={() => selectedId = vt.task.id}
              on:open={() => detailTask = { task: vt }}
              on:done={async () => { await updateTask(vt.task.id, { state: 'done' }); reviewData = await weeklyReview(); }}
              on:archive={async () => {
                if (vt.is_archived) await restoreTask(vt.task.id);
                else await archiveTask(vt.task.id);
                reviewData = await weeklyReview();
              }}
              on:trash={async () => { await trashTask(vt.task.id); reviewData = await weeklyReview(); }}
            />
          {/each}
        {/if}
      {/if}
    </div>
  {/if}
</div>
