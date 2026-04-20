<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import {
    startInboxSession, updateInboxDraft, commitInboxDecision,
    skipInboxItem, discardInboxSession, listProjects,
  } from '../api/client';
  import type { InboxSession, InboxStep, ProjectSummary } from '../api/types';

  const dispatch = createEventDispatcher<{ close: void; done: void }>();

  let session: InboxSession | null = null;
  let projects: ProjectSummary[] = [];
  let error = '';
  let loading = true;

  // Per-step local input state
  let waitingOnInput = '';
  let newProjectInput = '';
  let pickedProjectId = '';

  onMount(async () => {
    try {
      const [s, p] = await Promise.all([startInboxSession(), listProjects()]);
      session = s.session;
      projects = p.projects;
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  });

  $: step = session?.current.step as InboxStep | undefined;
  $: draft = session?.current.draft;
  $: progress = session?.progress;
  $: summary = session?.summary;

  async function decide(kind: string, extra: Record<string, string> = {}) {
    if (!session) return;
    try {
      const result = await commitInboxDecision(session.id, { kind: kind as any, ...extra });
      session = result.session;
      // Reset local input state for next item
      waitingOnInput = '';
      newProjectInput = '';
      pickedProjectId = '';
    } catch (e) {
      error = String(e);
    }
  }

  async function skip() {
    if (!session) return;
    try {
      const result = await skipInboxItem(session.id);
      session = result.session;
    } catch (e) {
      error = String(e);
    }
  }

  async function discard() {
    if (!session) return;
    try {
      await discardInboxSession(session.id);
    } catch (e) { /* ignore */ }
    dispatch('close');
  }

  async function updateDraftText(text: string) {
    if (!session) return;
    try {
      const result = await updateInboxDraft(session.id, { text });
      session = result.session;
    } catch (e) {
      error = String(e);
    }
  }

  // Step label for display
  function stepLabel(s?: InboxStep): string {
    switch (s) {
      case 'actionable':     return 'Is this actionable?';
      case 'not_actionable': return 'Not actionable — what to do?';
      case 'enrich':         return 'Enrich the task';
      case 'route':          return 'Where does this go?';
      case 'waiting_on':     return 'Who are you waiting for?';
      case 'new_project':    return 'Name the new project';
      case 'complete':       return 'Processing complete!';
      default:               return '';
    }
  }
</script>

<!-- Backdrop -->
<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
  class="fixed inset-0 bg-black/70 z-50 flex items-center justify-center"
  on:click|self={discard}
>
  <div class="bg-[#1a1a1a] border border-[#333] rounded-xl w-[600px] max-h-[90vh] flex flex-col shadow-2xl">

    <!-- Header with progress -->
    <div class="px-6 py-4 border-b border-[#2a2a2a] flex items-center justify-between">
      <span class="text-[#7c5cbf] font-semibold text-sm">Process Inbox</span>
      {#if progress && !session?.done}
        <span class="text-[#666] text-xs">
          {progress.current} of {progress.total}
        </span>
      {/if}
      <button class="text-[#555] hover:text-[#999] text-lg leading-none" on:click={discard}>✕</button>
    </div>

    <div class="flex-1 overflow-y-auto px-6 py-6">

      {#if loading}
        <div class="text-[#666] text-sm text-center py-8">Starting session…</div>

      {:else if error}
        <div class="text-red-400 text-sm bg-red-900/20 px-4 py-3 rounded">{error}</div>

      {:else if session?.done}
        <!-- ── Complete ─────────────────────────────────────────────────── -->
        <div class="text-center py-6">
          <div class="text-4xl mb-4">✅</div>
          <h2 class="text-lg font-semibold text-[#e8e8e8] mb-6">Inbox processed!</h2>
          {#if summary}
            <div class="grid grid-cols-2 gap-3 text-sm max-w-xs mx-auto text-left mb-8">
              {#if summary.trashed}   <div class="text-[#cc5555]">Trashed</div>    <div class="text-[#e8e8e8]">{summary.trashed}</div>  {/if}
              {#if summary.done}      <div class="text-[#6db36d]">Done (&lt;2min)</div><div class="text-[#e8e8e8]">{summary.done}</div>    {/if}
              {#if summary.someday}   <div class="text-[#6699bb]">Someday</div>    <div class="text-[#e8e8e8]">{summary.someday}</div>   {/if}
              {#if summary.waiting}   <div class="text-[#d4a843]">Waiting</div>    <div class="text-[#e8e8e8]">{summary.waiting}</div>   {/if}
              {#if summary.refiled}   <div class="text-[#9575d4]">Actions</div>    <div class="text-[#e8e8e8]">{summary.refiled}</div>   {/if}
              {#if summary.to_project}<div class="text-[#9575d4]">To projects</div><div class="text-[#e8e8e8]">{summary.to_project}</div>{/if}
              {#if summary.skipped}   <div class="text-[#666]">Skipped</div>      <div class="text-[#e8e8e8]">{summary.skipped}</div>   {/if}
            </div>
          {/if}
          <button
            class="px-6 py-2 bg-[#7c5cbf] hover:bg-[#9575d4] text-white rounded"
            on:click={() => dispatch('done')}
          >Done</button>
        </div>

      {:else if session && draft}
        <!-- ── Current item ───────────────────────────────────────────── -->

        <!-- Item card -->
        <div class="bg-[#242424] border border-[#333] rounded-lg p-4 mb-6">
          <div class="text-xs text-[#666] mb-2">{stepLabel(step)}</div>
          <div class="text-[#e8e8e8] text-base leading-snug">{draft.text}</div>
          {#if draft.deadline}
            <div class="mt-2 text-xs text-[#d4a843]">due {new Date(draft.deadline).toLocaleDateString()}</div>
          {/if}
          {#if draft.tags?.length}
            <div class="mt-2 flex gap-1">
              {#each draft.tags as tag}
                <span class="text-xs bg-[#2a2a3a] text-[#9575d4] px-2 py-0.5 rounded">{tag}</span>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Step-specific actions -->
        {#if step === 'actionable'}
          <div class="space-y-3">
            <p class="text-[#999] text-sm">Does this require action?</p>
            <div class="flex gap-3">
              <button
                class="flex-1 py-3 bg-[#2a3a2a] text-[#6db36d] border border-[#3a5a3a] rounded-lg
                       hover:bg-[#3a4a3a] text-sm font-medium"
                on:click={() => decide('single_action')}
              >
                ✓ Yes, it's actionable
              </button>
              <button
                class="flex-1 py-3 bg-[#3a2a2a] text-[#cc5555] border border-[#5a3a3a] rounded-lg
                       hover:bg-[#4a3a3a] text-sm font-medium"
                on:click={() => decide('trash')}
              >
                ✕ No, trash it
              </button>
            </div>
            <div class="flex gap-3">
              <button
                class="flex-1 py-2 bg-[#242424] text-[#6699bb] border border-[#333] rounded-lg
                       hover:bg-[#2a2a2a] text-sm"
                on:click={() => decide('someday')}
              >☁ Someday / Maybe</button>
              <button
                class="flex-1 py-2 bg-[#242424] text-[#666] border border-[#333] rounded-lg
                       hover:bg-[#2a2a2a] text-sm"
                on:click={skip}
              >→ Skip for now</button>
            </div>
          </div>

        {:else if step === 'enrich'}
          <!-- Quick enrich: edit text inline -->
          <div class="space-y-3">
            <p class="text-[#999] text-sm">Clarify or enrich this task, then choose where it goes:</p>
            <textarea
              class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                     text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf] resize-none"
              rows="2"
              value={draft.text}
              on:change={(e) => updateDraftText((e.target as HTMLTextAreaElement).value)}
            ></textarea>
            <div class="flex gap-3">
              <button
                class="flex-1 py-2 bg-[#242424] text-[#9575d4] border border-[#333] rounded-lg
                       hover:bg-[#2a2a3a] text-sm"
                on:click={() => decide('single_action')}
              >⚡ Single Action</button>
              <button
                class="flex-1 py-2 bg-[#242424] text-[#6db36d] border border-[#333] rounded-lg
                       hover:bg-[#2a3a2a] text-sm"
                on:click={() => decide('done')}
              >✓ Done (&lt;2 min)</button>
              <button
                class="flex-1 py-2 bg-[#242424] text-[#d4a843] border border-[#333] rounded-lg
                       hover:bg-[#3a3a2a] text-sm"
                on:click={() => { /* move to waiting_on step handled by route */ decide('waiting') }}
              >⏳ Waiting</button>
            </div>
          </div>

        {:else if step === 'route'}
          <div class="space-y-3">
            <p class="text-[#999] text-sm">Where does this task go?</p>
            <div class="grid grid-cols-2 gap-3">
              <button
                class="py-3 bg-[#2a2a3a] text-[#9575d4] border border-[#3a3a5a] rounded-lg
                       hover:bg-[#3a3a4a] text-sm"
                on:click={() => decide('single_action')}
              >⚡ Single Actions</button>
              <button
                class="py-3 bg-[#2a3a2a] text-[#6db36d] border border-[#3a5a3a] rounded-lg
                       hover:bg-[#3a4a3a] text-sm"
                on:click={() => decide('done')}
              >✓ Done (&lt;2 min)</button>
              <button
                class="py-3 bg-[#3a3a2a] text-[#d4a843] border border-[#5a5a3a] rounded-lg
                       hover:bg-[#4a4a3a] text-sm"
                on:click={() => decide('waiting')}
              >⏳ Waiting For…</button>
              <button
                class="py-3 bg-[#242444] text-[#6699bb] border border-[#333366] rounded-lg
                       hover:bg-[#2a2a55] text-sm"
                on:click={() => decide('someday')}
              >☁ Someday / Maybe</button>
            </div>

            <!-- Project picker -->
            <div class="pt-2 border-t border-[#2a2a2a]">
              <p class="text-xs text-[#666] mb-2">Or refile to a project:</p>
              <div class="flex gap-2">
                <select
                  bind:value={pickedProjectId}
                  class="flex-1 bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                         text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
                >
                  <option value="">Select project…</option>
                  {#each projects as p}
                    <option value={p.id}>{p.title}</option>
                  {/each}
                </select>
                <button
                  class="px-4 py-2 bg-[#7c5cbf] text-white text-sm rounded disabled:opacity-40"
                  disabled={!pickedProjectId}
                  on:click={() => decide('project', { project_id: pickedProjectId })}
                >→ Project</button>
              </div>
              <button
                class="mt-2 w-full py-2 border border-dashed border-[#333] text-[#666]
                       hover:border-[#555] hover:text-[#999] rounded text-xs"
                on:click={() => { pickedProjectId = ''; decide('new_project', { project_title: '(new)' }) }}
              >+ New project…</button>
            </div>
          </div>

        {:else if step === 'waiting_on'}
          <div class="space-y-3">
            <p class="text-[#999] text-sm">Who or what are you waiting for?</p>
            <input
              type="text"
              bind:value={waitingOnInput}
              placeholder="e.g. John re: budget approval"
              autofocus
              class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                     text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
              on:keydown={(e) => e.key === 'Enter' && decide('waiting', { waiting_on: waitingOnInput })}
            />
            <button
              class="w-full py-2 bg-[#7c5cbf] hover:bg-[#9575d4] text-white rounded text-sm
                     disabled:opacity-40"
              disabled={!waitingOnInput.trim()}
              on:click={() => decide('waiting', { waiting_on: waitingOnInput })}
            >Set Waiting For →</button>
          </div>

        {:else if step === 'new_project'}
          <div class="space-y-3">
            <p class="text-[#999] text-sm">Name the new project:</p>
            <input
              type="text"
              bind:value={newProjectInput}
              placeholder="Project title…"
              autofocus
              class="w-full bg-[#242424] border border-[#333] rounded px-3 py-2 text-sm
                     text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
              on:keydown={(e) => e.key === 'Enter' && decide('new_project', { project_title: newProjectInput })}
            />
            <button
              class="w-full py-2 bg-[#7c5cbf] hover:bg-[#9575d4] text-white rounded text-sm
                     disabled:opacity-40"
              disabled={!newProjectInput.trim()}
              on:click={() => decide('new_project', { project_title: newProjectInput })}
            >Create project + refile →</button>
          </div>
        {/if}

        <!-- Skip/quit footer -->
        {#if step !== 'complete'}
          <div class="flex justify-between mt-6 pt-4 border-t border-[#222]">
            <button class="text-xs text-[#555] hover:text-[#999]" on:click={skip}>Skip →</button>
            <button class="text-xs text-[#555] hover:text-[#999]" on:click={discard}>Quit processing</button>
          </div>
        {/if}

      {/if}
    </div>
  </div>
</div>
