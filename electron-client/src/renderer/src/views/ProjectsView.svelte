<script lang="ts">
  import { onMount } from 'svelte';
  import { projectsVersion, navigate } from '../stores/index';
  import {
    listProjects, getProject, createProject,
    updateTask, archiveTask, trashTask,
    createSubgroup, deleteSubgroup, renameSubgroup,
    addProjectTask, moveTaskToSubgroup, reorderTask,
  } from '../api/client';
  import type { ProjectSummary, ProjectLocation, Task } from '../api/types';
  import TaskDetail from '../components/TaskDetail.svelte';
  import { currentView } from '../stores/index';

  let summaries: ProjectSummary[] = [];
  let activeProject: ProjectLocation | null = null;
  let loading = true;
  let error = '';

  // Detail view state
  let selectedTaskId: string | null = null;
  let detailTask: Task | null = null;

  // New project form
  let showNewProject = false;
  let newProjectTitle = '';

  // New subgroup form
  let addingSubgroupId: string | null = null; // 'root' or subgroup id (unused, just a toggle)
  let newSubgroupTitle = '';

  // Add task form: keyed by subgroup id
  let addingTaskInSubgroup: string | null = null;
  let newTaskText = '';

  // Rename subgroup
  let renamingSubgroupId: string | null = null;
  let renameSubgroupTitle = '';

  async function loadList() {
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

  async function reloadDetail() {
    if (!activeProject) return;
    try {
      activeProject = await getProject(activeProject.project_id);
    } catch (e) {
      error = String(e);
    }
  }

  async function openProject(id: string) {
    try {
      activeProject = await getProject(id);
      navigate('project-detail', id);
      selectedTaskId = null;
    } catch (e) {
      error = String(e);
    }
  }

  onMount(loadList);
  $: if ($projectsVersion) { loadList(); if (activeProject) reloadDetail(); }

  // ── Flat task list for keyboard navigation ──────────────────────────────
  $: flatTasks = activeProject?.project.sub_groups.flatMap(sg =>
    sg.tasks.map(t => ({ task: t, sgId: sg.id }))
  ) ?? [];

  function selectedFlatIdx(): number {
    return flatTasks.findIndex(ft => ft.task.id === selectedTaskId);
  }

  function handleDetailKey(e: KeyboardEvent) {
    if ($currentView !== 'project-detail') return;
    if (detailTask) return; // task detail modal open
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
    if (e.metaKey || e.ctrlKey) return;

    const idx = selectedFlatIdx();

    switch (e.key) {
      case 'j':
      case 'ArrowDown': {
        e.preventDefault();
        const next = idx < flatTasks.length - 1 ? idx + 1 : idx;
        selectedTaskId = flatTasks[next]?.task.id ?? null;
        break;
      }
      case 'k':
      case 'ArrowUp': {
        e.preventDefault();
        const prev = idx > 0 ? idx - 1 : 0;
        selectedTaskId = flatTasks[prev]?.task.id ?? null;
        break;
      }
      case 'e':
      case 'Enter': {
        if (idx >= 0) { e.preventDefault(); openTaskDetail(flatTasks[idx].task); }
        break;
      }
      case 'd': {
        if (idx >= 0) { e.preventDefault(); markTaskDone(flatTasks[idx].task.id); }
        break;
      }
      case 'x': {
        if (idx >= 0) { e.preventDefault(); doTrashTask(flatTasks[idx].task.id); }
        break;
      }
      case 'A': {
        if (idx >= 0) { e.preventDefault(); doArchiveTask(flatTasks[idx].task.id); }
        break;
      }
      case 'Escape':
        navigate('projects');
        activeProject = null;
        break;
    }
  }

  function openTaskDetail(task: Task) {
    detailTask = task;
  }

  async function markTaskDone(id: string) {
    try { await updateTask(id, { state: 'done' }); await reloadDetail(); }
    catch (e) { error = String(e); }
  }

  async function doArchiveTask(id: string) {
    try { await archiveTask(id); await reloadDetail(); }
    catch (e) { error = String(e); }
  }

  async function doTrashTask(id: string) {
    try { await trashTask(id); await reloadDetail(); }
    catch (e) { error = String(e); }
  }

  async function doCreateProject() {
    const title = newProjectTitle.trim();
    if (!title) return;
    try {
      const loc = await createProject(title);
      newProjectTitle = '';
      showNewProject = false;
      await loadList();
      await openProject(loc.location.project_id);
    } catch (e) { error = String(e); }
  }

  async function doAddSubgroup() {
    if (!activeProject || !newSubgroupTitle.trim()) return;
    try {
      await createSubgroup(activeProject.project_id, newSubgroupTitle.trim());
      newSubgroupTitle = '';
      addingSubgroupId = null;
      await reloadDetail();
    } catch (e) { error = String(e); }
  }

  async function doRenameSubgroup(sgId: string) {
    if (!activeProject || !renameSubgroupTitle.trim()) return;
    try {
      await renameSubgroup(activeProject.project_id, sgId, renameSubgroupTitle.trim());
      renamingSubgroupId = null;
      renameSubgroupTitle = '';
      await reloadDetail();
    } catch (e) { error = String(e); }
  }

  async function doDeleteSubgroup(sgId: string) {
    if (!activeProject) return;
    const sg = activeProject.project.sub_groups.find(s => s.id === sgId);
    if (sg && sg.tasks.length > 0) {
      error = 'Cannot delete a non-empty sub-group. Move tasks first.';
      return;
    }
    try {
      await deleteSubgroup(activeProject.project_id, sgId);
      await reloadDetail();
    } catch (e) { error = String(e); }
  }

  async function doAddTask(sgId: string) {
    if (!activeProject || !newTaskText.trim()) return;
    try {
      await addProjectTask(activeProject.project_id, sgId, { text: newTaskText.trim() });
      newTaskText = '';
      addingTaskInSubgroup = null;
      await reloadDetail();
    } catch (e) { error = String(e); }
  }

  async function doReorder(taskId: string, delta: 1 | -1) {
    try {
      await reorderTask(taskId, delta);
      await reloadDetail();
    } catch (e) { error = String(e); }
  }

  function stateLabel(state?: string): string {
    switch (state) {
      case 'active':         return 'Active';
      case 'waiting-for':    return 'Waiting';
      case 'some-day/maybe': return 'Someday';
      case 'done':           return 'Done';
      case 'canceled':       return 'Canceled';
      default:               return 'Active';
    }
  }

  function stateIcon(state?: string): string {
    switch (state) {
      case 'next-action':    return '▶';
      case 'waiting-for':    return '⏳';
      case 'some-day/maybe': return '☁';
      case 'done':           return '✓';
      case 'canceled':       return '✕';
      default:               return '·';
    }
  }
</script>

<svelte:window on:keydown={handleDetailKey} />

{#if detailTask}
  <TaskDetail
    task={detailTask}
    on:close={() => { detailTask = null; reloadDetail(); }}
    on:saved={() => { detailTask = null; reloadDetail(); }}
    on:trashed={() => { detailTask = null; reloadDetail(); }}
  />
{/if}

<div class="flex flex-col h-full">

  {#if $currentView === 'projects'}
    <!-- ── Project list ────────────────────────────────────────────────── -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-4">
      <h1 class="text-lg font-semibold text-[#e8e8e8]">Projects</h1>
      <span class="text-[#666] text-sm">{summaries.length} project{summaries.length !== 1 ? 's' : ''}</span>
      <button
        class="ml-auto px-3 py-1 text-xs bg-[#7c5cbf] hover:bg-[#9575d4] text-white rounded"
        on:click={() => showNewProject = !showNewProject}
      >+ New project</button>
    </header>

    {#if showNewProject}
      <div class="px-6 py-3 border-b border-[#2a2a2a]">
        <form on:submit|preventDefault={doCreateProject} class="flex gap-2">
          <input
            type="text"
            bind:value={newProjectTitle}
            placeholder="Project title…"
            autofocus
            class="flex-1 bg-[#242424] border border-[#333] rounded px-3 py-1.5 text-sm
                   text-[#e8e8e8] placeholder-[#555] focus:outline-none focus:border-[#7c5cbf]"
          />
          <button
            type="submit"
            class="px-3 py-1.5 bg-[#7c5cbf] text-white text-sm rounded disabled:opacity-50"
            disabled={!newProjectTitle.trim()}
          >Create</button>
          <button
            type="button"
            class="px-3 py-1.5 text-[#666] text-sm rounded hover:text-[#999]"
            on:click={() => { showNewProject = false; newProjectTitle = ''; }}
          >Cancel</button>
        </form>
      </div>
    {/if}

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
              <span class="text-xs text-[#555]">{proj.task_count} task{proj.task_count !== 1 ? 's' : ''}</span>
              <span class="text-xs text-[#555]">{stateLabel(proj.state)}</span>
            </div>
            {#if proj.next_action}
              <div class="mt-0.5 text-xs text-[#666] truncate">→ {proj.next_action}</div>
            {/if}
            {#if proj.deadline}
              <div class="mt-0.5 text-xs text-[#d4a843]">due {new Date(proj.deadline).toLocaleDateString()}</div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>

  {:else if $currentView === 'project-detail' && activeProject}
    <!-- ── Project detail ──────────────────────────────────────────────── -->
    <header class="px-6 py-4 border-b border-[#2a2a2a] flex items-center gap-3">
      <button
        class="text-[#7c5cbf] hover:text-[#9575d4] text-sm flex-shrink-0"
        on:click={() => { navigate('projects'); activeProject = null; }}
      >← Projects</button>
      <h1 class="text-lg font-semibold text-[#e8e8e8] flex-1 truncate">{activeProject.project.title}</h1>
      <button
        class="px-2 py-1 text-xs text-[#666] hover:text-[#999] border border-[#333] rounded"
        on:click={() => { addingSubgroupId = 'new'; newSubgroupTitle = ''; }}
        title="Add sub-group"
      >+ group</button>
    </header>

    {#if error}
      <div class="px-6 py-2 text-red-400 text-sm border-b border-[#2a2a2a]">{error}</div>
    {/if}

    <!-- Add subgroup form -->
    {#if addingSubgroupId}
      <div class="px-6 py-3 border-b border-[#2a2a2a]">
        <form on:submit|preventDefault={doAddSubgroup} class="flex gap-2">
          <input
            type="text"
            bind:value={newSubgroupTitle}
            placeholder="Sub-group title…"
            autofocus
            class="flex-1 bg-[#242424] border border-[#333] rounded px-3 py-1.5 text-sm
                   text-[#e8e8e8] placeholder-[#555] focus:outline-none focus:border-[#7c5cbf]"
          />
          <button type="submit" class="px-3 py-1.5 bg-[#7c5cbf] text-white text-sm rounded">Add</button>
          <button type="button" class="px-3 py-1.5 text-[#666] text-sm"
            on:click={() => { addingSubgroupId = null; newSubgroupTitle = ''; }}>Cancel</button>
        </form>
      </div>
    {/if}

    <div class="flex-1 overflow-y-auto">
      {#each activeProject.project.sub_groups as sg (sg.id)}
        <div class="mb-1">
          <!-- Subgroup heading -->
          <div class="flex items-center gap-2 px-6 py-2 bg-[#161616] border-b border-[#222]">
            {#if renamingSubgroupId === sg.id}
              <form on:submit|preventDefault={() => doRenameSubgroup(sg.id)} class="flex gap-2 flex-1">
                <input
                  type="text"
                  bind:value={renameSubgroupTitle}
                  autofocus
                  class="flex-1 bg-[#242424] border border-[#333] rounded px-2 py-0.5 text-sm
                         text-[#e8e8e8] focus:outline-none focus:border-[#7c5cbf]"
                />
                <button type="submit" class="text-xs text-[#7c5cbf]">Save</button>
                <button type="button" class="text-xs text-[#666]"
                  on:click={() => { renamingSubgroupId = null; renameSubgroupTitle = ''; }}>Cancel</button>
              </form>
            {:else}
              <span class="text-xs font-semibold text-[#888] uppercase tracking-wider flex-1">{sg.title}</span>
              <button
                class="text-xs text-[#555] hover:text-[#999]"
                on:click={() => { renamingSubgroupId = sg.id; renameSubgroupTitle = sg.title; }}
                title="Rename"
              >rename</button>
              <button
                class="text-xs text-[#555] hover:text-[#cc5555]"
                on:click={() => doDeleteSubgroup(sg.id)}
                title="Delete sub-group"
              >delete</button>
              <button
                class="text-xs text-[#555] hover:text-[#7c5cbf]"
                on:click={() => { addingTaskInSubgroup = sg.id; newTaskText = ''; }}
                title="Add task"
              >+ task</button>
            {/if}
          </div>

          <!-- Add task form for this subgroup -->
          {#if addingTaskInSubgroup === sg.id}
            <div class="px-6 py-2 border-b border-[#222] bg-[#1a1a1a]">
              <form on:submit|preventDefault={() => doAddTask(sg.id)} class="flex gap-2">
                <input
                  type="text"
                  bind:value={newTaskText}
                  placeholder="Task text…"
                  autofocus
                  class="flex-1 bg-[#242424] border border-[#333] rounded px-3 py-1.5 text-sm
                         text-[#e8e8e8] placeholder-[#555] focus:outline-none focus:border-[#7c5cbf]"
                />
                <button type="submit" class="px-3 py-1.5 bg-[#7c5cbf] text-white text-sm rounded">Add</button>
                <button type="button" class="px-2 text-[#666] text-sm"
                  on:click={() => { addingTaskInSubgroup = null; newTaskText = ''; }}>Cancel</button>
              </form>
            </div>
          {/if}

          <!-- Tasks in subgroup -->
          {#each sg.tasks as task (task.id)}
            <!-- svelte-ignore a11y-click-events-have-key-events -->
            <!-- svelte-ignore a11y-no-static-element-interactions -->
            <div
              class="group flex items-start gap-3 px-6 py-2.5 border-b border-[#1d1d1d] cursor-pointer
                     {selectedTaskId === task.id ? 'bg-[#242424]' : 'hover:bg-[#1e1e1e]'}"
              on:click={() => selectedTaskId = task.id}
              on:dblclick={() => openTaskDetail(task)}
            >
              <span class="mt-0.5 text-xs w-4 flex-shrink-0 text-[#666]">{stateIcon(task.state)}</span>
              <div class="flex-1 min-w-0">
                <div class="text-sm text-[#e8e8e8] leading-snug
                  {task.state === 'done' || task.state === 'canceled' ? 'line-through text-[#555]' : ''}">
                  {task.text}
                </div>
                {#if task.deadline || task.tags?.length}
                  <div class="flex gap-3 mt-0.5 text-xs text-[#666]">
                    {#if task.deadline}
                      <span class="text-[#d4a843]">due {new Date(task.deadline).toLocaleDateString()}</span>
                    {/if}
                    {#each task.tags ?? [] as tag}
                      <span>{tag}</span>
                    {/each}
                  </div>
                {/if}
              </div>

              {#if selectedTaskId === task.id}
                <div class="flex items-center gap-1 flex-shrink-0">
                  <button class="text-[#555] hover:text-[#999] text-xs px-1"
                    on:click|stopPropagation={() => doReorder(task.id, -1)} title="Move up">↑</button>
                  <button class="text-[#555] hover:text-[#999] text-xs px-1"
                    on:click|stopPropagation={() => doReorder(task.id, 1)} title="Move down">↓</button>
                  <button class="text-xs px-2 py-0.5 bg-[#2a3a2a] text-[#6db36d] rounded hover:bg-[#3a4a3a]"
                    on:click|stopPropagation={() => markTaskDone(task.id)}>done</button>
                  <button class="text-xs px-2 py-0.5 bg-[#2a2a3a] text-[#7c5cbf] rounded hover:bg-[#3a3a4a]"
                    on:click|stopPropagation={() => openTaskDetail(task)}>edit</button>
                  <button class="text-xs px-2 py-0.5 bg-[#3a2a2a] text-[#cc5555] rounded hover:bg-[#4a3a3a]"
                    on:click|stopPropagation={() => doTrashTask(task.id)}>trash</button>
                </div>
              {/if}
            </div>
          {/each}

          {#if sg.tasks.length === 0 && addingTaskInSubgroup !== sg.id}
            <div class="px-6 py-2 text-[#444] text-xs italic">No tasks</div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
