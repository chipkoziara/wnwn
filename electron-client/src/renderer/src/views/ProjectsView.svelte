<script lang="ts">
  import { onMount } from 'svelte';
  import { projectsVersion, currentView, activeProjectId, navigate } from '../stores/index';
  import { listProjects, getProject } from '../api/client';
  import type { ProjectSummary, ProjectLocation } from '../api/types';

  let summaries: ProjectSummary[] = [];
  let activeProject: ProjectLocation | null = null;
  let loading = true;
  let error = '';

  async function load() {
    try {
      const data = await listProjects();
      summaries = data.projects;
      error = '';
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  async function openProject(id: string) {
    try {
      activeProject = await getProject(id);
      navigate('project-detail', id);
    } catch (e) {
      error = String(e);
    }
  }

  onMount(load);
  $: if ($projectsVersion) load();

  function stateLabel(state?: string): string {
    switch (state) {
      case 'active':        return 'Active';
      case 'waiting-for':   return 'Waiting';
      case 'some-day/maybe': return 'Someday';
      case 'done':          return 'Done';
      case 'canceled':      return 'Canceled';
      default:              return 'Active';
    }
  }
</script>

<div class="flex flex-col h-full">
  {#if $currentView === 'projects'}
    <!-- Project list -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-4">
      <h1 class="text-lg font-semibold text-[#e8e8e8]">Projects</h1>
      <span class="text-[#666] text-sm">{summaries.length} project{summaries.length !== 1 ? 's' : ''}</span>
    </header>

    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="px-6 py-8 text-[#666] text-sm">Loading…</div>
      {:else if error}
        <div class="px-6 py-4 text-red-400 text-sm">{error}</div>
      {:else if summaries.length === 0}
        <div class="px-6 py-8 text-[#555] text-sm">No projects yet.</div>
      {:else}
        {#each summaries as proj (proj.id)}
          <!-- svelte-ignore a11y-click-events-have-key-events -->
          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <div
            class="px-6 py-3 border-b border-[#222] hover:bg-[#1e1e1e] cursor-pointer"
            on:click={() => openProject(proj.id)}
          >
            <div class="flex items-center gap-3">
              <span class="text-sm text-[#e8e8e8] flex-1">{proj.title}</span>
              <span class="text-xs text-[#666]">{proj.task_count} task{proj.task_count !== 1 ? 's' : ''}</span>
              <span class="text-xs text-[#666]">{stateLabel(proj.state)}</span>
            </div>
            {#if proj.next_action}
              <div class="mt-0.5 text-xs text-[#666] truncate pl-0">
                Next: {proj.next_action}
              </div>
            {/if}
            {#if proj.deadline}
              <div class="mt-0.5 text-xs text-[#d4a843]">
                Due: {new Date(proj.deadline).toLocaleDateString()}
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>

  {:else if $currentView === 'project-detail' && activeProject}
    <!-- Project detail (stub — full implementation in Phase 2) -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <button
        class="text-[#7c5cbf] hover:text-[#9575d4] text-sm"
        on:click={() => navigate('projects')}
      >← Projects</button>
      <h1 class="text-lg font-semibold text-[#e8e8e8]">{activeProject.project.title}</h1>
    </header>

    <div class="flex-1 overflow-y-auto px-6 py-4">
      {#each activeProject.project.sub_groups as sg}
        <div class="mb-6">
          <h2 class="text-sm font-semibold text-[#999] uppercase tracking-wider mb-2">{sg.title}</h2>
          {#if sg.tasks.length === 0}
            <div class="text-[#555] text-sm pl-2">No tasks</div>
          {:else}
            {#each sg.tasks as task}
              <div class="py-2 border-b border-[#222] text-sm text-[#e8e8e8]">{task.text}</div>
            {/each}
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
