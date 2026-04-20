<script lang="ts">
  import { onMount } from 'svelte';
  import { currentView, navigate } from '../stores/index';
  import { listViews, runView, weeklyReview } from '../api/client';
  import type { SavedView, ViewTask, WeeklyReviewData } from '../api/types';
  import TaskRow from '../components/TaskRow.svelte';

  let views: SavedView[] = [];
  let activeView: SavedView | null = null;
  let viewResults: ViewTask[] = [];
  let reviewData: WeeklyReviewData | null = null;
  let loading = true;
  let resultsLoading = false;
  let error = '';
  let selectedId: string | null = null;

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
    navigate('views');
    try {
      const data = await runView(view.name);
      viewResults = data.tasks;
    } catch (e) {
      error = String(e);
    } finally {
      resultsLoading = false;
    }
  }

  async function openWeeklyReview() {
    navigate('weekly-review');
    resultsLoading = true;
    try {
      reviewData = await weeklyReview();
    } catch (e) {
      error = String(e);
    } finally {
      resultsLoading = false;
    }
  }
</script>

<div class="flex flex-col h-full">
  {#if $currentView === 'views' && !activeView}
    <!-- View list -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-4">
      <h1 class="text-lg font-semibold text-[#e8e8e8]">Views</h1>
    </header>

    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
      {:else if error}
        <div class="px-6 py-4 text-red-400 text-sm">{error}</div>
      {:else}
        <!-- Weekly Review special entry -->
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <div
          class="px-6 py-3 border-b border-[#222] hover:bg-[#1e1e1e] cursor-pointer flex items-center gap-3"
          on:click={openWeeklyReview}
        >
          <span class="text-base">📋</span>
          <span class="text-sm text-[#e8e8e8]">Weekly Review</span>
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
    <!-- View results -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <button
        class="text-[#7c5cbf] hover:text-[#9575d4] text-sm"
        on:click={() => { activeView = null; viewResults = []; }}
      >← Views</button>
      <h1 class="text-lg font-semibold text-[#e8e8e8]">{activeView.name}</h1>
      <span class="text-[#666] text-sm">{viewResults.length} result{viewResults.length !== 1 ? 's' : ''}</span>
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
            on:done={() => {}}
            on:archive={() => {}}
            on:trash={() => {}}
          />
        {/each}
      {/if}
    </div>

  {:else if $currentView === 'weekly-review'}
    <!-- Weekly Review stub -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <button
        class="text-[#7c5cbf] hover:text-[#9575d4] text-sm"
        on:click={() => { reviewData = null; navigate('views'); }}
      >← Views</button>
      <h1 class="text-lg font-semibold text-[#e8e8e8]">Weekly Review</h1>
    </header>

    <div class="flex-1 overflow-y-auto px-6 py-4">
      {#if resultsLoading}
        <div class="text-[#666] text-sm">Loading…</div>
      {:else if reviewData}
        <div class="mb-6">
          <h2 class="text-sm font-semibold text-[#999] uppercase tracking-wider mb-2">
            Projects Missing Next Action ({reviewData.projects_without_next_action.length})
          </h2>
          {#each reviewData.projects_without_next_action as proj}
            <div class="py-2 border-b border-[#222] text-sm text-[#e8e8e8]">{proj.title}</div>
          {/each}
          {#if reviewData.projects_without_next_action.length === 0}
            <div class="text-[#555] text-sm">All projects have next actions ✓</div>
          {/if}
        </div>

        <div class="mb-6">
          <h2 class="text-sm font-semibold text-[#999] uppercase tracking-wider mb-2">
            Aging Waiting For ({reviewData.aging_waiting_for.length})
          </h2>
          {#each reviewData.aging_waiting_for as vt}
            <div class="py-2 border-b border-[#222] text-sm text-[#e8e8e8]">{vt.task.text}</div>
          {/each}
          {#if reviewData.aging_waiting_for.length === 0}
            <div class="text-[#555] text-sm">No aging waiting-for items ✓</div>
          {/if}
        </div>

        <div class="mb-6">
          <h2 class="text-sm font-semibold text-[#999] uppercase tracking-wider mb-2">
            Someday / Maybe ({reviewData.someday_maybe.length})
          </h2>
          {#each reviewData.someday_maybe as vt}
            <div class="py-2 border-b border-[#222] text-sm text-[#e8e8e8]">{vt.task.text}</div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>
