<script lang="ts">
  import { onMount } from 'svelte';
  import { actionsVersion } from '../stores/index';
  import { listActions, updateTask, archiveTask, trashTask } from '../api/client';
  import type { Task } from '../api/types';
  import TaskRow from '../components/TaskRow.svelte';

  let tasks: Task[] = [];
  let loading = true;
  let error = '';
  let selectedId: string | null = null;

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
    await updateTask(id, { state: 'done' });
    await load();
  }

  async function archive(id: string) {
    await archiveTask(id);
    await load();
  }

  async function trash(id: string) {
    await trashTask(id);
    await load();
  }
</script>

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
      <div class="px-6 py-8 text-[#555] text-sm">No single actions yet.</div>
    {:else}
      {#each tasks as task (task.id)}
        <TaskRow
          {task}
          selected={selectedId === task.id}
          on:select={() => selectedId = task.id}
          on:done={() => markDone(task.id)}
          on:archive={() => archive(task.id)}
          on:trash={() => trash(task.id)}
        />
      {/each}
    {/if}
  </div>
</div>
