<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { updateTask, moveTaskToList, moveTaskToProject, archiveTask, trashTask } from '../api/client';
  import type { Task, TaskState } from '../api/types';

  export let task: Task;

  const dispatch = createEventDispatcher<{ close: void; saved: Task; trashed: void }>();

  // Working copy — mutations only go to the server on Save.
  let draft = { ...task, tags: [...(task.tags ?? [])] };
  let saving = false;
  let error = '';

  // Refile picker state
  let showRefilePicker = false;
  let refileProjectId = '';
  let refileSubgroupId = '';

  const STATES: { value: TaskState; label: string }[] = [
    { value: '',              label: 'Unprocessed' },
    { value: 'next-action',   label: 'Next Action' },
    { value: 'waiting-for',   label: 'Waiting For' },
    { value: 'some-day/maybe',label: 'Someday / Maybe' },
    { value: 'done',          label: 'Done' },
    { value: 'canceled',      label: 'Canceled' },
  ];

  async function save() {
    saving = true;
    error = '';
    try {
      const patch: Record<string, any> = {
        text:       draft.text,
        state:      draft.state,
        url:        draft.url ?? '',
        notes:      draft.notes ?? '',
        waiting_on: draft.waiting_on ?? '',
        tags:       draft.tags,
      };
      // Handle date clears
      const clear: string[] = [];
      if (!draft.deadline)  clear.push('deadline');
      else patch.deadline   = draft.deadline;
      if (!draft.scheduled) clear.push('scheduled');
      else patch.scheduled  = draft.scheduled;
      if (clear.length) patch.clear = clear;

      await updateTask(task.id, patch);
      dispatch('saved', draft as Task);
    } catch (e) {
      error = String(e);
    } finally {
      saving = false;
    }
  }

  async function doArchive() {
    await archiveTask(task.id);
    dispatch('close');
  }

  async function doTrash() {
    if (!confirm('Permanently delete this task?')) return;
    await trashTask(task.id);
    dispatch('trashed');
  }

  function addTag(e: KeyboardEvent) {
    if (e.key !== 'Enter' && e.key !== ',') return;
    e.preventDefault();
    const input = e.target as HTMLInputElement;
    const val = input.value.trim();
    if (val && !draft.tags.includes(val)) {
      draft.tags = [...draft.tags, val];
    }
    input.value = '';
  }

  function removeTag(tag: string) {
    draft.tags = draft.tags.filter(t => t !== tag);
  }

  function formatDateInput(iso?: string): string {
    if (!iso) return '';
    return iso.substring(0, 10); // YYYY-MM-DD for <input type="date">
  }

  function parseDateInput(val: string): string | undefined {
    return val ? val : undefined;
  }
</script>

<!-- Backdrop -->
<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
  class="fixed inset-0 bg-black/60 z-40 flex items-center justify-center"
  on:click|self={() => dispatch('close')}
>
  <div class="bg-[#1e1e1e] border border-[#333] rounded-lg w-[560px] max-h-[85vh] flex flex-col shadow-2xl z-50">
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-4 border-b border-[#2a2a2a]">
      <span class="text-sm text-[#999]">Task Detail</span>
      <button class="text-[#666] hover:text-[#999] text-lg leading-none" on:click={() => dispatch('close')}>✕</button>
    </div>

    <!-- Scrollable body -->
    <div class="flex-1 overflow-y-auto px-5 py-4 space-y-4">

      {#if error}
        <div class="text-red-400 text-sm bg-red-900/20 px-3 py-2 rounded">{error}</div>
      {/if}

      <!-- Text -->
      <div>
        <label for="td-text" class="block text-xs text-[#666] mb-1">Task</label>
        <textarea id="td-text"
          bind:value={draft.text}
          rows="2"
          class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                 text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf] resize-none"
        ></textarea>
      </div>

      <!-- State -->
      <div>
        <label for="td-state" class="block text-xs text-[#666] mb-1">State</label>
        <select id="td-state"
          bind:value={draft.state}
          class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                 text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
        >
          {#each STATES as s}
            <option value={s.value}>{s.label}</option>
          {/each}
        </select>
      </div>

      <!-- Waiting on (shown when state is waiting-for) -->
      {#if draft.state === 'waiting-for'}
        <div>
          <label for="td-waiting" class="block text-xs text-[#666] mb-1">Waiting on</label>
          <input
            id="td-waiting"
            type="text"
            bind:value={draft.waiting_on}
            placeholder="Who or what are you waiting for?"
            class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                   text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
          />
        </div>
      {/if}

      <!-- Dates row -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="td-deadline" class="block text-xs text-[#666] mb-1">Deadline</label>
          <input
            id="td-deadline"
            type="date"
            value={formatDateInput(draft.deadline)}
            on:change={(e) => draft.deadline = parseDateInput((e.target as HTMLInputElement).value)}
            class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                   text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
          />
        </div>
        <div>
          <label for="td-scheduled" class="block text-xs text-[#666] mb-1">Scheduled</label>
          <input
            id="td-scheduled"
            type="date"
            value={formatDateInput(draft.scheduled)}
            on:change={(e) => draft.scheduled = parseDateInput((e.target as HTMLInputElement).value)}
            class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                   text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
          />
        </div>
      </div>

      <!-- Tags -->
      <div>
        <label for="td-tag-input" class="block text-xs text-[#666] mb-1">Tags</label>
        <div class="flex flex-wrap gap-1 mb-1">
          {#each draft.tags as tag}
            <span class="flex items-center gap-1 bg-[#2a2a3a] text-[#9575d4] px-2 py-0.5 rounded text-xs">
              {tag}
              <button class="hover:text-white leading-none" on:click={() => removeTag(tag)}>✕</button>
            </span>
          {/each}
        </div>
        <input
          type="text"
          placeholder="Add tag, press Enter or comma"
          id="td-tag-input"
          on:keydown={addTag}
          class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                 text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
        />
      </div>

      <!-- URL -->
      <div>
        <label for="td-url" class="block text-xs text-[#666] mb-1">URL</label>
        <input
          id="td-url"
          type="url"
          bind:value={draft.url}
          placeholder="https://…"
          class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                 text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
        />
      </div>

      <!-- Notes -->
      <div>
        <label for="td-notes" class="block text-xs text-[#666] mb-1">Notes</label>
        <textarea
          id="td-notes"
          bind:value={draft.notes}
          rows="4"
          placeholder="Free-form notes (Markdown supported)"
          class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                 text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf] resize-none font-mono"
        ></textarea>
      </div>

      <!-- Read-only metadata -->
      <div class="text-xs text-[#555] space-y-0.5 pt-2 border-t border-[#2a2a2a]">
        <div>ID: <span class="font-mono">{task.id}</span></div>
        <div>Created: {new Date(task.created).toLocaleString()}</div>
        {#if task.modified_at}
          <div>Modified: {new Date(task.modified_at).toLocaleString()}</div>
        {/if}
        {#if task.archived_at}
          <div>Archived: {new Date(task.archived_at).toLocaleString()}</div>
        {/if}
        {#if task.source}
          <div>Source: {task.source}</div>
        {/if}
      </div>
    </div>

    <!-- Footer actions -->
    <div class="flex items-center justify-between px-5 py-3 border-t border-[#2a2a2a]">
      <div class="flex gap-2">
        <button
          class="px-3 py-1.5 text-xs rounded bg-[#2a2a3a] text-[#7c5cbf] hover:bg-[#3a3a4a]"
          on:click={doArchive}
        >Archive</button>
        <button
          class="px-3 py-1.5 text-xs rounded bg-[#3a2a2a] text-[#cc5555] hover:bg-[#4a3a3a]"
          on:click={doTrash}
        >Trash</button>
      </div>
      <div class="flex gap-2">
        <button
          class="px-3 py-1.5 text-xs rounded text-[#666] hover:text-[#999]"
          on:click={() => dispatch('close')}
        >Cancel</button>
        <button
          class="px-4 py-1.5 text-xs rounded bg-[#7c5cbf] hover:bg-[#9575d4] text-white
                 disabled:opacity-50"
          disabled={saving}
          on:click={save}
        >{saving ? 'Saving…' : 'Save'}</button>
      </div>
    </div>
  </div>
</div>
