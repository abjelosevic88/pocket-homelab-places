// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ListModal, PlaceModal, VisitModal } from './Forms'
import { Avatar, type Anchor, type MenuItem, Menu, MenuButton, modalOpen, rectOf, useIsMobile, useMediaQuery, useToast } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'ppl-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'place' | 'visit' | 'plan' | 'list'

/** Open a create dialog from anywhere (a page's button). */
export const openCreate = (k: CreateKind) => window.dispatchEvent(new CustomEvent('ppl:create', { detail: { kind: k } }))

/** The add menu: the sidebar's "Add" button and the phone's + button. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'Add a place', icon: 'pin', hint: 'A', onSelect: () => onCreate('place') },
    { label: 'Log a visit', icon: 'check-circle', hint: 'V', onSelect: () => onCreate('visit') },
    { label: 'Plan a visit', icon: 'calendar-plus', hint: 'B', onSelect: () => onCreate('plan') },
    'sep',
    { label: 'A new list', icon: 'list-plus', onSelect: () => onCreate('list') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const [sp, setSp] = useSearchParams()
  const { toast } = useToast()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [modal, setModal] = useState<{ kind: CreateKind } | null>(null)
  const [fabMenu, setFabMenu] = useState<Anchor | null>(null)

  const create = (k: CreateKind) => {
    closeDrawer()
    setModal({ kind: k })
  }
  const createRef = useRef(create)
  createRef.current = create

  // /?add=place (the PWA shortcut) opens a dialog
  useEffect(() => { const a = sp.get('add'); if (a) { createRef.current((['place', 'visit', 'plan', 'list'].includes(a) ? a : 'place') as CreateKind); setSp(new URLSearchParams(), { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind }>).detail; createRef.current(d.kind) }
    window.addEventListener('ppl:shortcuts', open)
    window.addEventListener('ppl:create', make)
    return () => { window.removeEventListener('ppl:shortcuts', open); window.removeEventListener('ppl:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'a': mk('place'); break
        case 'v': mk('visit'); break
        case 'b': mk('plan'); break
        case 'h': go('/'); break
        case 'p': go('/places'); break
        case 'm': go('/map'); break
        case 'l': go('/lists'); break
        case 'd': go('/visits'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'place', label: 'Add a place', icon: 'pin', hint: 'A', keywords: 'new restaurant cafe bar museum save', run: () => create('place') },
    { id: 'visit', label: 'Log a visit', icon: 'check-circle', hint: 'V', keywords: 'went ate rate review', run: () => create('visit') },
    { id: 'plan', label: 'Plan a visit', icon: 'calendar-plus', hint: 'B', keywords: 'book table reservation calendar', run: () => create('plan') },
    { id: 'list', label: 'A new list', icon: 'list-plus', keywords: 'collection trip date night', run: () => create('list') },
    { id: 'home', label: 'Overview', icon: 'home', hint: 'H', run: () => navigate('/') },
    { id: 'places', label: 'Places', icon: 'pin', hint: 'P', keywords: 'all want to go been favourites', run: () => navigate('/places') },
    { id: 'want', label: 'Places I want to try', icon: 'flag', keywords: 'want to go wishlist', run: () => navigate('/places?status=want') },
    { id: 'favs', label: 'My favourites', icon: 'heart', keywords: 'best loved', run: () => navigate('/places?favourite=1') },
    { id: 'map', label: 'Map', icon: 'map', hint: 'M', keywords: 'near me nearby', run: () => navigate('/map') },
    { id: 'lists', label: 'Lists', icon: 'list', hint: 'L', run: () => navigate('/lists') },
    { id: 'visits', label: 'Visits', icon: 'journal', hint: 'D', keywords: 'diary history spent costs', run: () => navigate('/visits') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'layers', keywords: 'movement money calendar contacts travel split core', run: () => navigate('/settings/apps') },
    { id: 'import', label: 'Import from Google Maps', icon: 'upload', keywords: 'takeout saved places csv', run: () => navigate('/settings/data') },
    { id: 'notify', label: 'Settings: notifications', icon: 'bell', keywords: 'push how was it', run: () => navigate('/settings/notifications') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  const close = () => setModal(null)
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner"><Outlet /></div></main>
        </div>
        {mobile && <button type="button" className="fab" aria-label="Add" title="Add" aria-haspopup="menu" aria-expanded={!!fabMenu} onClick={e => setFabMenu(fabMenu ? null : rectOf(e.currentTarget))}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {fabMenu && <Menu anchor={fabMenu} align="right" items={createItems(create)} onClose={() => setFabMenu(null)} width={250} />}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {modal?.kind === 'place' && <PlaceModal preset={listPreset(loc.pathname)} onClose={close} onSaved={p => navigate(`/places/${p.id}`)} />}
        {(modal?.kind === 'visit' || modal?.kind === 'plan') && <VisitModal plan={modal.kind === 'plan'} placeId={placeHere(loc.pathname)} onClose={close} />}
        {modal?.kind === 'list' && <ListModal onClose={close} onSaved={l => navigate(`/lists/${l.id}`)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

/** On a place's page, a new visit is at that place. */
function placeHere(path: string): number | undefined {
  const m = path.match(/^\/places\/(\d+)/)
  return m ? Number(m[1]) : undefined
}

/** On a list's page, a new place goes on that list. */
function listPreset(path: string): { lists?: number[] } {
  const m = path.match(/^\/lists\/(\d+)/)
  return m ? { lists: [Number(m[1])] } : {}
}

const SECTION: Record<string, [string, string]> = { '': ['Overview', '/'], places: ['Places', '/places'], map: ['Map', '/map'], lists: ['Lists', '/lists'], visits: ['Visits', '/visits'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('ppl:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
