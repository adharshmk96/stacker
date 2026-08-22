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
  /** Used to fill in a project's branch when the repository is picked */
  defaultBranch: string
}

export interface GitHubManifestStart {
  url: string
  manifest: Record<string, unknown>
}

export interface ExistingGitHubApp {
  name: string
  appId: number | null
  installationId: number | null
  privateKey: string
  webhookSecret: string
}
