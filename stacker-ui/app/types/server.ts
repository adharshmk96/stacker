export interface ServerSettings {
  instance: {
    hostname: string
    ip?: string
    version: string
    builtAt?: string
    startedAt: string
    docker?: string
    os?: string
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

export interface ServiceInfo {
  name: string
  image?: string
  version?: string
  running: number
  desired: number
  status: 'healthy' | 'degraded' | 'unavailable'
  updatedAt?: string
}
