<script lang="ts">
  import { onMount } from 'svelte';
  import { inboxVersion } from '../stores/index';
  import { listInbox, captureToInbox, trashTask, archiveTask, updateTask } from '../api/client';
  import type { Task } from '../api/types';
  import TaskRow from '../components/TaskRow.svelte';

  let tasks: Task[] = [];
  let loading = true;
  let error = '';
  let addText = '';
  let addingTask = false;
  let selectedId: string | null = null;

  async function load() {
    try {
      tasks = await listInbox();
      error = '';
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  // Reload whenever inbox version bumps (SSE event).
  $: if ($inboxVersion) load();

  async function addTask() {
    const text = addText.trim();
    if (!text) return;
    addText = '';
    try {
      await captureToInbox({ text });
      await load();
    } catch (e) {
      error = String(e);
    }
  }

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

  function handleKeydown(e: KeyboardEvent) {
    if (e.target instanceof HTMLInputElement) return;
    if (e.key === 'a') {
      document.getElementById('inbox-add-input')?.focus();
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

<div class="flex flex-col h-full">
  <!-- Header -->
  <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-4">
    <h1 class="text-lg font-semibold text-[#e8e8e8]">Inbox</h1>
    <span class="text-[#666] text-sm">{tasks.length} item{tasks.length !== 1 ? 's' : ''}</span>
  </header>

  <!-- Add task -->
  <div class="px-6 py-3 border-b border-[#2a2a2a]">
    <form on:submit|preventDefault={addTask} class="flex gap-2">
      <input
        id="inbox-add-input"
        type="text"
        bind:value={addText}
        placeholder="Add to inbox… (press a)"
        class="flex-1 bg-[#242424] border border-[#333] rounded px-3 py-1.5 text-sm
               text-[#e8e8e8] placeholder-[#555] focus:outline-none focus:border-[#7c5cbf]"
      />
      <button
        type="submit"
        class="px-3 py-1.5 bg-[#7c5cbf] hover:bg-[#9575d4] text-white text-sm rounded
               transition-colors disabled:opacity-50"
        disabled={!addText.trim()}
      >
        Add
      </button>
    </form>
  </div>

  <!-- Task list -->
  <div class="flex-1 overflow-y-auto">
    {#if loading}
      <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
    {:else if error}
      <div class="px-6 py-4 text-red-400 text-sm">{error}</div>
    {:else if tasks.length === 0}
      <div class="px-6 py-8 text-[#555] text-sm">
        Inbox is empty. Press <kbd class="bg-[#242424] px-1 rounded">a</kbd> to add a task.
      </div>
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
