package store

import "strings"

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Place is somewhere worth remembering: a restaurant, a café, a museum.
type Place struct {
	ID            int64    `json:"id"`
	UserID        int64    `json:"-"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Cuisine       string   `json:"cuisine"`
	Status        string   `json:"status"` // want | been
	Favourite     bool     `json:"favourite"`
	Closed        bool     `json:"closed"`
	Rating        float64  `json:"rating"`
	Price         int      `json:"price"`
	Address       string   `json:"address"`
	City          string   `json:"city"`
	Country       string   `json:"country"`
	Lat           *float64 `json:"lat"`
	Lng           *float64 `json:"lng"`
	Phone         string   `json:"phone"`
	Website       string   `json:"website"`
	MapURL        string   `json:"map_url"`
	Hours         string   `json:"hours"`
	Tags          string   `json:"tags"`
	Notes         string   `json:"notes"`
	RecommendedBy string   `json:"recommended_by"`
	ContactID     *int64   `json:"contact_id"`
	Currency      string   `json:"currency"`
	Source        string   `json:"source"`
	SourceRef     string   `json:"source_ref"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`

	// Filled by listings.
	Visits    int      `json:"visits"`
	LastVisit string   `json:"last_visit"`
	NextPlan  string   `json:"next_plan"`
	Spent     int64    `json:"spent"`
	Lists     []int64  `json:"lists"`
	Distance  *float64 `json:"distance_m,omitempty"`
}

// HasCoords reports whether the place is on the map.
func (p Place) HasCoords() bool { return p.Lat != nil && p.Lng != nil }

// Visit is one time you went, or a plan to go.
type Visit struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"-"`
	PlaceID        int64   `json:"place_id"`
	Date           string  `json:"date"`
	Time           string  `json:"time"`
	Planned        bool    `json:"planned"`
	Rating         float64 `json:"rating"`
	Notes          string  `json:"notes"`
	WithWhom       string  `json:"with_whom"`
	Minutes        int     `json:"minutes"`
	Spent          int64   `json:"spent"`
	Currency       string  `json:"currency"`
	MoneyTxID      *int64  `json:"money_tx_id"`
	MoneyBooked    bool    `json:"money_booked"`
	SplitGroupID   *int64  `json:"split_group_id"`
	SplitExpenseID *int64  `json:"split_expense_id"`
	CalEventID     *int64  `json:"cal_event_id"`
	TripID         *int64  `json:"trip_id"`
	TripName       string  `json:"trip_name"`
	Source         string  `json:"source"`
	SourceRef      string  `json:"source_ref"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`

	PlaceName string `json:"place_name"`
	PlaceKind string `json:"place_kind"`
	City      string `json:"city"`
}

// List is a person's own collection of places.
type List struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"-"`
	Name        string `json:"name"`
	Emoji       string `json:"emoji"`
	Description string `json:"description"`
	TripID      *int64 `json:"trip_id"`
	TripName    string `json:"trip_name"`
	Sort        int    `json:"sort"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`

	Count int `json:"count"`
	Been  int `json:"been"`
}

// ListEntry is a place on a list, with the note kept there.
type ListEntry struct {
	ListID  int64  `json:"list_id"`
	PlaceID int64  `json:"place_id"`
	Note    string `json:"note"`
	AddedAt string `json:"added_at"`
}

// InboxItem is "you were there — how was it?": a stay from Pocket Movement
// and/or a payment in Pocket Money at one of your places on one day.
type InboxItem struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	PlaceID    int64  `json:"place_id"`
	Date       string `json:"date"`
	Time       string `json:"time"`
	Minutes    int    `json:"minutes"`
	MovementID *int64 `json:"movement_id"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	MoneyTxID  *int64 `json:"money_tx_id"`
	Payee      string `json:"payee"`
	Status     string `json:"status"` // open | done | dismissed
	Notified   bool   `json:"-"`
	CreatedAt  string `json:"created_at"`

	PlaceName string `json:"place_name"`
	PlaceKind string `json:"place_kind"`
	City      string `json:"city"`
}

// Conn is how a person reaches one sibling Pocket app: "core" (an app link
// through Pocket Core), "direct" (URL + token) or "off"; "" = not chosen yet.
type Conn struct {
	Mode      string `json:"mode"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	PublicURL string `json:"public_url"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat   string `json:"date_format"`
	TimeFormat   string `json:"time_format"`
	WeekStart    int    `json:"week_start"` // 1 = Monday, 7 = Sunday
	Currency     string `json:"currency"`
	DistanceUnit string `json:"distance_unit"` // km | mi
	Online       bool   `json:"online"`        // look addresses up on OpenStreetMap, read map links

	// Notifications.
	NotifyToRate  bool   `json:"notify_to_rate"` // you were at one of your places: how was it?
	NotifyPlanned bool   `json:"notify_planned"` // the morning of a planned visit
	NotifyTrips   bool   `json:"notify_trips"`   // a trip in Pocket Travel: the places on your list there
	NotifyHour    int    `json:"notify_hour"`
	NotifyURL     string `json:"notify_url"`
	NotifyFormat  string `json:"notify_format"` // apprise | ntfy | json

	// Noticing visits: Pocket Movement's stays and Pocket Money's payments
	// at your places become "how was it?" in the inbox.
	MatchVisits   bool `json:"match_visits"`
	MatchPayments bool `json:"match_payments"`
	MatchMinutes  int  `json:"match_minutes"` // a stay this long counts as a visit
	MatchMeters   int  `json:"match_meters"`  // this close to the place

	// Pocket Money: what you spent on a visit.
	MoneyBook       bool  `json:"money_book"` // tick "Create a transaction" by default
	MoneyAccountID  int64 `json:"money_account_id"`
	MoneyCategoryID int64 `json:"money_category_id"`

	// Pocket Calendar: planned visits as events.
	CalendarPlans bool  `json:"calendar_plans"`
	CalendarID    int64 `json:"calendar_id"` // 0 = the person's default calendar

	// Pocket Split: the group a bill is split in by default.
	SplitGroupID int64 `json:"split_group_id"`

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI for "where should we go?": Pocket AI through Core, or an
	// OpenAI-compatible API with the person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// SplitTags turns "a, b ,c" into clean lowercase tags.
func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// JoinTags is the stored form of tags.
func JoinTags(s string) string { return strings.Join(SplitTags(s), ", ") }
