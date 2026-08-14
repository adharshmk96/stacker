<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import type { SshKey } from '~/types/sshKey'

useHead({ title: 'SSH Keys · Stacker' })

const { items, pending, error, load } = useSshKeys()
// Only for the "in use" counts — a key referenced by a node can't be deleted.
const { items: nodeItems, load: loadNodes } = useNodes()

// Client-side only: the stacker server is a local daemon, so there is nothing
// for the SSR pass to talk to.
onMounted(() => {
  load()
  loadNodes()
})

const search = ref('')

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return items.value

  return items.value.filter(key =>
    [key.name, key.type, key.fingerprint].some(field => field.toLowerCase().includes(term)))
})

const createOpen = ref(false)
const deleteOpen = ref(false)
const selected = ref<SshKey | null>(null)

function onDelete(key: SshKey) {
  selected.value = key
  deleteOpen.value = true
}

const toast = useToast()

async function copyPublicKey(key: SshKey) {
  await navigator.clipboard.writeText(key.publicKey)
  toast.add({ title: 'Public key copied', description: key.name, icon: 'i-lucide-clipboard-check' })
}

/** How many servers reference each key — deleting one is not free */
const usage = computed(() => {
  const counts = new Map<string, number>()
  for (const node of nodeItems.value) {
    counts.set(node.sshKeyId, (counts.get(node.sshKeyId) ?? 0) + 1)
  }
  return counts
})

const columns: TableColumn<SshKey>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'fingerprint', header: 'Fingerprint' },
  { id: 'usage', header: 'In use' },
  { accessorKey: 'createdAt', header: 'Created' },
  { id: 'actions' }
]

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
</script>

<template>
  <UDashboardPanel id="ssh-keys" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="SSH Keys">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #trailing>
          <UBadge
            :label="String(items.length)"
            color="neutral"
            variant="subtle"
            class="font-mono"
          />
        </template>

        <template #right>
          <UButton
            label="New key"
            icon="i-lucide-plus"
            class="shadow-lg shadow-primary/20"
            @click="createOpen = true"
          />
        </template>
      </UDashboardNavbar>

      <UDashboardToolbar>
        <template #left>
          <UInput
            v-model="search"
            icon="i-lucide-search"
            placeholder="Search name or fingerprint…"
            class="w-64"
          >
            <template v-if="search" #trailing>
              <UButton
                icon="i-lucide-x"
                color="neutral"
                variant="link"
                size="xs"
                aria-label="Clear search"
                @click="search = ''"
              />
            </template>
          </UInput>
        </template>
      </UDashboardToolbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <UAlert
        v-if="error"
        :description="error"
        title="Cannot reach the stacker server"
        icon="i-lucide-plug-zap"
        color="error"
        variant="subtle"
        class="mb-4 shrink-0"
        :actions="[{ label: 'Retry', color: 'neutral', variant: 'subtle', onClick: () => load(true) }]"
      />

      <UTable
        :data="filtered"
        :columns="columns"
        :loading="pending"
        class="stacker-table shrink-0 rounded-lg border border-default bg-default/60 backdrop-blur"
        :ui="{ tr: 'transition-colors hover:bg-elevated/40' }"
      >
        <template #name-cell="{ row }">
          <div class="flex items-center gap-3">
            <div class="flex size-8 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
              <UIcon name="i-lucide-key-round" class="size-4 text-primary" />
            </div>
            <div class="leading-tight">
              <p class="font-medium text-highlighted">{{ row.original.name }}</p>
              <p class="font-mono text-xs uppercase text-dimmed">{{ row.original.type }}</p>
            </div>
          </div>
        </template>

        <template #fingerprint-cell="{ row }">
          <span class="font-mono text-xs text-toned">{{ row.original.fingerprint }}</span>
        </template>

        <template #usage-cell="{ row }">
          <UBadge
            v-if="usage.get(row.original.id)"
            :label="`${usage.get(row.original.id)} ${usage.get(row.original.id) === 1 ? 'node' : 'nodes'}`"
            color="primary"
            variant="subtle"
            icon="i-lucide-server"
          />
          <span v-else class="text-sm text-dimmed">Unused</span>
        </template>

        <template #createdAt-cell="{ row }">
          {{ formatDate(row.original.createdAt) }}
        </template>

        <template #actions-cell="{ row }">
          <div class="flex justify-end gap-1">
            <UButton
              icon="i-lucide-copy"
              color="neutral"
              variant="ghost"
              aria-label="Copy public key"
              title="Copy public key"
              @click="copyPublicKey(row.original)"
            />
            <UButton
              icon="i-lucide-trash-2"
              color="error"
              variant="ghost"
              aria-label="Delete SSH key"
              title="Delete SSH key"
              @click="onDelete(row.original)"
            />
          </div>
        </template>

        <template #empty>
          <div class="flex flex-col items-center gap-3 py-6">
            <UIcon name="i-lucide-key-round" class="size-8 text-dimmed" />
            <p class="text-sm text-muted">
              {{ search ? 'No key matches this search.' : 'No SSH keys yet.' }}
            </p>
            <UButton
              v-if="search"
              label="Clear search"
              color="neutral"
              variant="subtle"
              @click="search = ''"
            />
            <UButton
              v-else
              label="Create your first key"
              icon="i-lucide-plus"
              @click="createOpen = true"
            />
          </div>
        </template>
      </UTable>

      <div class="flex items-center justify-between gap-3 border-t border-default pt-4 mt-auto">
        <p class="text-sm text-muted">
          {{ filtered.length }} of {{ items.length }} {{ items.length === 1 ? 'key' : 'keys' }}
        </p>
      </div>
    </template>
  </UDashboardPanel>

  <SshKeyCreateModal v-model:open="createOpen" />
  <SshKeyDeleteModal v-model:open="deleteOpen" :ssh-key="selected" />
</template>
