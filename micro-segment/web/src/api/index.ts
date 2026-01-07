import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    const { data } = response
    if (data.code === 0) {
      return data.data
    }
    return Promise.reject(new Error(data.message || '请求失败'))
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 类型定义
export interface Workload {
  id: string
  name: string
  host_id: string
  host_name: string
  domain: string
  service: string
  image: string
  policy_mode: string
  running: boolean
  ifaces: Record<string, IPAddr[]>
}

export interface IPAddr {
  ip: string
  scope: string
}

export interface Group {
  name: string
  comment: string
  domain: string
  policy_mode: string
  members: string[]
  criteria: GroupCriteria[]
  created_at: string
  updated_at: string
}

export interface GroupCriteria {
  key: string
  value: string
  op: string
}

export interface PolicyRule {
  id: number
  comment: string
  from: string
  to: string
  ports: string
  applications: number[]
  action: string
  disable: boolean
  priority: number
  created_at: string
  updated_at: string
}

export interface Connection {
  client_wl: string
  server_wl: string
  client_ip: string
  server_ip: string
  client_port: number
  server_port: number
  ip_proto: number
  application: number
  bytes: number
  sessions: number
  policy_action: number
  severity: number
}

export interface NetworkGraph {
  nodes: GraphNode[]
  links: GraphLink[]
}

export interface GraphNode {
  id: string
  name: string
  kind: string
  domain: string
  service: string
  policy_mode: string
}

export interface GraphLink {
  from: string
  to: string
  bytes: number
  sessions: number
  severity: number
  policy_action: number
}

export interface Stats {
  workloads: number
  groups: number
  policies: number
  hosts: number
  agents: number
  graph_nodes: number
  graph_links: number
}

// API 方法
export const getStats = (): Promise<Stats> => api.get('/stats')

export const getWorkloads = (): Promise<Workload[]> => api.get('/workloads')
export const getWorkload = (id: string): Promise<Workload> => api.get('/workload', { params: { id } })

export const getGroups = (): Promise<Group[]> => api.get('/groups')
export const getGroup = (name: string): Promise<Group> => api.get('/group', { params: { name } })
export const createGroup = (group: Partial<Group>): Promise<Group> => api.post('/group', group)
export const deleteGroup = (name: string): Promise<void> => api.delete('/group', { params: { name } })

export const getPolicies = (): Promise<PolicyRule[]> => api.get('/policies')
export const getPolicy = (id: number): Promise<PolicyRule> => api.get('/policy', { params: { id } })
export const createPolicy = (policy: Partial<PolicyRule>): Promise<PolicyRule> => api.post('/policy', policy)
export const updatePolicy = (policy: PolicyRule): Promise<PolicyRule> => api.put('/policy', policy)
export const deletePolicy = (id: number): Promise<void> => api.delete('/policy', { params: { id } })

export const getNetworkGraph = (): Promise<NetworkGraph> => api.get('/graph')

export default api
