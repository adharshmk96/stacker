import type { ExistingGitHubApp, GitHubApp, GitHubManifestStart, GitHubRepository } from '~/types/github'

const app = ref<GitHubApp | null>(null)
const repositories = ref<GitHubRepository[]>([])
const pending = ref(false)
const error = ref<string | null>(null)

let inflight: Promise<void> | null = null
let loaded = false

export function useGitHub() {
  const api = useApi()

  /**
   * Loads the app and its repositories.
   *
   * Guarded, because there are now two callers: the Settings page, which owns
   * the connection, and the project form, which only wants the repository list
   * to pick from. Two pages mounting at once should not mean two round trips,
   * and neither should have to know the other exists.
   */
  async function load(force = false) {
    if (loaded && !force) return
    if (inflight) return inflight

    pending.value = true
    error.value = null

    inflight = (async () => {
      try {
        app.value = await api.get<GitHubApp | null>('/github')
        if (app.value?.installationId) await loadRepositories()
        else repositories.value = []
        loaded = true
      } catch (err: any) {
        error.value = err.message
      } finally {
        pending.value = false
        inflight = null
      }
    })()

    return inflight
  }

  async function create(name: string, organization = '') {
    const target = 'stacker-github-app'
    const githubTab = window.open('', target)
    if (!githubTab) throw new Error('Allow popups for Stacker to continue with GitHub')

    try {
      const result = await api.post<GitHubManifestStart>('/github/apps', {
        name, organization, baseUrl: window.location.origin
      })
      const form = document.createElement('form')
      form.method = 'POST'
      form.action = result.url
      form.target = target
      const manifest = document.createElement('input')
      manifest.type = 'hidden'
      manifest.name = 'manifest'
      manifest.value = JSON.stringify(result.manifest)
      form.appendChild(manifest)
      document.body.appendChild(form)
      form.submit()
      form.remove()
    } catch (error) {
      githubTab.close()
      throw error
    }
  }

  function install() {
    if (!app.value?.slug) throw new Error('Create the GitHub App first')
    window.location.assign(`https://github.com/apps/${encodeURIComponent(app.value.slug)}/installations/new`)
  }

  async function connectExisting(existing: ExistingGitHubApp) {
    const connected = await api.post<GitHubApp>('/github/apps/existing', existing)
    app.value = connected
    await loadRepositories()
    loaded = true
  }

  async function loadRepositories() {
    repositories.value = await api.get<GitHubRepository[]>('/github/repositories')
  }

  async function disconnect() {
    await api.del('/github')
    app.value = null
    repositories.value = []
    // Cleared so the next load actually asks: reconnecting has to be able to
    // repopulate a list this just emptied.
    loaded = false
  }

  return { app, repositories, pending, error, load, create, install, connectExisting, loadRepositories, disconnect }
}
