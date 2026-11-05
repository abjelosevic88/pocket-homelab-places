// Your places on a map (Leaflet + OpenStreetMap). A pin per place, tinted by
// kind: filled for where you have been, outlined for where you want to go.
// Tapping a pin selects it (the page shows a card, never a popup over the
// map); with onPick, a tap or a drag sets a spot instead.
import { useEffect, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useApp } from '../lib/app-context'
import { kindIcon } from '../lib/places'
import type { Place } from '../lib/types'
import { iconSVG } from './icons'

const OSM = { url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png', attr: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>', max: 19 }
const DARK = {
  url: 'https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Base/MapServer/tile/{z}/{y}/{x}',
  labels: 'https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Reference/MapServer/tile/{z}/{y}/{x}',
  attr: 'Tiles &copy; Esri', max: 16,
}

type Spot = { lat: number; lng: number }

export function PlacesMap({ places, selected, onSelect, pick, onPick, me, fitKey = '', className = '' }: {
  places: Place[]; selected?: number | null; onSelect?: (id: number | null) => void
  pick?: Spot | null; onPick?: (s: Spot) => void; me?: Spot | null; fitKey?: string; className?: string
}) {
  const ref = useRef<HTMLDivElement>(null)
  const map = useRef<L.Map | null>(null)
  const pins = useRef<L.LayerGroup | null>(null)
  const tiles = useRef<L.Layer[]>([])
  const picker = useRef<L.Marker | null>(null)
  const meDot = useRef<L.CircleMarker | null>(null)
  const lastFit = useRef<string | null>(null)
  const cb = useRef({ onSelect, onPick })
  cb.current = { onSelect, onPick }
  const { theme } = useApp()

  // ---- create once ----
  useEffect(() => {
    if (!ref.current || map.current) return
    const m = L.map(ref.current, { zoomControl: false, attributionControl: true, worldCopyJump: true })
    L.control.zoom({ position: 'bottomright' }).addTo(m)
    m.setView([43.8563, 18.4131], 12)
    pins.current = L.layerGroup().addTo(m)
    m.on('click', e => {
      if (cb.current.onPick) cb.current.onPick({ lat: e.latlng.lat, lng: e.latlng.lng })
      else cb.current.onSelect?.(null)
    })
    map.current = m
    // the map may be laid out after it was made (a dialog, a tab): measure again
    const ro = new ResizeObserver(() => m.invalidateSize())
    ro.observe(ref.current)
    return () => { ro.disconnect(); m.remove(); map.current = null }
  }, [])

  // ---- tiles follow the theme ----
  useEffect(() => {
    const m = map.current
    if (!m) return
    tiles.current.forEach(t => m.removeLayer(t))
    tiles.current = theme === 'dark'
      ? [L.tileLayer(DARK.url, { attribution: DARK.attr, maxNativeZoom: DARK.max, maxZoom: 19 }), L.tileLayer(DARK.labels, { maxNativeZoom: DARK.max, maxZoom: 19 })]
      : [L.tileLayer(OSM.url, { attribution: OSM.attr, maxZoom: OSM.max })]
    tiles.current.forEach(t => t.addTo(m))
  }, [theme])

  // ---- pins ----
  useEffect(() => {
    const g = pins.current
    if (!g) return
    g.clearLayers()
    for (const p of places) {
      if (p.lat == null || p.lng == null) continue
      const cls = `pin k-${p.kind} ${p.status === 'want' ? 'want' : ''} ${p.closed ? 'closed' : ''} ${selected === p.id ? 'sel' : ''}`
      const icon = L.divIcon({ className: 'pin-wrap', html: `<div class="${cls}" title="">${iconSVG(kindIcon(p.kind), 13)}</div>`, iconSize: [26, 26], iconAnchor: [13, 31] })
      const mk = L.marker([p.lat, p.lng], { icon, title: p.name, keyboard: true, zIndexOffset: selected === p.id ? 1000 : p.status === 'been' ? 100 : 0 })
      mk.on('click', e => { L.DomEvent.stopPropagation(e); if (!cb.current.onPick) cb.current.onSelect?.(p.id) })
      g.addLayer(mk)
    }
  }, [places, selected])

  // ---- fit to the places when the set changes (not on every reload) ----
  useEffect(() => {
    const m = map.current
    if (!m) return
    const key = fitKey + '|' + (pick ? 'pick' : places.length)
    if (lastFit.current === key) return
    lastFit.current = key
    const pts = places.filter(p => p.lat != null && p.lng != null).map(p => L.latLng(p.lat!, p.lng!))
    if (pick) { m.setView([pick.lat, pick.lng], Math.max(m.getZoom(), 16)); return }
    if (pts.length === 1) m.setView(pts[0], 16)
    else if (pts.length > 1) m.fitBounds(L.latLngBounds(densest(pts)), { padding: [40, 40], maxZoom: 16 })
    else if (me) m.setView([me.lat, me.lng], 14)
  }, [places, fitKey, pick, me])

  // ---- the spot being picked ----
  useEffect(() => {
    const m = map.current
    if (!m) return
    if (!pick) { picker.current?.remove(); picker.current = null; return }
    if (!picker.current) {
      const icon = L.divIcon({ className: 'pin-wrap', html: `<div class="pin sel">${iconSVG('pin', 13)}</div>`, iconSize: [26, 26], iconAnchor: [13, 31] })
      picker.current = L.marker([pick.lat, pick.lng], { icon, draggable: true }).addTo(m)
      picker.current.on('dragend', () => { const ll = picker.current!.getLatLng(); cb.current.onPick?.({ lat: ll.lat, lng: ll.lng }) })
    } else picker.current.setLatLng([pick.lat, pick.lng])
  }, [pick])

  // ---- where you are ----
  useEffect(() => {
    const m = map.current
    if (!m) return
    meDot.current?.remove()
    meDot.current = me ? L.circleMarker([me.lat, me.lng], { radius: 7, color: '#fff', weight: 2, fillColor: '#2563eb', fillOpacity: 1 }).addTo(m) : null
  }, [me])

  return <div className={`pmap ${className}`}><div ref={ref} style={{ width: '100%', height: '100%' }} /></div>
}


/** The places spread over towns or countries? Then the map opens on the town
 *  with most of them (within 8 km of the busiest spot); the others are a zoom
 *  out away. */
function densest(pts: L.LatLng[]): L.LatLng[] {
  const all = L.latLngBounds(pts)
  if (all.getNorthWest().distanceTo(all.getSouthEast()) < 50000) return pts
  let best: L.LatLng[] = pts
  let bestN = 0
  for (const c of pts) {
    const near = pts.filter(p => p.distanceTo(c) < 8000)
    if (near.length > bestN) { best = near; bestN = near.length }
  }
  return bestN >= 2 ? best : pts
}
