import type { GitHubApp, GitHubManifestStart, GitHubRepository } from '~/types/github'

const app = ref<GitHubApp | null>(null)
const repositories = ref<GitHubRepository[]>([])
const pending = ref(false)
const error = ref<string | null>(null)

export function useGitHub() {
  const api = useApi()

  async function load() {
    pending.value = true
    error.value = null
    try {
      app.value = await api.get<GitHubApp | null>('/github')
      if (app.value?.installationId) await loadRepositories()
      else repositories.value = []
    } catch (err: any) {
      error.value = err.message
    } finally {
      pending.value = false
    }
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

  async function loadRepositories() {
    repositories.value = await api.get<GitHubRepository[]>('/github/repositories')
  }

  async function disconnect() {
    await api.del('/github')
    app.value = null
    repositories.value = []
  }

  return { app, repositories, pending, error, load, create, install, loadRepositories, disconnect }
}
