package store

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/Annany2002/velox-db/internal/zset"
)

// Store is the main data store for the database. It is thread-safe.
type Store struct {
	mu          sync.RWMutex
	data        map[string]interface{}
	expirations map[string]time.Time
}

// New creates a new Store
func New() *Store {
	return &Store{
		data:        make(map[string]interface{}),
		expirations: map[string]time.Time{},
	}
}

// isExpired checks if a key is expired and deletes it if it is.
// This is a helper for "passive expiration". It must be called from within a lock.
func (s *Store) IsExpired(key string) bool {
	expiry, ok := s.expirations[key]
	if ok && time.Now().After(expiry) {
		delete(s.data, key)
		delete(s.expirations, key)
		return true
	}
	return false
}

// SetExpiry sets an expiration time for a key.
func (s *Store) SetExpiry(key string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[key] // Touch the key to ensure it exists
	if !ok {
		return
	}
	s.expirations[key] = t
}

// GetExpiry retrieves the expiration time for a key.
func (s *Store) GetExpiry(key string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.expirations[key]
	return t, ok
}

// RandomKeysWithExpiry returns a sample of keys that have expirations.
// This is used by the active expiration janitor.
func (s *Store) RandomKeysWithExpiry(count int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, count)
	// In a real high-performance scenario, iterating all keys is bad.
	// For our purpose, we'll iterate but stop after finding `count` keys.
	// A better approach would be a separate data structure or random sampling.
	for key := range s.expirations {
		keys = append(keys, key)
		if len(keys) >= count {
			break
		}
	}
	return keys
}

// Set stores a string value for a key.
func (s *Store) Set(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	delete(s.expirations, key)
}

// Get retrieves a string value for a key.
func (s *Store) Get(key string) ([]byte, bool) {
	s.mu.Lock() // Use write lock to allow for deletion
	defer s.mu.Unlock()

	if s.IsExpired(key) {
		return nil, false
	}

	raw, ok := s.data[key]
	if !ok {
		return nil, false
	}
	val, ok := raw.([]byte)
	return val, ok
}

// Delete removes a key.
func (s *Store) Del(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[key]
	if ok {
		delete(s.data, key)
		delete(s.expirations, key)
	}
	return ok
}

// GetList retrieves a list value for a key.
func (s *Store) GetList(key string) (*list.List, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsExpired(key) {
		return nil, false
	}

	raw, ok := s.data[key]
	if !ok {
		return nil, false
	}
	l, ok := raw.(*list.List)
	return l, ok
}

// GetOrCreateList retrieves a list or creates it if it doesn't exist.
// This must be called within a write lock.
func (s *Store) GetOrCreateList(key string) (*list.List, error) {
	raw, ok := s.data[key]
	if !ok {
		// If the key doesn't exist, create a new list
		l := list.New()
		s.data[key] = l
		return l, nil
	}

	l, ok := raw.(*list.List)
	if !ok {
		return nil, fmt.Errorf("key holds a non-list value")
	}
	return l, nil
}

// GetOrCreateHash retrieves a hash or creates it if it doesn't exist.
// This must be called within a write lock.
func (s *Store) GetOrCreateHash(key string) (map[string][]byte, error) {
	raw, ok := s.data[key]
	if !ok {
		// If the key doesn't exist, create a new hash.
		h := make(map[string][]byte)
		s.data[key] = h
		return h, nil
	}

	// If the key exists, ensure it's a hash.
	h, ok := raw.(map[string][]byte)
	if !ok {
		return nil, fmt.Errorf("key holds a non-hash value")
	}
	return h, nil
}

// GetHash retrieves a hash value for a key.
// The first bool indicates if the key exists and is a hash.
func (s *Store) GetHash(key string) (map[string][]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsExpired(key) {
		return nil, false
	}

	raw, ok := s.data[key]
	if !ok {
		return nil, false
	}
	h, ok := raw.(map[string][]byte)
	return h, ok
}

// GetOrCreateSet retrieves a set or creates it if it doesn't exist.
// This must be called within a write lock.
func (s *Store) GetOrCreateSet(key string) (map[string]struct{}, error) {
	raw, ok := s.data[key]
	if !ok {
		// If the key doesn't exist, create a new set.
		set := make(map[string]struct{})
		s.data[key] = set
		return set, nil
	}

	// If the key exists, ensure it's a set.
	set, ok := raw.(map[string]struct{})
	if !ok {
		return nil, fmt.Errorf("key holds a non-set value")
	}
	return set, nil
}

// GetSet retrieves a set value for a key.
// The first bool indicates if the key exists and is a set.
func (s *Store) GetSet(key string) (map[string]struct{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsExpired(key) {
		return nil, false
	}

	raw, ok := s.data[key]
	if !ok {
		return nil, false
	}
	set, ok := raw.(map[string]struct{})
	return set, ok
}

// ForEach iterates over the data store. A read lock is held during iteration.
func (s *Store) ForEach(f func(key string, value interface{})) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, value := range s.data {
		f(key, value)
	}
}

// GetOrCreateZSet retrieves a sorted set or creates it if it doesn't exist.
// This must be called within a write lock.
func (s *Store) GetOrCreateZSet(key string) (*zset.ZSet, error) {
	raw, ok := s.data[key]
	if !ok {
		z := zset.NewZSet()
		s.data[key] = z
		return z, nil
	}
	z, ok := raw.(*zset.ZSet)
	if !ok {
		return nil, fmt.Errorf("key holds a non-zset value")
	}
	return z, nil
}

// GetZSet retrieves a sorted set for a key.
func (s *Store) GetZSet(key string) (*zset.ZSet, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsExpired(key) {
		return nil, false
	}

	raw, ok := s.data[key]
	if !ok {
		return nil, false
	}
	z, ok := raw.(*zset.ZSet)
	return z, ok
}

// Lock acquires a write lock on the store.
func (s *Store) Lock() {
	s.mu.Lock()
}

// Unlock releases a write lock.
func (s *Store) Unlock() {
	s.mu.Unlock()
}
