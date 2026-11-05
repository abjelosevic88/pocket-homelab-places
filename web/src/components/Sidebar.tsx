// Left column: the add button, sections, how many visits wait for a rating,
// your lists — the Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { Icon } from './icons'
import { createItems, openCreate, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { lists, toRate } = useData()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Places'
  const nav: { to: string; label: string; icon: string; end?: boolean; count?: string }[] = [
    { to: '/', label: 'Overview', icon: 'home', end: true, count: toRate ? String(toRate) : undefined },
    { to: '/places', label: 'Places', icon: 'pin' },
    { to: '/map', label: 'Map', icon: 'map' },
    { to: '/lists', label: 'Lists', icon: 'list' },
    { to: '/visits', label: 'Visits', icon: 'journal' },
  ]
  return (
    <aside className="sidebar" aria-label="Pocket Places">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Places' ? title : <>Pocket <b>Places</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
        <Icon name="plus" size={15} strokeWidth={2.2} />Add<Icon name="chevron-down" size={15} className="create-caret" />
      </button>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={248} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className="sb-count" title="Visits to rate">{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      <section className="sb-sec">
        <h4 className="sb-h"><Link to="/lists" className="sb-h-btn" onClick={onNavigate}>My lists</Link></h4>
        {lists && lists.length === 0 && <button type="button" className="sb-row sb-empty" onClick={() => { onNavigate(); openCreate('list') }}><Icon name="plus" size={14} />Make a list</button>}
        {(lists || []).map(l => (
          <NavLink key={l.id} to={`/lists/${l.id}`} onClick={onNavigate} className={({ isActive }) => `sb-row sb-list ${isActive ? 'active' : ''}`}>
            <span className="sb-list-ico" aria-hidden="true">{l.emoji || <Icon name="list" size={14} />}</span>
            <span className="grow ellipsis">{l.name}</span>
            <span className="muted small tnum">{l.count}</span>
          </NavLink>
        ))}
      </section>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('ppl:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
