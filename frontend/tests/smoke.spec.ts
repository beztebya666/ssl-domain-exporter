import { expect, test, type Page, type Route } from '@playwright/test'

type MockState = {
  authenticated: boolean
  backups: Array<{ name: string; path: string; size_bytes: number; modified_at: string }>
}

const bootstrap = {
  auth: {
    enabled: true,
    public_ui: false,
    anonymous_read_only: false,
    mode: 'basic',
  },
  prometheus: {
    enabled: true,
    path: '/metrics',
    public: false,
  },
  features: {
    http_check: true,
    cipher_check: true,
    ocsp_check: true,
    crl_check: true,
    caa_check: true,
    notifications: true,
    csv_export: true,
    timeline_view: true,
    dashboard_tag_filter: true,
    structured_logs: false,
  },
  alerts: {
    domain_expiry_warning_days: 30,
    domain_expiry_critical_days: 7,
    ssl_expiry_warning_days: 14,
    ssl_expiry_critical_days: 3,
  },
  domains: {
    default_check_mode: 'full',
  },
}

const adminUser = {
  authenticated: true,
  username: 'admin',
  role: 'admin',
  source: 'session',
  can_view: true,
  can_edit: true,
  can_admin: true,
  public_ui: false,
}

const anonymousUser = {
  authenticated: false,
  role: 'anonymous',
  source: 'anonymous',
  can_view: false,
  can_edit: false,
  can_admin: false,
  public_ui: false,
}

const domainsPage = {
  items: [
    {
      id: 1,
      name: 'example.com',
      port: 443,
      enabled: true,
      check_interval: 21600,
      tags: ['prod'],
      metadata: { env: 'prod', owner_email: 'owner@example.com' },
      sort_order: 1,
      custom_ca_pem: '',
      check_mode: 'full',
      dns_servers: '',
      created_at: '2026-03-24T10:00:00Z',
      updated_at: '2026-03-25T10:00:00Z',
      last_check: {
        id: 101,
        domain_id: 1,
        checked_at: '2026-03-25T10:00:00Z',
        domain_status: 'active',
        domain_registrar: 'Example Registrar',
        domain_created_at: null,
        domain_expires_at: null,
        domain_expiry_days: 40,
        domain_check_error: '',
        domain_source: 'rdap',
        ssl_issuer: 'Example CA',
        ssl_subject: 'example.com',
        ssl_valid_from: null,
        ssl_valid_until: null,
        ssl_expiry_days: 20,
        ssl_version: 'TLSv1.3',
        ssl_check_error: '',
        ssl_chain_valid: true,
        ssl_chain_length: 2,
        ssl_chain_error: '',
        ssl_chain_details: [],
        http_status_code: 200,
        http_redirects_https: true,
        http_hsts_enabled: true,
        http_hsts_max_age: '31536000',
        http_response_time_ms: 120,
        http_final_url: 'https://example.com',
        http_error: '',
        cipher_weak: false,
        cipher_weak_reason: '',
        cipher_grade: 'A',
        cipher_details: '',
        ocsp_status: 'good',
        ocsp_error: '',
        crl_status: 'good',
        crl_error: '',
        caa_present: true,
        caa: '0 issue "letsencrypt.org"',
        caa_query_domain: 'example.com',
        caa_error: '',
        registration_check_skipped: false,
        registration_skip_reason: '',
        dns_server_used: '',
        primary_reason_code: '',
        primary_reason_text: '',
        status_reasons: [],
        overall_status: 'ok',
        check_duration_ms: 250,
      },
    },
  ],
  total: 1,
  page: 1,
  page_size: 20,
  total_pages: 1,
  sort_by: 'status',
  sort_dir: 'asc',
}

const configPayload = {
  warnings: ['Default legacy admin credentials admin/admin are still active.'],
  server: { host: '0.0.0.0', port: '8080', allowed_origins: [] },
  database: { path: './data/checker.db' },
  auth: {
    enabled: true,
    mode: 'basic',
    username: 'admin',
    password: '__REDACTED__',
    api_key: '',
    protect_api: true,
    protect_metrics: false,
    protect_ui: true,
    session_ttl: '24h',
    cookie_name: 'ssl_domain_exporter_session',
    cookie_secure: false,
  },
  checker: {
    interval: '6h',
    timeout: '30s',
    concurrent_checks: 5,
    retry_count: 2,
  },
  features: bootstrap.features,
  alerts: bootstrap.alerts,
  status_policy: {
    badge_on_invalid_chain: true,
    badge_on_self_signed: true,
    badge_on_http_probe_error: true,
    badge_on_http_client_error: true,
    badge_on_cipher_warning: true,
    badge_on_ocsp_unknown: true,
    badge_on_crl_unknown: true,
    badge_on_caa_missing: true,
    badge_on_domain_lookup_error: true,
  },
  notifications: {
    timeout: '15s',
    webhook: { enabled: false, url: '', on_critical: true, on_warning: false },
    telegram: { enabled: false, bot_token: '', chat_id: '', on_critical: true, on_warning: false },
    email: {
      enabled: false,
      host: '',
      port: 587,
      username: '',
      password: '',
      from: '',
      to: [],
      mode: 'starttls',
      on_critical: true,
      on_warning: false,
      subject_prefix: '[SSL Domain Exporter]',
      insecure_skip_verify: false,
    },
  },
  dns: {
    servers: [],
    use_system_dns: true,
    timeout: '5s',
  },
  security: {
    csrf_enabled: true,
    rate_limit_enabled: true,
    login_requests: 10,
    login_window: '5m',
    admin_write_requests: 300,
    admin_window: '1m',
  },
  domains: {
    subdomain_fallback: true,
    fallback_depth: 5,
    default_check_mode: 'full',
  },
  prometheus: {
    enabled: true,
    path: '/metrics',
    labels: {
      export_tags: true,
      export_metadata: true,
      metadata_keys: [],
    },
  },
  maintenance: {
    backups_dir: './data/backups',
    check_retention_days: 0,
    audit_retention_days: 0,
    retention_sweep_interval: '24h',
  },
  logging: {
    json: false,
  },
}

async function mockApi(page: Page, initialAuth = true): Promise<MockState> {
  const state: MockState = {
    authenticated: initialAuth,
    backups: [
      {
        name: 'checker-20260325-100000.db',
        path: './data/backups/checker-20260325-100000.db',
        size_bytes: 1024 * 1024,
        modified_at: '2026-03-25T10:00:00Z',
      },
    ],
  }

  page.on('pageerror', (error) => {
    console.log('pageerror', error.message)
  })
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      console.log('console', msg.text())
    }
  })

  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (!path.startsWith('/api/')) {
      await route.continue()
      return
    }
    await fulfillApiRoute(route, state)
  })
  await page.route('**/health', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        status: 'ok',
        database: 'ok',
        scheduler: { started: true, in_flight: 0 },
      }),
    })
  })
  await page.route('**/ready', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        status: 'ok',
        database: 'ok',
        scheduler: { started: true, in_flight: 0 },
      }),
    })
  })

  return state
}

async function fulfillApiRoute(route: Route, state: MockState): Promise<void> {
  const request = route.request()
  const url = new URL(request.url())
  const path = url.pathname
  const method = request.method()

  const json = async (status: number, payload: unknown) => {
    await route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(payload),
    })
  }

  if (path === '/api/bootstrap') {
    await json(200, bootstrap)
    return
  }
  if (path === '/api/me') {
    await json(200, state.authenticated ? adminUser : anonymousUser)
    return
  }
  if (path === '/api/session/login' && method === 'POST') {
    state.authenticated = true
    await json(200, adminUser)
    return
  }
  if (path === '/api/session/logout' && method === 'POST') {
    state.authenticated = false
    await json(200, { ok: true })
    return
  }
  if (path === '/api/summary') {
    await json(200, { total: 1, ok: 1, warning: 0, critical: 0, error: 0, unknown: 0 })
    return
  }
  if (path === '/api/domains/search') {
    await json(200, domainsPage)
    return
  }
  if (path === '/api/domains') {
    await json(200, domainsPage.items)
    return
  }
  if (path === '/api/folders') {
    await json(200, [])
    return
  }
  if (path === '/api/tags') {
    await json(200, ['prod'])
    return
  }
  if (path === '/api/custom-fields') {
    await json(200, [])
    return
  }
  if (path === '/api/config') {
    await json(200, configPayload)
    return
  }
  if (path === '/api/users') {
    await json(200, [{
      id: 1,
      username: 'admin',
      role: 'admin',
      enabled: true,
      created_at: '2026-03-20T08:00:00Z',
      updated_at: '2026-03-25T08:00:00Z',
      last_login_at: '2026-03-25T10:00:00Z',
    }])
    return
  }
  if (path === '/api/notification-status' || path === '/api/notifications/status') {
    await json(200, [])
    return
  }
  if (path === '/api/audit-logs') {
    await json(200, [{
      id: 1,
      actor_username: 'admin',
      actor_role: 'admin',
      actor_source: 'session',
      action: 'update',
      resource: 'config',
      summary: 'Updated application config',
      details: { sections: ['security', 'maintenance'] },
      remote_addr: '127.0.0.1',
      request_id: 'req-test',
      created_at: '2026-03-25T10:00:00Z',
    }])
    return
  }
  if (path === '/api/maintenance/backups') {
    await json(200, state.backups)
    return
  }
  if (path === '/api/maintenance/backup' && method === 'POST') {
    const backup = {
      name: `checker-${state.backups.length + 1}.db`,
      path: `./data/backups/checker-${state.backups.length + 1}.db`,
      size_bytes: 2048 * 1024,
      modified_at: '2026-03-25T11:00:00Z',
    }
    state.backups.unshift(backup)
    await json(201, backup)
    return
  }
  if (path === '/api/maintenance/prune' && method === 'POST') {
    await json(200, { ok: true, days: 30, cutoff: '2026-02-24T00:00:00Z', removed: 42 })
    return
  }
  if (path === '/api/notifications/test' && method === 'POST') {
    await json(200, [{ channel: 'email', enabled: false, success: false, error: 'channel is disabled or not configured' }])
    return
  }

  await json(200, {})
}

test('dashboard and domains navigation render with mocked data', async ({ page }) => {
  await mockApi(page, true)
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
  await expect(page.getByText('example.com').first()).toBeVisible()

  await page.getByRole('link', { name: 'Domains' }).click()
  await expect(page.getByRole('heading', { name: 'Domains' })).toBeVisible()
  await expect(page.getByText('example.com').first()).toBeVisible()
})

test('administration page shows maintenance and audit surfaces', async ({ page }) => {
  const state = await mockApi(page, true)
  await page.goto('/settings')

  await expect(page.getByRole('heading', { name: 'Administration' })).toBeVisible()
  await page.getByText('Maintenance & Health').click()
  await expect(page.getByText('Audit retention days')).toBeVisible()
  await page.getByText('Audit Log').click()
  await expect(page.getByText('checker-20260325-100000.db').first()).toBeVisible()

  await page.getByRole('button', { name: 'Create backup' }).click()
  await expect(page.getByText(`checker-${state.backups.length}.db`).first()).toBeVisible()
})

test('login modal signs in and unlocks the app', async ({ page }) => {
  await mockApi(page, false)
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'Login required' })).toBeVisible()
  await page.getByRole('main').getByRole('button', { name: 'Sign in' }).click()

  await page.getByLabel('Username').fill('admin')
  await page.getByLabel('Password').fill('admin')
  await page.getByRole('dialog').getByRole('button', { name: 'Sign in' }).click()

  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
  await expect(page.getByText('example.com').first()).toBeVisible()
})
