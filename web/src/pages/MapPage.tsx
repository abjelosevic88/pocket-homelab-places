// Every place on one map, with the same filters as the list; tap a pin for its card.
import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { directionsURL, distanceText, placeLine } from '../lib/places'
import type { Place } from '../lib/types'
import { Card, Loading, useAsync } from '../components/ui'
import { Icon } from '../components/icons'
import { KindBadge, Price, Rating, StatusBadge } from '../components/Rows'
import { PlacesMap } from '../components/PlacesMap'
import { FilterBar, usePlaceFilters } from './Places'

export default function MapPage() {
  const { settings } = useApp()
  const f = usePlaceFilters()
  const q = f.query()
  const { data, reload } = useAsync(() => api.get<Place[]>(`${V1}/places?${q}`), [q])
  useOnChange(reload)
  const [sp, setSp] = useSearchParams()
  const [sel, setSel] = useState<number | null>(Number(sp.get('place')) || null)
  useEffect(() => { const n = new URLSearchParams(sp); if (sel) n.set('place', String(sel)); else n.delete('place'); setSp(n, { replace: true }) }, [sel]) // eslint-disable-line react-hooks/exhaustive-deps
  const near = f.get('near').split(',').map(Number)
  const me = near.length === 2 && near.every(Number.isFinite) && !f.label ? { lat: near[0], lng: near[1] } : null
  const onMap = useMemo(() => (data || []).filter(p => p.lat != null && p.lng != null), [data])
  const p = onMap.find(x => x.id === sel)
  const missing = (data?.length || 0) - onMap.length
  return (
    <div className="stack">
      <FilterBar total={data?.length} showSort={false} />
      {!data ? <Loading /> : (
        <div style={{ position: 'relative' }}>
          <PlacesMap places={onMap} selected={sel} onSelect={setSel} me={me} className="tall" fitKey={q} />
          {onMap.length === 0 && <div className="map-empty">No places on the map with these filters.</div>}
          {p && <div className="map-card">
            <Card>
              <div className="row" style={{ alignItems: 'flex-start' }}>
                <KindBadge kind={p.kind} size={40} />
                <div className="grow" style={{ minWidth: 0 }}>
                  <Link to={`/places/${p.id}`}><b>{p.name}</b></Link>
                  <div className="muted small row wrap" style={{ gap: 6 }}><span>{placeLine(p)}</span><Price value={p.price} />{p.distance_m != null && <span>{distanceText(p.distance_m, settings?.distance_unit)}</span>}</div>
                </div>
                {p.rating > 0 ? <Rating value={p.rating} /> : <StatusBadge p={p} />}
                <button type="button" className="btn ghost sm icon-only" aria-label="Close" onClick={() => setSel(null)}><Icon name="x" size={14} /></button>
              </div>
              {p.notes && <p className="small muted" style={{ margin: '8px 0 0' }}>{p.notes.length > 140 ? p.notes.slice(0, 140) + '…' : p.notes}</p>}
              <div className="row mt">
                <Link to={`/places/${p.id}`} className="btn sm primary">Open</Link>
                <a className="btn sm" href={directionsURL(p)} target="_blank" rel="noreferrer"><Icon name="navigation" size={14} />Directions</a>
              </div>
            </Card>
          </div>}
        </div>
      )}
      {missing > 0 && <p className="muted small">{missing} {missing === 1 ? 'place is' : 'places are'} not on the map yet (no spot): <Link to={`/places?${q}`}>see the list</Link> and add the address or the spot under Edit.</p>}
    </div>
  )
}
