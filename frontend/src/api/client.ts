import axios from 'axios'
import type {
  AppConfig,
  AuthMe,
  BootstrapConfig,
  Check,
  CheckHistoryPage,
  CustomField,
  CustomFieldWritePayload,
  Domain,
  DomainListParams,
  DomainListResponse,
  DomainImportRequest,
  DomainImportResponse,
  DomainWritePayload,
  Folder,
  NotificationDeliveryStatus,
  NotificationTestResult,
  Summary,
  TimelineResponse,
  UserAccount,
  UserWritePayload,
} from '../types'

const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true,
  timeout: 20000,
})

export const AUTH_STATUS_CHANGED_EVENT = 'auth:status-changed'
export const AUTH_UNAUTHORIZED_EVENT = 'auth:unauthorized'

function emitWindowEvent(name: string): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new Event(name))
}

api.interceptors.response.use(
  (response) => response,
  (err) => {
    if (axios.isAxiosError(err) && err.response?.status === 401) {
      emitWindowEvent(AUTH_UNAUTHORIZED_EVENT)
    }
    return Promise.reject(err)
  },
)

export async function checkAuthorization(): Promise<boolean> {
  try {
    const me = await fetchMe()
    return me.can_view
  } catch (err) {
    if (axios.isAxiosError(err) && err.response?.status === 401) {
      return false
    }
    throw err
  }
}

export const fetchBootstrap = (): Promise<BootstrapConfig> =>
  api.get('/bootstrap').then(r => r.data)

export const fetchMe = (): Promise<AuthMe> =>
  api.get('/me').then(r => r.data)

export const loginSession = async (username: string, password: string): Promise<AuthMe> => {
  const response = await api.post('/session/login', { username, password })
  emitWindowEvent(AUTH_STATUS_CHANGED_EVENT)
  return response.data
}

export const logoutSession = async (): Promise<void> => {
  await api.post('/session/logout')
  emitWindowEvent(AUTH_STATUS_CHANGED_EVENT)
}

export const fetchDomains = (): Promise<Domain[]> =>
  api.get('/domains').then(r => r.data)

export const fetchDomainsPage = (params: DomainListParams): Promise<DomainListResponse> =>
  api.get('/domains/search', { params: buildDomainListParams(params) }).then(r => r.data)

export const fetchDomain = (id: number): Promise<Domain> =>
  api.get(`/domains/${id}`).then(r => r.data)

export const createDomain = (data: DomainWritePayload): Promise<Domain> =>
  api.post('/domains', data).then(r => r.data)

export const updateDomain = (id: number, data: DomainWritePayload): Promise<Domain> =>
  api.put(`/domains/${id}`, data).then(r => r.data)

export const importDomains = (data: DomainImportRequest): Promise<DomainImportResponse> =>
  api.post('/domains/import', data).then(r => r.data)

export const deleteDomain = (id: number): Promise<void> =>
  api.delete(`/domains/${id}`).then(r => r.data)

export const triggerCheck = (id: number): Promise<Check> =>
  api.post(`/domains/${id}/check`).then(r => r.data)

export const reorderDomains = (ids: number[]): Promise<void> =>
  api.post('/domains/reorder', { ids }).then(() => undefined)

export const fetchFolders = (): Promise<Folder[]> =>
  api.get('/folders').then(r => r.data)

export const fetchTags = (): Promise<string[]> =>
  api.get('/tags').then(r => r.data)

export const fetchCustomFields = (includeDisabled = false): Promise<CustomField[]> =>
  api.get('/custom-fields', { params: includeDisabled ? { include_disabled: true } : undefined }).then(r => r.data)

export const createCustomField = (data: CustomFieldWritePayload): Promise<CustomField> =>
  api.post('/custom-fields', data).then(r => r.data)

export const updateCustomField = (id: number, data: CustomFieldWritePayload): Promise<CustomField> =>
  api.put(`/custom-fields/${id}`, data).then(r => r.data)

export const deleteCustomField = (id: number): Promise<void> =>
  api.delete(`/custom-fields/${id}`).then(() => undefined)

export const createFolder = (name: string): Promise<Folder> =>
  api.post('/folders', { name }).then(r => r.data)

export const updateFolder = (id: number, name: string): Promise<Folder> =>
  api.put(`/folders/${id}`, { name }).then(r => r.data)

export const deleteFolder = (id: number): Promise<void> =>
  api.delete(`/folders/${id}`).then(() => undefined)

export const fetchHistory = (id: number, limit = 50): Promise<Check[]> =>
  api.get(`/domains/${id}/history?limit=${limit}`).then(r => r.data)

export const fetchHistoryPage = (id: number, page = 1, pageSize = 20): Promise<CheckHistoryPage> =>
  api.get(`/domains/${id}/history`, { params: { page, page_size: pageSize } }).then(r => r.data)

export const fetchSummary = (): Promise<Summary> =>
  api.get('/summary').then(r => r.data)

export const fetchTimeline = (params?: {
  ssl_page?: number
  ssl_page_size?: number
  domain_page?: number
  domain_page_size?: number
}): Promise<TimelineResponse> =>
  api.get('/timeline', { params }).then(r => r.data)

export const fetchConfig = (): Promise<AppConfig> =>
  api.get('/config').then(r => r.data)

export const updateConfig = (data: Partial<AppConfig>): Promise<AppConfig> =>
  api.put('/config', data).then(r => r.data)

export const fetchNotificationStatus = (): Promise<NotificationDeliveryStatus[]> =>
  api.get('/notifications/status').then(r => r.data)

export const testNotifications = (): Promise<NotificationTestResult[]> =>
  api.post('/notifications/test').then(r => r.data)

export const fetchUsers = (): Promise<UserAccount[]> =>
  api.get('/users').then(r => r.data)

export const createUser = (data: UserWritePayload): Promise<UserAccount> =>
  api.post('/users', data).then(r => r.data)

export const updateUserAccount = (id: number, data: Partial<UserWritePayload>): Promise<UserAccount> =>
  api.put(`/users/${id}`, data).then(r => r.data)

export const deleteUserAccount = (id: number): Promise<void> =>
  api.delete(`/users/${id}`).then(() => undefined)

export const exportDomainsCsvUrl = (params?: DomainListParams): string => {
  const search = new URLSearchParams()
  const encoded = buildDomainListParams(params)
  if (encoded) {
    Object.entries(encoded).forEach(([key, value]) => {
      if (value == null || value === '' || key === 'page' || key === 'page_size') return
      search.set(key, String(value))
    })
  }
  const query = search.toString()
  return query ? `/api/domains/export.csv?${query}` : '/api/domains/export.csv'
}

function buildDomainListParams(params?: DomainListParams): Record<string, string | number> | undefined {
  if (!params) return undefined
  const encoded: Record<string, string | number> = {}
  Object.entries(params).forEach(([key, value]) => {
    if (value == null || value === '') return
    if (key === 'metadata_filters' && typeof value === 'object') {
      const entries = Object.entries(value as Record<string, string>).filter(([, filterValue]) => String(filterValue).trim() !== '')
      if (entries.length === 0) return
      encoded[key] = JSON.stringify(Object.fromEntries(entries))
      return
    }
    encoded[key] = value as string | number
  })
  return encoded
}
