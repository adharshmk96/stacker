import type { GitSource } from '~/types/project'

/**
 * Fetches compose YAML from a git repository for service pickers and previews.
 */
export function useGitComposePreview(git: Ref<GitSource>, enabled: Ref<boolean>) {
  const compose = ref('')
  const pending = ref(false)
  const error = ref('')

  let timer: ReturnType<typeof setTimeout> | undefined
  let request = 0

  watch(
    [git, enabled],
    () => {
      clearTimeout(timer)
      const current = ++request
      compose.value = ''
      error.value = ''

      if (!enabled.value || !git.value.repo.trim() || !git.value.branch.trim() || !git.value.composePath.trim()) {
        pending.value = false
        return
      }

      pending.value = true
      timer = setTimeout(async () => {
        try {
          const result = await useApi().post<{ compose: string }>('/projects/compose-preview', { git: git.value })
          if (current === request) compose.value = result.compose
        } catch (err) {
          if (current === request) {
            error.value = err instanceof Error ? err.message : 'Could not read the compose file.'
          }
        } finally {
          if (current === request) pending.value = false
        }
      }, 350)
    },
    { immediate: true, deep: true }
  )

  onBeforeUnmount(() => clearTimeout(timer))

  const services = computed(() =>
    composePreview(compose.value).services.map(service => ({
      label: service.name,
      value: service.name
    }))
  )

  return { compose, pending, error, services }
}
