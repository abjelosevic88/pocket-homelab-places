// Every add/edit dialog: a place (found on the map or from a pasted link), a
// visit or a plan, a list, putting a place on lists, splitting the bill and
// linking a payment from Pocket Money.
import React, { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { fmtDate, todayISO } from '../lib/format'
import { KINDS, money, moneyInput, parseMoney, whereAmI } from '../lib/places'
import type { CalResult, ContactItem, Found, MoneyResult, Payment, Picked, Place, PlaceList, SplitGroup, Trip, Visit } from '../lib/types'
import { CoreHint, Field, Loading, Modal, Segmented, errMsg, useDebounce, useToast } from './ui'
import { Select } from './Select'
import { Icon } from './icons'
import { KindBadge, Stars } from './Rows'
import { PlacesMap } from './PlacesMap'

function Actions({ onClose, busy, label, danger }: { onClose: () => void; busy: boolean; label: string; danger?: React.ReactNode }) {
  return <div className="form-actions">{danger}<span className="grow" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{busy ? 'Saving…' : label}</button></div>
}

/** Whether a Pocket app is reachable for this person, and why not. */
function useConn(slug: 'money' | 'calendar' | 'contacts' | 'split' | 'travel' | 'movement') {
  const { settings } = useApp()
  if (!settings) return { on: false, ready: false, detail: '', core: false }
  const core = settings.core_configured && settings.core_registered !== false
  const mode = settings[`${slug}_mode`]
  return { on: mode !== 'off' && !(mode === 'core' && !core), ready: settings[`${slug}_via`] !== '' && mode !== 'off', detail: settings[`${slug}_detail`], core }
}

function moneyToast(toast: ReturnType<typeof useToast>['toast'], m?: MoneyResult) {
  if (!m || m.skipped) return
  if (m.ok) toast(`Booked in Pocket Money${m.account ? ` on ${m.account}` : ''}`)
  else toast(`Saved, but Pocket Money said: ${m.error}`, 'err')
}

// ---------- a place ----------

export function PlaceModal({ place, preset, onClose, onSaved }: { place?: Place; preset?: Partial<Place>; onClose: () => void; onSaved?: (p: Place) => void }) {
  const { settings } = useApp()
  const { lists } = useData()
  const { toast } = useToast()
  const start: Partial<Place> = place || preset || {}
  const [f, setF] = useState({
    name: start.name || '', kind: start.kind || 'restaurant', cuisine: start.cuisine || '', status: start.status || 'want', favourite: !!start.favourite, closed: !!start.closed,
    rating: start.rating || 0, price: start.price || 0, address: start.address || '', city: start.city || '', country: start.country || '',
    lat: start.lat ?? null as number | null, lng: start.lng ?? null as number | null, phone: start.phone || '', website: start.website || '', map_url: start.map_url || '',
    hours: start.hours || '', tags: start.tags || '', notes: start.notes || '', recommended_by: start.recommended_by || '', contact_id: start.contact_id ?? null as number | null,
    currency: start.currency || '',
  })
  const [listIDs, setListIDs] = useState<number[]>(place ? place.lists : (preset?.lists || []))
  const [busy, setBusy] = useState(false)
  const [mapOpen, setMapOpen] = useState(false)
  const [pickContact, setPickContact] = useState(false)
  const contacts = useConn('contacts')
  const set = (patch: Partial<typeof f>) => setF(x => ({ ...x, ...patch }))
  const fill = (x: Found) => setF(cur => ({
    ...cur, name: cur.name && place ? cur.name : (x.name || cur.name), kind: x.kind && x.kind !== 'other' ? x.kind : cur.kind, cuisine: cur.cuisine || x.cuisine,
    address: x.address || cur.address, city: x.city || cur.city, country: x.country || cur.country, lat: x.lat ?? cur.lat, lng: x.lng ?? cur.lng,
    website: cur.website || x.website, phone: cur.phone || x.phone, hours: cur.hours || x.hours, map_url: x.map_url || cur.map_url,
  }))
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    const body = { ...f, rating: f.status === 'been' ? f.rating : 0 }
    try {
      let p: Place
      if (place) {
        p = await api.put<Place>(`${V1}/places/${place.id}`, body)
        const was = new Set(place.lists)
        for (const id of listIDs) if (!was.has(id)) await api.post(`${V1}/places/${p.id}/lists`, { list_id: id })
        for (const id of place.lists) if (!listIDs.includes(id)) await api.del(`${V1}/places/${p.id}/lists/${id}`)
      } else {
        p = await api.post<Place>(`${V1}/places`, { ...body, list_ids: listIDs })
      }
      toast(place ? 'Saved' : `${p.name} added`); announceChange(); onSaved?.(p); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const here = async () => {
    const s = await whereAmI()
    if (!s) { toast('Your browser did not share where you are', 'err'); return }
    set({ lat: s.lat, lng: s.lng }); setMapOpen(true)
    if (settings?.online && !f.address) api.get<Found>(`${V1}/reverse?at=${s.lat},${s.lng}`).then(x => setF(cur => ({ ...cur, address: cur.address || x.address, city: cur.city || x.city, country: cur.country || x.country }))).catch(() => {})
  }
  return (
    <Modal title={place ? `Edit ${place.name}` : 'Add a place'} onClose={onClose} size="lg">
      <form onSubmit={save} className="grid form-grid">
        {!place && <FindPlace onFound={fill} near={f.lat != null && f.lng != null ? { lat: f.lat, lng: f.lng } : null} />}
        <div className="two-col tight">
          <Field label="Name"><input value={f.name} onChange={e => set({ name: e.target.value })} required maxLength={200} autoFocus={!!place} placeholder="Kafeterija" /></Field>
          <Field group label="Kind of place"><Select ariaLabel="Kind" value={f.kind} onChange={v => set({ kind: v })} options={KINDS.map(k => ({ value: k.value, label: k.label }))} /></Field>
        </div>
        <div className="two-col">
          <Field group label="Have you been?"><Segmented value={f.status} onChange={v => set({ status: v })} options={[{ value: 'want', label: 'Want to go' }, { value: 'been', label: 'Been there' }]} /></Field>
          <Field group label="Price" className="price-pick"><Segmented value={f.price} onChange={v => set({ price: v })} options={[{ value: 0, label: '?' }, { value: 1, label: '€' }, { value: 2, label: '€€' }, { value: 3, label: '€€€' }, { value: 4, label: '€€€€' }]} /></Field>
        </div>
        {f.status === 'been' && <div className="two-col">
          <Field group label="Your rating" help={place?.visits ? 'Rating a visit updates this to the average of your visits.' : undefined}><Stars value={f.rating} onChange={v => set({ rating: v })} /></Field>
          <label className="check-row self-end"><input type="checkbox" checked={f.favourite} onChange={e => set({ favourite: e.target.checked })} /><Icon name="heart" size={14} />One of my favourites</label>
        </div>}
        <Field label="Cuisine or what it is" help="Bosnian, specialty coffee, natural wine, Roman ruins…"><input value={f.cuisine} onChange={e => set({ cuisine: e.target.value })} maxLength={120} /></Field>
        <div className="three-col tight">
          <Field label="Address"><input value={f.address} onChange={e => set({ address: e.target.value })} /></Field>
          <Field label="Town"><input value={f.city} onChange={e => set({ city: e.target.value })} /></Field>
          <Field label="Country code" help="BA, AT, PT…"><input value={f.country} onChange={e => set({ country: e.target.value.toUpperCase().slice(0, 2) })} maxLength={2} /></Field>
        </div>
        <div className="stack tight">
          <span className="small muted">{f.lat != null ? `On the map at ${f.lat.toFixed(5)}, ${f.lng!.toFixed(5)}` : 'Not on the map yet: find it above, or pick the spot.'}</span>
          <div className="row wrap">
            <button type="button" className="btn sm" onClick={() => setMapOpen(o => !o)}><Icon name="map" size={14} />{mapOpen ? 'Hide the map' : f.lat != null ? 'Move the pin' : 'Pick on the map'}</button>
            <button type="button" className="btn sm" onClick={here}><Icon name="navigation" size={14} />I'm here</button>
            {f.lat != null && <button type="button" className="btn sm ghost" onClick={() => set({ lat: null, lng: null })}>Remove</button>}
          </div>
          {mapOpen && <PlacesMap places={[]} className="pick" pick={f.lat != null && f.lng != null ? { lat: f.lat, lng: f.lng } : null} onPick={s => set({ lat: Number(s.lat.toFixed(6)), lng: Number(s.lng.toFixed(6)) })} />}
        </div>
        <div className="two-col tight">
          <Field label="Website"><input value={f.website} onChange={e => set({ website: e.target.value })} placeholder="https://…" /></Field>
          <Field label="Phone"><input type="tel" value={f.phone} onChange={e => set({ phone: e.target.value })} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Opening hours"><input value={f.hours} onChange={e => set({ hours: e.target.value })} placeholder="Mo-Sa 08:00-22:00" /></Field>
          <Field label="Tags" help="Comma between them."><input value={f.tags} onChange={e => set({ tags: e.target.value })} placeholder="terrace, date night, kids" /></Field>
        </div>
        <Field label="Recommended by">
          <div className="row">
            <input value={f.recommended_by} onChange={e => set({ recommended_by: e.target.value, contact_id: null })} placeholder="Who told you about it" />
            {contacts.on && <button type="button" className="btn" onClick={() => setPickContact(true)} disabled={!contacts.ready} title={contacts.ready ? 'From Pocket Contacts' : contacts.detail}><Icon name="users" size={15} /><span className="hide-xs">Contacts</span></button>}
          </div>
        </Field>
        <Field label="Notes"><textarea rows={3} value={f.notes} onChange={e => set({ notes: e.target.value })} placeholder="What to order, where to sit, book ahead…" /></Field>
        {lists && lists.length > 0 && <Field group label="On my lists"><div className="chips">{lists.map(l => (
          <button key={l.id} type="button" className={`chip ${listIDs.includes(l.id) ? 'on' : ''}`} aria-pressed={listIDs.includes(l.id)} onClick={() => setListIDs(ids => ids.includes(l.id) ? ids.filter(x => x !== l.id) : [...ids, l.id])}>{l.emoji && <span>{l.emoji}</span>}{l.name}</button>))}</div></Field>}
        <div className="two-col tight">
          <Field label="Currency there (optional)" help={`For what visits cost; otherwise ${settings?.currency || 'EUR'}.`}><input value={f.currency} onChange={e => set({ currency: e.target.value.toUpperCase().slice(0, 3) })} maxLength={3} placeholder={settings?.currency || 'EUR'} /></Field>
          {place && <label className="check-row self-end"><input type="checkbox" checked={f.closed} onChange={e => set({ closed: e.target.checked })} />Closed for good (kept, never suggested)</label>}
        </div>
        <Actions onClose={onClose} busy={busy} label={place ? 'Save' : 'Add place'} />
      </form>
      {pickContact && <PickerModal<ContactItem> title="Recommended by" url={`${V1}/links/contacts`} needQuery onClose={() => setPickContact(false)}
        onPick={c => { set({ recommended_by: c.fn || c.org, contact_id: c.id }); setPickContact(false) }}
        row={c => <><Icon name="user" size={15} /><span className="grow"><b>{c.fn || c.org}</b><span className="muted small block-line">{[c.org !== c.fn ? c.org : '', c.phones?.[0]].filter(Boolean).join(' · ')}</span></span></>} />}
    </Modal>
  )
}

/** Type a name to look it up on OpenStreetMap, or paste a Google / Apple Maps link. */
function FindPlace({ onFound, near }: { onFound: (f: Found) => void; near: { lat: number; lng: number } | null }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 500)
  const [items, setItems] = useState<Found[] | null>(null)
  const [choices, setChoices] = useState<Found[] | null>(null) // a link gave only a name: which one?
  const [busy, setBusy] = useState(false)
  const [me, setMe] = useState(near)
  useEffect(() => { if (!me) whereAmI(4000).then(setMe) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const isLink = /https?:\/\/\S+|^-?\d{1,2}\.\d+\s*,\s*-?\d{1,3}\.\d+$/.test(dq)
  const online = !!settings?.online && !settings.online_blocked
  useEffect(() => {
    if (!dq || isLink || !online || dq.length < 3) { setItems(null); return }
    let live = true
    setBusy(true)
    api.get<{ items: Found[]; error?: string }>(`${V1}/lookup?q=${encodeURIComponent(dq)}${me ? `&near=${me.lat},${me.lng}` : ''}`)
      .then(r => { if (live) setItems(r.items) }).catch(() => live && setItems([])).finally(() => live && setBusy(false))
    return () => { live = false }
  }, [dq, isLink, online, me])
  const resolve = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ ok: boolean; error?: string; place?: Found; choices?: Found[] }>(`${V1}/resolve`, { text: q, near: me ? `${me.lat},${me.lng}` : '' })
      if (!r.ok || !r.place) toast(r.error || 'Nothing found in that link', 'err')
      else if (r.choices?.length) { onFound(r.place); setChoices(r.choices); setQ(''); toast(`Found ${r.place.name}. Which one is it? Pick it below.`) }
      else { onFound(r.place); setQ(''); toast(r.place.lat != null ? (r.place.name ? `Found ${r.place.name}` : 'Found the spot') : `Found the name; add the address or pick the spot on the map`) }
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  useEffect(() => { if (isLink && dq) resolve() }, [dq]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="stack tight">
      <Field label="Find it" help={online ? 'Type the name and the town, or paste a link from Google Maps or Apple Maps.' : 'Paste a link from Google Maps or Apple Maps. Looking names up online is off (Settings → General).'}>
        <div className="row"><input value={q} onChange={e => { setQ(e.target.value); setChoices(null) }} placeholder={online ? 'Kafeterija Sarajevo, or a maps link' : 'A maps link'} autoFocus />{busy && <span className="spinner" />}</div>
      </Field>
      {choices && choices.length > 0 && <div className="stack tight">
        <span className="small muted">Places called {choices[0].name || 'that'}, nearest first:</span>
        <div className="found-list">{choices.map((x, i) => (
          <button key={i} type="button" className="found-row" onClick={() => { onFound(x); setChoices(null) }}>
            <KindBadge kind={x.kind} size={28} />
            <span className="grow"><b>{x.name}</b><small>{x.label}</small></span>
          </button>))}</div>
      </div>}
      {items && items.length > 0 && <div className="found-list">{items.map((x, i) => (
        <button key={i} type="button" className="found-row" onClick={() => { onFound(x); setItems(null); setQ('') }}>
          <KindBadge kind={x.kind} size={28} />
          <span className="grow"><b>{x.name}</b><small>{x.label}</small></span>
        </button>))}</div>}
      {items && items.length === 0 && !busy && <p className="muted small">Nothing found on OpenStreetMap; fill it in below.</p>}
    </div>
  )
}

// ---------- a visit or a plan ----------

export function VisitModal({ visit, place, placeId, plan, onClose, onSaved }: { visit?: Visit; place?: Place; placeId?: number; plan?: boolean; onClose: () => void; onSaved?: (v: Visit) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [places, setPlaces] = useState<Place[] | null>(place ? [place] : null)
  useEffect(() => { if (!place) api.get<Place[]>(`${V1}/places?sort=recent`).then(setPlaces).catch(() => setPlaces([])) }, [place])
  const [f, setF] = useState({
    place_id: visit?.place_id || place?.id || placeId || 0, planned: visit ? visit.planned : !!plan, date: visit?.date || (plan ? '' : todayISO()), time: visit?.time || '',
    rating: visit?.rating || 0, with_whom: visit?.with_whom || '', notes: visit?.notes || '', spent: moneyInput(visit?.spent), currency: visit?.currency || '',
  })
  const [book, setBook] = useState(settings?.money_book ?? false)
  const [cal, setCal] = useState(settings?.calendar_plans ?? true)
  const [busy, setBusy] = useState(false)
  const [sure, setSure] = useState(false)
  const [pickContact, setPickContact] = useState(false)
  const money = useConn('money')
  const calendar = useConn('calendar')
  const contacts = useConn('contacts')
  const cur = places?.find(p => p.id === f.place_id)
  const currency = f.currency || cur?.currency || settings?.currency || 'EUR'
  const set = (patch: Partial<typeof f>) => setF(x => ({ ...x, ...patch }))
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!f.place_id) { toast('Choose the place', 'err'); return }
    if (!f.date) { toast('Choose the day', 'err'); return }
    const spent = parseMoney(f.spent)
    if (f.spent.trim() && spent == null) { toast('What you spent is a number like 24 or 18.50', 'err'); return }
    setBusy(true)
    const body: Record<string, unknown> = { place_id: f.place_id, planned: f.planned, date: f.date, time: f.time, rating: f.planned ? 0 : f.rating, with_whom: f.with_whom, notes: f.notes,
      spent: f.planned ? 0 : (spent || 0), currency }
    if (!visit) { body.money = !f.planned && book && money.ready; body.calendar = f.planned && cal && calendar.ready }
    try {
      const r = visit ? await api.put<{ visit: Visit; money?: MoneyResult; calendar?: CalResult }>(`${V1}/visits/${visit.id}`, body)
        : await api.post<{ visit: Visit; money?: MoneyResult; calendar?: CalResult }>(`${V1}/visits`, body)
      toast(visit ? 'Saved' : f.planned ? 'Planned' : 'Visit logged')
      moneyToast(toast, r.money)
      if (r.calendar?.ok && r.calendar.event_id && !visit) toast('On your calendar')
      if (r.calendar && !r.calendar.ok) toast(`Saved, but Pocket Calendar said: ${r.calendar.error}`, 'err')
      announceChange(); onSaved?.(r.visit); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const remove = async () => {
    if (!visit) return
    if (!sure) { setSure(true); return }
    try { await api.del(`${V1}/visits/${visit.id}`); toast(visit.planned ? 'Plan removed' : 'Visit deleted'); announceChange(); onClose() } catch (err) { toast(errMsg(err), 'err') }
  }
  const title = visit ? (visit.planned ? 'Edit the plan' : 'Edit the visit') : f.planned ? 'Plan a visit' : 'Log a visit'
  return (
    <Modal title={place ? `${title}: ${place.name}` : title} onClose={onClose}>
      {!places ? <Loading /> : <form onSubmit={save} className="grid form-grid">
        {!place && <Field group label="Where"><Select ariaLabel="Place" value={f.place_id ? String(f.place_id) : ''} onChange={v => set({ place_id: Number(v) })} placeholder={places.length ? 'Choose the place' : 'Add a place first'}
          options={places.map(p => ({ value: String(p.id), label: p.name, hint: p.city }))} /></Field>}
        <Segmented value={f.planned ? 'plan' : 'went'} onChange={v => set({ planned: v === 'plan', date: v === 'plan' && f.date <= todayISO() ? '' : f.date || todayISO() })} options={[{ value: 'went', label: 'We went' }, { value: 'plan', label: 'Plan to go' }]} />
        <div className="two-col tight">
          <Field label="Day"><input type="date" value={f.date} onChange={e => set({ date: e.target.value })} required max={f.planned ? undefined : todayISO()} min={f.planned && !visit ? todayISO() : undefined} /></Field>
          <Field label="Time (optional)"><input type="time" value={f.time} onChange={e => set({ time: e.target.value })} /></Field>
        </div>
        {!f.planned && <Field group label="How was it?"><Stars value={f.rating} onChange={v => set({ rating: v })} /></Field>}
        <Field label="With whom">
          <div className="row">
            <input value={f.with_whom} onChange={e => set({ with_whom: e.target.value })} placeholder="Ana, Mirza" />
            {contacts.on && <button type="button" className="btn" onClick={() => setPickContact(true)} disabled={!contacts.ready} title={contacts.ready ? 'From Pocket Contacts' : contacts.detail}><Icon name="users" size={15} /><span className="hide-xs">Contacts</span></button>}
          </div>
        </Field>
        {!f.planned && <div className="two-col tight">
          <Field label={`What it cost (${currency})`}><input inputMode="decimal" value={f.spent} onChange={e => set({ spent: e.target.value })} placeholder="0.00" /></Field>
          <Field label="Currency"><input value={f.currency} onChange={e => set({ currency: e.target.value.toUpperCase().slice(0, 3) })} placeholder={currency} maxLength={3} /></Field>
        </div>}
        <Field label={f.planned ? 'Notes' : 'What you had, how it was'}><textarea rows={3} value={f.notes} onChange={e => set({ notes: e.target.value })} placeholder={f.planned ? 'Table for four, ask for the terrace' : 'The lamb was great; skip the dessert'} /></Field>
        {!visit && !f.planned && (parseMoney(f.spent) || 0) > 0 && money.on && <Tick checked={book} onChange={setBook} ready={money.ready} detail={money.detail} core={money.core}
          label={<><Icon name="wallet" size={14} /> Also create a transaction in Pocket Money</>} help="Leave it off when you paid by card and the payment comes into Pocket Money anyway; link it from the visit later." />}
        {!visit && f.planned && calendar.on && <Tick checked={cal} onChange={setCal} ready={calendar.ready} detail={calendar.detail} core={calendar.core} label={<><Icon name="calendar" size={14} /> Put it on my calendar</>} />}
        {visit?.money_booked && <p className="muted small row" style={{ gap: 6 }}><Icon name="wallet" size={14} />Booked in Pocket Money; a new date or cost goes there too.</p>}
        <Actions onClose={onClose} busy={busy} label={visit ? 'Save' : f.planned ? 'Plan it' : 'Log visit'}
          danger={visit && <button type="button" className="btn ghost danger-text" onClick={remove}><Icon name="trash" size={14} />{sure ? (visit.money_booked ? 'Delete it and its transaction' : 'Delete it') : 'Delete'}</button>} />
      </form>}
      {pickContact && <PickerModal<ContactItem> title="Who came along" url={`${V1}/links/contacts`} needQuery onClose={() => setPickContact(false)}
        onPick={c => { const n = c.fn || c.org; set({ with_whom: f.with_whom ? `${f.with_whom}, ${n}` : n }); setPickContact(false) }}
        row={c => <><Icon name="user" size={15} /><span className="grow"><b>{c.fn || c.org}</b></span></>} />}
    </Modal>
  )
}

/** A tick for something done in another Pocket app: greyed out with why when it is not reachable. */
function Tick({ checked, onChange, ready, detail, core, label, help }: { checked: boolean; onChange: (v: boolean) => void; ready: boolean; detail: string; core: boolean; label: React.ReactNode; help?: string }) {
  return (
    <div className="stack tight">
      <label className={`check-row ${ready ? '' : 'disabled'}`} title={ready ? '' : detail}>
        <input type="checkbox" checked={ready && checked} disabled={!ready} onChange={e => onChange(e.target.checked)} />
        <span>{label}</span>
      </label>
      {ready && help && <p className="muted small">{help}</p>}
      {!ready && <p className="muted small">{detail ? `Not connected: ${detail}.` : 'Connect it in Settings → Connected apps.'}</p>}
      {!ready && !core && <CoreHint />}
    </div>
  )
}

// ---------- lists ----------

export function ListModal({ list, onClose, onSaved }: { list?: PlaceList; onClose: () => void; onSaved?: (l: PlaceList) => void }) {
  const { toast } = useToast()
  const travel = useConn('travel')
  const [trips, setTrips] = useState<Trip[]>([])
  useEffect(() => { if (travel.ready) api.get<{ trips: Trip[] }>(`${V1}/trips?days=730&back=30`).then(r => setTrips(r.trips)).catch(() => {}) }, [travel.ready])
  const [f, setF] = useState({ name: list?.name || '', emoji: list?.emoji || '', description: list?.description || '', trip_id: list?.trip_id ?? null as number | null, trip_name: list?.trip_name || '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const l = list ? await api.put<PlaceList>(`${V1}/lists/${list.id}`, f) : await api.post<PlaceList>(`${V1}/lists`, f)
      toast(list ? 'Saved' : `${l.name} made`); announceChange(); onSaved?.(l); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={list ? `Edit ${list.name}` : 'A new list'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid form-grid">
        <div className="row" style={{ alignItems: 'flex-end' }}>
          <Field label="Emoji" className="num-sm"><input value={f.emoji} onChange={e => setF({ ...f, emoji: [...e.target.value].slice(-2).join('') })} placeholder="☕" /></Field>
          <Field label="Name" className="grow"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={120} autoFocus placeholder="Date night" /></Field>
        </div>
        <Field label="What it is for (optional)"><textarea rows={2} value={f.description} onChange={e => setF({ ...f, description: e.target.value })} /></Field>
        {travel.ready && trips.length > 0 ? (
          <Field group label="For a trip (optional)" help="Its places show on the trip in Pocket Travel."><Select ariaLabel="Trip" value={f.trip_id ? String(f.trip_id) : ''} clearLabel="No trip" placeholder="No trip"
            onChange={v => { const t = trips.find(x => String(x.trip_id) === v); setF({ ...f, trip_id: t ? t.trip_id : null, trip_name: t ? (t.name || t.destination) : '' }) }}
            options={trips.map(t => ({ value: String(t.trip_id), label: t.name || t.destination, hint: fmtDate(t.start, false) }))} /></Field>
        ) : <Field label="For a trip (optional)" help={travel.on ? 'Connect Pocket Travel to pick one of your trips.' : undefined}><input value={f.trip_name} onChange={e => setF({ ...f, trip_name: e.target.value })} placeholder="Lisbon in May" /></Field>}
        <Actions onClose={onClose} busy={busy} label={list ? 'Save' : 'Make the list'} />
      </form>
    </Modal>
  )
}

/** Tick the lists a place belongs on. */
export function ListsModal({ place, onClose }: { place: Place; onClose: () => void }) {
  const { lists } = useData()
  const { toast } = useToast()
  const [on, setOn] = useState<number[]>(place.lists)
  const [make, setMake] = useState(false)
  const flip = async (l: PlaceList) => {
    const has = on.includes(l.id)
    setOn(x => has ? x.filter(i => i !== l.id) : [...x, l.id])
    try {
      if (has) await api.del(`${V1}/places/${place.id}/lists/${l.id}`)
      else await api.post(`${V1}/places/${place.id}/lists`, { list_id: l.id })
      announceChange()
    } catch (e) { toast(errMsg(e), 'err'); setOn(x => has ? [...x, l.id] : x.filter(i => i !== l.id)) }
  }
  return (
    <Modal title={`Lists for ${place.name}`} onClose={onClose} size="sm" footer={<><button type="button" className="btn ghost" onClick={() => setMake(true)}><Icon name="plus" size={15} />New list</button><span className="grow" /><button type="button" className="btn primary" onClick={onClose}>Done</button></>}>
      {!lists ? <Loading /> : lists.length === 0 ? <p className="muted">No lists yet. Make one for date nights, coffee, a trip…</p> : (
        <div className="pick-list">{lists.map(l => (
          <label key={l.id} className="pick-row"><input type="checkbox" checked={on.includes(l.id)} onChange={() => flip(l)} /><span className="grow">{l.emoji} {l.name}</span><span className="muted small">{l.count}</span></label>))}</div>
      )}
      {make && <ListModal onClose={() => setMake(false)} onSaved={async l => { await api.post(`${V1}/places/${place.id}/lists`, { list_id: l.id }); setOn(x => [...x, l.id]); announceChange() }} />}
    </Modal>
  )
}

// ---------- Pocket Split: share the bill ----------

export function SplitModal({ visit, onClose, onDone }: { visit: Visit; onClose: () => void; onDone: () => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [groups, setGroups] = useState<Picked<SplitGroup> | null>(null)
  const [gid, setGid] = useState(0)
  const [members, setMembers] = useState<number[]>([])
  const [amount, setAmount] = useState(moneyInput(visit.spent))
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<Picked<SplitGroup>>(`${V1}/links/split/groups`).then(r => {
      setGroups(r)
      const g = r.items.find(x => x.id === settings?.split_group_id) || r.items[0]
      if (g) { setGid(g.id); setMembers(g.members.map(m => m.id)) }
    }).catch(e => setGroups({ connected: false, detail: errMsg(e), items: [] }))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const g = groups?.items.find(x => x.id === gid)
  const split = async () => {
    const a = parseMoney(amount)
    if (!a) { toast('Enter what the bill came to', 'err'); return }
    setBusy(true)
    try {
      const r = await api.post<{ group: string; url: string }>(`${V1}/visits/${visit.id}/split`, { group_id: gid, member_ids: members, amount: a })
      toast(`Split in ${r.group}`); announceChange(); onDone(); onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Split the bill" onClose={onClose} size="sm" footer={<><span className="grow" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={busy || !g || members.length < 2} onClick={split}>Split it</button></>}>
      {!groups ? <Loading /> : !groups.connected || groups.detail ? <p className="bad small">{groups.detail}</p> : groups.items.length === 0 ? <p className="muted">You are in no group in Pocket Split yet.</p> : (
        <div className="grid form-grid">
          <p className="muted small">You paid {visit.place_name} on {fmtDate(visit.date)}; it goes into Pocket Split as an expense shared evenly.</p>
          <Field group label="Group"><Select ariaLabel="Group" value={String(gid)} onChange={v => { const x = groups.items.find(i => String(i.id) === v); setGid(Number(v)); setMembers(x ? x.members.map(m => m.id) : []) }} options={groups.items.map(x => ({ value: String(x.id), label: x.name, hint: x.currency }))} /></Field>
          <Field label={`The bill (${visit.currency})`}><input inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} /></Field>
          {g && <Field group label="Shared by"><div className="chips">{g.members.map(m => (
            <button key={m.id} type="button" className={`chip ${members.includes(m.id) ? 'on' : ''}`} aria-pressed={members.includes(m.id)} onClick={() => setMembers(x => x.includes(m.id) ? x.filter(i => i !== m.id) : [...x, m.id])}>{m.name}{m.id === g.my_member_id ? ' (you)' : ''}</button>))}</div></Field>}
        </div>
      )}
    </Modal>
  )
}

// ---------- Pocket Money: link the payment ----------

export function PaymentModal({ visit, onClose, onDone }: { visit: Visit; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [res, setRes] = useState<Picked<Payment> | null>(null)
  useEffect(() => { api.get<Picked<Payment>>(`${V1}/visits/${visit.id}/payments`).then(setRes).catch(e => setRes({ connected: false, detail: errMsg(e), items: [] })) }, [visit.id])
  const link = async (p: Payment) => {
    try { await api.post<{ money: MoneyResult }>(`${V1}/visits/${visit.id}/money`, { tx_id: p.id }); toast('Linked to the payment in Pocket Money'); announceChange(); onDone(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="The payment in Pocket Money" onClose={onClose} size="sm">
      <p className="muted small">Payments around {fmtDate(visit.date)}; the ones at {visit.place_name} first.</p>
      <div className="pick-list">
        {!res ? <Loading /> : !res.connected || res.detail ? <p className="bad small">{res.detail}</p> : res.items.length === 0 ? <p className="muted small">No payments those days.</p>
          : res.items.map(p => (
            <button key={p.id} type="button" className="pick-row" onClick={() => link(p)}>
              <Icon name={p.likely ? 'check-circle' : 'wallet'} size={15} />
              <span className="grow"><b>{p.payee || p.description || 'Payment'}</b><span className="muted small block-line">{[fmtDate(p.date, false), p.time, p.likely ? p.why : ''].filter(Boolean).join(' · ')}</span></span>
              <span className="tnum">{money(Math.abs(p.foreign_amount || p.amount), p.foreign_currency || p.currency)}</span>
            </button>))}
      </div>
    </Modal>
  )
}

// ---------- pickers over the other apps ----------

export function PickerModal<T extends { id: number }>({ title, url, initial = '', onClose, onPick, row, needQuery }: { title: string; url: string; initial?: string; onClose: () => void; onPick: (t: T) => void; row: (t: T) => React.ReactNode; needQuery?: boolean }) {
  const [q, setQ] = useState(initial)
  const dq = useDebounce(q, 300)
  const [res, setRes] = useState<Picked<T> | null>(null)
  useEffect(() => {
    if (needQuery && !dq.trim()) { setRes({ connected: true, items: [] }); return }
    let live = true
    api.get<Picked<T>>(`${url}?q=${encodeURIComponent(dq)}`).then(r => { if (live) setRes(r) }).catch(e => live && setRes({ connected: false, detail: errMsg(e), items: [] }))
    return () => { live = false }
  }, [dq, url, needQuery])
  return (
    <Modal title={title} onClose={onClose} size="sm">
      <div className="grid" style={{ gap: 10 }}>
        <input value={q} onChange={e => setQ(e.target.value)} placeholder="Search…" autoFocus />
        <div className="pick-list">
          {!res ? <Loading /> : !res.connected || res.detail ? <p className="bad small">{res.detail}</p>
            : res.items.length === 0 ? <p className="muted small">{needQuery && !dq.trim() ? 'Type a name.' : 'Nothing found.'}</p>
            : res.items.map(it => <button key={it.id} type="button" className="pick-row" onClick={() => onPick(it)}>{row(it)}</button>)}
        </div>
      </div>
    </Modal>
  )
}
