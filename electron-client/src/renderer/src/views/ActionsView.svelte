<script lang="ts">
  import { onMount } from 'svelte';
  import { actionsVersion } from '../stores/index';
  import { listActions, updateTask, archiveTask, trashTask } from '../api/client';
  import type { Task } from '../api/types';
  import TaskRow from '../components/TaskRow.svelte';
  import TaskDetail from '../components/TaskDetail.svelte';

  let tasks: Task[] = [];
  let loading = true;
  let error = '';
  let selectedId: string | null = null;
  let detailTask: Task | null = null;

  async function load() {
    try {
      tasks = await listActions();
      error = '';
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);
  $: if ($actionsVersion) load();

  async function markDone(id: string) {
    try { await updateTask(id, { state: 'done' }); await load(); }
    catch (e) { error = String(e); }
  }

  async function archive(id: string) {
    try { await archiveTask(id); await load(); }
    catch (e) { error = String(e); }
  }

  async function trash(id: string) {
    try { await trashTask(id); await load(); }
    catch (e) { error = String(e); }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (detailTask) return;
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
    if (e.metaKey || e.ctrlKey) return;
    const idx = tasks.findIndex(t => t.id === selectedId);
    switch (e.key) {
      case 'j': case 'ArrowDown':
        e.preventDefault();
        selectedId = tasks[Math.min(idx + 1, tasks.length - 1)]?.id ?? null;
        break;
      case 'k': case 'ArrowUp':
        e.preventDefault();
        selectedId = tasks[Math.max(idx - 1, 0)]?.id ?? null;
        break;
      case 'e': case 'Enter':
        if (idx >= 0) { e.preventDefault(); detailTask = tasks[idx]; }
        break;
      case 'd':
        if (idx >= 0) { e.preventDefault(); markDone(tasks[idx].id); }
        break;
      case 'x':
        if (idx >= 0) { e.preventDefault(); trash(tasks[idx].id); }
        break;
      case 'A':
        if (idx >= 0) { e.preventDefault(); archive(tasks[idx].id); }
        break;
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if detailTask}
  <TaskDetail
    task={detailTask}
    on:close={() => { detailTask = null; load(); }}
    on:saved={() => { detailTask = null; load(); }}
    on:trashed={() => { detailTask = null; load(); }}
  />
{/if}

<div class="flex flex-col h-full">
  <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-4">
    <h1 class="text-lg font-semibold text-[#e8e8e8]">Single Actions</h1>
    <span class="text-[#666] text-sm">{tasks.length} item{tasks.length !== 1 ? 's' : ''}</span>
  </header>

  <div class="flex-1 overflow-y-auto">
    {#if loading}
      <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
    {:else if error}
      <div class="px-6 py-4 text-red-400 text-sm">{error}</div>
    {:else if tasks.length === 0}
      <div class="px-6 py-8 text-[#555] text-sm">No single actions. Refile inbox items here via Process Inbox.</div>
    {:else}
      {#each tasks as task (task.id)}
        <TaskRow
          {task}
          selected={selectedId === task.id}
          on:select={() => selectedId = task.id}
          on:open={() => detailTask = task}
          on:done={() => markDone(task.id)}
          on:archive={() => archive(task.id)}
          on:trash={() => trash(task.id)}
        />
      {/each}
    {/if}
  </div>

  {#if tasks.length > 0}
    <div class="px-6 py-2 border-t border-[#2a2a2a] text-xs text-[#555]">
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">j/k</kbd> navigate</span>
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">e</kbd> edit</span>
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">d</kbd> done</span>
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">A</kbd> archive</span>
      <span><kbd class="bg-[#242424] px-1 rounded">x</kbd> trash</span>
    </div>
  {/if}
</div>
