package wfe2

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/jmhodges/clock"
	corepb "github.com/letsencrypt/boulder/core/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// AccountGetter represents the ability to get an account by ID - either from the SA
// or from a cache.
type AccountGetter interface {
	GetRegistration(ctx context.Context, regID *sapb.RegistrationID, opts ...grpc.CallOption) (*corepb.Registration, error)
}

// accountCache is an implementation of AccountGetter that first tries a local
// in-memory cache, and if the account is not there, calls out to an underlying
// AccountGetter. It is safe for concurrent access so long as the underlying
// AccountGetter is.
type accountCache struct {
	// Note: This must be a regular mutex, not an RWMutex, because cache.Get()
	// actually mutates the lru.Cache (by updating the last-used info).
	sync.Mutex
	under    AccountGetter
	ttl      time.Duration
	cache    *lru.Cache
	clk      clock.Clock
	requests *prometheus.CounterVec
}

func NewAccountCache(
	under AccountGetter,
	maxEntries int,
	ttl time.Duration,
	clk clock.Clock,
	stats prometheus.Registerer,
) *accountCache {
	requestsCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_requests",
	}, []string{"status"})
	stats.MustRegister(requestsCount)
	return &accountCache{
		under:    under,
		ttl:      ttl,
		cache:    lru.New(maxEntries),
		clk:      clk,
		requests: requestsCount,
	}
}

type accountEntry struct {
	account *corepb.Registration
	expires time.Time
}

func (ac *accountCache) GetRegistration(ctx context.Context, regID *sapb.RegistrationID, opts ...grpc.CallOption) (*corepb.Registration, error) {
	ac.Lock()
	val, ok := ac.cache.Get(regID.Id)
	ac.Unlock()
	if !ok {
		ac.requests.WithLabelValues("miss").Inc()
		return ac.queryAndStore(ctx, regID)
	}
	entry, ok := val.(accountEntry)
	if !ok {
		ac.requests.WithLabelValues("wrongtype").Inc()
		return nil, fmt.Errorf("shouldn't happen: wrong type %T for cache entry", entry)
	}
	if entry.expires.Before(ac.clk.Now()) {
		// Note: this has a slight TOCTOU issue but it's benign. If the entry for this account
		// was expired off by some other goroutine and then a fresh one added, removing it a second
		// time will just cause a slightly lower cache rate.
		// We have to actively remove expired entries, because otherwise each retrieval counts as
		// a "use" and they won't exit the cache on their own.
		ac.Lock()
		ac.cache.Remove(regID.Id)
		ac.Unlock()
		ac.requests.WithLabelValues("expired").Inc()
		return ac.queryAndStore(ctx, regID)
	}
	if entry.account.Id != regID.Id {
		ac.requests.WithLabelValues("wrong id from cache").Inc()
		return nil, fmt.Errorf("shouldn't happen: wrong account ID. expected %d, got %d", regID.Id, entry.account.Id)
	}
	copied := new(corepb.Registration)
	proto.Merge(copied, entry.account)
	ac.requests.WithLabelValues("hit").Inc()
	return copied, nil
}

func (ac *accountCache) queryAndStore(ctx context.Context, regID *sapb.RegistrationID) (*corepb.Registration, error) {
	account, err := ac.under.GetRegistration(ctx, regID)
	if err != nil {
		return nil, err
	}
	if account.Id != regID.Id {
		ac.requests.WithLabelValues("wrong id from SA").Inc()
		return nil, fmt.Errorf("shouldn't happen: wrong account ID from backend. expected %d, got %d", regID.Id, account.Id)
	}
	// Make sure we have our own copy that no one has a pointer to.
	copied := new(corepb.Registration)
	proto.Merge(copied, account)
	ac.Lock()
	ac.cache.Add(regID.Id, accountEntry{
		account: copied,
		expires: ac.clk.Now().Add(ac.ttl),
	})
	ac.Unlock()
	return account, nil
}
