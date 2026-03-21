import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { LucideIcon } from 'lucide-react'
import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import {
  LayoutDashboard,
  Globe,
  Settings,
  Activity,
  Moon,
  Sun,
  GanttChartSquare,
  LogIn,
  LogOut,
  ShieldCheck,
  Loader2,
} from 'lucide-react'
import Dashboard from './pages/Dashboard'
import Domains from './pages/Domains'
import DomainDetail from './pages/DomainDetail'
import SettingsPage from './pages/Settings'
import Timeline from './pages/Timeline'
import {
  fetchConfig,
  checkAuthorization,
  getStoredBasicAuth,
  setStoredBasicAuth,
  clearStoredBasicAuth,
  AUTH_UNAUTHORIZED_EVENT,
  AUTH_STATUS_CHANGED_EVENT,
} from './api/client'

function NavItem({ to, icon: Icon, label }: { to: string; icon: LucideIcon; label: string }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
          isActive
            ? 'bg-blue-600/20 text-blue-400'
            : 'text-slate-400 hover:text-slate-100 hover:bg-slate-700'
        }`
      }
    >
      <Icon size={18} />
      {label}
    </NavLink>
  )
}

type Theme = 'dark' | 'light'
const THEME_KEY = 'ui-theme'

type AuthState = 'checking' | 'authorized' | 'unauthorized'

function getInitialTheme(): Theme {
  const saved = localStorage.getItem(THEME_KEY)
  if (saved === 'dark' || saved === 'light') {
    return saved
  }
  return 'dark'
}

export default function App() {
  const queryClient = useQueryClient()
  const [theme, setTheme] = useState<Theme>(getInitialTheme)
  const [authState, setAuthState] = useState<AuthState>('checking')
  const [authModalOpen, setAuthModalOpen] = useState(false)
  const [authUsername, setAuthUsername] = useState(getStoredBasicAuth()?.username ?? 'admin')
  const [authPassword, setAuthPassword] = useState(getStoredBasicAuth()?.password ?? 'admin')
  const [authError, setAuthError] = useState('')
  const isAuthorized = authState === 'authorized'

  const { data: cfg } = useQuery({
    queryKey: ['config'],
    queryFn: fetchConfig,
    retry: false,
    enabled: isAuthorized,
  })

  useEffect(() => {
    const isLight = theme === 'light'
    document.documentElement.classList.toggle('theme-light', isLight)
    document.documentElement.style.colorScheme = isLight ? 'light' : 'dark'
    localStorage.setItem(THEME_KEY, theme)
  }, [theme])

  useEffect(() => {
    let isMounted = true

    const refreshAuth = async () => {
      setAuthState('checking')
      try {
        const ok = await checkAuthorization()
        if (!isMounted) return
        setAuthState(ok ? 'authorized' : 'unauthorized')
      } catch {
        if (!isMounted) return
        setAuthState('unauthorized')
      }
    }

    void refreshAuth()

    const onUnauthorized = () => {
      if (!isMounted) return
      setAuthState('unauthorized')
      setAuthModalOpen(true)
    }

    const onStatusChanged = () => {
      const stored = getStoredBasicAuth()
      setAuthUsername(stored?.username ?? 'admin')
    }

    window.addEventListener(AUTH_UNAUTHORIZED_EVENT, onUnauthorized)
    window.addEventListener(AUTH_STATUS_CHANGED_EVENT, onStatusChanged)

    return () => {
      isMounted = false
      window.removeEventListener(AUTH_UNAUTHORIZED_EVENT, onUnauthorized)
      window.removeEventListener(AUTH_STATUS_CHANGED_EVENT, onStatusChanged)
    }
  }, [])

  const toggleTheme = () => {
    setTheme(current => (current === 'dark' ? 'light' : 'dark'))
  }

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setAuthError('')

    const user = authUsername.trim()
    if (!user || !authPassword) {
      setAuthError('Username and password are required.')
      return
    }

    setStoredBasicAuth(user, authPassword)

    try {
      const ok = await checkAuthorization()
      if (ok) {
        setAuthState('authorized')
        setAuthModalOpen(false)
        queryClient.invalidateQueries()
        return
      }
    } catch {
      // handled below
    }

    clearStoredBasicAuth()
    setAuthState('unauthorized')
    setAuthError('Authorization failed. Check username/password.')
    setAuthModalOpen(true)
  }

  const handleLogout = () => {
    clearStoredBasicAuth()
    setAuthState('unauthorized')
    setAuthModalOpen(true)
    setAuthError('')
    queryClient.clear()
  }

  const showAuthOverlay = useMemo(() => authState !== 'authorized' || authModalOpen, [authModalOpen, authState])
  const canCloseModal = authState === 'authorized'

  return (
    <BrowserRouter>
      <div className="flex h-screen overflow-hidden">
        <aside className="w-56 bg-slate-900 border-r border-slate-700/60 flex flex-col flex-shrink-0">
          <div className="p-5 border-b border-slate-700/60">
            <div className="flex items-center gap-2">
              <Activity size={22} className="text-blue-400" />
              <span className="font-bold text-white">SSL Domain Exporter</span>
            </div>
            <p className="text-xs text-gray-500 mt-1">Certificate, Domain & Validation Checks</p>
          </div>

          <nav className="flex-1 p-3 space-y-1">
            <NavItem to="/" icon={LayoutDashboard} label="Dashboard" />
            <NavItem to="/domains" icon={Globe} label="Domains" />
            {cfg?.features.timeline_view && (
              <NavItem to="/timeline" icon={GanttChartSquare} label="Timeline" />
            )}
            <NavItem to="/settings" icon={Settings} label="Settings" />
          </nav>

          <div className="p-3 border-t border-slate-700/60">
            <button className="btn-ghost w-full justify-center" onClick={toggleTheme}>
              {theme === 'dark' ? (
                <>
                  <Sun size={14} />
                  Light Theme
                </>
              ) : (
                <>
                  <Moon size={14} />
                  Dark Theme
                </>
              )}
            </button>
          </div>

          <div className="p-3 border-t border-slate-700/60">
            {isAuthorized ? (
              <button className="btn-ghost w-full justify-center" onClick={handleLogout}>
                <LogOut size={14} />
                Log out
              </button>
            ) : (
              <button className="btn-primary w-full justify-center" onClick={() => setAuthModalOpen(true)}>
                <LogIn size={14} />
                Log in
              </button>
            )}
          </div>

          <div className="p-3 border-t border-slate-700/60">
            <a
              href="/metrics"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-3 py-2 text-xs text-slate-500 hover:text-slate-300 transition-colors rounded-lg hover:bg-slate-700"
            >
              <Activity size={14} />
              Prometheus Metrics
            </a>
          </div>
        </aside>

        <main className="flex-1 overflow-auto">
          {isAuthorized ? (
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/domains" element={<Domains />} />
              <Route path="/domains/:id" element={<DomainDetail />} />
              <Route path="/timeline" element={<Timeline />} />
              <Route path="/settings" element={<SettingsPage />} />
            </Routes>
          ) : (
            <div className="h-full bg-slate-950" />
          )}
        </main>
      </div>

      {showAuthOverlay && (
        <div className="fixed inset-0 bg-slate-950/96 flex items-center justify-center z-[100] p-4">
          <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-900 shadow-2xl">
            <div className="px-5 py-4 border-b border-slate-800">
              <div className="flex items-center gap-2 text-white font-semibold">
                <ShieldCheck size={16} className="text-blue-400" />
                Authorization Required
              </div>
              <p className="text-xs text-slate-500 mt-1">Sign in with API credentials from `config.yaml` (`auth.username` / `auth.password`).</p>
            </div>

            <form className="p-5 space-y-4" onSubmit={handleLogin}>
              <div>
                <label className="label">Username</label>
                <input
                  className="input"
                  value={authUsername}
                  onChange={e => setAuthUsername(e.target.value)}
                  autoComplete="username"
                />
              </div>
              <div>
                <label className="label">Password</label>
                <input
                  className="input"
                  type="password"
                  value={authPassword}
                  onChange={e => setAuthPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </div>

              {authError && (
                <div className="text-xs text-amber-300 bg-amber-500/10 border border-amber-600/20 rounded-lg px-3 py-2">
                  {authError}
                </div>
              )}

              <div className="flex gap-3 pt-2">
                {canCloseModal ? (
                  <button type="button" className="btn-ghost flex-1 border border-slate-700" onClick={() => setAuthModalOpen(false)}>
                    Close
                  </button>
                ) : (
                  <div className="btn-ghost flex-1 border border-slate-700 justify-center opacity-70 pointer-events-none">Locked</div>
                )}
                <button type="submit" className="btn-primary flex-1" disabled={authState === 'checking'}>
                  {authState === 'checking' ? <Loader2 size={14} className="animate-spin" /> : <LogIn size={14} />}
                  Log in
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </BrowserRouter>
  )
}
