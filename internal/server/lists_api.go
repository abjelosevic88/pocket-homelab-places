package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

type listInput struct {
	Name        *string `json:"name"`
	Emoji       *string `json:"emoji"`
	Description *string `json:"description"`
	TripID      *int64  `json:"trip_id"`
	TripName    *string `json:"trip_name"`
	Sort        *int    `json:"sort"`
	PlaceIDs    []int64 `json:"place_ids"`
}

func (in listInput) apply(l *store.List, keys map[string]bool) string {
	trimP(&l.Name, in.Name)
	trimP(&l.Emoji, in.Emoji)
	trimP(&l.Description, in.Description)
	if keys["trip_id"] {
		l.TripID = in.TripID
		if in.TripID == nil {
			l.TripName = ""
		}
	}
	trimP(&l.TripName, in.TripName)
	if in.Sort != nil {
		l.Sort = *in.Sort
	}
	if l.Name == "" {
		return "give the list a name"
	}
	if len(l.Name) > 120 || len([]rune(l.Emoji)) > 8 || len(l.Description) > 4000 {
		return "that is too long"
	}
	return ""
}

func (s *Server) handleListLists(w http.ResponseWriter, r *http.Request) {
	ls, err := s.store.ListLists(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "lists")
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in listInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	l := store.List{}
	if msg := in.apply(&l, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateList(ctx, uid, &l); err != nil {
		s.fail(w, err, "add list")
		return
	}
	for _, p := range in.PlaceIDs {
		_ = s.store.AddToList(ctx, uid, l.ID, p, "")
	}
	nl, _ := s.store.GetList(ctx, uid, l.ID)
	s.publish(ctx, uid, "places.list_saved", "list:"+itoa(l.ID), listPayload(nl))
	writeJSON(w, http.StatusCreated, nl)
}

func listPayload(l *store.List) map[string]any {
	out := map[string]any{"list_id": l.ID, "name": l.Name, "emoji": l.Emoji, "count": l.Count, "title": strings.TrimSpace(l.Emoji + " " + l.Name), "path": "/lists/" + itoa(l.ID)}
	if l.TripID != nil {
		out["trip_id"], out["trip_name"] = *l.TripID, l.TripName
	}
	return out
}

func (s *Server) handleGetList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetList(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	f := placeFilter(r)
	f.ListID = l.ID
	ps, err := s.store.ListPlaces(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	entries, _ := s.store.ListEntries(ctx, uid, l.ID)
	notes := map[string]string{}
	for _, e := range entries {
		if e.Note != "" {
			notes[itoa(e.PlaceID)] = e.Note
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": l, "places": ps, "notes": notes})
}

func (s *Server) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l, err := s.store.GetList(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	var in listInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(l, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateList(ctx, uid, l); err != nil {
		s.fail(w, err, "save list")
		return
	}
	nl, _ := s.store.GetList(ctx, uid, l.ID)
	s.publish(ctx, uid, "places.list_saved", "list:"+itoa(l.ID), listPayload(nl))
	writeJSON(w, http.StatusOK, nl)
}

func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteList(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAddPlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		PlaceID int64  `json:"place_id"`
		Note    string `json:"note"`
	}
	if err := decode(r, &in); err != nil || in.PlaceID <= 0 {
		writeErr(w, http.StatusBadRequest, "choose a place")
		return
	}
	if err := s.store.AddToList(ctx, actorID(ctx), idParam(r, "id"), in.PlaceID, in.Note); err != nil {
		s.fail(w, err, "add to list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRemovePlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.RemoveFromList(ctx, actorID(ctx), idParam(r, "id"), idParam(r, "place")); err != nil {
		s.fail(w, err, "remove from list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
