package store

import (
	"container/list"
	"fmt"
	"sync"
)

// Store is the main data store for the database. It is thread-safe.
type Store struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

// New creates a new Store
func New() *Store {
	return &Store{
		data: make(map[string]interface{}),
	}
}

// Set stores a string value for a key.
func (s *Store) Set(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get retrieves a string value for a key.
func (s *Store) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	}
	return ok
}

// GetList retrieves a list value for a key.
func (s *Store) GetList(key string) (*list.List, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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

// ForEach iterates over the data store. A read lock is held during iteration.
func (s *Store) ForEach(f func(key string, value interface{})) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, value := range s.data {
		f(key, value)
	}
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.data[key]
	if !ok {
		return nil, false
	}
	h, ok := raw.(map[string][]byte)
	return h, ok
}

// Lock acquires a write lock on the store.
func (s *Store) Lock() {
	s.mu.Lock()
}

// Unlock releases a write lock.
func (s *Store) Unlock() {
	s.mu.Unlock()
}