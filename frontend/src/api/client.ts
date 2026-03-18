import axios from 'axios'
import type { Domain, Check, Summary, AppConfig, Folder } from '../types'

const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
})

type BasicAuth = {
  username: string
  password: string
}

const BASIC_AUTH_STORAGE_KEY = 'ssl-monitor-basic-auth'
export const AUTH_STATUS_CHANGED_EVENT = 'auth:status-changed'
export const AUTH_UNAUTHORIZED_EVENT = 'auth:unauthorized'

function toBasicHeader(username: string, password: string): string {
  return `Basic ${btoa(`${username}:${password}`)}`
}

function emitWindowEvent(name: string): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new Event(name))
}

export function getStoredBasicAuth(): BasicAuth | null {
  if (typeof window === 'undefined') return null
  const raw = localStorage.getItem(BASIC_AUTH_STORAGE_KEY)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as BasicAuth
    if (!parsed.username || !parsed.password) return null
    return parsed
  } catch {
    return null
  }
}

export function setStoredBasicAuth(username: string, password: string): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(BASIC_AUTH_STORAGE_KEY, JSON.stringify({ username, password }))
  emitWindowEvent(AUTH_STATUS_CHANGED_EVENT)
}

export function clearStoredBasicAuth(): void {
  if (typeof window === 'undefined') return
  localStorage.removeItem(BASIC_AUTH_STORAGE_KEY)
  emitWindowEvent(AUTH_STATUS_CHANGED_EVENT)
}

export async function checkAuthorization(): Promise<boolean> {
  try {
    await api.get('/config')
    return true
  } catch (err) {
    if (axios.isAxiosError(err) && err.response?.status === 401) {
      return false
    }
    throw err
  }
}

api.interceptors.request.use((config) => {
  const auth = getStoredBasicAuth()
  if (auth) {
    config.headers = config.headers ?? {}
    if (!config.headers.Authorization) {
      config.headers.Authorization = toBasicHeader(auth.username, auth.password)
    }
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (err) => {
    if (axios.isAxiosError(err) && err.response?.status === 401) {
      emitWindowEvent(AUTH_UNAUTHORIZED_EVENT)
    }
    return Promise.reject(err)
  },
)

export const fetchDomains = (): Promise<Domain[]> =>
  api.get('/domains').then(r => r.data)

export const fetchDomain = (id: number): Promise<Domain> =>
  api.get(`/domains/${id}`).then(r => r.data)

export const createDomain = (data: {
  name: string
  tags?: string
  check_interval?: number
  custom_ca_pem?: string
  port?: number
  folder_id?: number | null
}): Promise<Domain> =>
  api.post('/domains', data).then(r => r.data)

export const updateDomain = (id: number, data: Partial<Domain>): Promise<Domain> =>
  api.put(`/domains/${id}`, data).then(r => r.data)

export const deleteDomain = (id: number): Promise<void> =>
  api.delete(`/domains/${id}`).then(r => r.data)

export const triggerCheck = (id: number): Promise<Check> =>
  api.post(`/domains/${id}/check`).then(r => r.data)

export const reorderDomains = (ids: number[]): Promise<void> =>
  api.post('/domains/reorder', { ids }).then(() => undefined)

export const fetchFolders = (): Promise<Folder[]> =>
  api.get('/folders').then(r => r.data)

export const createFolder = (name: string): Promise<Folder> =>
  api.post('/folders', { name }).then(r => r.data)

export const updateFolder = (id: number, name: string): Promise<Folder> =>
  api.put(`/folders/${id}`, { name }).then(r => r.data)

export const deleteFolder = (id: number): Promise<void> =>
  api.delete(`/folders/${id}`).then(() => undefined)

export const fetchHistory = (id: number, limit = 50): Promise<Check[]> =>
  api.get(`/domains/${id}/history?limit=${limit}`).then(r => r.data)

export const fetchSummary = (): Promise<Summary> =>
  api.get('/summary').then(r => r.data)

export const fetchConfig = (): Promise<AppConfig> =>
  api.get('/config').then(r => r.data)

export const updateConfig = (data: Partial<AppConfig>): Promise<AppConfig> =>
  api.put('/config', data).then(r => r.data)

export const exportDomainsCsvUrl = (): string => '/api/domains/export.csv'
