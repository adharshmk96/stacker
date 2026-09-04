export interface ServerSettings {
  instance: {
    hostname: string
    ip?: string
    version: string
    builtAt?: string
    startedAt: string
    docker?: string
    os?: string
    revision?: string
    repository?: string
  }
  traefik: {
    domain: string
    https: boolean
    certificateResolver?: string
    backendTarget?: string
    httpRedirect: boolean
    publishedPorts: string[]
    stackName: string
    stackerService: ServiceInfo
    traefikService: ServiceInfo
  }
}

export interface UpdateCandidate {
  channel: 'stable' | 'edge'
  version: string
  revision: string
  publishedAt?: string
  available: boolean
}

export interface ServerUpdates {
  stable: UpdateCandidate
  edge: UpdateCandidate
  updating: boolean
  error?: string
}

export interface ServiceInfo {
  name: string
  image?: string
  version?: string
  running: number
  desired: number
  status: 'healthy' | 'degraded' | 'unavailable'
  updatedAt?: string
}
