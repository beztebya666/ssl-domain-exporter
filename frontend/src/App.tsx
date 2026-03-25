import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { LucideIcon } from 'lucide-react'
import { BrowserRouter, NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import {
  Activity,
  ChevronsLeft,
  ChevronsRight,
  GanttChartSquare,
  Globe,
  LayoutDashboard,
  Loader2,
  Lock,
  LogIn,
  LogOut,
  Menu,
  Moon,
  ShieldCheck,
  Sun,
  UserCog,
  X,
} from 'lucide-react'
import Dashboard from './pages/Dashboard'
import Domains from './pages/Domains'
import {
  AUTH_STATUS_CHANGED_EVENT,
  AUTH_UNAUTHORIZED_EVENT,
  fetchBootstrap,
  fetchMe,
  loginSession,
  logoutSession,
} from './api/client'
import ModalShell from './components/ModalShell'
import { DetailSkeleton, ListCardSkeleton, PageHeadingSkeleton, TableSkeleton } from './components/Skeleton'

const DomainDetailPage = lazy(() => import('./pages/DomainDetail'))
const SettingsPage = lazy(() => import('./pages/Settings'))
const TimelinePage = lazy(() => import('./pages/Timeline'))

type Theme = 'dark' | 'light'
const THEME_KEY = 'ui-theme'
const SIDEBAR_COLLAPSED_KEY = 'ui-sidebar-collapsed'

function getInitialTheme(): Theme {
  const saved = localStorage.getItem(THEME_KEY)
  if (saved === 'dark' || saved === 'light') {
    return saved
  }
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

function getInitialSidebarCollapsed(): boolean {
  return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === 'true'
}

function NavItem({
  to,
  icon: Icon,
  label,
  collapsed = false,
  onNavigate,
}: {
  to: string
  icon: LucideIcon
  label: string
  collapsed?: boolean
  onNavigate?: () => void
}) {
  return (
    <NavLink
      to={to}
      onClick={onNavigate}
      title={collapsed ? label : undefined}
      className={({ isActive }) =>
        `flex items-center rounded-lg border-l-2 py-2.5 text-sm font-medium transition-colors ${
          isActive
            ? 'border-blue-400 bg-blue-600/20 text-blue-400'
            : 'border-transparent text-slate-400 hover:border-slate-600 hover:text-slate-100 hover:bg-slate-700'
        } ${collapsed ? 'justify-center px-0' : 'gap-3 pl-[calc(0.75rem-2px)] pr-3'}`
      }
    >
      <Icon size={18} />
      {!collapsed && label}
    </NavLink>
  )
}

function AccessDenied({ onLogin }: { onLogin: () => void }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="max-w-lg rounded-2xl border border-slate-700 bg-slate-900/90 p-8 text-center shadow-2xl">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10 text-blue-400">
          <Lock size={20} />
        </div>
        <h2 className="text-xl font-semibold text-white">Administrator access required</h2>
        <p className="mt-2 text-sm text-slate-400">
          This area contains configuration and user management controls. Sign in with an administrator account to continue.
        </p>
        <button className="btn-primary mt-5" onClick={onLogin}>
          <LogIn size={14} />
          Sign in
        </button>
      </div>
    </div>
  )
}

function ViewLocked({ onLogin }: { onLogin: () => void }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="max-w-xl rounded-2xl border border-slate-700 bg-slate-900/90 p-8 text-center shadow-2xl">
        <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-blue-500/10 text-blue-400">
          <ShieldCheck size={24} />
        </div>
        <h1 className="text-2xl font-semibold text-white">Login required</h1>
        <p className="mt-3 text-sm text-slate-400">
          This deployment does not expose the monitoring dashboard publicly. Sign in with a local user account or the legacy admin credentials from <code>config.yaml</code>.
        </p>
        <button className="btn-primary mt-6" onClick={onLogin}>
          <LogIn size={14} />
          Sign in
        </button>
      </div>
    </div>
  )
}

function RouteFallback({ kind }: { kind: 'detail' | 'settings' | 'timeline' }) {
  if (kind === 'detail') {
    return <DetailSkeleton />
  }
  return (
    <div className="space-y-6 p-6">
      <PageHeadingSkeleton />
      {kind === 'timeline' ? <ListCardSkeleton count={5} /> : (
        <>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <ListCardSkeleton count={2} />
            <ListCardSkeleton count={2} />
          </div>
          <TableSkeleton rows={5} columns={4} />
        </>
      )}
    </div>
  )
}

function SidebarContent({
  canView,
  canAdmin,
  me,
  publicUI,
  bootstrap,
  metricsVisible,
  metricsPath,
  collapsed,
  onToggleCollapsed,
  theme,
  onToggleTheme,
  onLogin,
  onLogout,
  onNavigate,
}: {
  canView: boolean
  canAdmin: boolean
  me?: Awaited<ReturnType<typeof fetchMe>>
  publicUI: boolean
  bootstrap?: Awaited<ReturnType<typeof fetchBootstrap>>
  metricsVisible: boolean
  metricsPath: string
  collapsed: boolean
  onToggleCollapsed?: () => void
  theme: Theme
  onToggleTheme: () => void
  onLogin: () => void
  onLogout: () => void
  onNavigate?: () => void
}) {
  const roleBadge = useMemo(() => {
    if (!me?.authenticated) {
      return publicUI ? 'Public read-only' : 'Guest'
    }
    return `${me.role} access`
  }, [me?.authenticated, me?.role, publicUI])

  return (
    <>
      <div className="border-b border-slate-700/60 p-4">
        <div className={`flex items-start ${collapsed ? 'justify-center' : 'justify-between gap-3'}`}>
          <NavLink to="/" onClick={onNavigate} className={`group flex items-center ${collapsed ? 'justify-center' : 'gap-2.5'}`} title={collapsed ? 'SSL Domain Exporter' : undefined}>
          <div className="flex-shrink-0 rounded-lg bg-blue-500/15 p-1.5">
            <Activity size={18} className="text-blue-400" />
          </div>
          {!collapsed && (
            <div className="min-w-0">
            <div className="text-sm font-bold leading-tight text-white transition-colors group-hover:text-blue-400">SSL Domain Exporter</div>
            <div className="mt-0.5 text-[10px] leading-tight text-slate-500">Certificate, Domain & Validation Checks</div>
            </div>
          )}
        </NavLink>
          {!collapsed && onToggleCollapsed && (
            <button
              type="button"
              className="btn-ghost border border-slate-700 p-2"
              onClick={onToggleCollapsed}
              aria-label="Collapse desktop sidebar"
              title="Collapse sidebar"
            >
              <ChevronsLeft size={14} />
            </button>
          )}
        </div>
        {!collapsed && (
          <div className="mt-2.5 flex flex-wrap gap-1.5 text-[11px]">
          <span className="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 text-slate-300">{roleBadge}</span>
          {me?.can_edit && (
            <span className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2 py-0.5 text-emerald-300">Editor</span>
          )}
          </div>
        )}
        {collapsed && onToggleCollapsed && (
          <div className="mt-3 flex justify-center">
            <button
              type="button"
              className="btn-ghost border border-slate-700 p-2"
              onClick={onToggleCollapsed}
              aria-label="Expand desktop sidebar"
              title="Expand sidebar"
            >
              <ChevronsRight size={14} />
            </button>
          </div>
        )}
      </div>

      <nav className="flex-1 space-y-1 p-3">
        {canView && <NavItem to="/" icon={LayoutDashboard} label="Dashboard" collapsed={collapsed} onNavigate={onNavigate} />}
        {canView && <NavItem to="/domains" icon={Globe} label="Domains" collapsed={collapsed} onNavigate={onNavigate} />}
        {canView && bootstrap?.features.timeline_view && (
          <NavItem to="/timeline" icon={GanttChartSquare} label="Timeline" collapsed={collapsed} onNavigate={onNavigate} />
        )}
        {canAdmin && <NavItem to="/settings" icon={UserCog} label="Administration" collapsed={collapsed} onNavigate={onNavigate} />}
      </nav>

      <div className="space-y-2 border-t border-slate-700/60 p-3">
        <button className="btn-ghost w-full justify-center" onClick={onToggleTheme} title={collapsed ? (theme === 'dark' ? 'Light Theme' : 'Dark Theme') : undefined}>
          {theme === 'dark' ? (
            <>
              <Sun size={14} />
              {!collapsed && 'Light Theme'}
            </>
          ) : (
            <>
              <Moon size={14} />
              {!collapsed && 'Dark Theme'}
            </>
          )}
        </button>

        {me?.authenticated ? (
          <button className="btn-ghost w-full justify-center" onClick={onLogout} title={collapsed ? 'Log out' : undefined}>
            <LogOut size={14} />
            {!collapsed && 'Log out'}
          </button>
        ) : bootstrap?.auth.enabled ? (
          <button className="btn-primary w-full justify-center" onClick={onLogin} title={collapsed ? 'Sign in' : undefined}>
            <LogIn size={14} />
            {!collapsed && 'Sign in'}
          </button>
        ) : null}
      </div>

      {metricsVisible && (
        <div className="border-t border-slate-700/60 p-3">
          <a
            href={metricsPath}
            target="_blank"
            rel="noopener noreferrer"
            className={`flex items-center rounded-lg px-3 py-2 text-xs text-slate-500 transition-colors hover:bg-slate-700 hover:text-slate-300 ${collapsed ? 'justify-center' : 'gap-2'}`}
            title={collapsed ? 'Prometheus Metrics' : undefined}
          >
            <Activity size={14} />
            {!collapsed && 'Prometheus Metrics'}
          </a>
        </div>
      )}
    </>
  )
}

function AppShell() {
  const queryClient = useQueryClient()
  const location = useLocation()
  const [theme, setTheme] = useState<Theme>(getInitialTheme)
  const [authModalOpen, setAuthModalOpen] = useState(false)
  const [authUsername, setAuthUsername] = useState('')
  const [authPassword, setAuthPassword] = useState('')
  const [authError, setAuthError] = useState('')
  const [submittingAuth, setSubmittingAuth] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [desktopSidebarCollapsed, setDesktopSidebarCollapsed] = useState(getInitialSidebarCollapsed)

  const { data: bootstrap, isLoading: bootstrapLoading } = useQuery({
    queryKey: ['bootstrap'],
    queryFn: fetchBootstrap,
    retry: false,
  })
  const { data: me, isLoading: meLoading } = useQuery({
    queryKey: ['me'],
    queryFn: fetchMe,
    retry: false,
  })

  const canView = me?.can_view ?? false
  const canAdmin = me?.can_admin ?? false
  const publicUI = bootstrap?.auth.public_ui ?? false
  const metricsVisible = Boolean(bootstrap?.prometheus.enabled && (bootstrap.prometheus.public || canAdmin))
  const metricsPath = bootstrap?.prometheus.path || '/metrics'

  useEffect(() => {
    const isLight = theme === 'light'
    document.documentElement.classList.toggle('theme-light', isLight)
    document.documentElement.style.colorScheme = isLight ? 'light' : 'dark'
    localStorage.setItem(THEME_KEY, theme)
  }, [theme])

  useEffect(() => {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(desktopSidebarCollapsed))
  }, [desktopSidebarCollapsed])

  useEffect(() => {
    const onUnauthorized = () => {
      queryClient.invalidateQueries({ queryKey: ['me'] })
      setAuthModalOpen(true)
    }

    const onStatusChanged = () => {
      queryClient.invalidateQueries({ queryKey: ['me'] })
    }

    window.addEventListener(AUTH_UNAUTHORIZED_EVENT, onUnauthorized)
    window.addEventListener(AUTH_STATUS_CHANGED_EVENT, onStatusChanged)

    return () => {
      window.removeEventListener(AUTH_UNAUTHORIZED_EVENT, onUnauthorized)
      window.removeEventListener(AUTH_STATUS_CHANGED_EVENT, onStatusChanged)
    }
  }, [queryClient])

  useEffect(() => {
    setMobileNavOpen(false)
  }, [location.pathname, location.search])

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmittingAuth(true)
    setAuthError('')
    try {
      await loginSession(authUsername.trim(), authPassword)
      setAuthModalOpen(false)
      setAuthPassword('')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['me'] }),
        queryClient.invalidateQueries({ queryKey: ['config'] }),
      ])
    } catch {
      setAuthError('Authorization failed. Check username/password or user status.')
    } finally {
      setSubmittingAuth(false)
    }
  }

  const handleLogout = async () => {
    await logoutSession()
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['me'] }),
      queryClient.invalidateQueries({ queryKey: ['config'] }),
      queryClient.invalidateQueries({ queryKey: ['domains'] }),
      queryClient.invalidateQueries({ queryKey: ['domains-page'] }),
      queryClient.invalidateQueries({ queryKey: ['summary'] }),
    ])
  }

  if (bootstrapLoading || meLoading) {
    return (
      <div className="h-screen overflow-auto bg-slate-950 p-6">
        <div className="mx-auto max-w-6xl space-y-6">
          <PageHeadingSkeleton />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, index) => (
              <div key={index} className="card p-5">
                <div className="flex items-center gap-4">
                  <div className="h-12 w-12 animate-pulse rounded-2xl bg-slate-800/90" />
                  <div className="flex-1 space-y-2">
                    <div className="h-7 w-24 animate-pulse rounded-xl bg-slate-800/90" />
                    <div className="h-4 w-32 animate-pulse rounded-xl bg-slate-800/90" />
                  </div>
                </div>
              </div>
            ))}
          </div>
          <ListCardSkeleton count={4} />
          <TableSkeleton rows={5} columns={4} />
        </div>
      </div>
    )
  }

  return (
    <>
      <div className="flex h-screen overflow-hidden bg-slate-950">
        <aside className={`sidebar-shell hidden flex-shrink-0 flex-col border-r border-slate-700/60 bg-slate-900 shadow-[18px_0_36px_rgb(2_6_23/0.22)] transition-[width] duration-200 lg:flex ${desktopSidebarCollapsed ? 'w-[88px]' : 'w-60'}`}>
          <SidebarContent
            canView={canView}
            canAdmin={canAdmin}
            me={me}
            publicUI={publicUI}
            bootstrap={bootstrap}
            metricsVisible={metricsVisible}
            metricsPath={metricsPath}
            collapsed={desktopSidebarCollapsed}
            onToggleCollapsed={() => setDesktopSidebarCollapsed(current => !current)}
            theme={theme}
            onToggleTheme={() => setTheme(current => (current === 'dark' ? 'light' : 'dark'))}
            onLogin={() => setAuthModalOpen(true)}
            onLogout={handleLogout}
          />
        </aside>

        <main className="flex-1 overflow-auto">
          <div className="sticky top-0 z-30 border-b border-slate-800/80 bg-slate-950/95 px-4 py-3 lg:hidden">
            <div className="flex items-center justify-between gap-3">
              <button
                type="button"
                className="btn-ghost border border-slate-700 p-2"
                onClick={() => setMobileNavOpen(true)}
                aria-label="Open navigation"
              >
                <Menu size={18} />
              </button>
              <div className="min-w-0 flex-1 text-center">
                <div className="truncate text-sm font-semibold text-white">SSL Domain Exporter</div>
                <div className="truncate text-[11px] text-slate-500">Certificate, Domain & Validation Checks</div>
              </div>
              <button
                type="button"
                className="btn-ghost border border-slate-700 p-2"
                onClick={() => setTheme(current => (current === 'dark' ? 'light' : 'dark'))}
                aria-label="Toggle theme"
              >
                {theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
              </button>
            </div>
          </div>

          <div key={`${location.pathname}${location.search}`} className="page-transition min-h-full">
            {canView ? (
              <Routes>
                <Route path="/" element={<Dashboard me={me} bootstrap={bootstrap} />} />
                <Route path="/domains" element={<Domains me={me} bootstrap={bootstrap} />} />
                <Route
                  path="/domains/:id"
                  element={(
                    <Suspense fallback={<RouteFallback kind="detail" />}>
                      <DomainDetailPage me={me} bootstrap={bootstrap} />
                    </Suspense>
                  )}
                />
                <Route
                  path="/timeline"
                  element={bootstrap?.features.timeline_view ? (
                    <Suspense fallback={<RouteFallback kind="timeline" />}>
                      <TimelinePage bootstrap={bootstrap} />
                    </Suspense>
                  ) : <Navigate to="/" replace />}
                />
                <Route
                  path="/settings"
                  element={canAdmin ? (
                    <Suspense fallback={<RouteFallback kind="settings" />}>
                      <SettingsPage me={me} />
                    </Suspense>
                  ) : <AccessDenied onLogin={() => setAuthModalOpen(true)} />}
                />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            ) : (
              <ViewLocked onLogin={() => setAuthModalOpen(true)} />
            )}
          </div>
        </main>
      </div>

      {mobileNavOpen && (
        <div className="fixed inset-0 z-[110] lg:hidden">
          <button
            type="button"
            className="absolute inset-0 bg-slate-950/80"
            onClick={() => setMobileNavOpen(false)}
            aria-label="Close navigation overlay"
          />
          <div className="sidebar-sheet absolute inset-y-0 left-0 flex w-72 max-w-[85vw] flex-col border-r border-slate-700/60 bg-slate-900 shadow-2xl">
            <div className="flex items-center justify-between border-b border-slate-700/60 px-4 py-3">
              <div className="text-sm font-semibold text-white">Navigation</div>
              <button
                type="button"
                className="btn-ghost border border-slate-700 p-2"
                onClick={() => setMobileNavOpen(false)}
                aria-label="Close navigation"
              >
                <X size={16} />
              </button>
            </div>
            <SidebarContent
              canView={canView}
              canAdmin={canAdmin}
              me={me}
              publicUI={publicUI}
              bootstrap={bootstrap}
              metricsVisible={metricsVisible}
              metricsPath={metricsPath}
              collapsed={false}
              theme={theme}
              onToggleTheme={() => setTheme(current => (current === 'dark' ? 'light' : 'dark'))}
              onLogin={() => {
                setMobileNavOpen(false)
                setAuthModalOpen(true)
              }}
              onLogout={handleLogout}
              onNavigate={() => setMobileNavOpen(false)}
            />
          </div>
        </div>
      )}

      {authModalOpen && (
        <ModalShell
          onClose={() => setAuthModalOpen(false)}
          panelClassName="max-w-md"
          title={(
            <div id="sign-in-title" className="flex items-center gap-2 text-white">
              <ShieldCheck size={16} className="text-blue-400" />
              Sign in
            </div>
          )}
          description={<span>Use a local UI user account or the legacy admin credentials from <code>config.yaml</code>.</span>}
        >
          <form className="space-y-4" onSubmit={handleLogin}>
            <div>
              <label className="label" htmlFor="login-username">Username</label>
              <input
                id="login-username"
                className="input"
                value={authUsername}
                onChange={e => setAuthUsername(e.target.value)}
                autoComplete="username"
              />
            </div>
            <div>
              <label className="label" htmlFor="login-password">Password</label>
              <input
                id="login-password"
                className="input"
                type="password"
                value={authPassword}
                onChange={e => setAuthPassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>

            {authError && (
              <div className="rounded-lg border border-amber-600/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-300">
                {authError}
              </div>
            )}

            <div className="flex gap-3 pt-2">
              <button type="button" className="btn-ghost flex-1 border border-slate-700" onClick={() => setAuthModalOpen(false)}>
                Close
              </button>
              <button type="submit" className="btn-primary flex-1" disabled={submittingAuth}>
                {submittingAuth ? <Loader2 size={14} className="animate-spin" /> : <LogIn size={14} />}
                Sign in
              </button>
            </div>
          </form>
        </ModalShell>
      )}
    </>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AppShell />
    </BrowserRouter>
  )
}
