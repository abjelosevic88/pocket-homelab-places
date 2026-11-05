// What a place is, how it is shown, and the small formatting helpers.
import type { Place } from './types'

export const KINDS: { value: string; label: string; group: string; icon: string }[] = [
  { value: 'restaurant', label: 'Restaurant', group: 'eat', icon: 'utensils' },
  { value: 'cafe', label: 'Café', group: 'eat', icon: 'coffee' },
  { value: 'bar', label: 'Bar or pub', group: 'eat', icon: 'glass' },
  { value: 'bakery', label: 'Bakery', group: 'eat', icon: 'bread' },
  { value: 'street_food', label: 'Street food and takeaway', group: 'eat', icon: 'flame' },
  { value: 'dessert', label: 'Dessert and ice cream', group: 'eat', icon: 'cake' },
  { value: 'market', label: 'Market', group: 'shop', icon: 'cart' },
  { value: 'shop', label: 'Shop', group: 'shop', icon: 'store' },
  { value: 'museum', label: 'Museum or gallery', group: 'see', icon: 'landmark' },
  { value: 'sight', label: 'Sight or landmark', group: 'see', icon: 'camera' },
  { value: 'park', label: 'Park or garden', group: 'see', icon: 'tree' },
  { value: 'viewpoint', label: 'Viewpoint', group: 'see', icon: 'sunrise' },
  { value: 'beach', label: 'Beach or lake', group: 'see', icon: 'waves' },
  { value: 'nature', label: 'Hike or nature', group: 'see', icon: 'mountain' },
  { value: 'activity', label: 'Something to do', group: 'see', icon: 'ticket' },
  { value: 'hotel', label: 'Hotel or stay', group: 'stay', icon: 'bed' },
  { value: 'other', label: 'Other', group: 'other', icon: 'pin' },
]
export const GROUPS: { value: string; label: string; icon: string }[] = [
  { value: 'eat', label: 'Eat & drink', icon: 'utensils' },
  { value: 'see', label: 'See & do', icon: 'camera' },
  { value: 'shop', label: 'Shops & markets', icon: 'store' },
  { value: 'stay', label: 'Stays', icon: 'bed' },
]
export const kindOf = (k: string) => KINDS.find(x => x.value === k) || KINDS[KINDS.length - 1]
export const kindIcon = (k: string) => kindOf(k).icon
export const kindLabel = (k: string) => kindOf(k).label

/** "€€" for a price level (1–4), "" when unknown. */
export const priceText = (n: number) => (n > 0 ? '€'.repeat(n) : '')

/** "Restaurant · Bosnian · Sarajevo" */
export function placeLine(p: Pick<Place, 'kind' | 'cuisine' | 'city'>, withCity = true): string {
  return [p.cuisine || kindLabel(p.kind), withCity ? p.city : ''].filter(Boolean).join(' · ')
}

export function money(minor: number | null | undefined, currency = 'EUR', whole = false): string {
  const v = (minor || 0) / 100
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency, minimumFractionDigits: whole ? 0 : 2, maximumFractionDigits: whole ? 0 : 2 }).format(v)
  } catch {
    return `${v.toFixed(whole ? 0 : 2)} ${currency}`
  }
}

/** A typed amount ("12,50" or "12.50") → minor units, null when empty or bad. */
export function parseMoney(s: string): number | null {
  const t = s.trim().replace(/\s/g, '').replace(/,(\d{1,2})$/, '.$1').replace(/,/g, '')
  if (!t) return null
  const n = Number(t)
  if (!Number.isFinite(n) || n < 0) return null
  return Math.round(n * 100)
}
export const moneyInput = (minor: number | null | undefined) => minor == null || minor === 0 ? '' : (minor / 100).toFixed(2)

/** 850 m, 3.2 km (or miles). */
export function distanceText(m: number | null | undefined, unit: 'km' | 'mi' = 'km'): string {
  if (m == null) return ''
  if (unit === 'mi') {
    const mi = m / 1609.34
    return mi < 0.1 ? `${Math.round(m * 1.094)} yd` : `${mi < 10 ? mi.toFixed(1) : Math.round(mi)} mi`
  }
  return m < 1000 ? `${Math.round(m / 10) * 10} m` : `${m < 10000 ? (m / 1000).toFixed(1) : Math.round(m / 1000)} km`
}

/** Directions in the phone's maps app (Apple Maps on iPhones, else Google Maps). */
export function directionsURL(p: Pick<Place, 'lat' | 'lng' | 'name' | 'address' | 'city'>): string {
  const apple = /iPhone|iPad|Macintosh/.test(navigator.userAgent) && 'ontouchend' in document
  const q = p.lat != null && p.lng != null ? `${p.lat},${p.lng}` : [p.name, p.address, p.city].filter(Boolean).join(', ')
  return apple ? `https://maps.apple.com/?daddr=${encodeURIComponent(q)}&q=${encodeURIComponent(p.name)}` : `https://www.google.com/maps/dir/?api=1&destination=${encodeURIComponent(q)}`
}

/** The person's current spot from the browser, or null. */
export function whereAmI(timeout = 8000): Promise<{ lat: number; lng: number } | null> {
  return new Promise(resolve => {
    if (!navigator.geolocation) { resolve(null); return }
    navigator.geolocation.getCurrentPosition(p => resolve({ lat: p.coords.latitude, lng: p.coords.longitude }), () => resolve(null), { timeout, maximumAge: 120000, enableHighAccuracy: false })
  })
}

/** "1 h 20 min" */
export function minutesText(m: number): string {
  if (!m) return ''
  const h = Math.floor(m / 60), r = m % 60
  return h ? (r ? `${h} h ${r} min` : `${h} h`) : `${r} min`
}

export const ratingText = (r: number) => (r ? r.toLocaleString(undefined, { maximumFractionDigits: 1 }) : '')

/** Flag emoji for a country code. */
export function flag(cc: string): string {
  if (!/^[A-Za-z]{2}$/.test(cc)) return ''
  return String.fromCodePoint(...[...cc.toUpperCase()].map(c => 0x1f1e6 + c.charCodeAt(0) - 65))
}
