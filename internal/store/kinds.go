package store

import (
	"math"
	"strings"
	"unicode"
)

// Kind is what sort of place it is. The web app has the same list
// (web/src/lib/places.ts) with icons.
type Kind struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Group string `json:"group"` // eat | shop | see | stay | other
}

// Kinds are every kind of place, in the order the forms show them.
var Kinds = []Kind{
	{"restaurant", "Restaurant", "eat"},
	{"cafe", "Café", "eat"},
	{"bar", "Bar or pub", "eat"},
	{"bakery", "Bakery", "eat"},
	{"street_food", "Street food and takeaway", "eat"},
	{"dessert", "Dessert and ice cream", "eat"},
	{"market", "Market", "shop"},
	{"shop", "Shop", "shop"},
	{"museum", "Museum or gallery", "see"},
	{"sight", "Sight or landmark", "see"},
	{"park", "Park or garden", "see"},
	{"viewpoint", "Viewpoint", "see"},
	{"beach", "Beach or lake", "see"},
	{"nature", "Hike or nature", "see"},
	{"activity", "Something to do", "see"},
	{"hotel", "Hotel or stay", "stay"},
	{"other", "Other", "other"},
}

// ValidKind reports whether k is a known kind.
func ValidKind(k string) bool {
	for _, x := range Kinds {
		if x.Value == k {
			return true
		}
	}
	return false
}

// KindLabel is the plain-language name of a kind.
func KindLabel(k string) string {
	for _, x := range Kinds {
		if x.Value == k {
			return x.Label
		}
	}
	return "Place"
}

// KindGroup is the group a kind belongs to.
func KindGroup(k string) string {
	for _, x := range Kinds {
		if x.Value == k {
			return x.Group
		}
	}
	return "other"
}

// Place statuses.
const (
	StatusWant = "want"
	StatusBeen = "been"
)

// Inbox states.
const (
	InboxOpen      = "open"
	InboxDone      = "done"
	InboxDismissed = "dismissed"
)

// Meters is the distance between two points on the Earth, in metres.
func Meters(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

// accents maps the letters with marks found in European place names to
// their plain form (no golang.org/x/text: one dependency less).
var accents = func() *strings.Replacer {
	pairs := []string{}
	for plain, marked := range map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđ", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ", "i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ",
		"l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő", "r": "ŕŗř", "s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
	} {
		for _, r := range marked {
			pairs = append(pairs, string(r), plain)
		}
	}
	pairs = append(pairs, "ß", "ss", "æ", "ae", "œ", "oe", "þ", "th")
	return strings.NewReplacer(pairs...)
}()

// Fold lowercases s and strips accents and punctuation, for matching names:
// "Caffè Tito's" → "caffe titos", "Ćevabdžinica Željo" → "cevabdzinica zeljo".
func Fold(s string) string {
	out := accents.Replace(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range out {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case r == '\'' || r == '’':
		default:
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// generic words that say nothing about which place it is
var nameNoise = map[string]bool{"the": true, "a": true, "restaurant": true, "restoran": true, "cafe": true, "caffe": true, "kafana": true, "bar": true,
	"pub": true, "and": true, "i": true, "de": true, "la": true, "le": true, "il": true, "el": true, "da": true, "d": true, "s": true, "shop": true, "doo": true, "d o o": true,
	"pizzeria": true, "bistro": true, "grill": true, "pekara": true, "bakery": true, "gmbh": true, "ltd": true, "sa": true, "srl": true}

// NameMatches reports whether a payee or a place name from another app is
// this place: the place's distinctive words all appear in it (at least one
// word of four letters or more), e.g. "CAFFE TITO SARAJEVO 0123" is Café Tito.
func NameMatches(place, other string) bool {
	p, o := Fold(place), " "+Fold(other)+" "
	if p == "" || len(o) < 4 {
		return false
	}
	words := 0
	long := false
	for _, w := range strings.Fields(p) {
		if nameNoise[w] {
			continue
		}
		words++
		if len([]rune(w)) >= 4 {
			long = true
		}
		if !strings.Contains(o, " "+w+" ") {
			return false
		}
	}
	return words > 0 && long
}
