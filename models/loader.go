package models

import (
	"database/sql"
	"github.com/juju/errors"
	"net/http"
	"golang.org/x/net/context"
	"time"
	)

var Db *sql.DB

// Loader middleware
func Loader (next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := context.WithValue(ctx, "loader", Service)
		next.ServeHTTP(w, r.WithContext(newCtx))
	}
	return http.HandlerFunc(fn)
}

type MatchType string

const (
	MatchEquals = MatchType(1 << iota)
	MatchBefore
	MatchAfter
	MatchFuzzy
)

type LoadItemsFilter struct {
	Key []string
	Type []string
	MediaType []string
	SubmittedBy []string
	Parent string
	OP string
	SubmittedAt time.Time
	SubmittedAtMatchType MatchType
	Content string
	ContentMatchType MatchType
	MaxItems int
}

type LoadItemFilter struct {
	Key string
}

type CanLoad interface {
	LoadItem(f LoadItemFilter) (Item, error)
	LoadItems(f LoadItemsFilter) (ItemCollection, error)
}

var Service LoaderService

type LoaderService struct {
	DB *sql.DB
}

func (l LoaderService) LoadItem(f LoadItemFilter) (Item, error) {
	return Item{}, errors.Errorf("not implemented")
}

func (l LoaderService) LoadItems(f LoadItemsFilter) (ItemCollection, error) {
	return LoadItems(f.MaxItems)
}