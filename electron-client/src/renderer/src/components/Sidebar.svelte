<script lang="ts">
  import { currentView, navigate } from '../stores/index';

  const navItems = [
    { id: 'inbox',    label: 'Inbox',          key: '1', icon: '📥' },
    { id: 'actions',  label: 'Actions',         key: '2', icon: '⚡' },
    { id: 'projects', label: 'Projects',        key: '3', icon: '📁' },
    { id: 'views',    label: 'Views & Review',  key: '4', icon: '🔍' },
  ] as const;

  $: activeBase = $currentView === 'project-detail' ? 'projects'
    : $currentView === 'weekly-review' ? 'views'
    : $currentView;
</script>

<nav
  class="w-48 flex-shrink-0 bg-[#141414] border-r border-[#2a2a2a] flex flex-col"
  style="padding-top: env(titlebar-area-height, 28px)"
>
  <!-- App title -->
  <div class="px-4 py-3 border-b border-[#2a2a2a]">
    <span class="text-[#7c5cbf] font-semibold tracking-wide text-sm">wnwn</span>
  </div>

  <!-- Nav items -->
  <ul class="flex-1 py-2 list-none m-0 p-0">
    {#each navItems as item}
      <li>
        <button
          class="w-full text-left px-4 py-2 flex items-center gap-2 text-sm transition-colors
            {activeBase === item.id
              ? 'bg-[#2a1f44] text-[#9575d4]'
              : 'text-[#999] hover:bg-[#242424] hover:text-[#e8e8e8]'}"
          on:click={() => navigate(item.id)}
        >
          <span class="text-base">{item.icon}</span>
          <span class="flex-1">{item.label}</span>
          <span class="text-[#555] text-xs font-mono">{item.key}</span>
        </button>
      </li>
    {/each}
  </ul>

  <!-- Footer -->
  <div class="px-4 py-3 border-t border-[#2a2a2a] text-[#555] text-xs">
    v0.1.0
  </div>
</nav>
