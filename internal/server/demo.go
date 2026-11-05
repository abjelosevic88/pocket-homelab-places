package server

import (
	"context"
	"math/rand"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

type demoPlace struct {
	key, name, kind, cuisine, city, country, address string
	lat, lng                                         float64
	price                                            int
	status                                           string
	fav                                              bool
	tags, notes, rec, website, hours                 string
	visits                                           int // how many visits over the last year
	rating                                           float64
}

// demoPlaces are real, well-known places (addresses and spots are close
// enough for a demo; nothing here is advice).
var demoPlaces = []demoPlace{
	{key: "zeljo", name: "Ćevabdžinica Željo", kind: "restaurant", cuisine: "Bosnian, ćevapi", city: "Sarajevo", country: "BA", address: "Kundurdžiluk 19", lat: 43.85972, lng: 18.43069, price: 1, status: "been", fav: true, tags: "local, quick lunch", notes: "Ten in somun with onion and kajmak. Cash only.", visits: 9, rating: 5},
	{key: "bosna", name: "Buregdžinica Bosna", kind: "street_food", cuisine: "Burek, pita", city: "Sarajevo", country: "BA", address: "Bravadžiluk 11", lat: 43.85948, lng: 18.43181, price: 1, status: "been", tags: "local, quick lunch", notes: "Krompiruša and a jogurt. Goes fast before noon.", visits: 5, rating: 4.5},
	{key: "inat", name: "Inat Kuća", kind: "restaurant", cuisine: "Bosnian", city: "Sarajevo", country: "BA", address: "Veliki Alifakovac 1", lat: 43.85781, lng: 18.43509, price: 2, status: "been", tags: "terrace, river, visitors", notes: "Take visitors here; the story of the house moved across the river. Begova čorba.", visits: 3, rating: 4},
	{key: "kibe", name: "Kibe Mahala", kind: "restaurant", cuisine: "Bosnian, grill", city: "Sarajevo", country: "BA", address: "Vrbanjuša 164", lat: 43.86292, lng: 18.44221, price: 3, status: "been", fav: true, tags: "view, date night, book ahead", notes: "Book the table by the window. Lamb under the sač.", visits: 2, rating: 5, hours: "12:00–23:00"},
	{key: "ribica", name: "Zlatna Ribica", kind: "bar", cuisine: "Wine, cocktails", city: "Sarajevo", country: "BA", address: "Kaptol 5", lat: 43.85902, lng: 18.42440, price: 2, status: "been", fav: true, tags: "cosy, late", notes: "Full of antiques; the menu comes in an old book.", visits: 6, rating: 4.5},
	{key: "kino", name: "Kino Bosna", kind: "bar", cuisine: "Beer, sevdah", city: "Sarajevo", country: "BA", address: "Alipašina 19", lat: 43.85688, lng: 18.41201, price: 1, status: "been", tags: "live music, late", notes: "Monday night sevdah. Go after ten.", visits: 2, rating: 4},
	{key: "tito", name: "Café Tito", kind: "cafe", cuisine: "Coffee, beer", city: "Sarajevo", country: "BA", address: "Zmaja od Bosne bb", lat: 43.85364, lng: 18.40083, price: 1, status: "been", tags: "garden, kids", notes: "Garden with old army jeeps; the kids love it.", visits: 4, rating: 4},
	{key: "cejf", name: "Ministry of Ćejf", kind: "cafe", cuisine: "Specialty coffee", city: "Sarajevo", country: "BA", address: "Kovači 26", lat: 43.86011, lng: 18.43003, price: 1, status: "been", fav: true, tags: "coffee, work", notes: "Flat white and a seat by the window. Wi-Fi works.", visits: 14, rating: 4.5},
	{key: "kafeterija", name: "Kafeterija", kind: "cafe", cuisine: "Specialty coffee", city: "Sarajevo", country: "BA", address: "Ćurčiluk veliki 8", lat: 43.85925, lng: 18.42838, price: 1, status: "been", fav: true, tags: "coffee", notes: "Their own roast; buy a bag of the Ethiopian.", visits: 11, rating: 5},
	{key: "dveri", name: "Dveri", kind: "restaurant", cuisine: "Bosnian, homestyle", city: "Sarajevo", country: "BA", address: "Prote Bakovića 12", lat: 43.85918, lng: 18.43019, price: 2, status: "been", tags: "cosy, date night", notes: "Small, book on Fridays. The sogan dolma.", visits: 2, rating: 4.5},
	{key: "klopa", name: "Klopa", kind: "restaurant", cuisine: "Modern Bosnian", city: "Sarajevo", country: "BA", address: "Gazi Husrev-begova 6", lat: 43.85966, lng: 18.42612, price: 2, status: "been", tags: "vegetarian, lunch", notes: "Changing lunch menu; good vegetarian choice.", visits: 3, rating: 4},
	{key: "ducan", name: "Baklava Ducan", kind: "dessert", cuisine: "Baklava, tufahija", city: "Sarajevo", country: "BA", address: "Ferhadija 3", lat: 43.85998, lng: 18.43103, price: 1, status: "been", tags: "kids, sweets", notes: "Tufahija with walnuts; take a box home.", visits: 4, rating: 4.5},
	{key: "ascinica", name: "Ašćinica Hadžibajrić", kind: "restaurant", cuisine: "Bosnian, cooked dishes", city: "Sarajevo", country: "BA", address: "Veliki Ćurčiluk 59", lat: 43.85993, lng: 18.43184, price: 1, status: "been", tags: "local, lunch", notes: "Point at what looks good. Closes at five.", visits: 2, rating: 4},
	{key: "zuta", name: "Žuta Tabija", kind: "viewpoint", city: "Sarajevo", country: "BA", address: "Jekovac", lat: 43.86133, lng: 18.43778, status: "been", fav: true, tags: "view, sunset, kids", notes: "Sunset and the iftar cannon in Ramadan.", visits: 3, rating: 5},
	{key: "vrelo", name: "Vrelo Bosne", kind: "park", city: "Ilidža", country: "BA", address: "Vrelo Bosne, Ilidža", lat: 43.81870, lng: 18.26900, status: "been", tags: "kids, walk, nature", notes: "Walk the alley or take the fijaker; feed the swans.", visits: 2, rating: 4.5},
	{key: "museum", name: "National Museum of Bosnia and Herzegovina", kind: "museum", city: "Sarajevo", country: "BA", address: "Zmaja od Bosne 3", lat: 43.85514, lng: 18.40253, status: "been", tags: "rainy day", notes: "The Haggadah is only shown sometimes; ask at the desk.", visits: 1, rating: 4},
	{key: "childhood", name: "War Childhood Museum", kind: "museum", city: "Sarajevo", country: "BA", address: "Logavina 32", lat: 43.86125, lng: 18.42762, status: "want", tags: "rainy day", notes: "Everyone says give it two hours.", rec: "Lejla"},
	{key: "pivara", name: "Sarajevska Pivara", kind: "bar", cuisine: "Beer hall", city: "Sarajevo", country: "BA", address: "Franjevačka 15", lat: 43.85765, lng: 18.43502, price: 2, status: "been", tags: "groups, beer", visits: 2, rating: 3.5},
	{key: "trebevic", name: "Trebević cable car", kind: "activity", city: "Sarajevo", country: "BA", address: "Hrvatin bb", lat: 43.85648, lng: 18.43627, status: "been", tags: "kids, view", notes: "Go up, walk to the bobsled track, come down before dark.", visits: 2, rating: 4.5},
	{key: "lukomir", name: "Lukomir", kind: "nature", city: "Konjic", country: "BA", address: "Lukomir village, Bjelašnica", lat: 43.66301, lng: 18.19361, status: "want", tags: "day trip, hike", notes: "Highest village in the country. Road open May to October.", rec: "Mirza"},
	{key: "markale", name: "Markale", kind: "market", city: "Sarajevo", country: "BA", address: "Mula Mustafe Bašeskije", lat: 43.85861, lng: 18.42243, status: "been", tags: "fruit, local", visits: 3, rating: 4},
	{key: "zara", name: "Zara iz Duvara", kind: "restaurant", cuisine: "Bosnian, home cooking", city: "Sarajevo", country: "BA", address: "Kovači 17", lat: 43.86124, lng: 18.43108, price: 2, status: "want", tags: "date night", notes: "Small and cosy; call before.", rec: "Ana"},
	{key: "princeva", name: "Park Prinčeva", kind: "restaurant", cuisine: "Grill, international", city: "Sarajevo", country: "BA", address: "Iza Hrida 7", lat: 43.86158, lng: 18.43348, price: 3, status: "want", tags: "view, date night", notes: "Best view of the city at night."},
	{key: "avlija", name: "Avlija", kind: "restaurant", cuisine: "Bosnian, modern", city: "Sarajevo", country: "BA", address: "Avde Sumbula 2", lat: 43.86356, lng: 18.41619, price: 2, status: "want", tags: "garden, kids", rec: "Ana"},
	{key: "europe", name: "Hotel Europe", kind: "hotel", city: "Sarajevo", country: "BA", address: "Vladislava Skarića 5", lat: 43.85862, lng: 18.42471, price: 3, status: "been", tags: "visitors", notes: "Where family stays when they visit. The café downstairs is nice too.", visits: 1, rating: 4},
	{key: "most", name: "Stari Most", kind: "sight", city: "Mostar", country: "BA", address: "Stari Most", lat: 43.33729, lng: 17.81501, status: "been", tags: "day trip", notes: "Go early; by noon it's full.", visits: 1, rating: 5},
	{key: "sadrvan", name: "Šadrvan", kind: "restaurant", cuisine: "Herzegovinian", city: "Mostar", country: "BA", address: "Jusovina 11", lat: 43.33761, lng: 17.81462, price: 2, status: "been", tags: "day trip, terrace", visits: 1, rating: 4},
	{key: "kravica", name: "Kravica waterfall", kind: "nature", city: "Ljubuški", country: "BA", address: "Kravica", lat: 43.15687, lng: 17.60800, status: "want", tags: "day trip, swim, kids", notes: "Swim in summer; take water shoes."},
	{key: "central", name: "Café Central", kind: "cafe", cuisine: "Viennese café", city: "Vienna", country: "AT", address: "Herrengasse 14", lat: 48.21040, lng: 16.36543, price: 3, status: "been", tags: "classic", notes: "Queue in the morning; the Melange and an apple strudel.", visits: 1, rating: 4, website: "https://cafecentral.wien"},
	{key: "figl", name: "Figlmüller", kind: "restaurant", cuisine: "Austrian, schnitzel", city: "Vienna", country: "AT", address: "Wollzeile 5", lat: 48.20870, lng: 16.37469, price: 3, status: "been", fav: true, tags: "book ahead", notes: "Schnitzel bigger than the plate; potato salad.", visits: 1, rating: 4.5},
	{key: "nasch", name: "Naschmarkt", kind: "market", city: "Vienna", country: "AT", address: "Naschmarkt", lat: 48.19862, lng: 16.36343, status: "been", visits: 1, rating: 4},
	{key: "albertina", name: "Albertina", kind: "museum", city: "Vienna", country: "AT", address: "Albertinaplatz 1", lat: 48.20447, lng: 16.36827, status: "want", tags: "art"},
	{key: "timeout", name: "Time Out Market", kind: "market", cuisine: "Food hall", city: "Lisbon", country: "PT", address: "Av. 24 de Julho 49", lat: 38.70706, lng: -9.14582, price: 2, status: "want", tags: "groups"},
	{key: "belem", name: "Pastéis de Belém", kind: "bakery", cuisine: "Pastel de nata", city: "Lisbon", country: "PT", address: "R. de Belém 84", lat: 38.69750, lng: -9.20322, price: 1, status: "want", tags: "sweets", notes: "Eat them warm with cinnamon.", rec: "Mirza"},
	{key: "monte", name: "Miradouro da Senhora do Monte", kind: "viewpoint", city: "Lisbon", country: "PT", address: "Largo Monte", lat: 38.71929, lng: -9.13271, status: "want", tags: "view, sunset"},
	{key: "ramiro", name: "Cervejaria Ramiro", kind: "restaurant", cuisine: "Seafood", city: "Lisbon", country: "PT", address: "Av. Almirante Reis 1", lat: 38.72058, lng: -9.13546, price: 3, status: "want", tags: "seafood", notes: "Prawns, then a steak sandwich for dessert. No bookings; queue."},
	{key: "lxf", name: "LX Factory", kind: "shop", city: "Lisbon", country: "PT", address: "R. Rodrigues de Faria 103", lat: 38.70370, lng: -9.17840, status: "want", tags: "books, Sunday"},
	{key: "flores", name: "Taberna da Rua das Flores", kind: "restaurant", cuisine: "Portuguese petiscos", city: "Lisbon", country: "PT", address: "R. das Flores 103", lat: 38.71051, lng: -9.14398, price: 2, status: "want", tags: "small plates"},
}

// seedDemo fills the owner's account with places in Sarajevo, Mostar,
// Vienna and a coming trip to Lisbon, a year of visits with what they cost,
// plans, lists and two "how was it?" (DEMO_DATA, for `make run`, the CI
// smoke test and the screenshots). Dates are relative to today, so the
// demo always looks current.
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	// with AUTH_MODE=none this runs inside the first request; finish even
	// when that client goes away
	ctx = context.WithoutCancel(ctx)
	if ps, _ := s.store.ListPlaces(ctx, uid, store.PlaceFilter{Limit: 1}); len(ps) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	today := store.Today()
	day := func(n int) string { return store.AddDays(today, n) }
	ids := map[string]int64{}
	company := []string{"", "", "Ana", "Ana", "Ana, Mirza", "Lejla", "the kids", "Ana and the kids", "work"}
	for i, d := range demoPlaces {
		lat, lng := d.lat, d.lng
		p := store.Place{Name: d.name, Kind: d.kind, Cuisine: d.cuisine, City: d.city, Country: d.country, Address: d.address, Lat: &lat, Lng: &lng,
			Price: d.price, Status: d.status, Favourite: d.fav, Tags: d.tags, Notes: d.notes, RecommendedBy: d.rec, Website: d.website, Hours: d.hours, Rating: d.rating}
		if d.country == "AT" || d.country == "PT" {
			p.Currency = "EUR"
		} else {
			p.Currency = "BAM"
		}
		if err := s.store.CreatePlace(ctx, uid, &p); err != nil {
			return err
		}
		// spread "added" over the last year, newest last
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE places SET created_at = ? WHERE id = ?`, day(-360+i*9)+"T10:00:00.000Z", p.ID)
		ids[d.key] = p.ID
		if d.visits == 0 {
			continue
		}
		base := map[string]int64{"cafe": 650, "dessert": 1200, "street_food": 1100, "bar": 2600, "restaurant": 5200, "market": 2400, "hotel": 0, "museum": 1500, "activity": 2000}[d.kind]
		for n := 0; n < d.visits; n++ {
			back := 8 + rng.Intn(330)
			if d.country != "BA" {
				back = 150 + rng.Intn(3) // the Vienna weekend
			}
			if d.key == "most" || d.key == "sadrvan" {
				back = 96
			}
			v := store.Visit{PlaceID: p.ID, Date: day(-back), Rating: d.rating, Currency: p.Currency}
			if n > 0 && rng.Intn(3) == 0 {
				v.Rating = clampRating(d.rating - 0.5)
			}
			if base > 0 {
				v.Spent = base + int64(rng.Intn(int(base)))
				v.Spent -= v.Spent % 50
			}
			if d.kind == "restaurant" || d.kind == "bar" {
				v.Time = []string{"13:15", "19:30", "20:00", "21:00"}[rng.Intn(4)]
				v.WithWhom = company[rng.Intn(len(company))]
			} else if d.kind == "cafe" {
				v.Time = []string{"08:40", "10:15", "16:30"}[rng.Intn(3)]
				v.Minutes = 30 + rng.Intn(60)
			}
			if n == 0 && d.notes != "" && rng.Intn(2) == 0 {
				v.Notes = strings.SplitN(d.notes, ".", 2)[0] + "."
			}
			if err := s.store.CreateVisit(ctx, uid, &v); err != nil {
				return err
			}
		}
	}
	// a plan in three days, one whose day has passed (did you go?), one later
	for _, v := range []store.Visit{
		{PlaceID: ids["kibe"], Date: day(3), Time: "19:30", Planned: true, WithWhom: "Ana", Notes: "Anniversary. Table by the window.", Currency: "BAM"},
		{PlaceID: ids["zara"], Date: day(-2), Time: "20:00", Planned: true, WithWhom: "Ana, Mirza", Currency: "BAM"},
		{PlaceID: ids["childhood"], Date: day(9), Planned: true, WithWhom: "the kids", Currency: "BAM"},
	} {
		if err := s.store.CreateVisit(ctx, uid, &v); err != nil {
			return err
		}
	}

	lists := []struct {
		name, emoji, desc, trip string
		keys                    []string
	}{
		{"Coffee in Sarajevo", "☕", "Where the coffee is good and you can sit a while.", "", []string{"cejf", "kafeterija", "tito", "ribica"}},
		{"Date night", "🕯️", "For the two of us.", "", []string{"kibe", "princeva", "dveri", "zara", "ribica"}},
		{"With the kids", "🧒", "Room to run, something sweet, nothing too long.", "", []string{"vrelo", "trebevic", "ducan", "childhood", "tito", "zuta"}},
		{"Day trips", "🚗", "An hour or two from Sarajevo.", "", []string{"lukomir", "kravica", "most", "sadrvan"}},
		{"Lisbon in May", "🇵🇹", "Mirza's tips and what we read.", "Lisbon", []string{"timeout", "belem", "monte", "ramiro", "lxf", "flores"}},
	}
	for i, l := range lists {
		lst := store.List{Name: l.name, Emoji: l.emoji, Description: l.desc, TripName: l.trip, Sort: i}
		if err := s.store.CreateList(ctx, uid, &lst); err != nil {
			return err
		}
		for _, k := range l.keys {
			note := ""
			if k == "belem" {
				note = "Go before 10 to skip the queue"
			}
			if err := s.store.AddToList(ctx, uid, lst.ID, ids[k], note); err != nil {
				return err
			}
		}
	}

	// "how was it?": a stay seen by Pocket Movement with its payment, and a payment alone
	mv, tx1, tx2 := int64(48211), int64(90412), int64(90388)
	for _, it := range []store.InboxItem{
		{PlaceID: ids["kafeterija"], Date: day(-1), Time: "09:05", Minutes: 48, MovementID: &mv},
		{PlaceID: ids["kafeterija"], Date: day(-1), Time: "09:07", Amount: 950, Currency: "BAM", MoneyTxID: &tx1, Payee: "KAFETERIJA SARAJEVO"},
		{PlaceID: ids["klopa"], Date: day(-3), Time: "13:40", Amount: 6400, Currency: "BAM", MoneyTxID: &tx2, Payee: "KLOPA DOO"},
	} {
		if _, err := s.store.NoteInbox(ctx, uid, it); err != nil {
			return err
		}
	}
	_, _ = s.store.DB.ExecContext(ctx, `UPDATE inbox SET notified = 1 WHERE user_id = ?`, uid)

	st, _ := s.store.GetSettings(ctx, uid)
	st.Currency = "BAM"
	return s.store.SaveUserSettings(ctx, uid, st)
}

func clampRating(r float64) float64 {
	if r < 1 {
		return 1
	}
	return r
}
