<script setup lang="ts">
import type { NavigationMenu, NavigationMenuItem } from '@nuxt/ui'

// Roomier rows than the default sidebar; the active marker itself is in main.css.
const navUi: NavigationMenu['ui'] = {
  list: 'space-y-1',
  link: 'px-2.5 py-2 gap-3 text-[13px]',
  linkLeadingIcon: 'size-4.5'
}

const route = useRoute()

// Infrastructure and application navigation.
const links = computed<NavigationMenuItem[][]>(() => [
  [
    {
      label: 'Projects',
      icon: 'i-lucide-package',
      to: '/dashboard/projects',
      // `/new` and `/:id` are their own routes, so match the whole subtree.
      active: route.path.startsWith('/dashboard/projects')
    },
    { label: 'Deployments', icon: 'i-lucide-rocket', to: '/dashboard/deployments' }
  ],
  [
    { label: 'Nodes', icon: 'i-lucide-server', to: '/dashboard/nodes' },
    {
      label: 'Swarm',
      icon: 'i-lucide-boxes',
      // Every resource tab is its own route, so the entry stays lit across all
      // of them rather than only on the one it links to.
      to: '/dashboard/swarm/services',
      active: route.path.startsWith('/dashboard/swarm')
    },
  ],
  [
    {
      label: 'Settings',
      icon: 'i-lucide-settings',
      // Every settings tab is its own route, so keep the entry lit across all.
      to: '/dashboard/settings/account',
      active: route.path.startsWith('/dashboard/settings')
    }
  ]
])
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
        <p
          v-if="!collapsed"
          class="px-2.5 pb-2 pt-1 font-mono text-[10px] uppercase tracking-[0.12em] text-dimmed"
        >
          Application
        </p>
        <UNavigationMenu
          orientation="vertical"
          :collapsed="collapsed"
          :items="links[0]"
          class="stacker-nav"
          :ui="navUi"
        />

        <p
          v-if="!collapsed"
          class="px-2.5 pb-2 pt-5 font-mono text-[10px] uppercase tracking-[0.12em] text-dimmed"
        >
          Infrastructure
        </p>
        <UNavigationMenu
          orientation="vertical"
          :collapsed="collapsed"
          :items="links[1]"
          class="stacker-nav"
          :ui="navUi"
        />
        <UNavigationMenu
          orientation="vertical"
          :collapsed="collapsed"
          :items="links[2]"
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
