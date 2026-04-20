<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { currentView, navigate, handleServerEvent } from './stores/index';
  import { connectEvents } from './api/client';
  import Sidebar from './components/Sidebar.svelte';
  import InboxView from './views/InboxView.svelte';
  import ActionsView from './views/ActionsView.svelte';
  import ProjectsView from './views/ProjectsView.svelte';
  import ViewsView from './views/ViewsView.svelte';

  let eventSource: EventSource | null = null;

  onMount(() => {
    // Connect to SSE stream for live updates.
    eventSource = connectEvents(handleServerEvent);
  });

  onDestroy(() => {
    eventSource?.close();
  });

  // Keyboard navigation between top-level views.
  function handleKeydown(e: KeyboardEvent) {
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
    // No modifiers for nav shortcuts.
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    switch (e.key) {
      case '1': navigate('inbox'); break;
      case '2': navigate('actions'); break;
      case '3': navigate('projects'); break;
      case '4': navigate('views'); break;
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

<div class="flex h-screen bg-[#1a1a1a] text-[#e8e8e8] overflow-hidden">
  <!-- Sidebar -->
  <Sidebar />

  <!-- Main content -->
  <main class="flex-1 overflow-hidden flex flex-col">
    {#if $currentView === 'inbox'}
      <InboxView />
    {:else if $currentView === 'actions'}
      <ActionsView />
    {:else if $currentView === 'projects' || $currentView === 'project-detail'}
      <ProjectsView />
    {:else if $currentView === 'views' || $currentView === 'weekly-review'}
      <ViewsView />
    {/if}
  </main>
</div>
