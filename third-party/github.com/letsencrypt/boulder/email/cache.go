package email

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/golang/groupcache/lru"
	"github.com/prometheus/client_golang/prometheus"
)

type EmailCache struct {
	sync.Mutex
	cache    *lru.Cache
	requests *prometheus.CounterVec
}

func NewHashedEmailCache(maxEntries int, stats prometheus.Registerer) *EmailCache {
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "email_cache_requests",
	}, []string{"status"})
	stats.MustRegister(requests)

	return &EmailCache{
		cache:    lru.New(maxEntries),
		requests: requests,
	}
}

func hashEmail(email string) string {
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:])
}

func (c *EmailCache) Seen(email string) bool {
	if c == nil {
		// If the cache is nil we assume it was not configured.
		return false
	}

	hash := hashEmail(email)

	c.Lock()
	defer c.Unlock()

	_, ok := c.cache.Get(hash)
	if !ok {
		c.requests.WithLabelValues("miss").Inc()
		return false
	}

	c.requests.WithLabelValues("hit").Inc()
	return true
}

func (c *EmailCache) Remove(email string) {
	if c == nil {
		// If the cache is nil we assume it was not configured.
		return
	}

	hash := hashEmail(email)

	c.Lock()
	defer c.Unlock()

	c.cache.Remove(hash)
}

// StoreIfAbsent stores the email in the cache if it is not already present, as
// a single atomic operation. It returns true if the email was stored and false
// if it was already in the cache. If the cache is nil, true is always returned.
func (c *EmailCache) StoreIfAbsent(email string) bool {
	if c == nil {
		// If the cache is nil we assume it was not configured.
		return true
	}

	hash := hashEmail(email)

	c.Lock()
	defer c.Unlock()

	_, ok := c.cache.Get(hash)
	if ok {
		c.requests.WithLabelValues("hit").Inc()
		return false
	}
	c.cache.Add(hash, nil)
	c.requests.WithLabelValues("miss").Inc()
	return true
}
