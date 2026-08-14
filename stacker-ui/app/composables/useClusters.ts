import type { Cluster } from '~/types/cluster'
import { DEFAULT_CLUSTER } from '~/types/cluster'

/**
 * Clusters, static for now.
 *
 * The switcher is wired against this composable rather than a constant so that
 * turning clusters into real, server-backed records is a change to `items` and
 * nothing else. `active` is module-scope state, shared by every caller — the
 * same pattern as `useNodes`.
 */

const items = ref<Cluster[]>([DEFAULT_CLUSTER])
const activeId = ref(DEFAULT_CLUSTER.id)

export function useClusters() {
  // Falls back to the default rather than going undefined, so the switcher
  // always has something to render.
  const active = computed(() =>
    items.value.find(cluster => cluster.id === activeId.value) ?? DEFAULT_CLUSTER)

  function select(id: string) {
    if (!items.value.some(cluster => cluster.id === id)) return
    activeId.value = id
  }

  return { items, active, activeId, select }
}
