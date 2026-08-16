export interface GitHubApp {
  id: string
  name: string
  appId: number
  slug: string
  installationId: number
  account: string
  accountType: string
  repositorySelection: 'all' | 'selected' | ''
  createdAt: string
  updatedAt: string
}

export interface GitHubRepository {
  id: number
  fullName: string
  private: boolean
  htmlUrl: string
}

export interface GitHubManifestStart {
  url: string
  manifest: Record<string, unknown>
}
