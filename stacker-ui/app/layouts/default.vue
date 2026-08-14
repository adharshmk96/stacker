<script setup lang="ts">
import type { NavigationMenu, NavigationMenuItem } from '@nuxt/ui'

// Roomier rows than the default sidebar; the active marker itself is in main.css.
const navUi: NavigationMenu['ui'] = {
  list: 'space-y-1',
  link: 'px-2.5 py-2 gap-3 text-[13px]',
  linkLeadingIcon: 'size-4.5'
}

// Nodes and SSH Keys are live today; the rest are placeholders for upcoming menus.
const links: NavigationMenuItem[][] = [
  [
    { label: 'Nodes', icon: 'i-lucide-server', to: '/dashboard/nodes' },
    { label: 'SSH Keys', icon: 'i-lucide-key-round', to: '/dashboard/ssh-keys' }
  ],
  [
    // No `to` until the routes exist — a disabled item still resolves its link.
    { label: 'Stacks', icon: 'i-lucide-layers', disabled: true }
  ]
]
</script>

<template>
  <UDashboardGroup>
    <UDashboardSidebar collapsible resizable :ui="{ footer: 'border-t border-default' }">
      <template #header="{ collapsed }">
        <div class="flex items-center gap-2.5">
          <div class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 ring-1 ring-primary/25">
            <UIcon name="i-lucide-container" class="size-4.5 text-primary" />
          </div>
          <div v-if="!collapsed" class="flex items-baseline gap-2">
            <span class="font-mono font-semibold tracking-tight text-highlighted">stacker</span>
            <span class="font-mono text-[10px] text-dimmed">v0.1</span>
          </div>
        </div>
      </template>

      <template #default="{ collapsed }">
        <ClusterSwitcher :collapsed="collapsed" class="mb-2" />

        <p
          v-if="!collapsed"
          class="px-2.5 pb-2 pt-1 font-mono text-[10px] uppercase tracking-[0.12em] text-dimmed"
        >
          Infrastructure
        </p>
        <UNavigationMenu
          orientation="vertical"
          :collapsed="collapsed"
          :items="links[0]"
          class="stacker-nav"
          :ui="navUi"
        />
        <UNavigationMenu
          orientation="vertical"
          :collapsed="collapsed"
          :items="links[1]"
          class="stacker-nav mt-auto"
          :ui="navUi"
        />
      </template>

      <template #footer="{ collapsed }">
        <AccountMenu :collapsed="collapsed" />
      </template>
    </UDashboardSidebar>

    <slot />
  </UDashboardGroup>
</template>
