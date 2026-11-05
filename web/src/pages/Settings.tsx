import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, CalendarItem, ConnMode, MoneyAccount, MoneyCategory, Picked, Settings as SettingsT, SplitGroup, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'

type TabId = 'general' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Your currency, distances, looking places up online, dates and the app title.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: '"How was it?" after you visited one of your places, the morning of a plan, and trips with places on your list.' },
  { id: 'apps', label: 'Connected apps', icon: 'layers', sub: 'Pocket Movement and Pocket Money (noticing your visits), Pocket Calendar (plans), Pocket Contacts, Pocket Travel and Pocket Split.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who picks "where should we go?": Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Homepage widgets, Siri Shortcuts and scripts.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Bring your places in from Google Maps, export them, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Money and distances" icon="wallet">
        <SettingRow title="Your currency" help="For what visits cost; a place abroad can have its own under Edit."><input value={f.currency} onChange={e => setF({ ...f, currency: e.target.value.toUpperCase() })} maxLength={3} className="set-time" /></SettingRow>
        <SettingRow title="Distances in"><Segmented size="sm" value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v })} options={[{ value: 'km', label: 'Kilometres' }, { value: 'mi', label: 'Miles' }]} /></SettingRow>
      </Section>
      <Section title="Looking places up" icon="globe" sub="When you add a place, its name is looked up on OpenStreetMap (address, spot, website, opening hours) and a pasted Google Maps or Apple Maps link is opened to read the name and the spot. What you type goes to OpenStreetMap; nothing else does.">
        <SettingRow title="Look places up online" help={f.online_blocked ? 'Off while this box shows demo data.' : undefined}><Switch checked={f.online && !f.online_blocked} disabled={f.online_blocked} onChange={v => setF({ ...f, online: v })} label="Look places up online" /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const now = async () => { try { const r = await api.post<{ sent: boolean; body: string }>(`${V1}/notify/check`); toast(r.sent ? 'Sent' : r.body ? `Nothing sent (no push URL): ${r.body.split('\n')[0]}` : 'No plans or trips to tell you about today') } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/places" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to tell you" icon="clock">
        <SettingRow title="How was it?" help="Soon after Pocket Movement or Pocket Money notice you were at one of your places (never at night: those wait for the morning)."><Switch checked={f.notify_to_rate} onChange={v => setF({ ...f, notify_to_rate: v })} label="How was it" /></SettingRow>
        <SettingRow title="Today's plans" help="In the morning of a planned visit."><Switch checked={f.notify_planned} onChange={v => setF({ ...f, notify_planned: v })} label="Today's plans" /></SettingRow>
        <SettingRow title="Trips" help="Three days before a trip in Pocket Travel: how many places on your list are there."><Switch checked={f.notify_trips} onChange={v => setF({ ...f, notify_trips: v })} label="Trips" /></SettingRow>
        <SettingRow title="Morning message at" help="Plans and trips come in one message at this hour (server time)."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        <button type="button" className="btn" onClick={now}><Icon name="refresh" size={15} />Check now</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; name: string; icon: string; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'movement', name: 'Pocket Movement', icon: 'footprints', title: 'Pocket Movement — notice your visits', sub: 'When you stayed at one of your places for a while, it asks how it was. A place\'s page shows how often you have been there.', token: 'Pocket Movement → Settings → API tokens', port: '8197' },
  { slug: 'money', name: 'Pocket Money', icon: 'wallet', title: 'Pocket Money — what you paid', sub: 'A payment at one of your places (at its spot, or under its name) goes with the visit, so the cost is filled in without booking it twice. You can also book what a visit cost as a transaction.', token: 'Pocket Money → Settings → API tokens', port: '8178' },
  { slug: 'calendar', name: 'Pocket Calendar', icon: 'calendar', title: 'Pocket Calendar — plans on your calendar', sub: 'A planned visit (a table booked, a museum on Saturday) goes on your calendar with the address, and moves when you change the plan. Your own events are never touched.', token: 'Pocket Calendar → Settings → API tokens', port: '8190' },
  { slug: 'contacts', name: 'Pocket Contacts', icon: 'users', title: 'Pocket Contacts — who told you, who came along', sub: 'Pick who recommended a place and who came along from your contacts.', token: 'Pocket Contacts → Settings → API tokens', port: '8191' },
  { slug: 'travel', name: 'Pocket Travel', icon: 'plane', title: 'Pocket Travel — places on your trips', sub: 'Coming trips show how many of your places are at the destination; a list can be for a trip; visits during a trip are filed with it.', token: 'Pocket Travel → Settings → API tokens', port: '8186' },
  { slug: 'split', name: 'Pocket Split', icon: 'users', title: 'Pocket Split — share the bill', sub: '"Split the bill" on a visit adds it to one of your groups as an expense you paid, shared evenly.', token: 'Pocket Split → Settings → API tokens', port: '8188' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { toast } = useToast()
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Through <b>Pocket Core</b> Places acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        return (
          <Section key={a.slug} title={a.title} icon={a.icon} actions={connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={a.name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {a.slug === 'money' && eff(mode) !== 'off' && via && <MoneyOptions f={f} setF={setF} save={save} />}
            {a.slug === 'calendar' && eff(mode) !== 'off' && <div className="mt">
              <SettingRow title="Plans go on my calendar" help="The tick in the plan form starts on; you can untick it each time.">
                <Switch checked={f.calendar_plans} disabled={!via} onChange={v => save({ calendar_plans: v })} label="Calendar" /></SettingRow>
              {via && f.calendar_plans && <CalendarOptions f={f} save={save} />}
            </div>}
            {a.slug === 'split' && eff(mode) !== 'off' && via && <SplitOptions f={f} save={save} />}
          </Section>
        )
      })}
      <Matching f={f} setF={setF} save={save} />
      <Section title="Other Pocket apps" icon="layers" sub="Places offers your places to the rest of the box through Pocket Core: Pocket AI answers “where did we have that lamb?” and can save a place or log a visit for you, Pocket Day writes your visits into your journal, Pocket Travel shows your places on each trip. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>places:read</dt><dd className="muted">Your places with ratings, lists and notes, visits with what they cost, plans and what waits to be rated.</dd></div>
          <div><dt>places:write</dt><dd className="muted">Add a place and log or plan a visit; only what that app added itself can be changed by it.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function MoneyOptions({ f, setF, save }: { f: SettingsT; setF: (s: SettingsT) => void; save: (p?: Partial<SettingsT>) => void }) {
  const { data: accts } = useAsync(() => api.get<Picked<MoneyAccount>>(`${V1}/links/money/accounts`), [])
  const { data: cats } = useAsync(() => api.get<Picked<MoneyCategory>>(`${V1}/links/money/categories?account_id=${f.money_account_id || 0}`), [f.money_account_id])
  return (
    <div className="mt">
      <SettingRow title="Create a transaction by default" help="Leave it off when your payments come into Pocket Money anyway (a card, a bank import): Places finds them and links them to the visit instead."><Switch checked={f.money_book} onChange={v => save({ money_book: v })} label="Book by default" /></SettingRow>
      <SettingRow title="Account" help="Otherwise your everyday account.">
        <Select ariaLabel="Account" value={f.money_account_id ? String(f.money_account_id) : ''} onChange={v => { setF({ ...f, money_account_id: Number(v) || 0 }); save({ money_account_id: Number(v) || 0 }) }} clearLabel="Everyday account" placeholder="Everyday account"
          options={(accts?.items || []).map(a => ({ value: String(a.id), label: a.name, hint: a.currency }))} />
      </SettingRow>
      <SettingRow title="Category" help="Otherwise one named like restaurants or eating out.">
        <Select ariaLabel="Category" value={f.money_category_id ? String(f.money_category_id) : ''} onChange={v => { setF({ ...f, money_category_id: Number(v) || 0 }); save({ money_category_id: Number(v) || 0 }) }} clearLabel="Choose for me" placeholder="Choose for me"
          options={(cats?.items || []).map(c => ({ value: String(c.id), label: c.full_name || c.name }))} />
      </SettingRow>
      {accts && !accts.connected && <p className="bad small">{accts.detail}</p>}
    </div>
  )
}

function CalendarOptions({ f, save }: { f: SettingsT; save: (p?: Partial<SettingsT>) => void }) {
  const { data: cals } = useAsync(() => api.get<Picked<CalendarItem>>(`${V1}/links/calendar/calendars`), [])
  return (
    <SettingRow title="Calendar" help="Otherwise your default calendar.">
      <Select ariaLabel="Calendar" value={f.calendar_id ? String(f.calendar_id) : ''} onChange={v => save({ calendar_id: Number(v) || 0 })} clearLabel="My default calendar" placeholder="My default calendar"
        options={(cals?.items || []).map(c => ({ value: String(c.id), label: c.name }))} />
    </SettingRow>
  )
}

function SplitOptions({ f, save }: { f: SettingsT; save: (p?: Partial<SettingsT>) => void }) {
  const { data: groups } = useAsync(() => api.get<Picked<SplitGroup>>(`${V1}/links/split/groups`), [])
  return (
    <div className="mt">
      <SettingRow title="Group to start with" help="Split the bill offers this group first; it remembers the last one you used.">
        <Select ariaLabel="Group" value={f.split_group_id ? String(f.split_group_id) : ''} onChange={v => save({ split_group_id: Number(v) || 0 })} clearLabel="The first group" placeholder="The first group"
          options={(groups?.items || []).map(g => ({ value: String(g.id), label: g.name, hint: g.currency }))} />
      </SettingRow>
      {groups && !groups.connected && <p className="bad small">{groups.detail}</p>}
    </div>
  )
}

/** Noticing visits: what counts as having been at one of your places. */
function Matching({ f, setF, save }: { f: SettingsT; setF: (s: SettingsT) => void; save: (p?: Partial<SettingsT>) => void }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const scan = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ added: number; stays: number; payments: number; errors: string[] }>(`${V1}/inbox/scan`)
      toast(r.added ? `${r.added} new to rate on the overview` : r.errors.length ? r.errors[0] : `Nothing new (${r.stays} stays and ${r.payments} payments at your places in the last two weeks are already logged or answered)`, r.errors.length && !r.added ? 'err' : 'ok')
      if (r.added) window.dispatchEvent(new CustomEvent('ppl:changed'))
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Section title="Noticing your visits" icon="footprints" sub="Every half hour Places looks at your own Pocket Movement stays and Pocket Money payments of the last days and asks “how was it?” for the ones at your places. Nothing is logged until you answer.">
      <SettingRow title="Stays from Pocket Movement"><Switch checked={f.match_visits} disabled={f.movement_via === ''} onChange={v => save({ match_visits: v })} label="Stays" /></SettingRow>
      <SettingRow title="Payments from Pocket Money"><Switch checked={f.match_payments} disabled={f.money_via === ''} onChange={v => save({ match_payments: v })} label="Payments" /></SettingRow>
      <SettingRow title="A stay counts after" help="Minutes in one spot."><input type="number" min={5} max={600} value={f.match_minutes} onChange={e => setF({ ...f, match_minutes: Number(e.target.value) })} onBlur={() => save({ match_minutes: f.match_minutes })} className="set-time" /></SettingRow>
      <SettingRow title="This close to the place" help="Metres; GPS indoors is often 50 m off."><input type="number" min={30} max={1000} step={10} value={f.match_meters} onChange={e => setF({ ...f, match_meters: Number(e.target.value) })} onBlur={() => save({ match_meters: f.match_meters })} className="set-time" /></SettingRow>
      <div className="row"><button type="button" className="btn sm" onClick={scan} disabled={busy || (f.movement_via === '' && f.money_via === '')}><Icon name="refresh" size={14} />{busy ? 'Looking…' : 'Look now (last two weeks)'}</button></div>
    </Section>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="On the overview, “Where should we go?” sends what you asked and your places' names, kinds, cuisines, towns, prices, ratings, tags and notes (never your visits, the people you went with or what you spent) and picks up to three. Nothing is sent unless you press the button. Questions about your places go to Pocket AI through the Ask button at the top.">
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Places does not talk to ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own places, visits, lists and connections. Admins manage accounts; they do not see anyone else’s places.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their places, visits and lists." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or save a place from a Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /summary</code></dt><dd className="muted">How many places, what waits to be rated, the next plans — for Homepage or a morning Shortcut.</dd></div>
          <div><dt><code>POST /places</code></dt><dd className="muted"><code>{'{"name":"Kafeterija","kind":"cafe","city":"Sarajevo"}'}</code> — save a place (a Share-sheet Shortcut).</dd></div>
          <div><dt><code>POST /visits</code></dt><dd className="muted"><code>{'{"place_id":12,"rating":5,"spent":2400}'}</code> — log a visit; costs are in cents.</dd></div>
          <div><dt><code>GET /places?near=lat,lng&radius_km=1</code></dt><dd className="muted">Your places around a spot, nearest first.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">One day's visits and plans — what Pocket Day reads.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { toast } = useToast()
  const { user } = useApp()
  const [busy, setBusy] = useState(false)
  const done = (r: Record<string, number>) => { toast('Imported: ' + (Object.entries(r).map(([k, v]) => `${v} ${k}`).join(', ') || 'nothing new')); window.dispatchEvent(new CustomEvent('ppl:changed')) }
  const importFiles = async (files: FileList) => {
    setBusy(true)
    try {
      for (const file of Array.from(files)) {
        const text = await file.text()
        const name = file.name.replace(/\.(csv|json)$/i, '')
        if (/\.csv$/i.test(file.name)) {
          const res = await fetch(`${V1}/import/csv?list=${encodeURIComponent(name)}`, { method: 'POST', body: text, headers: { 'Content-Type': 'text/csv' }, credentials: 'same-origin' })
          const j = await res.json(); if (!res.ok) throw new Error(`${file.name}: ${j.error}`); done(j)
        } else {
          const res = await fetch(`${V1}/import${/saved places|reviews/i.test(name) ? '' : `?list=${encodeURIComponent(name)}`}`, { method: 'POST', body: text, headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin' })
          const j = await res.json(); if (!res.ok) throw new Error(`${file.name}: ${j.error}`); done(j)
        }
      }
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose">
      <Section title="From Google Maps" icon="upload" sub={<>Get your places from <a href="https://takeout.google.com" target="_blank" rel="noreferrer">Google Takeout</a>: choose <b>Maps (your places)</b> and <b>Saved</b>. Then pick the files here: <code>Saved Places.json</code> (your starred places), <code>Reviews.json</code> (the places you rated, with your stars and words) and the CSV of each saved list (<code>Want to go.csv</code>, <code>Favourites.csv</code>…), which become lists of the same name. Importing a file twice adds nothing.</>}>
        <label className="btn primary"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Choose files…'}<input type="file" multiple accept=".json,.csv,application/json,text/csv" hidden onChange={e => e.target.files?.length && importFiles(e.target.files)} /></label>
        <p className="muted small mt">Any other CSV works too when its first row names the columns: <code>name, kind, status, rating, price, cuisine, address, city, country, lat, lng, website, phone, tags, notes</code>.</p>
      </Section>
      <Section title="Export" icon="archive" sub="Everything of yours as one JSON file (places, visits, lists), or your places as a CSV that opens in a spreadsheet. A Pocket Places export goes back in with the same button above; links to Pocket Money, Calendar and Split stay behind.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Places as CSV</a></div>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-places.db</code> in the data directory.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Places', counts.places, 'pin'], ['Visits', counts.visits, 'journal'], ['Lists', counts.lists, 'list'], ['To rate', counts.inbox, 'star']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Places</h3>
          <p className="muted small">The places you love and the ones you want to try: restaurants, cafés, bars, museums and parks on a list and a map, with your ratings, notes, price and lists, every visit with who came and what it cost, and “how was it?” when Pocket Movement or Pocket Money notice you were there. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-places" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Movement, Money, Calendar, Travel, Split, Pocket AI and more. Maps © OpenStreetMap contributors.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}
