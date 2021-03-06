package api

import (
	"fmt"
	as "github.com/go-ap/activitystreams"
	json "github.com/go-ap/jsonld"
	"github.com/mariusor/littr.go/app"
	ap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/internal/errors"
	"net/http"
)

// HandleService
// GET /api/self
func (h handler) HandleService(w http.ResponseWriter, r *http.Request) {
	us := ap.Service{}

	id := fmt.Sprintf("%s/self", h.repo.BaseURL)
	rr := r.Context().Value(app.RepositoryCtxtKey)

	var err error
	var inf app.Info
	if repo, ok := rr.(app.CanLoadInfo); ok {
		if inf, err = repo.LoadInfo(); err != nil {
			h.HandleError(w, r, errors.NewNotValid(err, "ooops!"))
			return
		}
	}

	us.ID = as.ObjectID(id)
	us.Type = as.ServiceType
	us.Name.Set(as.NilLangRef, inf.Title)
	us.URL = as.IRI(inf.URI)
	us.Inbox = as.IRI(fmt.Sprintf("%s/inbox", id))
	us.Outbox = as.IRI(fmt.Sprintf("%s/outbox", id))
	us.Following = as.IRI(fmt.Sprintf("%s/following", id))
	us.Liked = as.IRI(fmt.Sprintf("%s/liked", id))
	//us.Summary.Set(as.NilLangRef, "This is a link aggregator similar to hacker news and reddit")
	us.Summary.Set(as.NilLangRef, inf.Summary)
	us.Content.Set(as.NilLangRef, string(app.Markdown(inf.Description)))

	us.AttributedTo = as.IRI("https://github.com/mariusor")
	data, _ := json.WithContext(GetContext()).Marshal(us)
	w.Header().Set("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
