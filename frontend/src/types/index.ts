export interface ChainCert {
  subject: string
  issuer: string
  valid_from: string
  valid_to: string
  is_ca: boolean
  is_self_signed: boolean
}

export interface Check {
  id: number
  domain_id: number
  checked_at: string

  domain_status: string
  domain_registrar: string
  domain_created_at: string | null
  domain_expires_at: string | null
  domain_expiry_days: number | null
  domain_check_error: string
  domain_source: string

  ssl_issuer: string
  ssl_subject: string
  ssl_valid_from: string | null
  ssl_valid_until: string | null
  ssl_expiry_days: number | null
  ssl_version: string
  ssl_check_error: string

  ssl_chain_valid: boolean
  ssl_chain_length: number
  ssl_chain_error: string
  ssl_chain_details: ChainCert[]

  http_status_code: number
  http_redirects_https: boolean
  http_hsts_enabled: boolean
  http_hsts_max_age: string
  http_response_time_ms: number
  http_final_url: string
  http_error: string

  cipher_weak: boolean
  cipher_weak_reason: string
  cipher_grade: string
  cipher_details: string

  ocsp_status: string
  ocsp_error: string
  crl_status: string
  crl_error: string

  caa_present: boolean
  caa: string
  caa_query_domain: string
  caa_error: string

  registration_check_skipped: boolean
  registration_skip_reason: string
  dns_server_used: string
  primary_reason_code: string
  primary_reason_text: string
  status_reasons: StatusReason[]

  overall_status: 'ok' | 'warning' | 'critical' | 'error' | 'unknown'
  check_duration_ms: number
}

export interface StatusReason {
  code: string
  severity: 'advisory' | 'warning' | 'critical' | 'error'
  summary: string
  detail?: string
}

export interface Domain {
  id: number
  name: string
  port: number
  enabled: boolean
  check_interval: number
  tags: string[]
  metadata: Record<string, string>
  folder_id?: number | null
  sort_order: number
  custom_ca_pem: string
  check_mode: string
  dns_servers: string
  created_at: string
  updated_at: string
  last_check?: Check
}

export interface DomainWritePayload {
  name?: string
  domain?: string
  tags?: string[] | string | null
  metadata?: Record<string, string> | null
  enabled?: boolean
  check_interval?: number
  custom_ca_pem?: string
  port?: number
  folder_id?: number | null
  check_mode?: string
  dns_servers?: string
}

export interface DomainImportSummary {
  total: number
  created: number
  updated: number
  skipped: number
  failed: number
}

export interface DomainImportResult {
  index: number
  name?: string
  action: string
  error?: string
  domain?: Domain
}

export interface DomainImportResponse {
  mode: string
  dry_run: boolean
  summary: DomainImportSummary
  results: DomainImportResult[]
}

export interface DomainImportRequest {
  mode?: 'create_only' | 'upsert'
  dry_run?: boolean
  trigger_checks?: boolean
  defaults?: Record<string, unknown>
  domains: Array<Record<string, unknown>>
}

export interface Folder {
  id: number
  name: string
  domain_count: number
  sort_order: number
  created_at: string
  updated_at: string
}

export interface CustomFieldOption {
  id?: number
  field_id?: number
  value: string
  label: string
  sort_order?: number
}

export interface CustomField {
  id: number
  key: string
  label: string
  type: 'text' | 'textarea' | 'email' | 'url' | 'date' | 'select'
  required: boolean
  placeholder: string
  help_text: string
  sort_order: number
  visible_in_table: boolean
  visible_in_details: boolean
  visible_in_export: boolean
  filterable: boolean
  enabled: boolean
  options: CustomFieldOption[]
  created_at: string
  updated_at: string
}

export interface CustomFieldWritePayload {
  key: string
  label: string
  type: CustomField['type']
  required: boolean
  placeholder: string
  help_text: string
  sort_order: number
  visible_in_table: boolean
  visible_in_details: boolean
  visible_in_export: boolean
  filterable: boolean
  enabled: boolean
  options: CustomFieldOption[]
}

export interface DomainListParams {
  search?: string
  status?: string
  tag?: string
  folder_id?: number | string
  metadata_filters?: Record<string, string>
  ssl_expiry_lte?: number
  domain_expiry_lte?: number
  sort_by?: 'custom' | 'name' | 'status' | 'ssl_expiry' | 'domain_expiry' | 'last_check' | 'created_at'
  sort_dir?: 'asc' | 'desc'
  page?: number
  page_size?: number
}

export interface DomainListResponse {
  items: Domain[]
  total: number
  page: number
  page_size: number
  total_pages: number
  sort_by: string
  sort_dir: string
}

export interface CheckHistoryPage {
  items: Check[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface TimelineEntry {
  domain_id: number
  name: string
  kind: 'ssl' | 'domain'
  days: number
  issuer?: string
}

export interface TimelinePage {
  items: TimelineEntry[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface TimelineSummary {
  ssl_critical: number
  ssl_warning: number
  domain_critical: number
  domain_warning: number
}

export interface TimelineResponse {
  summary: TimelineSummary
  ssl: TimelinePage
  domain: TimelinePage
}

export interface Summary {
  total: number
  ok: number
  warning: number
  critical: number
  error: number
  unknown: number
}

export interface AppConfig {
  warnings?: string[]
  server: {
    host: string
    port: string
    allowed_origins: string[]
  }
  database: {
    path: string
  }
  auth: {
    enabled: boolean
    mode: 'basic' | 'api_key' | 'both'
    username: string
    password: string
    api_key: string
    protect_api: boolean
    protect_metrics: boolean
    protect_ui: boolean
    session_ttl: string
    cookie_name: string
    cookie_secure: boolean
  }
  checker: {
    interval: string
    timeout: string
    concurrent_checks: number
    retry_count: number
  }
  features: {
    http_check: boolean
    cipher_check: boolean
    ocsp_check: boolean
    crl_check: boolean
    caa_check: boolean
    notifications: boolean
    csv_export: boolean
    timeline_view: boolean
    dashboard_tag_filter: boolean
    structured_logs: boolean
  }
  alerts: {
    domain_expiry_warning_days: number
    domain_expiry_critical_days: number
    ssl_expiry_warning_days: number
    ssl_expiry_critical_days: number
  }
  status_policy: {
    badge_on_invalid_chain: boolean
    badge_on_self_signed: boolean
    badge_on_http_probe_error: boolean
    badge_on_http_client_error: boolean
    badge_on_cipher_warning: boolean
    badge_on_ocsp_unknown: boolean
    badge_on_crl_unknown: boolean
    badge_on_caa_missing: boolean
    badge_on_domain_lookup_error: boolean
  }
  notifications: {
    timeout: string
    webhook: {
      enabled: boolean
      url: string
      on_critical: boolean
      on_warning: boolean
    }
    telegram: {
      enabled: boolean
      bot_token: string
      chat_id: string
      on_critical: boolean
      on_warning: boolean
    }
    email: {
      enabled: boolean
      host: string
      port: number
      username: string
      password: string
      from: string
      to: string[]
      mode: 'starttls' | 'tls' | 'none'
      on_critical: boolean
      on_warning: boolean
      subject_prefix: string
      insecure_skip_verify: boolean
    }
  }
  dns: {
    servers: string[]
    use_system_dns: boolean
    timeout: string
  }
  security: {
    csrf_enabled: boolean
    rate_limit_enabled: boolean
    login_requests: number
    login_window: string
    admin_write_requests: number
    admin_window: string
  }
  domains: {
    subdomain_fallback: boolean
    fallback_depth: number
    default_check_mode: string
  }
  prometheus: {
    enabled: boolean
    path: string
    labels: {
      export_tags: boolean
      export_metadata: boolean
      metadata_keys: string[]
    }
  }
  maintenance: {
    backups_dir: string
    check_retention_days: number
    audit_retention_days: number
    retention_sweep_interval: string
  }
  logging: {
    json: boolean
  }
}

export interface BootstrapConfig {
  auth: {
    enabled: boolean
    public_ui: boolean
    anonymous_read_only: boolean
    mode: 'basic' | 'api_key' | 'both'
  }
  prometheus: {
    enabled: boolean
    path: string
    public: boolean
  }
  features: AppConfig['features']
  alerts: AppConfig['alerts']
  domains: {
    default_check_mode: string
  }
}

export interface AuthMe {
  authenticated: boolean
  username?: string
  role: 'anonymous' | 'viewer' | 'editor' | 'admin'
  source: string
  can_view: boolean
  can_edit: boolean
  can_admin: boolean
  public_ui: boolean
}

export interface NotificationDeliveryStatus {
  channel: 'webhook' | 'telegram' | 'email'
  enabled: boolean
  last_attempt_at?: string | null
  last_success_at?: string | null
  last_error?: string
}

export type NotificationChannel = 'webhook' | 'telegram' | 'email'

export interface NotificationTestResult {
  channel: NotificationChannel
  enabled: boolean
  success: boolean
  error?: string
}

export interface NotificationTestRequest {
  channel?: NotificationChannel
  features?: Pick<AppConfig['features'], 'notifications'>
  notifications?: AppConfig['notifications']
}

export interface UserAccount {
  id: number
  username: string
  role: 'viewer' | 'editor' | 'admin'
  enabled: boolean
  last_login_at?: string | null
  created_at: string
  updated_at: string
}

export interface UserWritePayload {
  username: string
  role: 'viewer' | 'editor' | 'admin'
  enabled?: boolean
  password?: string
}

export interface AuditLog {
  id: number
  actor_user_id?: number | null
  actor_username: string
  actor_role: string
  actor_source: string
  action: string
  resource: string
  resource_id?: number | null
  summary: string
  details?: Record<string, unknown>
  remote_addr: string
  request_id: string
  created_at: string
}

export interface BackupFile {
  name: string
  path: string
  size_bytes: number
  modified_at: string
}

export interface HealthStatus {
  status: 'ok' | 'degraded'
  database: string
  scheduler: {
    started: boolean
    in_flight?: number
    last_error?: string
    last_tick_at?: string | null
    last_session_cleanup_at?: string | null
    last_retention_sweep_at?: string | null
  }
}
