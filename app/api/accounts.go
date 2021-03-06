package api

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/mariusor/littr.go/app"

	"github.com/mariusor/littr.go/app/frontend"

	"context"
	goap "github.com/go-ap/activitypub"
	as "github.com/go-ap/activitystreams"
	json "github.com/go-ap/jsonld"
	"github.com/go-chi/chi"
	ap "github.com/mariusor/littr.go/app/activitypub"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/internal/log"
)

type objectID struct {
	baseURL string
	query   url.Values
}

func getObjectID(s string) as.ObjectID {
	return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, s))
}

func apAccountID(a app.Account) as.ObjectID {
	if len(a.Hash) >= 8 {
		return as.ObjectID(fmt.Sprintf("%s/%s", ActorsURL, a.Hash.String()))
	}
	return as.ObjectID(fmt.Sprintf("%s/anonymous", ActorsURL))
}

func loadAPLike(vote app.Vote) as.ObjectOrLink {
	id, _ := BuildObjectIDFromItem(*vote.Item)
	lID := BuildObjectIDFromVote(vote)
	whomArt := as.IRI(BuildActorID(*vote.SubmittedBy))
	if vote.Weight == 0 {
		l := as.UndoNew(lID, as.IRI(id))
		l.AttributedTo = whomArt
		return *l
	} else if vote.Weight > 0 {
		l := as.LikeNew(lID, as.IRI(id))
		l.AttributedTo = whomArt
		return *l
	} else {
		l := as.DislikeNew(lID, as.IRI(id))
		l.AttributedTo = whomArt
		return *l
	}
}

func loadAPActivity(it app.Item) as.Activity {
	a := loadAPPerson(*it.SubmittedBy)
	ob := loadAPItem(it)

	obID := string(*ob.GetID())

	act := as.Activity{
		Parent: as.Parent{
			Type:      as.CreateType,
			ID:        as.ObjectID(fmt.Sprintf("%s", strings.Replace(obID, "/object", "", 1))),
			Published: it.SubmittedAt,
			To:        as.ItemCollection{as.IRI("https://www.w3.org/ns/activitystreams#Public")},
			CC:        as.ItemCollection{as.IRI(BuildGlobalOutboxID())},
		},
	}

	act.Object = ob
	act.Actor = a.GetLink()
	switch ob.GetType() {
	case as.TombstoneType:
		act.Type = as.DeleteType
		act.Actor = as.IRI(BuildActorID(app.AnonymousAccount))
	}
	return act
}

func itemURL(item app.Item) as.IRI {
	return as.IRI(frontend.ItemPermaLink(item))
}

func loadAPItem(item app.Item) as.Item {
	o := ap.Article{}

	if id, ok := BuildObjectIDFromItem(item); ok {
		o.ID = id
	}
	if item.MimeType == app.MimeTypeURL {
		o.Type = as.PageType
		o.URL = as.IRI(item.Data)
	} else {
		wordCount := strings.Count(item.Data, " ") +
			strings.Count(item.Data, "\t") +
			strings.Count(item.Data, "\n") +
			strings.Count(item.Data, "\r\n")
		if wordCount > 300 {
			o.Type = as.ArticleType
		} else {
			o.Type = as.NoteType
		}

		if len(item.Hash) > 0 {
			o.URL = itemURL(item)
		}
		o.Name = make(as.NaturalLanguageValues, 0)
		switch item.MimeType {
		case app.MimeTypeMarkdown:
			o.Object.Source.MediaType = as.MimeType(item.MimeType)
			o.MediaType = as.MimeType(app.MimeTypeHTML)
			if item.Data != "" {
				o.Source.Content.Set("en", string(item.Data))
				o.Content.Set("en", string(app.Markdown(string(item.Data))))
			}
		case app.MimeTypeText:
			fallthrough
		case app.MimeTypeHTML:
			o.MediaType = as.MimeType(item.MimeType)
			o.Content.Set("en", string(item.Data))
		}
	}

	o.Published = item.SubmittedAt
	o.Updated = item.UpdatedAt

	if item.Deleted() {
		del := as.Tombstone{
			Parent: as.Object{
				ID:   o.ID,
				Type: as.TombstoneType,
			},
			FormerType: o.Type,
			Deleted:    o.Updated,
		}
		if item.Parent != nil {
			if par, ok := BuildObjectIDFromItem(*item.Parent); ok {
				del.InReplyTo = as.IRI(par)
			}
		}
		if item.OP != nil {
			if op, ok := BuildObjectIDFromItem(*item.OP); ok {
				del.Context = as.IRI(op)
			}
		}

		return del
	}

	//o.Generator = as.IRI(app.Instance.BaseURL)
	o.Score = item.Score / app.ScoreMultiplier
	if item.Title != "" {
		o.Name.Set("en", string(item.Title))
	}
	if item.SubmittedBy != nil {
		id := BuildActorID(*item.SubmittedBy)
		o.AttributedTo = as.IRI(id)
	}
	if item.Parent != nil {
		id, _ := BuildObjectIDFromItem(*item.Parent)
		o.InReplyTo = as.IRI(id)
	}
	if item.OP != nil {
		id, _ := BuildObjectIDFromItem(*item.OP)
		o.Context = as.IRI(id)
	}
	if item.Metadata != nil {
		m := item.Metadata
		if m.Mentions != nil || m.Tags != nil {
			o.Tag = make(as.ItemCollection, 0)
			for _, men := range m.Mentions {
				t := as.Object{
					ID:   as.ObjectID(men.URL),
					Type: as.MentionType,
					Name: as.NaturalLanguageValues{{Ref: as.NilLangRef, Value: men.Name}},
				}
				o.Tag.Append(t)
			}
			for _, tag := range m.Tags {
				t := as.Object{
					ID:   as.ObjectID(tag.URL),
					Name: as.NaturalLanguageValues{{Ref: as.NilLangRef, Value: tag.Name}},
				}
				o.Tag.Append(t)
			}
		}
	}

	return &o
}
func accountURL(acc app.Account) as.IRI {
	return as.IRI(fmt.Sprintf("%s%s", app.Instance.BaseURL, frontend.AccountPermaLink(acc)))
}

func loadAPPerson(a app.Account) *ap.Person {
	p := ap.Person{}
	p.Type = as.PersonType
	p.Name = as.NaturalLanguageValuesNew()
	p.PreferredUsername = as.NaturalLanguageValuesNew()

	if a.HasMetadata() {
		if a.Metadata.Blurb != nil && len(a.Metadata.Blurb) > 0 {
			p.Summary = as.NaturalLanguageValuesNew()
			p.Summary.Set(as.NilLangRef, string(a.Metadata.Blurb))
		}
		if len(a.Metadata.Icon.URI) > 0 {
			avatar := as.ObjectNew(as.ImageType)
			avatar.MediaType = as.MimeType(a.Metadata.Icon.MimeType)
			avatar.URL = as.IRI(a.Metadata.Icon.URI)
			p.Icon = avatar
		}
	}

	p.PreferredUsername.Set("en", a.Handle)

	if a.IsFederated() {
		p.ID = as.ObjectID(a.Metadata.ID)
		p.Name.Set("en", a.Metadata.Name)
		if len(a.Metadata.InboxIRI) > 0 {
			p.Inbox = as.IRI(a.Metadata.InboxIRI)
		}
		if len(a.Metadata.OutboxIRI) > 0 {
			p.Outbox = as.IRI(a.Metadata.OutboxIRI)
		}
		if len(a.Metadata.LikedIRI) > 0 {
			p.Liked = as.IRI(a.Metadata.LikedIRI)
		}
		if len(a.Metadata.FollowersIRI) > 0 {
			p.Followers = as.IRI(a.Metadata.FollowersIRI)
		}
		if len(a.Metadata.FollowingIRI) > 0 {
			p.Following = as.IRI(a.Metadata.FollowingIRI)
		}
		if len(a.Metadata.URL) > 0 {
			p.URL = as.IRI(a.Metadata.URL)
		}
	} else {
		p.Name.Set("en", a.Handle)

		p.Outbox = as.IRI(BuildCollectionID(a, new(goap.Outbox)))
		p.Inbox = as.IRI(BuildCollectionID(a, new(goap.Inbox)))
		p.Liked = as.IRI(BuildCollectionID(a, new(goap.Liked)))

		p.URL = accountURL(a)

		if !a.CreatedAt.IsZero() {
			p.Published = a.CreatedAt
		}
		if !a.UpdatedAt.IsZero() {
			p.Updated = a.UpdatedAt
		}
	}
	if len(a.Hash) >= 8 {
		p.ID = apAccountID(a)
	}

	p.Score = a.Score
	if a.IsValid() && a.HasMetadata() && a.Metadata.Key != nil && a.Metadata.Key.Public != nil {
		p.PublicKey = ap.PublicKey{
			ID:           as.ObjectID(fmt.Sprintf("%s#main-key", p.ID)),
			Owner:        as.IRI(p.ID),
			PublicKeyPem: fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----", base64.StdEncoding.EncodeToString(a.Metadata.Key.Public)),
		}
	}
	oauthURL := strings.Replace(BaseURL, "api", "oauth", 1)
	p.Endpoints = goap.Endpoints{
		SharedInbox:                as.IRI(fmt.Sprintf("%s/self/inbox", BaseURL)),
		OauthAuthorizationEndpoint: as.IRI(fmt.Sprintf("%s/authorize", oauthURL)),
		OauthTokenEndpoint:         as.IRI(fmt.Sprintf("%s/token", oauthURL)),
	}

	return &p
}

func loadAPVoteCollection(o as.CollectionInterface, votes app.VoteCollection) (as.CollectionInterface, error) {
	if votes == nil || len(votes) == 0 {
		return nil, nil
	}
	for _, vote := range votes {
		o.Append(loadAPLike(vote))
	}

	return o, nil
}

func loadAPItemCollection(o as.CollectionInterface, items app.ItemCollection) (as.CollectionInterface, error) {
	if items == nil || len(items) == 0 {
		return nil, nil
	}
	for _, item := range items {
		o.Append(loadAPActivity(item))
	}

	return o, nil
}

func loadAPAccountCollection(o as.CollectionInterface, accounts app.AccountCollection) (as.CollectionInterface, error) {
	if accounts == nil || len(accounts) == 0 {
		return nil, nil
	}
	for _, acc := range accounts {
		o.Append(loadAPPerson(acc))
	}

	return o, nil
}

// GET /api/self/following/:handle
func (h handler) HandleActor(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(app.AccountCtxtKey)

	var ok bool
	var a app.Account
	if a, ok = val.(app.Account); !ok {
		h.logger.Error("could not load Account from Context")
	}
	p := loadAPPerson(a)
	if p.Outbox != nil {
		p.Outbox = p.Outbox.GetLink()
	}
	if p.Liked != nil {
		p.Liked = p.Liked.GetLink()
	}
	if p.Inbox != nil {
		p.Inbox = p.Inbox.GetLink()
	}

	j, err := json.WithContext(GetContext()).Marshal(p)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"trace": errors.Details(err),
		}).Error(err.Error())
		h.HandleError(w, r, errors.NewNotValid(err, "unable to marshall goap object"))
		return
	}

	w.Header().Set("Content-Type", "application/activity+json")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

func getCollectionFromReq(r *http.Request) string {
	collection := chi.URLParam(r, "collection")
	//if path.Base(r.URL.Path) == "replies" {
	//	collection = "replies"
	//}
	base := path.Base(r.URL.Path)
	if collection == "" && isValidCollectionName(base) {
		collection = base
	}
	return collection
}

// GET /api/self/following/:handle/:collection/:hash
// GET /api/:collection/:hash
func (h handler) HandleCollectionActivity(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	val := r.Context().Value(app.ItemCtxtKey)
	collection := getCollectionFromReq(r)
	var el as.ObjectOrLink
	switch strings.ToLower(collection) {
	case "replies":
		fallthrough
	case "inbox":
		fallthrough
	case "outbox":
		item, ok := val.(app.Item)
		if !ok {
			err := errors.New("could not load Item from Context")
			h.HandleError(w, r, errors.NewNotFound(err, "not found"))
			return
		}
		el = loadAPActivity(item)
		if err != nil {
			h.HandleError(w, r, errors.NewNotFound(err, "not found"))
			return
		}
	case "liked":
		if v, ok := val.(app.Vote); !ok {
			err := errors.Errorf("could not load Vote from Context")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		} else {
			el = loadAPLike(v)
		}
	case "following":
	default:
		err := errors.Errorf("collection %s not found", collection)
		h.HandleError(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GET /api/self/following/:handle/:collection/:hash/object
// GET /api/:collection/:hash/object
func (h handler) HandleCollectionActivityObject(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	val := r.Context().Value(app.ItemCtxtKey)
	collection := getCollectionFromReq(r)
	var el as.ObjectOrLink
	switch strings.ToLower(collection) {
	case "inbox":
		fallthrough
	case "replies":
		fallthrough
	case "outbox":
		i, ok := val.(app.Item)
		if !ok {
			h.logger.Error("could not load Item from Context")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		}
		el = loadAPItem(i)
		val := r.Context().Value(app.RepositoryCtxtKey)
		if service, ok := val.(app.CanLoadItems); ok && len(i.Hash) > 0 {
			replies, _, err := service.LoadItems(app.Filters{
				LoadItemsFilter: app.LoadItemsFilter{
					InReplyTo: []string{i.Hash.String()},
				},
				MaxItems: MaxContentItems,
			})
			if err != nil {
				h.logger.WithContext(log.Ctx{
					"trace": errors.Details(err),
				}).Error(err.Error())
			}
			if len(replies) > 0 {
				if o, ok := el.(ap.Article); ok {
					o.Replies = as.IRI(BuildRepliesCollectionID(o))
					el = o
				}
			}
		}
	case "liked":
		if v, ok := val.(app.Vote); !ok {
			err := errors.Errorf("could not load Vote from Context")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return
		} else {
			el = loadAPLike(v)
		}
	case "following":
		// skip
	default:
		err := errors.Errorf("collection %s not found", collection)
		h.HandleError(w, r, errors.NewNotFound(err, "not found"))
		return
	}

	data, err = json.WithContext(GetContext()).Marshal(el)
	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func loadCollection(items app.Collection, count uint, typ string, filters app.Paginator, baseUrl string) (as.Item, error) {
	getURL := func(f app.Paginator) string {
		qs := ""
		if f != nil {
			qs = f.QueryString()
		}
		return fmt.Sprintf("%s%s", baseUrl, qs)
	}

	var haveItems, moreItems, lessItems bool
	var bp, fp, cp, pp, np app.Paginator

	oc := as.OrderedCollection{}
	oc.ID = as.ObjectID(getURL(bp))
	oc.Type = as.OrderedCollectionType

	f, _ := filters.(*app.Filters)
	if len(f.LoadItemsFilter.AttributedTo) == 1 {
		f.LoadItemsFilter.AttributedTo = nil
	}
	switch typ {
	case "inbox":
		fallthrough
	case "replies":
		fallthrough
	case "outbox":
		if col, ok := items.(app.ItemCollection); ok {
			if _, err := loadAPItemCollection(&oc, col); err != nil {
				return nil, err
			}
			haveItems = len(col) > 0
		} else {
			return nil, errors.New("could not load items")
		}
	case "liked":
		if col, ok := items.(app.VoteCollection); ok {
			if _, err := loadAPVoteCollection(&oc, col); err != nil {
				return nil, err
			}
			if len(f.LoadVotesFilter.AttributedTo) == 1 {
				f.LoadVotesFilter.AttributedTo = nil
			}
			haveItems = len(col) > 0
		} else {
			return nil, errors.New("could not load items")
		}
	case "followed":
		fallthrough
	case "following":
		if col, ok := items.(app.AccountCollection); ok {
			if _, err := loadAPAccountCollection(&oc, col); err != nil {
				return nil, err
			}
			haveItems = len(col) > 0
		} else {
			return nil, errors.New("could not load items")
		}
	}

	moreItems = int(count) > ((f.Page + 1) * f.MaxItems)
	lessItems = f.Page > 1
	if filters != nil {
		bp = filters.BasePage()
		fp = filters.FirstPage()
		cp = filters.CurrentPage()
	}

	if haveItems {
		firstURL := getURL(fp)
		oc.First = as.IRI(firstURL)

		if f.Page >= 1 {
			curURL := getURL(cp)
			page := as.OrderedCollectionPageNew(&oc)
			page.ID = as.ObjectID(curURL)

			if moreItems {
				np = filters.NextPage()
				nextURL := getURL(np)
				page.Next = as.IRI(nextURL)
			}
			if lessItems {
				pp = filters.PrevPage()
				prevURL := getURL(pp)
				page.Prev = as.IRI(prevURL)
			}
			page.TotalItems = count
			return page, nil
		}
	}

	oc.TotalItems = count
	return oc, nil
}

// GET /api/self/:collection
// GET /api/self/following/:handle/:collection
// GET /api/self/:collection/:hash/replies
// GET /api/self/following/:handle/:collection/:hash/replies
func (h handler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	var page as.Item

	typ := getCollectionFromReq(r)

	items := r.Context().Value(app.CollectionCtxtKey)
	count, _ := r.Context().Value(app.CollectionCountCtxtKey).(uint)

	filters := r.Context().Value(app.FilterCtxtKey)
	f, _ := filters.(app.Paginator)
	baseURL := fmt.Sprintf("%s%s", h.repo.BaseURL, strings.Replace(r.URL.Path, "/api", "", 1))
	page, err = loadCollection(items, count, typ, f, baseURL)

	data, err = json.WithContext(GetContext()).Marshal(page)
	if err != nil {
		h.logger.Error(err.Error())
		h.HandleError(w, r, errors.NewNotValid(err, "unable to marshal collection"))
		return
	}

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h handler) LoadActivity(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		errFn := func(err error) {
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("Activity load error")
			h.HandleError(w, r, err)
			return
		}
		if r.Method == http.MethodGet {
			errFn(errors.MethodNotAllowedf("invalid %s request", r.Method))
			return
		}

		a := ap.Activity{}
		if body, err := ioutil.ReadAll(r.Body); err != nil || len(body) == 0 {
			errFn(errors.NewNotValid(err, "unable to read request body"))
			return
		} else {
			if err := json.Unmarshal(body, &a); err != nil {
				errFn(errors.NewNotValid(err, "unable to unmarshal JSON request"))
				return
			}
		}
		// TODO(marius) Does this make any sense?
		// When the object of the activity is nil, we can consider it to be the actor that the current
		// Inbox/Outbox URL belongs to. This might not apply to all Activities.
		//if a.Object == nil && a.GetType() == as.FollowType {
		//	a.Object = as.IRI(path.Dir(r.URL.String()))
		//}
		ctx := context.WithValue(r.Context(), app.ItemCtxtKey, a)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

var notLocalIRI = UserError{msg: "Not a local IRI"}

func validateLocalIRI(i as.IRI) error {
	if len(i) == 0 {
		// empty IRI is valid(ish)
		return nil
	}
	if path.IsAbs(i.String()) {
		return nil
	}
	if app.Instance.HostName == i.String() {
		return nil
	}
	if !app.HostIsLocal(i.String()) {
		e := notLocalIRI
		e.ID = i
		return e
	}
	return nil
}

func host(u string) string {
	if pu, err := url.ParseRequestURI(u); err == nil {
		return pu.Host
	}
	return u
}

type actorMissingError struct {
	err   error
	actor as.Item
}

type objectMissingError struct {
	err    error
	object as.Item
}

type activityError struct {
	actor  error
	object error
	other  []error
}

type validator struct {
	r app.Repository
}

func (v validator) Validate(ep as.IRI, obj as.Item) (bool, goap.ValidationErrors) {
	var res bool
	var errs goap.ValidationErrors
	switch obj.GetType() {
	case as.ActorType:
		if _, err := validateActor(obj, v.r); err != nil {
			errs.Add(err)
		} else {
			res = true
		}
	case as.ObjectType:
		if _, err := validateObject(obj, v.r, ""); err != nil {
			errs.Add(err)
		} else {
			res = true
		}
	}
	return res, errs
}

func (a actorMissingError) Error() string {
	return fmt.Sprintf("received actor hash does not exist on local instance %s", a.actor.GetLink())
}

func (a objectMissingError) Error() string {
	return fmt.Sprintf("received object hash does not exist on local instance %s", a.object.GetLink())
}

func (a activityError) Error() string {
	return fmt.Sprintf("received activity is missing %s %s", a.actor.Error(), a.object.Error())
}

func validateLocalActor(a as.Item, repo app.CanLoadAccounts) (as.Item, error) {
	var err error
	if err = validateLocalIRI(a.GetLink()); err == nil {
		return validateActor(a, repo)
	}
	return a, errors.NewMethodNotAllowed(err, "actor")
}

func validateActor(a as.Item, repo app.CanLoadAccounts) (as.Item, error) {
	p := ap.Person{}

	var err error
	if err = validateIRIIsBlocked(a.GetLink()); err != nil {
		return nil, errors.NewMethodNotAllowed(err, "object is blocked")
	}
	if err = validateIRIBelongsToBlackListedInstance(a.GetLink()); err != nil {
		return p, errors.NewMethodNotAllowed(err, "actor belongs to blocked instance")
	}

	isLocalActor := true
	if err = validateLocalIRI(a.GetLink()); err != nil {
		isLocalActor = false
	}

	acct := app.Account{}
	acct.FromActivityPub(a)

	if len(acct.Hash)+len(acct.Handle) == 0 {
		return p, errors.Errorf("unable to load a valid actor identifier from IRI %s", a.GetLink())
	} else {
		f := app.Filters{}
		if len(acct.Hash) == 0 && len(acct.Handle) > 0 {
			if !isLocalActor {
				f.LoadAccountsFilter.IRI = a.GetLink().String()
			} else {
				f.Handle = []string{acct.Handle}
			}
		}
		if len(acct.Handle) == 0 && len(acct.Hash) > 0 {
			if !isLocalActor {
				f.LoadAccountsFilter.IRI = a.GetLink().String()
			} else {
				f.LoadAccountsFilter.Key = app.Hashes{acct.Hash}
			}
		}
		acct, err = repo.LoadAccount(f)
		if err != nil {
			return p, actorMissingError{err: err, actor: a}
		}
		p = *loadAPPerson(acct)
	}
	return p, nil
}

func validateItemType(typ as.ActivityVocabularyType, validTypes []as.ActivityVocabularyType) error {
	for _, t := range validTypes {
		if typ == t {
			return nil
		}
	}
	return errors.Errorf("object type %s is not valid for current context", typ)
}

func getValidObjectTypes(typ as.ActivityVocabularyType) []as.ActivityVocabularyType {
	switch typ {
	case as.UpdateType:
		fallthrough
	case as.CreateType:
		fallthrough
	case as.UndoType:
		fallthrough
	case as.LikeType:
		fallthrough
	case as.DislikeType:
		// these are the locally supported ActivityStreams Object types
		return []as.ActivityVocabularyType{
			as.NoteType,
			as.ArticleType,
			as.DocumentType,
			as.PageType,
		}
	case as.DeleteType:
		return []as.ActivityVocabularyType{
			as.NoteType,
			as.ArticleType,
			as.DocumentType,
			as.PageType,
			// not sure if we need the other types
			as.TombstoneType,
		}
	}
	return nil
}

func validateObject(a as.Item, repo app.CanLoadItems, typ as.ActivityVocabularyType) (as.Item, error) {
	var o as.Item
	if a == nil {
		return a, nil
	}

	var err error
	if err = validateIRIIsBlocked(a.GetLink()); err != nil {
		return nil, errors.NewMethodNotAllowed(err, "object is blocked")
	}
	if err = validateIRIBelongsToBlackListedInstance(a.GetLink()); err != nil {
		return nil, errors.NewMethodNotAllowed(err, "object belongs to blocked instance")
	}

	isLocalObject := true
	if err := validateLocalIRI(a.GetLink()); err != nil {
		isLocalObject = false
	}

	cont := app.Item{}
	cont.FromActivityPub(a)

	if a.IsLink() {
		if !isLocalObject {
			// we need to dereference the object
			return o, objectMissingError{err: err, object: a}
		}
	} else {
		if err = validateItemType(a.GetType(), getValidObjectTypes(typ)); err != nil {
			return a, errors.NewNotValid(err, fmt.Sprintf("failed to validate object for %s activity", typ))
		}
	}

	switch typ {
	case as.CreateType:
		if len(cont.Hash) > 0 {
			// dunno if this is an error
		}
	case as.UpdateType:
		if len(cont.Hash) == 0 {
			return o, objectMissingError{err: err, object: a}
		}
	}

	o = loadAPItem(cont)
	return o, nil
	// @todo(marius): see what this was about
	//switch typ {
	//case as.UpdateType:
	//	fallthrough
	//case as.CreateType:
	//	// @todo(marius): implement create/edit/delete
	//	cont := app.Item{}
	//	cont.FromActivityPub(a)
	//	if len(cont.Hash) > 0 {
	//		cont, err = db.Config.LoadItem(app.LoadItemsFilter{
	//			Key: []string{string(cont.Hash)},
	//		})
	//		if err == nil {
	//			a = loadAPItem(cont)
	//		}
	//	}
	//case as.UndoType:
	//	fallthrough
	//case as.LikeType:
	//	fallthrough
	//case as.DislikeType:
	//	vot := app.Vote{}
	//	vot.FromActivityPub(a)
	//	if len(vot.Item.Hash) > 0 {
	//		vot, err = db.Config.LoadVote(app.LoadVotesFilter{
	//			ItemKey: []string{string(vot.Item.Hash)},
	//		})
	//		if err == nil {
	//			a = loadAPLike(vot)
	//		}
	//	}
	//	oID := a.GetID()
	//	if len(*oID) == 0 {
	//		return a, errors.Errorf("%sed object needs to be local and have a valid ID", typ)
	//	}
	//	if err := validateLocalIRI(a.GetLink()); err != nil {
	//		return a, errors.Annotatef(err, "%sed object should have local resolvable IRI", typ)
	//	}
	//default:
	//	return a, errors.Annotatef(err, "%s unknown activity type", typ)
	//}
}

func validateIRIBelongsToBlackListedInstance(iri as.IRI) error {
	// @todo(marius): add a proper method of loading blocked instances
	blockedInstances := []string{
		"mastodon.social",
	}
	for _, blocked := range blockedInstances {
		if strings.Contains(iri.String(), blocked) {
			return errors.NotValidf("%s", iri)
		}
	}
	return nil
}

func validateIRIIsBlocked(iri as.IRI) error {
	// @todo(marius): add a proper method of loading blocked IRIs
	blockedIRIs := []string{
		"https://example.com/actors/jonathan.doe",
	}
	u, err := url.Parse(iri.String())
	if err != nil {
		return nil
	}
	u.Path = path.Clean(u.Path)
	for _, blocked := range blockedIRIs {
		if u.String() == blocked {
			return errors.NotValidf("%s", iri)
		}
	}
	return nil
}

func validateRecipients(a ap.Activity) error {
	a.RecipientsDeduplication()

	checkCollection := func(base string, col ...as.Item) bool {
		if len(base) == 0 {
			return true
		}
		if col == nil || len(col) == 0 {
			return false
		}
		if col != nil && len(col) == 1 && col[0] == nil {
			return true
		}
		if col != nil && len(col) > 0 {
			for _, tgt := range col {
				if tgt == nil {
					continue
				}
				tgtUrl := tgt.GetLink().String()
				if strings.Contains(tgtUrl, base) {
					return true
				}
			}
		}
		return false
	}

	// @todo(marius): handle https://www.w3.org/ns/activitystreams#Public targets
	lT := host(app.Instance.BaseURL)
	valid := checkCollection(lT, a.To...) ||
		checkCollection(lT, a.CC...) ||
		checkCollection(lT, a.Bto...) ||
		checkCollection(lT, a.BCC...) ||
		checkCollection(lT, a.Actor) ||
		checkCollection(lT, a.InReplyTo) ||
		checkCollection(lT, a.Context)

	if !valid {
		return errors.NotValidf("local instance can not be found in the recipients list")
	}

	return nil
}

func validateInboxActivityType(typ as.ActivityVocabularyType) error {
	var validTypes = []as.ActivityVocabularyType{
		as.CreateType,
		as.LikeType,
		as.DislikeType,
		//as.UpdateType, // @todo(marius): not implemented
		//as.DeleteType, // @todo(marius): not implemented
		//as.UndoType,   // @todo(marius): not implemented
		as.FollowType, // @todo(marius): not implemented
	}
	if err := validateItemType(typ, validTypes); err != nil {
		return errors.NewNotValid(err, "failed to validate activity type for inbox collection")
	}
	return nil
}

func validateInboxActivity(a ap.Activity, repo app.CanLoad) (ap.Activity, error) {
	// TODO(marius): need to add a step to verify the Activity Actor against the one loaded from the Authorization header.
	if err := validateInboxActivityType(a.GetType()); err != nil {
		return a, errors.NewNotValid(err, "failed to validate activity type for inbox collection")
	}
	if err := validateRecipients(a); err != nil {
		return a, errors.NewNotValid(err, "invalid audience for activity")
	}
	aErr := activityError{}
	if p, err := validateActor(a.Actor, repo); err != nil {
		if errors.IsMethodNotAllowed(err) {
			return a, err
		}
		aErr.actor = err
	} else {
		a.Actor = p
	}
	if o, err := validateObject(a.Object, repo, a.GetType()); err != nil {
		aErr.object = err
	} else {
		a.Object = o
	}

	var err error
	if aErr.object != nil || aErr.actor != nil {
		err = aErr
	}
	return a, err
}

func validateOutboxActivityType(typ as.ActivityVocabularyType) error {
	var validTypes = []as.ActivityVocabularyType{
		as.CreateType,
		as.UpdateType,
		as.LikeType,
		as.DislikeType,
		as.DeleteType,
		as.UndoType, // @todo(marius): not implemented yet
	}
	if err := validateItemType(typ, validTypes); err != nil {
		return errors.Annotate(err, "failed to validate activity type for outbox collection")
	}
	return nil
}

func validateOutboxActivity(a ap.Activity, repo app.CanLoadAccounts) (ap.Activity, error) {
	if err := validateOutboxActivityType(a.GetType()); err != nil {
		return a, errors.Annotate(err, "failed to validate activity type for outbox collection")
	}
	if p, err := validateLocalActor(a.Actor, repo); err != nil {
		return a, errors.Annotate(err, "failed to validate actor for outbox collection")
	} else {
		a.Actor = p
	}
	if o, err := validateObject(a.Object, repo.(app.CanLoadItems), a.GetType()); err != nil {
		return a, errors.Annotate(err, "failed to validate object for outbox collection")
	} else {
		a.Object = o
	}
	return a, nil
}

func (h *handler) saveActivityContent(a ap.Activity, r *http.Request, w http.ResponseWriter) (int, string) {
	status := http.StatusInternalServerError
	var location string
	switch a.GetType() {
	case as.FollowType:
		status = http.StatusAccepted
	case as.DeleteType:
		fallthrough
	case as.UpdateType:
		fallthrough
	case as.CreateType:
		it := app.Item{}
		if err := it.FromActivityPub(a); err != nil {
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("unable to load item from ActivityPub object")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return http.StatusNotFound, ""
		}
		if repo, ok := app.ContextItemSaver(r.Context()); ok {
			newIt, err := repo.SaveItem(it)
			if err != nil {
				h.logger.WithContext(log.Ctx{
					"err":     err,
					"trace":   errors.Details(err),
					"item":    it.Hash,
					"account": it.SubmittedBy.Hash,
				}).Error(err.Error())
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
				return http.StatusNotFound, ""
			}
			if newIt.UpdatedAt.IsZero() {
				// we need to make a difference between created vote and updated vote
				// created - http.StatusCreated
				status = http.StatusCreated
				location = fmt.Sprintf("%s/self/following/%s/outbox/%s", h.repo.BaseURL, newIt.SubmittedBy.Hash, newIt.Hash)
			} else {
				// updated - http.StatusOK
				status = http.StatusOK
			}
		}
	case as.UndoType:
		fallthrough
	case as.DislikeType:
		fallthrough
	case as.LikeType:
		v := app.Vote{}
		if err := v.FromActivityPub(a); err != nil {
			h.logger.WithContext(log.Ctx{
				"err":   err,
				"trace": errors.Details(err),
			}).Error("unable to load vote from ActivityPub object")
			h.HandleError(w, r, errors.NewNotValid(err, "not found"))
			return http.StatusNotFound, ""
		}
		if repo, ok := app.ContextVoteSaver(r.Context()); ok {
			newVot, err := repo.SaveVote(v)
			if err != nil {
				h.logger.WithContext(log.Ctx{
					"err":      err,
					"trace":    errors.Details(err),
					"saveVote": v.SubmittedBy.Hash,
				}).Error(err.Error())
				h.HandleError(w, r, errors.NewNotValid(err, "not found"))
				return http.StatusNotFound, ""
			}
			if newVot.UpdatedAt.IsZero() {
				// we need to make a difference between created vote and updated vote
				// created - http.StatusCreated
				status = http.StatusCreated
				location = fmt.Sprintf("%s/self/following/%s/liked/%s", h.repo.BaseURL, newVot.SubmittedBy.Hash, newVot.Item.Hash)
			} else {
				// updated - http.StatusOK
				status = http.StatusOK
			}
		}
	}
	return status, location
}

func (h *handler) ClientRequest(w http.ResponseWriter, r *http.Request) {
	a, _ := app.ContextActivity(r.Context())

	var err error
	status := http.StatusNotImplemented
	var location string
	repo, _ := app.ContextLoader(r.Context())

	// here we're missing a way to store the specific collection IRI we've received the activity to
	if a, err = validateOutboxActivity(a, repo); err != nil {
		h.HandleError(w, r, err)
		return
	}
	if h.acc != nil {
		// validate if http-signature matches the current Activity.Actor
		account := *h.acc
		if a.Actor.GetLink() != loadAPPerson(account).GetLink() {
			h.HandleError(w, r, errors.Forbiddenf("The activity actor is not authorized to add"))
			return
		}
	}

	if repo, ok := app.ContextActivitySaver(r.Context()); ok == true {
		if i, err := repo.SaveActivity(a, as.IRI(fmt.Sprintf("%s", r.URL.Path))); err != nil {
			h.logger.Errorf("Can't save client activity %s: %s", a.GetType(), err)
		} else {
			a = i.(ap.Activity)
		}
	}

	status, location = h.saveActivityContent(a, r, w)
	if err != nil {
		h.logger.WithContext(log.Ctx{
			"actor":   a.Actor.GetLink(),
			"object":  a.Object.GetLink(),
			"from":    r.RemoteAddr,
			"headers": r.Header,
			"err":     err,
			"trace":   errors.Details(err),
		}).Error("activity validation error")
		h.HandleError(w, r, err)
		return
	}

	w.Header().Add("Content-Type", "application/activity+json; charset=utf-8")
	if status == http.StatusCreated {
		w.Header().Add("Location", location)
	}
	w.WriteHeader(status)
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	if status >= 400 {
		w.Write([]byte(`{"status": "nok"}`))
	} else {
		w.Write([]byte(`{"status": "ok"}`))
	}
}

func (h *handler) ServerRequest(w http.ResponseWriter, r *http.Request) {
	a, _ := app.ContextActivity(r.Context())
	errFn := func(err error, fmt string, it ...interface{}) {
		h.logger.WithContext(log.Ctx{
			"actor":   a.Actor.GetLink(),
			"object":  a.Object.GetLink(),
			"from":    r.RemoteAddr,
			"headers": r.Header,
			"err":     err,
			"trace":   errors.Details(err),
		}).Errorf(fmt, it...)
		h.HandleError(w, r, err)
	}
	var err error
	status := http.StatusNotImplemented
	var location string
	repo, _ := app.ContextLoader(r.Context())
	actorNeedsSaving := false
	var actor as.Item
	acc := app.Account{}
	if a, err = validateInboxActivity(a, repo); err != nil {
		if e, ok := err.(activityError); ok {
			if eact, ok := e.actor.(actorMissingError); ok {
				actorNeedsSaving = true
				actor = eact.actor
			}
			//if eobj, ok := e.object.(objectMissingError); ok {
			//	a.Object = eobj.object
			//}
		} else {
			errFn(err, "")
			return
		}
	}

	if repo, ok := app.ContextActivitySaver(r.Context()); ok == true {
		if i, err := repo.SaveActivity(a, as.IRI(fmt.Sprintf("%s", r.URL.Path))); err != nil {
			h.logger.Errorf("Can't save server activity %s: %s", a.GetType(), err)
		} else {
			a = i.(ap.Activity)
		}
	}

	if actorNeedsSaving {
		var ok bool
		var repo app.CanSaveAccounts
		if repo, ok = app.ContextSaver(r.Context()); !ok {
			errFn(errors.NotValidf("unable get persistence repository"), "")
			return
		}
		if actorNeedsSaving && actor != nil {
			// @todo(marius): move this to its own function
			//if !actor.IsObject() {
			//	if actor, err = h.repo.client.LoadIRI(actor.GetLink()); err != nil || !actor.IsObject() {
			//		errFn(errors.NewNotFound(err, fmt.Sprintf("failed to load remote actor %s", actor.GetLink())), "")
			//		return
			//	}
			//	// @fixme :needs_queueing:
			//	if err = acc.FromActivityPub(actor); err != nil {
			//		errFn(errors.NewNotFound(err, fmt.Sprintf("failed to load account from remote actor %s", actor.GetLink())), "")
			//		return
			//	}
			//}

			if acc, err = repo.SaveAccount(*h.acc); err != nil {
				errFn(errors.NewNotFound(err, fmt.Sprintf("failed to save local account for remote actor")), "")
				return
			}
			a.Actor = loadAPPerson(acc)
		}
	}

	status, location = h.saveActivityContent(a, r, w)

	if err != nil {
		h.logger.WithContext(log.Ctx{
			"actor":   a.Actor.GetLink(),
			"object":  a.Object.GetLink(),
			"from":    r.RemoteAddr,
			"headers": r.Header,
			"err":     err,
			"trace":   errors.Details(err),
		}).Error("activity validation error")
		h.HandleError(w, r, err)
		return
	}

	w.Header().Add("Content-Type", "application/activity+json; charset=utf-8")
	if status == http.StatusCreated {
		w.Header().Add("Location", location)
	}
	w.WriteHeader(status)
	//w.Header().Set("X-Content-Type-Options", "nosniff")
	if status >= 400 {
		w.Write([]byte(`{"status": "nok"}`))
	} else if status < 300 {
		w.Write([]byte(`{"status": "ok"}`))
	}
}
