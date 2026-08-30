<script setup lang="ts">
import '@xterm/xterm/css/xterm.css'
import type { NavigationMenuItem } from '@nuxt/ui'
import type { Node, NodeKeyStatus, Reachability, SwarmRole, SwarmResult } from '~/types/node'

/**
 * The node detail page. Each section is its own route (`…/:id/swarm`), so a
 * tab is linkable and the back button moves between them — the same shape as
 * the project detail page.
 */

definePageMeta({ key: route => String(route.params.id) })

const route = useRoute()
const router = useRouter()
const toast = useToast()

const {
  items,
  sshKeys,
  error,
  load,
  testKey,
  ping,
  hasManager,
  demoteSwarm,
  leaveSwarm,
  refreshSwarm
} = useNodes()

const nodeId = computed(() => String(route.params.id))
const node = computed(() => items.value.find(item => item.id === nodeId.value))

const loading = ref(true)

onMounted(async () => {
  await load(true)
  loading.value = false

  // A fresh look at this one node's own state, not the whole fleet's — cheap
  // enough to run every time its page opens.
  if (node.value && !node.value.local) {
    ping(node.value)
    refreshSwarm(node.value)
  }
})

const tabs = [
  { key: 'overview', label: 'Overview', icon: 'i-lucide-layout-dashboard' },
  { key: 'swarm', label: 'Swarm', icon: 'i-lucide-boxes' },
  { key: 'terminal', label: 'Terminal', icon: 'i-lucide-terminal' }
] as const

const tab = computed(() => String(route.params.tab))

// An unknown segment is a 404 rather than a silently blank panel.
watchEffect(() => {
  if (!tabs.some(item => item.key === tab.value)) {
    throw createError({ statusCode: 404, statusMessage: 'Unknown node tab', fatal: true })
  }
})

const tabItems = computed<NavigationMenuItem[]>(() =>
  tabs.map(item => ({
    label: item.label,
    icon: item.icon,
    to: `/dashboard/nodes/${nodeId.value}/${item.key}`
  })))

useHead(() => ({ title: `${node.value?.name ?? 'Node'} · Stacker` }))

const keyName = computed(() => sshKeys.value.find(key => key.id === node.value?.sshKeyId)?.name)

const statusMeta: Record<NodeKeyStatus, { icon: string, class: string, label: string }> = {
  ok: { icon: 'i-lucide-circle-check', class: 'text-success', label: 'Key authentication works' },
  failed: { icon: 'i-lucide-circle-x', class: 'text-error', label: 'Key authentication failed' },
  unknown: { icon: 'i-lucide-circle-dashed', class: 'text-dimmed', label: 'Key not installed yet' }
}

const stateMeta: Record<Reachability, { label: string, dot: string, text: string, hint: string }> = {
  online: { label: 'Online', dot: 'bg-success', text: 'text-success', hint: 'The host answered the last check' },
  offline: { label: 'Offline', dot: 'bg-error', text: 'text-error', hint: 'The host did not answer the last check' },
  unknown: { label: 'Unknown', dot: 'bg-muted', text: 'text-dimmed', hint: 'Not checked yet' }
}

const stateOf = computed(() => stateMeta[node.value?.reachability || 'unknown'])

const swarmMeta: Record<SwarmRole, { label: string, icon: string, color: 'primary' | 'success' | 'neutral' }> = {
  manager: { label: 'Manager', icon: 'i-lucide-crown', color: 'primary' },
  worker: { label: 'Worker', icon: 'i-lucide-boxes', color: 'success' },
  none: { label: 'Not configured', icon: 'i-lucide-circle-dashed', color: 'neutral' }
}

const managerCount = computed(() => items.value.filter(item => item.swarmRole === 'manager').length)

const formatTime = (value?: string) =>
  value
    ? new Date(value).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
    : 'Never'

/* ---- reachability / key checks ---- */

const pinging = ref(false)
const testing = ref(false)

async function onPing() {
  if (!node.value) return
  pinging.value = true
  try {
    const updated = await ping(node.value)
    toast.add({
      title: updated.reachability === 'online' ? 'Node is online' : 'Node is offline',
      description: updated.reachabilityMessage,
      icon: updated.reachability === 'online' ? 'i-lucide-wifi' : 'i-lucide-wifi-off',
      color: updated.reachability === 'online' ? 'success' : 'error'
    })
  } catch (err) {
    toast.add({
      title: 'Could not check the node',
      description: err instanceof Error ? err.message : undefined,
      color: 'error'
    })
  } finally {
    pinging.value = false
  }
}

async function onTest() {
  if (!node.value) return
  testing.value = true
  try {
    const result = await testKey(node.value)
    toast.add({
      title: result.ok ? 'Connection OK' : 'Connection failed',
      description: result.message,
      icon: result.ok ? 'i-lucide-circle-check' : 'i-lucide-circle-x',
      color: result.ok ? 'success' : 'error'
    })
  } finally {
    testing.value = false
  }
}

async function copySshCommand() {
  if (!node.value) return
  const command = `ssh -p ${node.value.port} ${node.value.ssh}`
  await navigator.clipboard.writeText(command)
  toast.add({ title: 'Copied', description: command, icon: 'i-lucide-clipboard-check' })
}

/* ---- swarm actions ---- */

const swarmBusy = ref(false)
const configureOpen = ref(false)
const promoteOpen = ref(false)

async function runSwarm(action: (node: Node) => Promise<SwarmResult>, title: string) {
  if (!node.value) return
  swarmBusy.value = true
  try {
    const result = await action(node.value)
    toast.add({ title, description: result.message, icon: 'i-lucide-boxes', color: 'success' })
  } catch (err) {
    toast.add({
      title: `Could not ${title.toLowerCase()}`,
      description: err instanceof Error ? err.message : undefined,
      color: 'error'
    })
  } finally {
    swarmBusy.value = false
  }
}

/* ---- edit / delete ---- */

const formOpen = ref(false)
const deleteOpen = ref(false)

function onDeleted() {
  router.push('/dashboard/nodes')
}

/* ---- terminal ---- */

const { status: termStatus, error: termError, open: openTerminal, reconnect: reconnectTerminal, dispose: disposeTerminal }
  = useNodeTerminal()

const termHost = useTemplateRef<HTMLElement>('termHost')

const termStatusMeta: Record<typeof termStatus.value, { label: string, color: 'neutral' | 'success' | 'error' }> = {
  connecting: { label: 'Connecting…', color: 'neutral' },
  open: { label: 'Connected', color: 'success' },
  closed: { label: 'Disconnected', color: 'neutral' },
  error: { label: 'Failed', color: 'error' }
}

const canOpenTerminal = computed(() => !!node.value && (node.value.local || node.value.keyStatus === 'ok'))

// The shell only runs while its own tab is open — leaving the tab (or the
// page) closes the socket, the same rule the terminal modal follows: a shell
// nobody is watching is a shell still holding an ssh connection open.
watch([tab, () => node.value?.id], async ([currentTab]) => {
  if (currentTab !== 'terminal') {
    disposeTerminal()
    return
  }

  await nextTick()
  if (termHost.value && node.value && canOpenTerminal.value) openTerminal(termHost.value, node.value)
}, { immediate: true })

onBeforeUnmount(disposeTerminal)
</script>

<template>
  <UDashboardPanel id="node-detail" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar :title="node?.name ?? 'Node'">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #trailing>
          <UBadge v-if="node?.local" label="This machine" color="neutral" variant="subtle" />
          <UBadge
            v-if="node"
            :label="stateOf.label"
            :color="node.reachability === 'online' ? 'success' : node.reachability === 'offline' ? 'error' : 'neutral'"
            variant="subtle"
          />
          <UBadge
            v-if="node"
            :label="swarmMeta[node.swarmRole].label"
            :icon="swarmMeta[node.swarmRole].icon"
            :color="swarmMeta[node.swarmRole].color"
            variant="subtle"
          />
        </template>

        <template #right>
          <UButton
            label="All nodes"
            icon="i-lucide-arrow-left"
            color="neutral"
            variant="ghost"
            to="/dashboard/nodes"
          />
          <UButton
            v-if="node"
            label="Edit"
            icon="i-lucide-pencil"
            color="neutral"
            variant="subtle"
            @click="formOpen = true"
          />
        </template>
      </UDashboardNavbar>

      <UDashboardToolbar>
        <template #left>
          <UNavigationMenu :items="tabItems" highlight class="-mb-px" />
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
      />

      <div v-if="loading" class="flex shrink-0 justify-center py-12">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" />
      </div>

      <div v-else-if="node" class="w-full shrink-0 space-y-6">
        <!-- Overview -->
        <template v-if="tab === 'overview'">
          <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
            <header class="mb-4 flex items-center justify-between gap-3">
              <div>
                <h2 class="text-sm font-semibold text-highlighted">Connection</h2>
                <p class="text-sm text-muted">How stacker reaches this host.</p>
              </div>
              <div class="flex gap-2">
                <UButton
                  label="Check status"
                  icon="i-lucide-activity"
                  size="xs"
                  color="neutral"
                  variant="subtle"
                  :loading="pinging"
                  :disabled="node.local"
                  @click="onPing"
                />
                <UButton
                  label="Test connection"
                  icon="i-lucide-plug-zap"
                  size="xs"
                  color="neutral"
                  variant="subtle"
                  :loading="testing"
                  :disabled="node.local"
                  @click="onTest"
                />
              </div>
            </header>

            <dl class="grid gap-4 sm:grid-cols-2">
              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">SSH</dt>
                <dd class="mt-1 flex items-center gap-2 font-mono text-sm text-toned">
                  <template v-if="node.local">local — no ssh</template>
                  <template v-else>
                    {{ node.ssh }}:{{ node.port }}
                    <UButton
                      icon="i-lucide-copy"
                      size="xs"
                      color="neutral"
                      variant="ghost"
                      aria-label="Copy SSH command"
                      @click="copySshCommand"
                    />
                  </template>
                </dd>
              </div>

              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">SSH key</dt>
                <dd class="mt-1 flex items-center gap-2 text-sm text-toned">
                  <template v-if="node.local">No key needed</template>
                  <template v-else>
                    <UIcon
                      :name="statusMeta[node.keyStatus].icon"
                      class="size-4 shrink-0"
                      :class="statusMeta[node.keyStatus].class"
                    />
                    {{ keyName ?? 'Unknown key' }}
                    <span class="text-xs text-dimmed">— {{ statusMeta[node.keyStatus].label }}</span>
                  </template>
                </dd>
              </div>

              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Reachability</dt>
                <dd class="mt-1 flex items-center gap-2 text-sm" :class="stateOf.text">
                  <span class="size-2 rounded-full" :class="stateOf.dot" />
                  {{ stateOf.label }}
                  <span v-if="node.reachabilityMessage" class="text-xs text-dimmed">
                    — {{ node.reachabilityMessage }}
                  </span>
                </dd>
                <dd class="mt-0.5 text-xs text-dimmed">Checked {{ formatTime(node.reachableCheckedAt) }}</dd>
              </div>

              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Key last checked</dt>
                <dd class="mt-1 text-sm text-toned">{{ formatTime(node.keyCheckedAt) }}</dd>
              </div>
            </dl>

            <dl class="mt-5 grid gap-4 border-t border-default pt-4 sm:grid-cols-2">
              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Created</dt>
                <dd class="mt-1 text-sm text-toned">{{ formatTime(node.createdAt) }}</dd>
              </div>
              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Last updated</dt>
                <dd class="mt-1 text-sm text-toned">{{ formatTime(node.updatedAt) }}</dd>
              </div>
            </dl>
          </section>

          <section class="rounded-lg border border-error/40 bg-default/60 p-5 backdrop-blur">
            <h2 class="text-sm font-semibold text-highlighted">Delete node</h2>
            <p class="mb-4 mt-1 text-sm text-muted">
              Removes this node from stacker. The server itself is untouched, but any stack
              targeting it will need a new host. This cannot be undone.
            </p>
            <UButton
              label="Delete node"
              icon="i-lucide-trash-2"
              color="error"
              variant="subtle"
              :disabled="node.local || node.swarmRole !== 'none'"
              :title="node.local
                ? 'The local node cannot be deleted'
                : node.swarmRole !== 'none'
                  ? 'Leave the swarm first'
                  : undefined"
              @click="deleteOpen = true"
            />
          </section>
        </template>

        <!-- Swarm -->
        <section
          v-else-if="tab === 'swarm'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Swarm</h2>
            <p class="text-sm text-muted">What stacker last saw on this host, and how to change it.</p>
          </header>

          <dl class="grid gap-4 sm:grid-cols-2">
            <div>
              <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Role</dt>
              <dd class="mt-1">
                <UBadge
                  :label="swarmMeta[node.swarmRole].label"
                  :icon="swarmMeta[node.swarmRole].icon"
                  :color="swarmMeta[node.swarmRole].color"
                  variant="subtle"
                />
              </dd>
            </div>

            <div v-if="node.swarmRole === 'manager' && node.swarmAddr">
              <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Advertise address</dt>
              <dd class="mt-1 font-mono text-sm text-toned">{{ node.swarmAddr }}:2377</dd>
            </div>

            <div v-if="node.swarmNodeId">
              <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Docker node id</dt>
              <dd class="mt-1 font-mono text-xs text-dimmed">{{ node.swarmNodeId }}</dd>
            </div>

            <div>
              <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Last synced</dt>
              <dd class="mt-1 text-sm text-toned">{{ formatTime(node.swarmSyncedAt) }}</dd>
            </div>
          </dl>

          <UAlert
            v-if="node.swarmError"
            :description="node.swarmError"
            title="Last swarm reading did not match"
            icon="i-lucide-triangle-alert"
            color="warning"
            variant="subtle"
            class="mt-4"
          />

          <div class="mt-5 flex flex-wrap gap-2 border-t border-default pt-4">
            <UButton
              v-if="node.swarmRole === 'none' && !node.local"
              label="Configure"
              icon="i-lucide-boxes"
              size="sm"
              :disabled="!hasManager"
              :title="!hasManager ? 'A swarm manager is required before a node can join' : undefined"
              @click="configureOpen = true"
            />
            <UBadge
              v-else-if="node.swarmRole === 'none'"
              label="Rerun install.sh to restore the local swarm manager"
              icon="i-lucide-triangle-alert"
              color="warning"
              variant="subtle"
            />

            <template v-else>
              <UButton
                label="Refresh swarm state"
                icon="i-lucide-refresh-cw"
                size="sm"
                color="neutral"
                variant="subtle"
                :loading="swarmBusy"
                @click="runSwarm(refreshSwarm, 'Refresh swarm state')"
              />

              <UButton
                v-if="!node.local"
                label="Reconfigure"
                icon="i-lucide-wrench"
                size="sm"
                color="neutral"
                variant="subtle"
                @click="configureOpen = true"
              />

              <UButton
                v-if="node.swarmRole === 'worker'"
                label="Promote to manager"
                icon="i-lucide-crown"
                size="sm"
                color="neutral"
                variant="subtle"
                @click="promoteOpen = true"
              />
              <UButton
                v-else-if="managerCount > 1"
                label="Demote to worker"
                icon="i-lucide-arrow-down"
                size="sm"
                color="neutral"
                variant="subtle"
                :loading="swarmBusy"
                @click="runSwarm(demoteSwarm, 'Demote node')"
              />

              <UButton
                v-if="node.swarmRole === 'worker' || managerCount > 1"
                label="Leave swarm"
                icon="i-lucide-log-out"
                size="sm"
                color="error"
                variant="subtle"
                :loading="swarmBusy"
                @click="runSwarm(leaveSwarm, 'Leave swarm')"
              />
            </template>
          </div>
        </section>

        <!-- Terminal -->
        <section
          v-else-if="tab === 'terminal'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4 flex items-center justify-between gap-2">
            <div>
              <h2 class="text-sm font-semibold text-highlighted">Terminal</h2>
              <p class="text-sm text-muted">
                Shell on {{ node.local ? 'this machine' : `${node.ssh}:${node.port}` }}.
                Leaving this tab ends the session.
              </p>
            </div>

            <div class="flex items-center gap-2">
              <UBadge
                :label="termStatusMeta[termStatus].label"
                :color="termStatusMeta[termStatus].color"
                variant="subtle"
                :icon="termStatus === 'open' ? 'i-lucide-terminal' : 'i-lucide-plug-zap'"
              />
              <UButton
                v-if="termStatus === 'closed' || termStatus === 'error'"
                label="Reconnect"
                icon="i-lucide-refresh-cw"
                size="xs"
                color="neutral"
                variant="subtle"
                @click="reconnectTerminal(node)"
              />
            </div>
          </header>

          <UAlert
            v-if="!canOpenTerminal"
            title="Connection not verified"
            description="Stacker has not confirmed key authentication for this node. Test the connection from the Overview tab first."
            icon="i-lucide-triangle-alert"
            color="warning"
            variant="subtle"
            class="mb-3"
          />

          <UAlert
            v-if="termError"
            :description="termError"
            title="Could not open the shell"
            icon="i-lucide-triangle-alert"
            color="error"
            variant="subtle"
            class="mb-3"
          />

          <div
            ref="termHost"
            class="h-[60vh] overflow-hidden rounded-lg border border-default bg-black p-2"
          />
        </section>
      </div>

      <div v-else class="flex shrink-0 flex-col items-center gap-3 py-12">
        <UIcon name="i-lucide-server-off" class="size-8 text-dimmed" />
        <p class="text-sm text-muted">This node no longer exists.</p>
        <UButton label="Back to nodes" color="neutral" variant="subtle" to="/dashboard/nodes" />
      </div>
    </template>
  </UDashboardPanel>

  <NodeFormModal v-model:open="formOpen" :node="node" />
  <NodeDeleteModal v-model:open="deleteOpen" :node="node" @deleted="onDeleted" />
  <NodeSwarmConfigureModal v-model:open="configureOpen" :node="node" />
  <NodeSwarmPromoteModal v-model:open="promoteOpen" :node="node" :manager-count="managerCount" />
</template>
