package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-places/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// aiFor is the person's AI: Pocket AI through Core, their own
// OpenAI-compatible key, or nil with why.
func (s *Server) aiFor(ctx context.Context, uid int64, st store.Settings) (*ai.Client, string, string) {
	email := ""
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		email = u.Email
	}
	pocket, status := s.core.SelectAI(ctx, email, st.AIProvider, st.OpenAIKey != "")
	if pocket != nil {
		if !status.Ready {
			return nil, "", "Pocket AI: " + status.Detail
		}
		c := ai.New("", "", "", 150*time.Second)
		c.Pocket = pocket.WithModel(st.AIPocketModel)
		return c, "pocket", ""
	}
	if st.OpenAIKey == "" {
		return nil, "", status.Detail
	}
	return ai.New(st.OpenAIBaseURL, st.OpenAIKey, st.OpenAIModel, 150*time.Second), "openai", ""
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	cl, used, why := s.aiFor(ctx, actorID(ctx), st)
	writeJSON(w, http.StatusOK, map[string]any{"ready": cl != nil, "provider": used, "detail": why})
}

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, status := s.core.SelectAI(ctx, userFrom(ctx).Email, "pocket", false)
	if c == nil || !status.Ready {
		writeErr(w, http.StatusServiceUnavailable, status.Detail)
		return
	}
	models, err := c.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func validAIProvider(v string) bool { return core.ValidAIProvider(v) }

func validPocketModel(v string) bool { return core.ValidInferenceModel(v) }

const aiVoice = `You help one person choose where to go from their own saved places (restaurants, cafés, bars, museums, parks and so on). You only ever pick from the list you are given, by its id; never invent a place. Weigh what they ask for (mood, cuisine, price, company, time of day, how far) against each place's kind, cuisine, price, rating, tags, notes and distance. Prefer places they want to try when that fits; a favourite is a safe choice. Leave out places closed for good. Write each reason in one plain sentence, in the language of the request, without emojis or exclamation marks.`

// pickInput is "where should we go?".
type pickInput struct {
	Prompt string   `json:"prompt"`
	Near   string   `json:"near"` // "lat,lng" of the person right now
	City   string   `json:"city"`
	ListID int64    `json:"list_id"`
	Status string   `json:"status"` // want | been | ""
	Kinds  []string `json:"kinds"`
}

// handlePick asks the AI to pick up to three of the person's own places for
// what they ask ("a quiet dinner for two, not too dear"). Only the places'
// own fields go to the AI, never visits, people or money.
func (s *Server) handlePick(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in pickInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Prompt == "" || len(in.Prompt) > 600 {
		writeErr(w, http.StatusBadRequest, "say what you feel like, in a sentence")
		return
	}
	f := store.PlaceFilter{OpenOnly: true, City: in.City, ListID: in.ListID, Status: in.Status, Sort: "rating"}
	if lat, lng, ok := latLng(in.Near); ok {
		f.Near, f.Sort = &[2]float64{lat, lng}, "near"
	}
	places, err := s.store.ListPlaces(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "places")
		return
	}
	if len(in.Kinds) > 0 {
		keep := places[:0]
		for _, p := range places {
			if oneOf(p.Kind, in.Kinds) {
				keep = append(keep, p)
			}
		}
		places = keep
	}
	if len(places) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "There are no places to choose from with these filters."})
		return
	}
	if len(places) > 150 {
		places = places[:150]
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Request: %s\nToday is %s, %s.\n\nPlaces (id | name | kind | cuisine | town | price 1-4 | rating 1-5 | status | favourite | tags | distance | notes):\n",
		in.Prompt, time.Now().Format("Monday 2 January 2006"), time.Now().Format("15:04"))
	byID := map[int64]store.Place{}
	for _, p := range places {
		byID[p.ID] = p
		dist := ""
		if p.Distance != nil {
			dist = fmt.Sprintf("%.1f km", *p.Distance/1000)
		}
		notes := p.Notes
		if len(notes) > 160 {
			notes = notes[:160]
		}
		fav := ""
		if p.Favourite {
			fav = "favourite"
		}
		fmt.Fprintf(&b, "%d | %s | %s | %s | %s | %d | %.1f | %s | %s | %s | %s | %s\n", p.ID, p.Name, store.KindLabel(p.Kind), p.Cuisine, p.City, p.Price, p.Rating,
			map[string]string{"want": "want to try", "been": "been"}[p.Status], fav, p.Tags, dist, strings.ReplaceAll(notes, "\n", " "))
	}
	b.WriteString(`
Pick one to three places for the request. Answer with JSON only: {"picks":[{"id":12,"why":"one sentence"}],"note":"one short sentence when nothing fits well, else empty"}`)
	var out struct {
		Picks []struct {
			ID  int64  `json:"id"`
			Why string `json:"why"`
		} `json:"picks"`
		Note string `json:"note"`
	}
	if err := cl.JSON(ctx, aiVoice, b.String(), &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type pickOut struct {
		Place store.Place `json:"place"`
		Why   string      `json:"why"`
	}
	picks := []pickOut{}
	seen := map[int64]bool{}
	for _, p := range out.Picks {
		pl, ok := byID[p.ID]
		if !ok || seen[p.ID] || len(picks) == 3 {
			continue
		}
		seen[p.ID] = true
		picks = append(picks, pickOut{Place: pl, Why: strings.TrimSpace(p.Why)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "picks": picks, "note": strings.TrimSpace(out.Note)})
}
