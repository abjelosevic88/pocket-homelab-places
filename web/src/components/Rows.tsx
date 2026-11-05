// The rows and badges every list shares: places, visits, "how was it?".
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange } from '../lib/data'
import { fmtClock, fmtDate, todayISO } from '../lib/format'
import { distanceText, kindIcon, minutesText, money, placeLine, priceText, ratingText } from '../lib/places'
import type { InboxItem, Place, Visit } from '../lib/types'
import { Icon } from './icons'
import { errMsg, useToast } from './ui'

/** The kind's icon on its tint. */
export function KindBadge({ kind, size = 32, round = false }: { kind: string; size?: number; round?: boolean }) {
  return <span className={`kind-badge k-${kind} ${round ? 'round' : ''}`} style={{ width: size, height: size }} aria-hidden="true"><Icon name={kindIcon(kind)} size={Math.round(size * 0.52)} /></span>
}

/** ★ 4.5 */
export function Rating({ value }: { value: number }) {
  if (!value) return null
  return <span className="rating-num" title={`${ratingText(value)} of 5`}><Icon name="star" size={13} />{ratingText(value)}</span>
}

/** Five stars to tap; tapping the current one clears it. */
export function Stars({ value, onChange, size = 22 }: { value: number; onChange: (v: number) => void; size?: number }) {
  return (
    <span className="stars" role="radiogroup" aria-label="Rating">
      {[1, 2, 3, 4, 5].map(n => (
        <button key={n} type="button" role="radio" aria-checked={Math.round(value) === n} className={n <= Math.round(value) ? 'on' : ''} aria-label={`${n} of 5`} onClick={() => onChange(Math.round(value) === n ? 0 : n)}>
          <Icon name="star" size={size} />
        </button>
      ))}
    </span>
  )
}

export function Price({ value }: { value: number }) {
  if (!value) return null
  return <span className="price" title={['', 'Cheap', 'Moderate', 'Pricey', 'Expensive'][value]}>{priceText(value)}</span>
}

export function StatusBadge({ p }: { p: Pick<Place, 'status' | 'closed'> }) {
  if (p.closed) return <span className="status-badge closed">Closed</span>
  return p.status === 'want' ? <span className="status-badge want">Want to go</span> : null
}

export function PlaceRow({ p, note, showCity = true }: { p: Place; note?: string; showCity?: boolean }) {
  const { settings } = useApp()
  const sub = [placeLine(p, showCity), note || (p.last_visit ? `last ${fmtDate(p.last_visit, false)}` : p.recommended_by ? `from ${p.recommended_by}` : '')].filter(Boolean).join(' · ')
  return (
    <Link to={`/places/${p.id}`} className={`list-row ${p.closed ? 'is-archived' : ''}`}>
      <KindBadge kind={p.kind} />
      <span className="grow row-main">
        <span className="row-title"><span className="ellipsis">{p.name}</span>{p.favourite && <span className="fav-mark" title="Favourite"><Icon name="heart" size={13} /></span>}</span>
        <span className="muted small ellipsis">{sub}</span>
      </span>
      {p.distance_m != null && <span className="muted small tnum hide-xs">{distanceText(p.distance_m, settings?.distance_unit)}</span>}
      <Price value={p.price} />
      {p.rating > 0 ? <Rating value={p.rating} /> : <StatusBadge p={p} />}
    </Link>
  )
}

export function VisitRow({ v, showPlace = true }: { v: Visit; showPlace?: boolean }) {
  const when = fmtDate(v.date) + (v.time ? `, ${fmtClock(v.time)}` : '')
  const sub = showPlace
    ? [when, v.city, v.with_whom && `with ${v.with_whom}`, v.minutes ? minutesText(v.minutes) : ''].filter(Boolean).join(' · ')
    : [v.time && fmtClock(v.time), v.with_whom && `with ${v.with_whom}`, v.notes].filter(Boolean).join(' · ')
  return (
    <Link to={`/places/${v.place_id}?visit=${v.id}`} className="list-row visit-row">
      <KindBadge kind={v.place_kind} />
      <span className="grow row-main">
        <span className="row-title"><span className="ellipsis">{showPlace ? v.place_name : fmtDate(v.date)}</span>{v.planned && <span className="status-badge want">Planned</span>}</span>
        {sub && <span className="muted small ellipsis">{sub}</span>}
      </span>
      {(v.money_tx_id || v.split_expense_id) && <Icon name="wallet" size={14} className="muted" title={v.split_expense_id ? 'Split in Pocket Split' : 'In Pocket Money'} />}
      {v.cal_event_id && <Icon name="calendar" size={14} className="muted" title="On your calendar" />}
      <span className="tnum row-amount">{v.spent ? money(v.spent, v.currency) : ''}</span>
      {v.rating > 0 && <Rating value={v.rating} />}
    </Link>
  )
}

function dayWord(d: string): string {
  const today = todayISO()
  if (d === today) return 'today'
  const y = new Date(today + 'T12:00:00')
  y.setDate(y.getDate() - 1)
  if (d === `${y.getFullYear()}-${String(y.getMonth() + 1).padStart(2, '0')}-${String(y.getDate()).padStart(2, '0')}`) return 'yesterday'
  return 'on ' + new Date(d + 'T12:00:00').toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'short' })
}

/** "You were at Kafeterija yesterday for 48 min, paid 9.50 KM — how was it?" */
export function RateCard({ item, onDone }: { item: InboxItem; onDone: () => void }) {
  const { toast } = useToast()
  const [rating, setRating] = useState(0)
  const [busy, setBusy] = useState(false)
  const accept = async (r: number) => {
    setBusy(true)
    try { await api.post(`${V1}/inbox/${item.id}/accept`, { rating: r }); toast(`Visit to ${item.place_name} logged`); announceChange(); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const dismiss = async () => {
    setBusy(true)
    try { await api.post(`${V1}/inbox/${item.id}/dismiss`); announceChange(); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="rate-card">
      <div className="rate-card-head">
        <KindBadge kind={item.place_kind} size={36} round />
        <div className="grow" style={{ minWidth: 0 }}>
          <Link to={`/places/${item.place_id}`}><b>{item.place_name}</b></Link>
          <span className="muted small">You were there {dayWord(item.date)}{item.time ? ` at ${fmtClock(item.time)}` : ''}</span>
        </div>
      </div>
      <div className="why">
        {item.minutes > 0 && <span title="Seen by Pocket Movement"><Icon name="footprints" size={13} />{minutesText(item.minutes)}</span>}
        {item.amount > 0 && <span title={item.payee ? `In Pocket Money: ${item.payee}` : 'In Pocket Money'}><Icon name="wallet" size={13} />{money(item.amount, item.currency)}</span>}
      </div>
      <div className="row wrap">
        <span className="small muted">How was it?</span>
        <Stars value={rating} onChange={v => { setRating(v); if (v) accept(v) }} />
      </div>
      <div className="row">
        <button type="button" className="btn sm" disabled={busy} onClick={() => accept(0)}><Icon name="check" size={14} />Log without rating</button>
        <button type="button" className="btn sm ghost" disabled={busy} onClick={dismiss}>I wasn't there</button>
      </div>
    </div>
  )
}

/** A plan whose day has passed: did you go? */
export function PastPlanCard({ v, onDone }: { v: Visit; onDone: () => void }) {
  const { toast } = useToast()
  const [rating, setRating] = useState(0)
  const [busy, setBusy] = useState(false)
  const went = async (r: number) => {
    setBusy(true)
    try { await api.put(`${V1}/visits/${v.id}`, { planned: false, rating: r }); toast(`Visit to ${v.place_name} logged`); announceChange(); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const didNot = async () => {
    setBusy(true)
    try { await api.del(`${V1}/visits/${v.id}`); toast('Plan removed'); announceChange(); onDone() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="rate-card">
      <div className="rate-card-head">
        <KindBadge kind={v.place_kind} size={36} round />
        <div className="grow" style={{ minWidth: 0 }}>
          <Link to={`/places/${v.place_id}`}><b>{v.place_name}</b></Link>
          <span className="muted small">You planned to go {dayWord(v.date)}{v.with_whom ? `, with ${v.with_whom}` : ''}</span>
        </div>
      </div>
      <div className="row wrap"><span className="small muted">Did you go? How was it?</span><Stars value={rating} onChange={r => { setRating(r); if (r) went(r) }} /></div>
      <div className="row">
        <button type="button" className="btn sm" disabled={busy} onClick={() => went(0)}><Icon name="check" size={14} />We went</button>
        <button type="button" className="btn sm ghost" disabled={busy} onClick={didNot}>We didn't</button>
      </div>
    </div>
  )
}
