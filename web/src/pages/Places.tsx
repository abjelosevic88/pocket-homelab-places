// All places with filters; one place's page with its visits, details, lists and what is near.
import type React from 'react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useOnChange } from '../lib/data'
import { fmtClock, fmtDate, todayISO } from '../lib/format'
import { GROUPS, directionsURL, flag, minutesText, money, placeLine, ratingText, whereAmI } from '../lib/places'
import type { CalResult, CityCount, MoneyResult, Place, PlaceDetail, Visit } from '../lib/types'
import { Card, Confirm, Empty, Loading, Modal, errMsg, useAsync, useDebounce, useToast, type MenuItem, MenuButton } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ListsModal, PaymentModal, PlaceModal, SplitModal, VisitModal } from '../components/Forms'
import { KindBadge, PlaceRow, Price, RateCard, Rating, StatusBadge, VisitRow } from '../components/Rows'
import { PlacesMap } from '../components/PlacesMap'
import { openCreate, useCrumb } from '../components/Layout'

const SORTS = [{ value: 'name', label: 'A to Z' }, { value: 'rating', label: 'Best rated' }, { value: 'recent', label: 'Been lately' }, { value: 'added', label: 'Newest' }, { value: 'visits', label: 'Most visits' }, { value: 'near', label: 'Nearest' }]

/** The place filters, kept in the address so links and the back button work. */
export function usePlaceFilters() {
  const [sp, setSp] = useSearchParams()
  const get = (k: string) => sp.get(k) || ''
  const set = (patch: Record<string, string>) => {
    const n = new URLSearchParams(sp)
    for (const [k, v] of Object.entries(patch)) { if (v) n.set(k, v); else n.delete(k) }
    setSp(n, { replace: true })
  }
  const query = (extra: Record<string, string> = {}) => {
    const n = new URLSearchParams()
    for (const k of ['status', 'favourite', 'group', 'city', 'q', 'sort', 'near', 'radius_km', 'tag']) if (sp.get(k)) n.set(k, sp.get(k)!)
    for (const [k, v] of Object.entries(extra)) if (v) n.set(k, v)
    return n.toString()
  }
  return { get, set, query, label: get('label') }
}

export function FilterBar({ total, showSort = true }: { total?: number; showSort?: boolean }) {
  const f = usePlaceFilters()
  const { toast } = useToast()
  const [q, setQ] = useState(f.get('q'))
  const dq = useDebounce(q, 300)
  useEffect(() => { if (dq !== f.get('q')) f.set({ q: dq }) }, [dq]) // eslint-disable-line react-hooks/exhaustive-deps
  const { data: cities } = useAsync(() => api.get<CityCount[]>(`${V1}/cities`), [])
  const status = f.get('favourite') ? 'fav' : f.get('status')
  const near = async () => {
    if (f.get('near') && !f.label) { f.set({ near: '', radius_km: '', sort: f.get('sort') === 'near' ? '' : f.get('sort') }); return }
    const s = await whereAmI()
    if (!s) { toast('Your browser did not share where you are', 'err'); return }
    f.set({ near: `${s.lat.toFixed(5)},${s.lng.toFixed(5)}`, sort: 'near', label: '', radius_km: '' })
  }
  return (
    <div className="stack">
      <div className="chips scroll">
        {[{ v: '', l: 'All' }, { v: 'want', l: 'Want to go' }, { v: 'been', l: 'Been there' }, { v: 'fav', l: 'Favourites' }].map(c => (
          <button key={c.v} type="button" className={`chip ${status === c.v ? 'on' : ''}`} aria-pressed={status === c.v} onClick={() => f.set(c.v === 'fav' ? { favourite: '1', status: '' } : { status: c.v, favourite: '' })}>{c.l}</button>))}
        <button type="button" className={`chip ${f.get('near') ? 'on' : ''}`} aria-pressed={!!f.get('near')} onClick={near}><Icon name="navigation" size={13} />{f.label || 'Near me'}</button>
      </div>
      <div className="row wrap filters">
        <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder="Search name, cuisine, notes…" aria-label="Search places" />
        <Select ariaLabel="Kind" value={f.get('group')} onChange={v => f.set({ group: v })} clearLabel="Every kind" placeholder="Every kind" options={GROUPS.map(g => ({ value: g.value, label: g.label }))} />
        {cities && cities.length > 1 && <Select ariaLabel="Town" value={f.get('city')} onChange={v => f.set({ city: v })} clearLabel="Every town" placeholder="Every town" options={cities.map(c => ({ value: c.city, label: c.city, hint: String(c.count) }))} />}
        {showSort && <Select ariaLabel="Sort" value={f.get('sort') || 'name'} onChange={v => f.set({ sort: v === 'name' ? '' : v })} options={SORTS.map(s => ({ ...s, disabled: s.value === 'near' && !f.get('near') }))} />}
        {total != null && <span className="muted small">{total} {total === 1 ? 'place' : 'places'}</span>}
      </div>
    </div>
  )
}

export default function Places() {
  const f = usePlaceFilters()
  const q = f.query()
  const { data, reload, error } = useAsync(() => api.get<Place[]>(`${V1}/places?${q}`), [q])
  useOnChange(reload)
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Places</h1><p className="muted">Where you love to go, and where you want to go next.</p></div>
        <Link to={`/map?${q}`} className="btn"><Icon name="map" size={15} />On the map</Link>
        <button type="button" className="btn primary" onClick={() => openCreate('place')}><Icon name="plus" size={15} />Add a place</button>
      </div>
      <FilterBar total={data?.length} />
      {error ? <div className="callout danger"><Icon name="alert" size={15} />{error}</div> : !data ? <Loading />
        : data.length === 0 ? <Empty title={q ? 'Nothing matches' : 'No places yet'} icon="pin">{q ? 'Try fewer filters.' : 'Add the restaurants, cafés and sights you love or want to try.'}</Empty>
        : <Card flush><div className="list">{data.map(p => <PlaceRow key={p.id} p={p} />)}</div></Card>}
    </div>
  )
}

// ---------- one place ----------

export function PlacePage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { toast } = useToast()
  const { data, reload, error } = useAsync(() => api.get<PlaceDetail>(`${V1}/places/${id}`), [id])
  useOnChange(reload)
  const [modal, setModal] = useState<'' | 'edit' | 'visit' | 'plan' | 'lists' | 'delete'>('')
  useCrumb(data?.place.name)
  const openVisit = Number(sp.get('visit')) || 0
  const visit = useMemo(() => data?.visits.find(v => v.id === openVisit), [data, openVisit])
  const closeVisit = () => { const n = new URLSearchParams(sp); n.delete('visit'); setSp(n, { replace: true }) }
  if (error) return <div className="callout danger"><Icon name="alert" size={15} />{error}</div>
  if (!data) return <Loading />
  const p = data.place
  const done = data.visits.filter(v => !v.planned)
  const plans = data.visits.filter(v => v.planned)
  const patch = async (body: Partial<Place>, msg: string) => { try { await api.put(`${V1}/places/${p.id}`, body); toast(msg); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const more = (): MenuItem[] => [
    { label: p.favourite ? 'Not a favourite' : 'Make it a favourite', icon: 'heart', onSelect: () => patch({ favourite: !p.favourite }, p.favourite ? 'Taken off your favourites' : 'One of your favourites') },
    { label: p.status === 'want' ? 'Mark as been there' : 'Mark as want to go', icon: 'flag', onSelect: () => patch({ status: p.status === 'want' ? 'been' : 'want' }, 'Saved') },
    { label: p.closed ? 'Open again' : 'Closed for good', icon: 'x-circle', onSelect: () => patch({ closed: !p.closed }, 'Saved') },
    'sep',
    { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setModal('delete') },
  ]
  const facts: [string, React.ReactNode][] = [
    ['pin', (p.address || p.city) && <span key="a">{[p.address, p.city].filter(Boolean).join(', ')} {flag(p.country)}</span>],
    ['phone', p.phone && <a key="p" href={`tel:${p.phone.replace(/\s/g, '')}`}>{p.phone}</a>],
    ['globe', p.website && <a key="w" href={p.website} target="_blank" rel="noreferrer">{p.website.replace(/^https?:\/\/(www\.)?/, '').replace(/\/$/, '')}</a>],
    ['clock', p.hours],
    ['users', p.recommended_by && <span key="r">Recommended by {p.recommended_by}</span>],
    ['tag', p.tags && <span key="t" className="chips">{p.tags.split(',').map(t => t.trim()).filter(Boolean).map(t => <Link key={t} to={`/places?tag=${encodeURIComponent(t)}`} className="list-tag">{t}</Link>)}</span>],
    ['external', p.map_url && <a key="m" href={p.map_url} target="_blank" rel="noreferrer">Open the map link</a>],
  ]
  return (
    <div className="stack loose">
      <div className="detail-head">
        <KindBadge kind={p.kind} size={52} />
        <div className="grow" style={{ minWidth: 0 }}>
          <h1 className="page-title">{p.name}{p.favourite && <span className="fav-mark" title="Favourite"><Icon name="heart" size={18} /></span>}<StatusBadge p={p} /></h1>
          <p className="muted row wrap" style={{ gap: 8 }}><span>{placeLine(p)}</span><Price value={p.price} />{p.rating > 0 && <Rating value={p.rating} />}{p.visits > 0 && <span>{p.visits} {p.visits === 1 ? 'visit' : 'visits'}</span>}</p>
        </div>
      </div>
      <div className="row wrap toolbar">
        <button type="button" className="btn primary" onClick={() => setModal('visit')}><Icon name="check-circle" size={15} />Log a visit</button>
        <button type="button" className="btn" onClick={() => setModal('plan')}><Icon name="calendar-plus" size={15} />Plan</button>
        <button type="button" className="btn" onClick={() => setModal('lists')}><Icon name="list" size={15} />Lists{p.lists.length > 0 && <span className="badge">{p.lists.length}</span>}</button>
        {(p.lat != null || p.address) && <a className="btn" href={directionsURL(p)} target="_blank" rel="noreferrer"><Icon name="navigation" size={15} />Directions</a>}
        <span className="grow" />
        <button type="button" className="btn ghost" onClick={() => setModal('edit')}><Icon name="edit" size={15} />Edit</button>
        <MenuButton label="More" items={more} align="right"><Icon name="more" size={16} /></MenuButton>
      </div>
      {data.inbox.length > 0 && <div className="rate-cards">{data.inbox.map(it => <RateCard key={it.id} item={it} onDone={reload} />)}</div>}
      {p.closed && <div className="callout warn"><Icon name="info" size={15} /><div>Closed for good. It stays with its visits, but is never suggested.</div></div>}
      <div className="two-grid profile">
        <div className="stack">
          {plans.length > 0 && <Card title="Planned" flush><div className="list">{plans.map(v => <VisitRow key={v.id} v={v} showPlace={false} />)}</div></Card>}
          <Card title="Visits" sub={done.length ? visitsSub(data) : undefined} flush actions={<button type="button" className="btn ghost sm" onClick={() => setModal('visit')}><Icon name="plus" size={14} />Log</button>}>
            {done.length === 0 ? <p className="muted pad">{p.status === 'want' ? 'Not been yet. When you go, log it here with how it was.' : 'No visits logged yet.'}</p>
              : <div className="list">{done.map(v => <VisitRow key={v.id} v={v} showPlace={false} />)}</div>}
            {data.movement && (data.movement.visits > 0 || data.movement.error) && <div className="pad muted small row" style={{ gap: 6 }}><Icon name="footprints" size={13} />
              {data.movement.error ? data.movement.error : <>Pocket Movement saw you here on {data.movement.visits} {data.movement.visits === 1 ? 'day' : 'days'}, {minutesText(data.movement.minutes)} in all, last on {data.movement.url ? <a href={data.movement.url} target="_blank" rel="noreferrer">{fmtDate(data.movement.last)}</a> : fmtDate(data.movement.last)}.</>}</div>}
          </Card>
        </div>
        <div className="stack">
          <Card title="About it">
            <div className="place-facts">
              {facts.filter(([, v]) => v).map(([ic, v], i) => <div key={i} className="fact"><Icon name={ic} size={15} /><div className="grow">{v}</div></div>)}
              {facts.every(([, v]) => !v) && <p className="muted">Add the address, the website or opening hours under Edit.</p>}
            </div>
            {p.notes && <p className="notes">{p.notes}</p>}
          </Card>
          {p.lat != null && p.lng != null && <PlacesMap places={[p, ...data.nearby]} selected={p.id} className="small" fitKey={`p${p.id}`} onSelect={x => x && x !== p.id && navigate(`/places/${x}`)} />}
          {data.lists.length > 0 && <Card title="On your lists">
            <div className="chips">{data.lists.map(l => <Link key={l.id} to={`/lists/${l.id}`} className="chip">{l.emoji} {l.name}</Link>)}</div>
            {Object.keys(data.list_notes).length > 0 && <div className="stack tight mt">{data.lists.filter(l => data.list_notes[l.id]).map(l => <p key={l.id} className="small muted">{l.emoji} {l.name}: {data.list_notes[l.id]}</p>)}</div>}
          </Card>}
          {data.nearby.length > 0 && <Card title="Close by" sub="Your places within a few minutes' walk" flush><div className="list">{data.nearby.map(x => <PlaceRow key={x.id} p={x} />)}</div></Card>}
        </div>
      </div>
      {modal === 'edit' && <PlaceModal place={p} onClose={() => setModal('')} onSaved={reload} />}
      {(modal === 'visit' || modal === 'plan') && <VisitModal place={p} plan={modal === 'plan'} onClose={() => setModal('')} onSaved={reload} />}
      {modal === 'lists' && <ListsModal place={p} onClose={() => setModal('')} />}
      {visit && <VisitSheet visit={visit} place={p} onClose={closeVisit} onChanged={reload} />}
      {modal === 'delete' && <Confirm title={`Delete ${p.name}?`} message={<>The place goes with its {done.length} {done.length === 1 ? 'visit' : 'visits'} and plans. Transactions Places booked in Pocket Money are removed too; a bill split in Pocket Split stays there. To keep it without suggestions, mark it <b>Closed for good</b> instead.</>} confirmLabel="Delete"
        onCancel={() => setModal('')} onConfirm={async () => { setModal(''); try { await api.del(`${V1}/places/${p.id}`); toast('Deleted'); announceChange(); navigate('/places') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function visitsSub(d: PlaceDetail): string {
  const spent = d.spent.map(r => money(r.total, r.currency, true)).join(' + ')
  const done = d.visits.filter(v => !v.planned)
  const first = done[done.length - 1]?.date
  return [first && `since ${fmtDate(first)}`, spent && `${spent} in all`].filter(Boolean).join(' · ')
}

/** One visit: what it was, and what to do with it in the other apps. */
function VisitSheet({ visit: v, place, onClose, onChanged }: { visit: Visit; place: Place; onClose: () => void; onChanged: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState(false)
  const [split, setSplit] = useState(false)
  const [pay, setPay] = useState(false)
  const moneyOn = !!settings && settings.money_mode !== 'off'
  const moneyReady = !!settings && settings.money_via !== ''
  const splitOn = !!settings && settings.split_mode !== 'off'
  const splitReady = !!settings && settings.split_via !== ''
  const calReady = !!settings && settings.calendar_via !== ''
  const txURL = v.money_tx_id && settings?.money_open_url ? `${settings.money_open_url.replace(/\/$/, '')}/transactions/${v.money_tx_id}` : ''
  const book = async () => {
    try { const r = await api.post<{ money: MoneyResult }>(`${V1}/visits/${v.id}/money`, {}); if (r.money.ok) toast(`Booked in Pocket Money${r.money.account ? ` on ${r.money.account}` : ''}`); else toast(r.money.error || 'Pocket Money did not answer', 'err'); announceChange(); onChanged() } catch (e) { toast(errMsg(e), 'err') }
  }
  const unlink = async () => {
    try { await api.post(`${V1}/visits/${v.id}/money`, { unlink: true }); toast(v.money_booked ? 'Transaction removed from Pocket Money' : 'Unlinked'); announceChange(); onChanged() } catch (e) { toast(errMsg(e), 'err') }
  }
  const toCal = async () => {
    try { const r = await api.post<{ calendar: CalResult }>(`${V1}/visits/${v.id}/calendar`); if (r.calendar.ok) toast('On your calendar'); else toast(r.calendar.error || 'Pocket Calendar did not answer', 'err'); announceChange(); onChanged() } catch (e) { toast(errMsg(e), 'err') }
  }
  if (edit) return <VisitModal visit={v} place={place} onClose={() => { setEdit(false); onClose() }} onSaved={onChanged} />
  return (
    <Modal title={v.planned ? `Planned: ${fmtDate(v.date)}` : fmtDate(v.date)} onClose={onClose} size="sm" footer={<><span className="grow" /><button type="button" className="btn" onClick={onClose}>Close</button><button type="button" className="btn primary" onClick={() => setEdit(true)}><Icon name="edit" size={15} />Edit</button></>}>
      <div className="stack">
        <dl className="set-dl">
          {v.time && <div><dt>Time</dt><dd>{fmtClock(v.time)}</dd></div>}
          {v.rating > 0 && <div><dt>How it was</dt><dd>{'★'.repeat(Math.round(v.rating))} <span className="muted">{ratingText(v.rating)}</span></dd></div>}
          {v.with_whom && <div><dt>With</dt><dd>{v.with_whom}</dd></div>}
          {v.minutes > 0 && <div><dt>How long</dt><dd>{minutesText(v.minutes)}</dd></div>}
          {v.spent > 0 && <div><dt>What it cost</dt><dd>{money(v.spent, v.currency)}</dd></div>}
          {v.trip_name && <div><dt>Trip</dt><dd>{v.trip_name}</dd></div>}
          {v.notes && <div><dt>Notes</dt><dd className="notes" style={{ margin: 0 }}>{v.notes}</dd></div>}
        </dl>
        {!v.planned && moneyOn && <div className="stack tight">
          {v.money_tx_id ? <div className="row wrap"><Icon name="wallet" size={14} /><span className="grow small">{v.money_booked ? 'Booked in Pocket Money by Places.' : 'Linked to the payment in Pocket Money.'}</span>
            {txURL && <a className="btn sm" href={txURL} target="_blank" rel="noreferrer">Open</a>}<button type="button" className="btn sm ghost" onClick={unlink}>{v.money_booked ? 'Remove' : 'Unlink'}</button></div>
            : <div className="row wrap">
              <button type="button" className="btn sm" disabled={!moneyReady} title={moneyReady ? '' : settings?.money_detail} onClick={() => setPay(true)}><Icon name="link" size={14} />Find the payment</button>
              <button type="button" className="btn sm" disabled={!moneyReady || v.spent <= 0} title={!moneyReady ? settings?.money_detail : v.spent <= 0 ? 'Enter what it cost first' : ''} onClick={book}><Icon name="wallet" size={14} />Book it in Pocket Money</button>
            </div>}
        </div>}
        {!v.planned && splitOn && (v.split_expense_id ? <p className="small muted row" style={{ gap: 6 }}><Icon name="users" size={14} />Split in Pocket Split.</p>
          : <div className="row"><button type="button" className="btn sm" disabled={!splitReady} title={splitReady ? '' : settings?.split_detail} onClick={() => setSplit(true)}><Icon name="users" size={14} />Split the bill</button></div>)}
        {v.planned && settings?.calendar_mode !== 'off' && (v.cal_event_id ? <p className="small muted row" style={{ gap: 6 }}><Icon name="calendar" size={14} />On your calendar.</p>
          : v.date >= todayISO() && <div className="row"><button type="button" className="btn sm" disabled={!calReady} title={calReady ? '' : settings?.calendar_detail} onClick={toCal}><Icon name="calendar" size={14} />Put it on my calendar</button></div>)}
      </div>
      {split && <SplitModal visit={v} onClose={() => setSplit(false)} onDone={onChanged} />}
      {pay && <PaymentModal visit={v} onClose={() => setPay(false)} onDone={onChanged} />}
    </Modal>
  )
}

