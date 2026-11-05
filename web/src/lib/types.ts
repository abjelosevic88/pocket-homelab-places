export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'movement' | 'money' | 'calendar' | 'contacts' | 'travel' | 'split'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number; currency: string; distance_unit: 'km' | 'mi'
  online: boolean; online_blocked: boolean
  notify_to_rate: boolean; notify_planned: boolean; notify_trips: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  match_visits: boolean; match_payments: boolean; match_minutes: number; match_meters: number
  money_book: boolean; money_account_id: number; money_category_id: number
  calendar_plans: boolean; calendar_id: number; split_group_id: number
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string
}

export type Status = 'want' | 'been'

export interface Place {
  id: number; name: string; kind: string; cuisine: string; status: Status; favourite: boolean; closed: boolean; rating: number; price: number
  address: string; city: string; country: string; lat: number | null; lng: number | null; phone: string; website: string; map_url: string; hours: string
  tags: string; notes: string; recommended_by: string; contact_id: number | null; currency: string; source: string; source_ref: string
  created_at: string; updated_at: string
  visits: number; last_visit: string; next_plan: string; spent: number; lists: number[]; distance_m?: number
}

export interface Visit {
  id: number; place_id: number; date: string; time: string; planned: boolean; rating: number; notes: string; with_whom: string; minutes: number
  spent: number; currency: string; money_tx_id: number | null; money_booked: boolean; split_group_id: number | null; split_expense_id: number | null
  cal_event_id: number | null; trip_id: number | null; trip_name: string; source: string; source_ref: string; created_at: string; updated_at: string
  place_name: string; place_kind: string; city: string
}

export interface PlaceList {
  id: number; name: string; emoji: string; description: string; trip_id: number | null; trip_name: string; sort: number; created_at: string; updated_at: string
  count: number; been: number
}

export interface InboxItem {
  id: number; place_id: number; date: string; time: string; minutes: number; movement_id: number | null; amount: number; currency: string
  money_tx_id: number | null; payee: string; status: string; created_at: string; place_name: string; place_kind: string; city: string
}

export interface SpentRow { currency: string; total: number; visits: number }
export interface CityCount { city: string; country: string; count: number; want: number }

export interface Trip {
  trip_id: number; name: string; destination: string; start: string; end: string; days_until: number; lat: number | null; lng: number | null
  want: number; been: number; visits: number; list_ids: number[]; url: string
}

export interface MovementSummary { visits: number; minutes: number; first: string; last: string; url: string; error?: string }

export interface PlaceDetail {
  place: Place; visits: Visit[]; lists: PlaceList[]; list_notes: Record<string, string>; nearby: Place[]; inbox: InboxItem[]; spent: SpentRow[]
  movement: MovementSummary | null; links: Record<string, string>
}

export interface Overview {
  today: string; counts: { places: number; want: number; been: number; favourites: number; lists: number; to_rate: number }
  inbox: InboxItem[]; past_plans: Visit[]; planned: Visit[]; recent: Visit[]; favourites: Place[]; want: Place[]; trips: Trip[]; trips_error: string
  spent_year: SpentRow[]; visits_year: number; cities: CityCount[]; lists: PlaceList[]
}

export interface Found {
  name: string; kind: string; cuisine: string; address: string; city: string; country: string; lat: number | null; lng: number | null
  website: string; phone: string; hours: string; map_url: string; label: string
}

export interface MoneyResult { ok: boolean; tx_id?: number; account?: string; url?: string; via?: string; error?: string; skipped?: boolean }
export interface CalResult { ok: boolean; event_id?: number; error?: string }

export interface Picked<T> { connected: boolean; detail?: string; items: T[] }
export interface MoneyAccount { id: number; name: string; kind: string; currency: string; everyday: boolean }
export interface MoneyCategory { id: number; name: string; full_name: string }
export interface CalendarItem { id: number; name: string; color: string; writable: boolean; is_default: boolean }
export interface ContactItem { id: number; fn: string; org: string; title: string; emails: string[] | null; phones: string[] | null }
export interface SplitGroup { id: number; name: string; currency: string; members: { id: number; name: string; user_id: number | null }[]; my_member_id: number }
export interface Payment { id: number; kind: string; date: string; time: string; amount: number; currency: string; foreign_amount: number; foreign_currency: string; payee: string; description: string; likely: boolean; why: string }
export interface Kind { value: string; label: string; group: string }
