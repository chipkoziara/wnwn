<script lang="ts">
  import { onMount } from 'svelte';
  import { inboxVersion } from '../stores/index';
  import { listInbox, captureToInbox, trashTask, archiveTask, updateTask, moveTaskToList } from '../api/client';
  import type { Task } from '../api/types';
  import TaskRow from '../components/TaskRow.svelte';
  import TaskDetail from '../components/TaskDetail.svelte';
  import InboxProcessor from '../components/InboxProcessor.svelte';

  let tasks: Task[] = [];
  let loading = true;
  let error = '';
  let addText = '';
  let selectedId: string | null = null;
  let detailTask: Task | null = null;
  let showProcessor = false;

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
  $: if ($inboxVersion) load();

  async function addTask() {
    const text = addText.trim();
    if (!text) return;
    addText = '';
    try {
      await captureToInbox({ text });
      await load();
    } catch (e) { error = String(e); }
  }

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

  async function refileToActions(id: string) {
    try { await moveTaskToList(id, 'single-actions'); await load(); }
    catch (e) { error = String(e); }
  }

  function openDetail(task: Task) { detailTask = task; }

  function handleKeydown(e: KeyboardEvent) {
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
        if (idx >= 0) { e.preventDefault(); openDetail(tasks[idx]); }
        break;
      case 'a':
        document.getElementById('inbox-add-input')?.focus();
        break;
      case 'P':
        showProcessor = true;
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

{#if showProcessor}
  <InboxProcessor
    on:close={() => { showProcessor = false; load(); }}
    on:done={() => { showProcessor = false; load(); }}
  />
{/if}

<div class="flex flex-col h-full">
  <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-4">
    <h1 class="text-lg font-semibold text-[#e8e8e8]">Inbox</h1>
    <span class="text-[#666] text-sm">{tasks.length} item{tasks.length !== 1 ? 's' : ''}</span>
    {#if tasks.length > 0}
      <button
        class="ml-auto px-3 py-1 text-xs bg-[#2a1f44] text-[#9575d4] border border-[#4a3a6a] rounded hover:bg-[#3a2f54]"
        on:click={() => showProcessor = true}
        title="Process Inbox (P)"
      >Process Inbox (P)</button>
    {/if}
  </header>

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
        class="px-3 py-1.5 bg-[#7c5cbf] hover:bg-[#9575d4] text-white text-sm rounded disabled:opacity-50"
        disabled={!addText.trim()}
      >Add</button>
    </form>
  </div>

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
          on:open={() => openDetail(task)}
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
      <span class="mr-4"><kbd class="bg-[#242424] px-1 rounded">x</kbd> trash</span>
      <span><kbd class="bg-[#242424] px-1 rounded">P</kbd> process inbox</span>
    </div>
  {/if}
</div>
