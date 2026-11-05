// Overview: visits to rate, "where should we go?", what is planned, where you
// went lately, favourites, places waiting for you, trips coming up.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { money, placeLine, whereAmI } from '../lib/places'
import type { Overview as OverviewT, Place } from '../lib/types'
import { Card, CoreHint, Empty, Loading, errMsg, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { KindBadge, PastPlanCard, PlaceRow, RateCard, Rating, VisitRow } from '../components/Rows'
import { openCreate } from '../components/Layout'

export default function Overview() {
  const { user } = useApp()
  const { data: o, reload, error } = useAsync(() => api.get<OverviewT>(`${V1}/overview`), [])
  useOnChange(reload)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} />{error}</div>
  if (!o) return <Loading />
  const first = (user?.name || '').split(' ')[0]
  if (o.counts.places === 0) return (
    <Card className="welcome">
      <div className="welcome-body">
        <img src="/favicon.svg" alt="" width={56} height={56} />
        <div className="grow">
          <h2>Welcome to Pocket Places</h2>
          <p className="muted">Keep the restaurants, cafés, bars and sights you love and the ones you want to try, with your rating, notes and what it cost. Make lists for date nights or a trip. With Pocket Movement and Pocket Money connected, it notices when you were at one of your places and asks how it was.</p>
          <div className="row wrap mt">
            <button type="button" className="btn primary" onClick={() => openCreate('place')}><Icon name="plus" size={15} />Add a place</button>
            <Link to="/settings/data" className="btn"><Icon name="upload" size={15} />Bring them in from Google Maps</Link>
          </div>
        </div>
      </div>
    </Card>
  )
  const toRate = o.inbox.length + o.past_plans.length
  const cur = o.spent_year[0]?.currency
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow">
          <h1 className="page-title">{first && first !== 'Admin' ? `Hello, ${first}` : 'Your places'}</h1>
          <p className="muted">{toRate > 0 ? `${toRate} ${toRate === 1 ? 'visit waits' : 'visits wait'} for your rating.` : `${o.counts.places} places, ${o.counts.want} you want to try.`}</p>
        </div>
      </div>

      {toRate > 0 && <section className="stack">
        <h2 className="group-h">How was it?</h2>
        <div className="rate-cards">
          {o.inbox.map(it => <RateCard key={`i${it.id}`} item={it} onDone={reload} />)}
          {o.past_plans.map(v => <PastPlanCard key={`p${v.id}`} v={v} onDone={reload} />)}
        </div>
      </section>}

      {o.trips.map(t => (
        <div key={t.trip_id} className="callout info trip">
          <Icon name="plane" size={15} />
          <div className="grow"><b>{t.name || t.destination}</b> — {t.days_until > 0 ? `${fmtDate(t.start)} to ${fmtDate(t.end)}` : `until ${fmtDate(t.end)}`}.{' '}
            {t.want + t.been > 0 ? <>{t.want} {t.want === 1 ? 'place' : 'places'} on your list there{t.been ? `, ${t.been} you have been to` : ''}. <Link to={tripLink(t)}>See them</Link></> : <>Nothing saved there yet. <button type="button" className="link-btn" onClick={() => openCreate('place')}>Add a place</button></>}
            {t.url && <> · <a href={t.url} target="_blank" rel="noreferrer">The trip</a></>}</div>
        </div>))}

      <WhereToGo />

      <div className="tiles">
        <Link to="/places?status=want" className="tile">
          <div className="tile-head"><span className="tile-ico"><Icon name="flag" size={14} /></span>Want to try</div>
          <div className="tile-val">{o.counts.want}</div>
        </Link>
        <Link to="/places?status=been" className="tile">
          <div className="tile-head"><span className="tile-ico"><Icon name="check-circle" size={14} /></span>Been there</div>
          <div className="tile-val">{o.counts.been}</div>
          <div className="muted small">{o.counts.favourites} favourites</div>
        </Link>
        <Link to="/visits" className="tile">
          <div className="tile-head"><span className="tile-ico"><Icon name="journal" size={14} /></span>Visits this year</div>
          <div className="tile-val">{o.visits_year}</div>
        </Link>
        <Link to="/visits" className="tile">
          <div className="tile-head"><span className="tile-ico"><Icon name="wallet" size={14} /></span>Spent this year</div>
          <div className="tile-val">{cur ? money(o.spent_year[0].total, cur, true) : '—'}</div>
          {o.spent_year.length > 1 && <div className="muted small">{o.spent_year.slice(1).map(r => money(r.total, r.currency, true)).join(' + ')}</div>}
        </Link>
      </div>

      <div className="two-grid">
        <Card title="Coming up" flush actions={<button type="button" className="btn ghost sm" onClick={() => openCreate('plan')}><Icon name="calendar-plus" size={14} />Plan a visit</button>}>
          {o.planned.length === 0 ? <p className="muted pad">Booked a table, or going to a museum on Saturday? Plan it and it goes on your calendar.</p>
            : <div className="list">{o.planned.map(v => <VisitRow key={v.id} v={v} />)}</div>}
        </Card>
        <Card title="Lately" flush actions={<Link to="/visits" className="btn ghost sm">All visits</Link>}>
          {o.recent.length === 0 ? <Empty title="No visits yet" icon="journal">Log where you went, how it was and what it cost.</Empty>
            : <div className="list">{o.recent.map(v => <VisitRow key={v.id} v={v} />)}</div>}
        </Card>
      </div>

      <div className="two-grid">
        <Card title="Favourites" flush actions={<Link to="/places?favourite=1" className="btn ghost sm">All</Link>}>
          {o.favourites.length === 0 ? <p className="muted pad">Mark the places you always go back to with the heart.</p>
            : <div className="list">{o.favourites.map(p => <PlaceRow key={p.id} p={p} />)}</div>}
        </Card>
        <Card title="Waiting for you" sub="The places you saved most lately" flush actions={<Link to="/places?status=want" className="btn ghost sm">All</Link>}>
          {o.want.length === 0 ? <p className="muted pad">Heard of a good place? Add it as "want to go" and it waits here.</p>
            : <div className="list">{o.want.map(p => <PlaceRow key={p.id} p={p} />)}</div>}
        </Card>
      </div>

      {o.cities.length > 1 && <Card title="Towns">
        <div className="chips">{o.cities.map(c => <Link key={c.city + c.country} to={`/places?city=${encodeURIComponent(c.city)}`} className="chip">{c.city}<span className="n">{c.count}</span></Link>)}</div>
      </Card>}
    </div>
  )
}

function tripLink(t: OverviewT['trips'][number]): string {
  if (t.list_ids.length === 1) return `/lists/${t.list_ids[0]}`
  if (t.lat != null && t.lng != null) return `/places?near=${t.lat},${t.lng}&radius_km=40&label=${encodeURIComponent(t.name || t.destination)}`
  return `/places?q=${encodeURIComponent((t.destination || t.name).split(',')[0])}`
}

/** "Where should we go?" — the AI picks from your own places. */
function WhereToGo() {
  const { settings } = useApp()
  const { toast } = useToast()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  const [q, setQ] = useState('')
  const [near, setNear] = useState(false)
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState<{ picks: { place: Place; why: string }[]; note: string } | null>(null)
  const coreOn = !!settings?.core_configured && settings.core_registered !== false
  const ask = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!q.trim()) return
    setBusy(true); setRes(null)
    try {
      const spot = near ? await whereAmI() : null
      if (near && !spot) toast('Your browser did not share where you are; picking from everywhere', 'err')
      const r = await api.post<{ ok: boolean; error?: string; picks: { place: Place; why: string }[]; note: string }>(`${V1}/ai/pick`, { prompt: q, near: spot ? `${spot.lat},${spot.lng}` : '' })
      if (!r.ok) toast(r.error || 'The AI did not answer', 'err')
      else setRes(r)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  if (!ai) return null
  return (
    <Card title={<span className="row" style={{ gap: 8 }}><Icon name="sparkles" size={16} />Where should we go?</span>} sub="Say what you feel like; the AI picks from your own places.">
      <form onSubmit={ask} className="stack">
        <div className="ask-box">
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="A quiet dinner for two, not too dear" disabled={!ai.ready} aria-label="What you feel like" />
          <button type="button" className={`chip ${near ? 'on' : ''}`} aria-pressed={near} onClick={() => setNear(n => !n)} disabled={!ai.ready}><Icon name="navigation" size={13} />Near me</button>
          <button className="btn primary" disabled={busy || !ai.ready || !q.trim()}>{busy ? 'Thinking…' : 'Pick for me'}</button>
        </div>
        {!ai.ready && <div className="muted small">No AI connected{ai.detail ? ` (${ai.detail})` : ''}. Set it up in <Link to="/settings/ai">Settings → AI</Link>.{!coreOn && <CoreHint />}</div>}
        {res && <div>
          {res.picks.map(({ place: p, why }) => (
            <div key={p.id} className="pick-card">
              <KindBadge kind={p.kind} size={36} />
              <div className="grow" style={{ minWidth: 0 }}>
                <Link to={`/places/${p.id}`}><b>{p.name}</b></Link> <span className="muted small">{placeLine(p)}</span> <Rating value={p.rating} />
                <div className="why">{why}</div>
              </div>
            </div>))}
          {res.note && <p className="muted small">{res.note}</p>}
        </div>}
      </form>
    </Card>
  )
}
