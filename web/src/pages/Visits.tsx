// Every visit, month by month, with what it cost; one year at a time.
import { useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useOnChange } from '../lib/data'
import { todayISO } from '../lib/format'
import { money } from '../lib/places'
import type { SpentRow, Visit } from '../lib/types'
import { Card, Empty, Loading, Segmented, useAsync, useDebounce } from '../components/ui'
import { Icon } from '../components/icons'
import { VisitRow } from '../components/Rows'
import { openCreate } from '../components/Layout'

export default function Visits() {
  const year = Number(todayISO().slice(0, 4))
  const [y, setY] = useState(year)
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 300)
  const url = `${V1}/visits?planned=false&from=${y}-01-01&to=${y}-12-31&limit=500${dq ? `&q=${encodeURIComponent(dq)}` : ''}`
  const { data, reload, error } = useAsync(() => api.get<{ items: Visit[]; total: number; spent: SpentRow[] }>(url), [url])
  useOnChange(reload)
  const months = useMemo(() => {
    const out: { key: string; label: string; items: Visit[] }[] = []
    for (const v of data?.items || []) {
      const key = v.date.slice(0, 7)
      let m = out[out.length - 1]
      if (!m || m.key !== key) { m = { key, label: new Date(key + '-15T12:00:00').toLocaleDateString(undefined, { month: 'long' }), items: [] }; out.push(m) }
      m.items.push(v)
    }
    return out
  }, [data])
  const places = new Set((data?.items || []).map(v => v.place_id)).size
  const top = useMemo(() => {
    const n = new Map<string, number>()
    for (const v of data?.items || []) n.set(v.place_name, (n.get(v.place_name) || 0) + 1)
    return [...n.entries()].sort((a, b) => b[1] - a[1])[0]
  }, [data])
  const spentOf = (vs: Visit[]) => {
    const by = new Map<string, number>()
    for (const v of vs) if (v.spent) by.set(v.currency, (by.get(v.currency) || 0) + v.spent)
    return [...by.entries()].map(([c, t]) => money(t, c, true)).join(' + ')
  }
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Visits</h1><p className="muted">Where you went, with whom, how it was and what it cost.</p></div>
        <button type="button" className="btn primary" onClick={() => openCreate('visit')}><Icon name="plus" size={15} />Log a visit</button>
      </div>
      <div className="row wrap filters">
        <Segmented size="sm" value={y} onChange={setY} options={[year - 2, year - 1, year].map(v => ({ value: v, label: String(v) }))} />
        <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder="Search place, people, notes…" aria-label="Search visits" />
      </div>
      {error ? <div className="callout danger"><Icon name="alert" size={15} />{error}</div> : !data ? <Loading /> : <>
        <div className="stats-row">
          <div className="stat"><span className="stat-l">Visits</span><span className="stat-v tnum">{data.total}</span></div>
          <div className="stat"><span className="stat-l">Places</span><span className="stat-v tnum">{places}</span></div>
          <div className="stat"><span className="stat-l">Spent</span><span className="stat-v tnum">{data.spent.length ? data.spent.map(r => money(r.total, r.currency, true)).join(' + ') : '—'}</span></div>
          {top && <div className="stat"><span className="stat-l">Most visits</span><span className="stat-v ellipsis">{top[0]}</span><span className="stat-s small">{top[1]} times</span></div>}
        </div>
        {data.items.length === 0 ? <Empty title={dq ? 'Nothing matches' : `No visits in ${y}`} icon="journal">Log a visit from a place's page, or answer "how was it?" on the overview.</Empty> : (
          <Card flush>
            <div className="list">{months.map(m => (
              <div key={m.key}>
                <div className="month-h"><span>{m.label}</span><span className="tnum">{m.items.length} · {spentOf(m.items)}</span></div>
                {m.items.map(v => <VisitRow key={v.id} v={v} />)}
              </div>))}</div>
          </Card>
        )}
      </>}
    </div>
  )
}
