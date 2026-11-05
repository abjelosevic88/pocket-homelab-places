// Your lists ("Date night", "Lisbon in May"); one list with its places, notes and map.
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData, useOnChange } from '../lib/data'
import type { Place, PlaceList } from '../lib/types'
import { Card, Confirm, Empty, Loading, errMsg, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { ListModal } from '../components/Forms'
import { PlaceRow } from '../components/Rows'
import { PlacesMap } from '../components/PlacesMap'
import { openCreate, useCrumb } from '../components/Layout'

export default function Lists() {
  const { lists } = useData()
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Lists</h1><p className="muted">Your own collections: coffee in town, date night, a trip.</p></div>
        <button type="button" className="btn primary" onClick={() => openCreate('list')}><Icon name="plus" size={15} />A new list</button>
      </div>
      {!lists ? <Loading /> : lists.length === 0 ? <Empty title="No lists yet" icon="list">Make one for the places that go together, then add places to it from their page.</Empty> : (
        <div className="list-cards">{lists.map(l => (
          <Link key={l.id} to={`/lists/${l.id}`} className="list-card">
            <span className="emoji" aria-hidden="true">{l.emoji || '📍'}</span>
            <b>{l.name}</b>
            <span className="muted small">{l.count} {l.count === 1 ? 'place' : 'places'}{l.count ? `, ${l.been} been` : ''}{l.trip_name ? ` · ${l.trip_name}` : ''}</span>
            {l.count > 0 && <span className="bar" aria-hidden="true"><span style={{ width: `${Math.round((l.been / l.count) * 100)}%` }} /></span>}
          </Link>))}</div>
      )}
    </div>
  )
}

export function ListPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [status, setStatus] = useState('')
  const { data, reload, error } = useAsync(() => api.get<{ list: PlaceList; places: Place[]; notes: Record<string, string> }>(`${V1}/lists/${id}${status ? `?status=${status}` : ''}`), [id, status])
  useOnChange(reload)
  const [modal, setModal] = useState<'' | 'edit' | 'delete'>('')
  const [sel, setSel] = useState<number | null>(null)
  useCrumb(data?.list.name)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} />{error}</div>
  if (!data) return <Loading />
  const l = data.list
  const onMap = data.places.filter(p => p.lat != null)
  const remove = async (p: Place) => { try { await api.del(`${V1}/lists/${l.id}/places/${p.id}`); toast(`${p.name} taken off the list`); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className="stack loose">
      <div className="detail-head">
        <span className="list-emoji" aria-hidden="true">{l.emoji || '📍'}</span>
        <div className="grow" style={{ minWidth: 0 }}>
          <h1 className="page-title">{l.name}</h1>
          <p className="muted">{[l.description, `${l.count} ${l.count === 1 ? 'place' : 'places'}, ${l.been} been`, l.trip_name && `for ${l.trip_name}`].filter(Boolean).join(' · ')}</p>
        </div>
      </div>
      <div className="row wrap toolbar">
        <button type="button" className="btn primary" onClick={() => openCreate('place')}><Icon name="plus" size={15} />Add a place</button>
        <div className="chips">{[{ v: '', l: 'All' }, { v: 'want', l: 'Want to go' }, { v: 'been', l: 'Been there' }].map(c => <button key={c.v} type="button" className={`chip ${status === c.v ? 'on' : ''}`} aria-pressed={status === c.v} onClick={() => setStatus(c.v)}>{c.l}</button>)}</div>
        <span className="grow" />
        <button type="button" className="btn ghost" onClick={() => setModal('edit')}><Icon name="edit" size={15} />Edit</button>
        <button type="button" className="btn ghost danger-text" onClick={() => setModal('delete')}><Icon name="trash" size={15} />Delete</button>
      </div>
      {data.places.length === 0 ? <Empty title={status ? 'None here' : 'Nothing on this list yet'} icon="list">Add places with the button above, or from a place's page with Lists.</Empty> : (
        <div className="two-grid">
          <Card flush>
            <div className="list">{data.places.map(p => (
              <div key={p.id} className="list-row" style={{ padding: 0 }}>
                <div className="grow" style={{ minWidth: 0 }}><PlaceRow p={p} note={data.notes[p.id]} /></div>
                <button type="button" className="btn ghost sm icon-only" style={{ marginRight: 10 }} onClick={() => remove(p)} aria-label={`Take ${p.name} off the list`} title="Take it off the list"><Icon name="x" size={14} /></button>
              </div>))}</div>
          </Card>
          {onMap.length > 0 && <PlacesMap places={onMap} selected={sel} onSelect={x => { setSel(x); if (x) navigate(`/places/${x}`) }} fitKey={`l${l.id}${status}`} />}
        </div>
      )}
      {modal === 'edit' && <ListModal list={l} onClose={() => setModal('')} onSaved={reload} />}
      {modal === 'delete' && <Confirm title={`Delete ${l.name}?`} message="The list goes; its places stay." confirmLabel="Delete"
        onCancel={() => setModal('')} onConfirm={async () => { setModal(''); try { await api.del(`${V1}/lists/${l.id}`); toast('List deleted'); announceChange(); navigate('/lists') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
