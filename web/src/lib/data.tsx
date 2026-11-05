// The person's lists and how many visits wait for a rating, loaded once for
// every picker and the sidebar; any change in the app ("ppl:changed")
// reloads them.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { InboxItem, PlaceList, Visit } from './types'

interface Data { lists: PlaceList[] | null; toRate: number; reload: () => void }
const Ctx = createContext<Data>({ lists: null, toRate: 0, reload: () => {} })
export const useData = () => useContext(Ctx)

/** Something was added or changed: lists and pages reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('ppl:changed'))
export function useOnChange(cb: () => void) {
  const ref = React.useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('ppl:changed', h)
    return () => window.removeEventListener('ppl:changed', h)
  }, [])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [lists, setLists] = useState<PlaceList[] | null>(null)
  const [toRate, setToRate] = useState(0)
  const reload = useCallback(() => {
    api.get<PlaceList[]>(`${V1}/lists`).then(setLists).catch(() => setLists([]))
    api.get<{ items: InboxItem[]; past_plans: Visit[] }>(`${V1}/inbox`).then(r => setToRate(r.items.length + r.past_plans.length)).catch(() => {})
  }, [])
  useEffect(() => { reload() }, [reload])
  useOnChange(reload)
  return <Ctx.Provider value={{ lists, toRate, reload }}>{children}</Ctx.Provider>
}
