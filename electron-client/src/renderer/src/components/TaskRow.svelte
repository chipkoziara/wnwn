<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { Task } from '../api/types';

  export let task: Task;
  export let selected = false;
  export let showSource = false; // for view results

  const dispatch = createEventDispatcher<{
    select: void;
    done: void;
    archive: void;
    trash: void;
    open: void;
  }>();

  $: isOverdue = task.deadline && new Date(task.deadline) < new Date() && task.state !== 'done' && task.state !== 'canceled';
  $: deadlineLabel = task.deadline ? formatDate(task.deadline) : null;
  $: scheduledLabel = task.scheduled ? formatDate(task.scheduled) : null;

  function formatDate(iso: string): string {
    const d = new Date(iso);
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    d.setHours(0, 0, 0, 0);
    const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return 'today';
    if (diff === 1) return 'tomorrow';
    if (diff === -1) return 'yesterday';
    if (diff < 0) return `${-diff}d ago`;
    if (diff <= 7) return `in ${diff}d`;
    return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  function stateColor(state: string): string {
    switch (state) {
      case 'next-action':   return 'text-[#6db36d]';
      case 'waiting-for':   return 'text-[#d4a843]';
      case 'some-day/maybe': return 'text-[#6699bb]';
      case 'done':          return 'text-[#555]';
      case 'canceled':      return 'text-[#555]';
      default:              return 'text-[#999]';
    }
  }

  function stateIcon(state: string): string {
    switch (state) {
      case 'next-action':   return '▶';
      case 'waiting-for':   return '⏳';
      case 'some-day/maybe': return '☁';
      case 'done':          return '✓';
      case 'canceled':      return '✕';
      default:              return '·';
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
  class="group flex items-start gap-3 px-6 py-2.5 cursor-pointer border-b border-[#222]
         {selected ? 'bg-[#242424]' : 'hover:bg-[#1e1e1e]'}"
  on:click={() => dispatch('select')}
  on:dblclick={() => dispatch('open')}
>
  <!-- State icon -->
  <span class="mt-0.5 text-xs w-4 flex-shrink-0 {stateColor(task.state)}">
    {stateIcon(task.state)}
  </span>

  <!-- Task text + metadata -->
  <div class="flex-1 min-w-0">
    <div class="flex items-baseline gap-2">
      <span
        class="text-sm leading-snug
               {task.state === 'done' || task.state === 'canceled' ? 'line-through text-[#555]' : 'text-[#e8e8e8]'}"
      >
        {task.text}
      </span>
      {#if task.url}
        <span class="text-[#666] text-xs flex-shrink-0">🔗</span>
      {/if}
    </div>

    <!-- Metadata row -->
    <div class="flex items-center gap-3 mt-0.5 text-xs text-[#666]">
      {#if showSource}
        <span class="bg-[#242424] px-1.5 py-0.5 rounded text-[#777]">{task.source ?? 'inbox'}</span>
      {/if}
      {#if scheduledLabel}
        <span class="text-[#6699bb]">sched: {scheduledLabel}</span>
      {/if}
      {#if deadlineLabel}
        <span class="{isOverdue ? 'text-[#cc5555]' : 'text-[#d4a843]'}">
          due: {deadlineLabel}
        </span>
      {/if}
      {#if task.tags && task.tags.length > 0}
        {#each task.tags as tag}
          <span class="text-[#666]">{tag}</span>
        {/each}
      {/if}
      {#if task.waiting_on}
        <span class="text-[#d4a843]">⏳ {task.waiting_on}</span>
      {/if}
    </div>
  </div>

  <!-- Action buttons (shown on hover / when selected) -->
  {#if selected}
    <div class="flex items-center gap-1 flex-shrink-0">
      <button
        class="px-2 py-0.5 text-xs rounded bg-[#2a3a2a] text-[#6db36d] hover:bg-[#3a4a3a]"
        on:click|stopPropagation={() => dispatch('done')}
        title="Mark done (d)"
      >done</button>
      <button
        class="px-2 py-0.5 text-xs rounded bg-[#2a2a3a] text-[#7c5cbf] hover:bg-[#3a3a4a]"
        on:click|stopPropagation={() => dispatch('archive')}
        title="Archive (A)"
      >archive</button>
      <button
        class="px-2 py-0.5 text-xs rounded bg-[#3a2a2a] text-[#cc5555] hover:bg-[#4a3a3a]"
        on:click|stopPropagation={() => dispatch('trash')}
        title="Trash (x)"
      >trash</button>
    </div>
  {/if}
</div>
