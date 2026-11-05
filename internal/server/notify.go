package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// pushes runs every minute; once a day, at each person's own hour, it sends
// one message with today's planned visits and the trips starting in three
// days with places on your list there. "How was it?" goes out on its own
// as soon as a visit is noticed (inbox.go).
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.dailyCheck(ctx, uid, st, now)
	}
}

// dailyCheck builds and sends one person's daily message.
func (s *Server) dailyCheck(ctx context.Context, uid int64, st store.Settings, now time.Time) (string, bool) {
	today := store.Today()
	var lines []string
	if st.NotifyPlanned {
		planned, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(true), From: today, To: today, Oldest: true})
		for _, v := range planned {
			line := "• Today: " + v.PlaceName
			if v.Time != "" {
				line += " at " + v.Time
			}
			if v.WithWhom != "" {
				line += ", with " + v.WithWhom
			}
			lines = append(lines, line)
		}
	}
	if st.NotifyTrips {
		trips, _ := s.tripsAhead(ctx, uid, 3, 0)
		for _, t := range trips {
			if t.DaysUntil < 1 || t.DaysUntil > 3 || t.Want+t.Been == 0 || !s.once(ctx, fmt.Sprintf("trip:%d:%d:%s", uid, t.TripID, t.Start)) {
				continue
			}
			when := "tomorrow"
			if t.DaysUntil > 1 {
				when = "in " + itoa(int64(t.DaysUntil)) + " days"
			}
			line := fmt.Sprintf("• %s starts %s: %d %s on your list there", firstNonEmpty(t.Name, t.Destination, "Your trip"), when, t.Want, plural(t.Want, "place", "places"))
			if t.Been > 0 {
				line += fmt.Sprintf(", %d you liked before", t.Been)
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 || st.NotifyURL == "" {
		return strings.Join(lines, "\n"), false
	}
	body := strings.Join(lines, "\n")
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Your places today", Body: body, Priority: "info", Tags: []string{"round_pushpin"}, Link: s.cfg.BaseURL + "/"}); err != nil {
		s.log.Warn("notify", "user", uid, "err", err)
		return body, false
	}
	return body, true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Places", Body: notifyTestBody, Priority: "info", Tags: []string{"round_pushpin"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. \"How was it?\" after you visited one of your places, today's plans and trips with places on your list will arrive here."
