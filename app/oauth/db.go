package oauth

import (
	"encoding/json"
	"fmt"
	"github.com/go-pg/pg"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/openshift/osin"
	"net/http"
	"time"
)

type logger struct {
	l log.Logger
}

func (l logger) Printf(format string, v ...interface{}) {
	l.l.Debugf(format, v...)
}

func NewOAuth(db *pg.DB, l log.Logger) (*osin.Server, error) {
	config := osin.ServerConfig{
		AuthorizationExpiration:   86400,
		AccessExpiration:          2678400,
		TokenType:                 "Bearer",
		AllowedAuthorizeTypes:     osin.AllowedAuthorizeType{osin.CODE},
		AllowedAccessTypes:        osin.AllowedAccessType{osin.AUTHORIZATION_CODE},
		ErrorStatusCode:           http.StatusForbidden,
		AllowClientSecretInParams: false,
		AllowGetAccessRequest:     false,
		RetainTokenAfterRefresh:   true,
		RedirectUriSeparator:      "\n",
		//RequirePKCEForPublicClients: true,
	}
	store := New(db, l)
	s := osin.NewServer(&config, store)
	s.Logger = logger{l: l}
	return s, nil
}

// Storage implements interface "github.com/RangelReale/osin".Storage and interface "github.com/ory/osin-storage".Storage
type Storage struct {
	db *pg.DB
	l  log.Logger
}

// New returns a new postgres storage instance.
func New(db *pg.DB, l log.Logger) *Storage {
	return &Storage{db, l}
}

// Clone the storage if needed. For example, using mgo, you can clone the session with session.Clone
// to avoid concurrent access problems.
// This is to avoid cloning the connection at each method access.
// Can return itself if not a problem.
func (s *Storage) Clone() osin.Storage {
	return s
}

// Close the resources the Storage potentially holds (using Clone for example)
func (s *Storage) Close() {
	//s.db.Close()
}

type cl struct {
	Id          string
	Secret      string
	RedirectUri string
	Extra       json.RawMessage
}

// GetClient loads the client by id
func (s *Storage) GetClient(id string) (osin.Client, error) {
	q := "SELECT id, secret, redirect_uri, extra FROM client WHERE id=?"
	var cl cl
	var c osin.DefaultClient
	if _, err := s.db.QueryOne(&cl, q, id); err == pg.ErrNoRows {
		return nil, errors.NewNotFound(err, "")
	} else if err != nil {
		s.l.WithContext(log.Ctx{"id": id, "table": "client", "operation": "select"}).Error(err.Error())
		return &c, errors.Annotatef(err, "DB query error")
	}
	c.Id = cl.Id
	c.Secret = cl.Secret
	c.RedirectUri = cl.RedirectUri
	c.UserData = cl.Extra

	return &c, nil
}

// UpdateClient updates the client (identified by it's id) and replaces the values with the values of client.
func (s *Storage) UpdateClient(c osin.Client) error {
	data, err := assertToString(c.GetUserData())
	if err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId()}).Error(err.Error())
		return err
	}

	if _, err := s.db.Exec("UPDATE client SET (secret, redirect_uri, extra) = (?2, ?3, ?4) WHERE id=?1", c.GetId(), c.GetSecret(), c.GetRedirectUri(), data); err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId(), "table": "client", "operation": "update"}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	return nil
}

// CreateClient stores the client in the database and returns an error, if something went wrong.
func (s *Storage) CreateClient(c osin.Client) error {
	data, err := assertToString(c.GetUserData())
	if err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId()}).Error(err.Error())
		return err
	}

	if _, err := s.db.Exec("INSERT INTO client (id, secret, redirect_uri, extra) VALUES (?0, ?1, ?2, ?3)", c.GetId(), c.GetSecret(), c.GetRedirectUri(), data); err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId(), "redirect_uri": c.GetRedirectUri(), "table": "client", "operation": "insert"}).Errorf(err.Error())
		return errors.Annotate(err, "")
	}
	return nil
}

// RemoveClient removes a client (identified by id) from the database. Returns an error if something went wrong.
func (s *Storage) RemoveClient(id string) (err error) {
	if _, err = s.db.Exec("DELETE FROM client WHERE id=?", id); err != nil {
		s.l.WithContext(log.Ctx{"id": id, "table": "client", "operation": "delete"}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"id": id}).Debugf("removed client")
	return nil
}

// SaveAuthorize saves authorize data.
func (s *Storage) SaveAuthorize(data *osin.AuthorizeData) (err error) {
	extra, err := assertToString(data.UserData)
	if err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId(), "code": data.Code}).Error(err.Error())
		return err
	}

	var params = []interface{}{
		data.Client.GetId(),
		data.Code,
		data.ExpiresIn,
		data.Scope,
		data.RedirectUri,
		data.State,
		data.CreatedAt,
		extra,
	}

	if _, err = s.db.Query(nil, "INSERT INTO authorize (client, code, expires_in, scope, redirect_uri, state, created_at, extra) "+
		"VALUES (?0, ?1, ?2, ?3, ?4, ?5, ?6, ?7)", params...); err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId(), "table": "authorize", "operation": "insert", "code": data.Code}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	return nil
}

type auth struct {
	Client      string
	Code        string
	ExpiresIn   time.Duration
	Scope       string
	RedirectURI string
	State       string
	CreatedAt   time.Time
	Extra       json.RawMessage
}

// LoadAuthorize looks up AuthorizeData by a code.
// Client information MUST be loaded together.
// Optionally can return error if expired.
func (s *Storage) LoadAuthorize(code string) (*osin.AuthorizeData, error) {
	var data osin.AuthorizeData

	var auth auth
	q := "SELECT client, code, expires_in, scope, redirect_uri, state, created_at, extra FROM authorize WHERE code=? LIMIT 1"
	if _, err := s.db.QueryOne(&auth, q, code); err == pg.ErrNoRows {
		return nil, errors.NotFoundf("")
	} else if err != nil {
		s.l.WithContext(log.Ctx{"code": code, "table": "authorize", "operation": "select"}).Error(err.Error())
		return nil, errors.Annotate(err, "")
	}
	data.Code = auth.Code
	data.ExpiresIn = int32(auth.ExpiresIn)
	data.Scope = auth.Scope
	data.RedirectUri = auth.RedirectURI
	data.State = auth.State
	data.CreatedAt = auth.CreatedAt
	data.UserData = auth.Extra

	c, err := s.GetClient(auth.Client)
	if err != nil {
		return nil, err
	}

	if data.ExpireAt().Before(time.Now()) {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.Errorf("Token expired at %s.", data.ExpireAt().String())
	}

	data.Client = c
	return &data, nil
}

// RemoveAuthorize revokes or deletes the authorization code.
func (s *Storage) RemoveAuthorize(code string) (err error) {
	if _, err = s.db.Exec("DELETE FROM authorize WHERE code=?", code); err != nil {
		s.l.WithContext(log.Ctx{"code": code, "table": "authorize", "operation": "delete"}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"code": code}).Debugf("removed authorization token")
	return nil
}

// SaveAccess writes AccessData.
// If RefreshToken is not blank, it must save in a way that can be loaded using LoadRefresh.
func (s *Storage) SaveAccess(data *osin.AccessData) (err error) {
	prev := ""
	authorizeData := &osin.AuthorizeData{}

	if data.AccessData != nil {
		prev = data.AccessData.AccessToken
	}

	if data.AuthorizeData != nil {
		authorizeData = data.AuthorizeData
	}

	extra, err := assertToString(data.UserData)
	if err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}

	if data.RefreshToken != "" {
		if err := s.saveRefresh(tx, data.RefreshToken, data.AccessToken); err != nil {
			s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
			return err
		}
	}

	if data.Client == nil {
		return errors.New("data.Client must not be nil")
	}

	_, err = tx.Exec("INSERT INTO access (client, authorize, previous, access_token, refresh_token, expires_in, scope, redirect_uri, created_at, extra) VALUES (?0, ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)", data.Client.GetId(), authorizeData.Code, prev, data.AccessToken, data.RefreshToken, data.ExpiresIn, data.Scope, data.RedirectUri, data.CreatedAt, extra)
	if err != nil {
		if rbe := tx.Rollback(); rbe != nil {
			s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(rbe.Error())
			return errors.Annotate(rbe, "")
		}
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}

	if err = tx.Commit(); err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}

	return nil
}

type acc struct {
	Client       string
	Authorize    string
	Previous     string
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
	Scope        string
	RedirectURI  string
	CreatedAt    time.Time
	Extra        json.RawMessage
}

// LoadAccess retrieves access data by token. Client information MUST be loaded together.
// AuthorizeData and AccessData DON'T NEED to be loaded if not easily available.
// Optionally can return error if expired.
func (s *Storage) LoadAccess(code string) (*osin.AccessData, error) {
	var result osin.AccessData

	var acc acc
	q := "SELECT " +
		"client, authorize, previous, access_token, refresh_token, expires_in, scope, redirect_uri, created_at, extra " +
		"FROM access WHERE access_token=? LIMIT 1"
	if _, err := s.db.QueryOne(&acc, q, code); err == pg.ErrNoRows {
		return nil, errors.NewNotFound(err, "")
	} else if err != nil {
		return nil, errors.Annotate(err, "")
	}
	result.AccessToken = acc.AccessToken
	result.RefreshToken = acc.RefreshToken
	result.ExpiresIn = int32(acc.ExpiresIn)
	result.Scope = acc.Scope
	result.RedirectUri = acc.RedirectURI
	result.CreatedAt = acc.CreatedAt
	result.UserData = acc.Extra
	client, err := s.GetClient(acc.Client)
	if err != nil {
		s.l.WithContext(log.Ctx{"code": code, "table": "access", "operation": "select"}).Error(err.Error())
		return nil, err
	}

	result.Client = client
	result.AuthorizeData, _ = s.LoadAuthorize(acc.Authorize)
	prevAccess, _ := s.LoadAccess(acc.Previous)
	result.AccessData = prevAccess
	return &result, nil
}

// RemoveAccess revokes or deletes an AccessData.
func (s *Storage) RemoveAccess(code string) (err error) {
	_, err = s.db.Exec("DELETE FROM access WHERE access_token=?", code)
	if err != nil {
		s.l.WithContext(log.Ctx{"code": code, "table": "access", "operation": "delete"}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"code": code}).Debugf("removed access token")
	return nil
}

type ref struct {
	Access string
}

// LoadRefresh retrieves refresh AccessData. Client information MUST be loaded together.
// AuthorizeData and AccessData DON'T NEED to be loaded if not easily available.
// Optionally can return error if expired.
func (s *Storage) LoadRefresh(code string) (*osin.AccessData, error) {
	var ref ref
	q := "SELECT access FROM refresh WHERE token=? LIMIT 1"
	if _, err := s.db.QueryOne(&ref, q, code); err == pg.ErrNoRows {
		return nil, errors.NewNotFound(err, "")
	} else if err != nil {

		return nil, errors.Annotate(err, "")
	}
	return s.LoadAccess(ref.Access)
}

// RemoveRefresh revokes or deletes refresh AccessData.
func (s *Storage) RemoveRefresh(code string) error {
	_, err := s.db.Exec("DELETE FROM refresh WHERE token=?", code)
	if err != nil {
		s.l.WithContext(log.Ctx{"code": code, "table": "refresh", "operation": "delete"}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"code": code}).Debugf("removed refresh token")
	return nil
}

func (s *Storage) saveRefresh(tx *pg.Tx, refresh, access string) (err error) {
	_, err = tx.Exec("INSERT INTO refresh (token, access) VALUES (?0, ?1)", refresh, access)
	if err != nil {
		if rbe := tx.Rollback(); rbe != nil {
			s.l.WithContext(log.Ctx{"code": access, "table": "refresh", "operation": "insert"}).Error(rbe.Error())
			return errors.Annotate(rbe, "")
		}
		return errors.Annotate(err, "")
	}
	return nil
}

func assertToString(in interface{}) (string, error) {
	var ok bool
	var data string
	if in == nil {
		return "", nil
	} else if data, ok = in.(string); ok {
		return data, nil
	} else if byt, ok := in.([]byte); ok {
		return string(byt), nil
	} else if byt, ok := in.(json.RawMessage); ok {
		return string(byt), nil
	} else if str, ok := in.(fmt.Stringer); ok {
		return str.String(), nil
	}
	return "", errors.Errorf(`Could not assert "%v" to string`, in)
}
