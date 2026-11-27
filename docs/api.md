# API

JSON over HTTP at `/api/v1`. Authenticate with the session cookie, a personal token (`Authorization: Bearer ppl_…`, Settings → API & shortcuts), a Pocket Core device or user token, or a Pocket Core app-link token. Everything is per person: another person's ids answer 404. Money is in **minor units** (cents); dates are `YYYY-MM-DD`, times `HH:MM`. PUT bodies are partial: only the keys you send change.

## Overview

| Route | What |
|---|---|
| `GET /overview` | Counts (places, want, been, favourites, lists, to rate), the inbox, plans whose day passed, coming plans, recent visits, favourites, places waiting for you, trips from Pocket Travel with the places there, visits and spending this year, top towns, lists |
| `GET /summary` | The short version for dashboards and Pocket AI: counts, `to_rate`, the next plans, visits and spending this month, the last visit |
| `GET /day/{date}` | `{visits, planned, added, spent}` — one day with places, what Pocket Day writes into the journal |
| `GET /search?q&limit` | `{places, visits, lists}` (accents don't matter: "cevapi" finds "ćevapi") |
| `GET /spending?from&to` | What visits cost per currency and per month |
| `GET /trips?days=365&back=0` | Pocket Travel trips with `want`, `been` (your places at the destination: within 40 km of its spot or in a town named in it), `visits` and the lists made for it |
| `GET /trip-guide?trip_id&lat&lng&destination&from&to` | For one trip: `{lists, want, been, visits, spent}` — what Pocket Travel shows on a trip's page |

## Places

| Route | What |
|---|---|
| `GET /places?status&kind&group&city&country&tag&q&list_id&favourite=1&open=1&sort&near=lat,lng&radius_km&limit` | `status` want or been; `group` eat, see, shop, stay, other; `sort` name, rating, recent, added, visits, near (with `near`, each place has `distance_m`). Each place has `visits`, `last_visit`, `next_plan`, `spent`, `lists` |
| `POST /places` | `name`, `kind` (`GET /kinds`), `cuisine`, `status`, `favourite`, `closed`, `rating` 0–5, `price` 0–4, `address`, `city`, `country` (ISO code), `lat`, `lng`, `phone`, `website`, `map_url`, `hours`, `tags` ("a, b"), `notes`, `recommended_by`, `contact_id`, `currency`, `list_ids`; from a linked app also `source_ref` (sending the same one twice returns the first) |
| `GET /places/{id}` | `{place, visits, lists, list_notes, nearby, inbox, spent, movement}` (`movement`: how often Pocket Movement saw you there in two years) |
| `PUT/DELETE /places/{id}` | Deleting removes its visits; transactions Places booked go from Pocket Money and planned events from Pocket Calendar |
| `POST /places/{id}/lists {list_id, note}`, `DELETE /places/{id}/lists/{list_id}` | Put on / take off a list |
| `GET /kinds`, `GET /cities`, `GET /tags` | What there is to filter by |
| `GET /lookup?q&near=lat,lng` | Look a name up on OpenStreetMap: `{items: [{name, kind, cuisine, address, city, country, lat, lng, website, phone, hours, map_url, label}]}` (only with the person's "Look places up online" on) |
| `GET /reverse?at=lat,lng` | The address at a spot |
| `POST /resolve {text}` | Read a pasted Google Maps, Apple Maps or OpenStreetMap link (short links are followed) or "lat, lng" |

## Visits and plans

| Route | What |
|---|---|
| `GET /visits?place_id&from&to&planned&trip_id&q&limit&offset` | Newest first: `{items, total, spent}` |
| `POST /visits` | `place_id`, `date`, `time`, `planned` (default: true for a day in the future), `rating`, `notes`, `with_whom`, `minutes`, `spent`, `currency`, `trip_id`; `money: true` books the cost in Pocket Money (`account_id`, `category_id`), `money_tx_id` links a payment already there, `calendar: true` puts a plan on Pocket Calendar (default: the setting). A visit during a Pocket Travel trip is filed with it. Answers `{visit, money?, calendar?}` |
| `GET/PUT/DELETE /visits/{id}` | A new date or cost follows to the transaction Places booked; a plan's event moves with it |
| `POST /visits/{id}/money` | `{}` books it, `{tx_id}` links a payment, `{unlink: true}` forgets it (a transaction Places booked is removed) |
| `GET /visits/{id}/payments` | Pocket Money payments around the day, the likely ones first |
| `POST /visits/{id}/calendar` | Put a plan on the calendar now |
| `POST /visits/{id}/split {group_id, member_ids, amount}` | Add the bill to a Pocket Split group, paid by you, shared evenly |

## Lists

`GET/POST /lists` (`name`, `emoji`, `description`, `trip_id`, `trip_name`, `sort`, `place_ids`), `GET/PUT/DELETE /lists/{id}` (GET: `{list, places, notes}`, with the place filters), `POST /lists/{id}/places {place_id, note}`, `DELETE /lists/{id}/places/{place_id}`.

## "How was it?"

| Route | What |
|---|---|
| `GET /inbox` | `{items, past_plans}`: stays from Pocket Movement and payments from Pocket Money at your places (one per place and day: `minutes`, `amount`, `currency`, `money_tx_id`, `payee`), and plans whose day passed |
| `POST /inbox/{id}/accept {rating, notes, with_whom, spent}` | The visit, with the time, how long and the payment linked |
| `POST /inbox/{id}/dismiss` | Never asked again |
| `POST /inbox/scan` | Look at the last two weeks now: `{added, stays, payments, errors}` |

## AI

`GET /ai/status`, `GET /ai/models`, `POST /ai/pick {prompt, near: "lat,lng", city, list_id, status, kinds}` → `{ok, picks: [{place, why}], note}` — up to three of the person's own places.

## The other Pocket apps

`GET /links/money/accounts`, `/links/money/categories?account_id`, `/links/calendar/calendars`, `/links/contacts?q`, `/links/split/groups` answer `{connected, detail, items}` from the person's own connection.

## Pocket Core app links

Places offers `places:read` (overview, summary, day, search, spending, trips, trip guide, places, visits, lists, inbox, cities, tags, kinds) and `places:write` (`POST /places`, `POST /visits`, and `PUT/DELETE` of the places and visits that app added). Everything else answers 403 to a link token.

It uses Pocket Movement (`movement:read`), Pocket Money (`transactions:read`, `accounts:read`, `transactions:write`), Pocket Calendar (`calendar:read`, `calendar:write`), Pocket Contacts (`contacts:read`), Pocket Travel (`trips:read`), Pocket Split (`groups:read`, `expenses:write`) and Pocket AI inference. Events, each with `user_email`: `places.place_saved` (subject `place:<id>`; `place_id`, `name`, `kind`, `status`, `city`, `country`, `rating`, `favourite`, `lat`, `lng`), `places.visit_logged` and `places.visit_planned` (subject `visit:<id>`; `visit_id`, `place_id`, `place`, `date`, `time`, `rating`, `spent`, `currency`, `with`), `places.to_rate` (subject `inbox:<id>`) and `places.list_saved` (subject `list:<id>`).

## Pocket Search, Pocket Day and Pocket Hub

Places answers the four routes every Pocket app can offer (`docs/capabilities.md` in Pocket Core), so Pocket Search, Pocket Day and Pocket Hub find it on their own. Each has its own link scope that opens that one route and nothing else (`places:read` and `places:write` do not open them); a signed-in person or a personal token can call them too. They see only the person's own places, visits and lists. Ids are `place:<id>`, `visit:<id>` or `list:<id>`; addresses (`url`) are paths in the app (`/places/12`, `/places/12?visit=40`, `/lists/3`).

| Route | Scope | What |
|---|---|---|
| `GET /pocket/search?q&limit` | `pocket:search` | `{items}`: your places with every word of `q` in the name, kind, cuisine, town, address, tags, notes or who recommended it (as `GET /places?q=`, accents don't matter), your lists by name and description, and your visits by their notes. A place whose name has the words first, then other places, lists, visits. A place's `snippet` is kind · cuisine · town · "Want to try" or "Been there, 4.5 stars", its `date` the last visit and its `link` the website; a visit's title is like "Dinner at Pirpa". Words without a letter or digit find nothing. `limit` 1–50 (8) |
| `GET /pocket/day/{date}?tz=` | `pocket:day` | `{date, summary, items, metrics}`: what `GET /day/{date}` has, in the common shape: the visits (`kind` `visit`; `time`, `minutes`, `sub` town · with whom · rating, `amount` + `currency` what it cost, `note`), the visits planned for the day (`kind` `planned`) and the places saved that day (`kind` `added`; imports left out, a place with a visit that day shows once). "Saved that day" is in `tz`, else the server's time zone. `summary` is like "Dinner at Pirpa, 4.5 stars", "2 visits; 1 place saved"; `metrics` are Visits and Spent (one currency) |
| `GET /pocket/widget` | `pocket:widget` | `{line, metrics, items, url}`: the plans coming up first (at most 4, `detail` "Friday 19:30 · with Ana"), then the newest places you want to try, 6 rows in all. `line` is the next plan ("Dinner at Pirpa on Friday"), else "12 places to try"; `metrics` are Planned and To try. Nothing planned and nothing to try: empty |
| `POST /pocket/capture` | `pocket:capture` | `{request_id, kind: "place", title, text}` adds a place you want to try, as `POST /places` with a name alone (a restaurant, nothing looked up), and answers `201 {id, title, url}`. The name is `title`, else the first line of `text`; the rest becomes the notes. A name you already have (however it is written) answers `400`. The place has no `source`, so the app that sent the line cannot change or remove it later. The same `request_id` again answers the same place and adds nothing; with other words it answers `400` |

## Settings, data

`GET/PUT /settings`, `GET /settings/system`, `POST /integrations/test {which}`, `POST /notify/test`, `POST /notify/check` (send the morning message now), `GET/POST/DELETE /tokens`, `GET /backup` (admin), `GET /export.json`, `GET /export.csv`, `POST /import` (a Pocket Places export, or Google Takeout's Saved Places / Reviews; `?list=Name` puts them on a list), `POST /import/csv?list=Name` (a Google Maps saved list, or any CSV with a header row). Both imports also take the multipart field `file` with an optional form field `list` (Pocket Move): Google GeoJSON then lands on "Google Maps" when no list is named, a CSV on the list named after its file. `GET /pocket/portable` describes the exports and imports for `move:export` / `move:import` links. Health: `GET /healthz`, `/readyz`, `/metrics`.
