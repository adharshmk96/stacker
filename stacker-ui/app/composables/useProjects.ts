import type { Domain, Environment, Project, ProjectPayload } from '~/types/project'

/**
 * Projects — placeholder only.
 *
 * There is no `/api/projects` yet, so the list lives in module scope and is
 * seeded with a couple of rows. Everything is synchronous and lost on reload;
 * when the server side lands this file becomes the only thing to rewrite
 * (compare `useNodes`, which has the same surface over real requests).
 */

export function blankDomain(): Domain {
  return {
    id: crypto.randomUUID(),
    host: '',
    service: '',
    port: 80,
    tls: 'auto',
    redirectWww: false
  }
}

export function blankEnvironment(name = 'production'): Environment {
  return {
    id: crypto.randomUUID(),
    name,
    branch: '',
    variables: [],
    secrets: [],
    domains: [],
    trigger: { kind: 'manual', pattern: '' },
    deploy: {
      strategy: 'rolling',
      replicas: 1,
      placement: '',
      healthGraceSec: 30,
      autoRollback: true,
      alwaysPull: false
    }
  }
}

export function blankProject(): ProjectPayload {
  return {
    name: '',
    description: '',
    sourceKind: 'git',
    git: { provider: 'github', repo: '', branch: 'main', composePath: 'docker-compose.yml' },
    compose: '',
    environments: [blankEnvironment()]
  }
}

const now = new Date().toISOString()

const items = ref<Project[]>([
  {
    id: 'p-storefront',
    name: 'storefront',
    description: 'Public web store and its edge cache',
    sourceKind: 'git',
    git: {
      provider: 'github',
      repo: 'acme/storefront',
      branch: 'main',
      composePath: 'deploy/docker-compose.yml'
    },
    compose: '',
    environments: [
      {
        ...blankEnvironment('production'),
        id: 'e-storefront-prod',
        branch: 'main',
        variables: [
          { key: 'NODE_ENV', value: 'production' },
          { key: 'API_URL', value: 'https://api.acme.dev' }
        ],
        secrets: [{ key: 'SESSION_SECRET', value: '••••••••' }],
        domains: [
          { id: 'd-shop', host: 'shop.acme.dev', service: 'web', port: 3000, tls: 'auto', redirectWww: true }
        ],
        trigger: { kind: 'tag', pattern: 'v*' },
        deploy: {
          strategy: 'rolling',
          replicas: 3,
          placement: 'node.labels.tier==edge',
          healthGraceSec: 30,
          autoRollback: true,
          alwaysPull: false
        }
      },
      {
        ...blankEnvironment('staging'),
        id: 'e-storefront-staging',
        branch: 'develop',
        variables: [{ key: 'NODE_ENV', value: 'staging' }],
        domains: [
          { id: 'd-shop-staging', host: 'staging.acme.dev', service: 'web', port: 3000, tls: 'auto', redirectWww: false }
        ],
        trigger: { kind: 'push', pattern: '' }
      }
    ],
    createdAt: now,
    updatedAt: now
  },
  {
    id: 'p-metrics',
    name: 'metrics',
    description: 'Prometheus and Grafana, pinned to the manager node',
    sourceKind: 'compose',
    git: { provider: 'github', repo: '', branch: 'main', composePath: 'docker-compose.yml' },
    compose: 'services:\n  prometheus:\n    image: prom/prometheus:latest\n    ports:\n      - "9090:9090"\n',
    environments: [
      {
        ...blankEnvironment('production'),
        id: 'e-metrics-prod',
        variables: [{ key: 'RETENTION', value: '30d' }],
        domains: [
          { id: 'd-metrics', host: 'metrics.acme.dev', service: 'grafana', port: 3000, tls: 'auto', redirectWww: false }
        ],
        deploy: {
          strategy: 'recreate',
          replicas: 1,
          placement: 'node.role==manager',
          healthGraceSec: 15,
          autoRollback: false,
          alwaysPull: true
        }
      }
    ],
    createdAt: now,
    updatedAt: now
  }
])

export function useProjects() {
  const find = (id: string) => items.value.find(project => project.id === id) ?? null

  function create(payload: ProjectPayload) {
    const stamp = new Date().toISOString()
    const project: Project = {
      ...payload,
      id: `p-${crypto.randomUUID().slice(0, 8)}`,
      createdAt: stamp,
      updatedAt: stamp
    }
    items.value = [project, ...items.value]
    return project
  }

  function update(id: string, payload: ProjectPayload) {
    const index = items.value.findIndex(project => project.id === id)
    if (index === -1) throw new Error(`No project ${id}`)

    const project: Project = {
      ...items.value[index]!,
      ...payload,
      updatedAt: new Date().toISOString()
    }
    items.value = items.value.toSpliced(index, 1, project)
    return project
  }

  function remove(id: string) {
    items.value = items.value.filter(project => project.id !== id)
  }

  return { items, find, create, update, remove }
}
