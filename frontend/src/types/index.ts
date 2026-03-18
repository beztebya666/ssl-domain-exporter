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

  overall_status: 'ok' | 'warning' | 'critical' | 'error' | 'unknown'
  check_duration_ms: number
}

export interface Domain {
  id: number
  name: string
  port: number
  enabled: boolean
  check_interval: number
  tags: string
  folder_id?: number | null
  sort_order: number
  custom_ca_pem: string
  created_at: string
  updated_at: string
  last_check?: Check
}

export interface Folder {
  id: number
  name: string
  sort_order: number
  created_at: string
  updated_at: string
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
  server: {
    host: string
    port: string
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
  notifications: {
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
  }
  domains: {
    subdomain_fallback: boolean
    fallback_depth: number
  }
  prometheus: {
    enabled: boolean
    path: string
  }
  logging: {
    json: boolean
  }
}
