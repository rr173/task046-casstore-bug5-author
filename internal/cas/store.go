// Package cas implements an in-memory content-addressable block store.
//
// A block's address is the SHA-256 digest of its content. Identical content
// is stored once (deduplicated) and reference-counted; a physical block is
// removed only when its reference count reaches zero.
package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Errors returned by the store. Callers may distinguish a malformed hash
// (ErrInvalidHash) from a valid-but-absent one (ErrNotFound).
var (
	ErrNotFound    = errors.New("cas: block not found")
	ErrInvalidHash = errors.New("cas: invalid hash format")
)

// sha256HexLen is the length of a SHA-256 digest in lowercase hex characters.
const sha256HexLen = 64

// Stats is a snapshot of the store's current state.
type Stats struct {
	Blocks int   // unique physical blocks currently held
	Bytes  int64 // total bytes of unique physical blocks
	Refs   int   // sum of reference counts (logical references)
	Puts   int64 // cumulative successful Put operations
}

// Store is an in-memory content-addressable block store keyed by SHA-256.
type Store struct {
	mu     sync.Mutex
	blocks map[string][]byte // hash -> content (defensively copied)
	refs   map[string]int    // hash -> reference count
	size   int64             // sum of unique block content lengths
	puts   int64             // cumulative Put calls
}

// New returns an empty content-addressable store.
func New() *Store {
	return &Store{
		blocks: make(map[string][]byte),
		refs:   make(map[string]int),
	}
}

// HashOf returns the lowercase SHA-256 hex digest of content.
func HashOf(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// ValidateHash canonicalizes and validates a hash argument. It lowercases
// the input and requires exactly 64 hexadecimal characters. A malformed
// value yields ErrInvalidHash (never ErrNotFound).
func ValidateHash(h string) (string, error) {
	low := strings.ToLower(h)
	if len(low) != sha256HexLen {
		return "", ErrInvalidHash
	}
	for i := 0; i < len(low); i++ {
		c := low[i]
		if !isHexByte(c) {
			return "", ErrInvalidHash
		}
	}
	return low, nil
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

// Put stores content under its SHA-256 digest and returns the digest. If the
// content is already present, no new physical block is created; the existing
// block's reference count is incremented instead. The returned stored flag
// reports whether a new physical block was created. Empty content is a valid
// block. The caller's slice is copied and may be freely mutated afterwards.
func (s *Store) Put(content []byte) (hash string, stored bool, err error) {
	h := HashOf(content)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	if _, ok := s.refs[h]; ok {
		s.refs[h]++
		return h, false, nil
	}
	c := make([]byte, len(content))
	copy(c, content)
	s.blocks[h] = c
	s.refs[h] = 1
	s.size += int64(len(c))
	return h, true, nil
}

// Get returns the content associated with hash. It returns ErrInvalidHash for
// a malformed hash and ErrNotFound for a valid but absent hash. The returned
// slice is a copy and may be modified freely.
func (s *Store) Get(hash string) ([]byte, error) {
	h, err := ValidateHash(hash)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.blocks[h]
	if !ok {
		return nil, ErrNotFound
	}
	var out []byte
	if len(c) > 0 {
		out = make([]byte, len(c))
		copy(out, c)
	}
	return out, nil
}

// Has reports whether hash refers to a currently-held block.
func (s *Store) Has(hash string) (bool, error) {
	h, err := ValidateHash(hash)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.blocks[h]
	return ok, nil
}

// Delete decrements the reference count of hash. The physical block is
// removed only when the count reaches zero. removed reports whether the
// physical block was removed by this call. Returns ErrNotFound for a valid
// but absent hash.
func (s *Store) Delete(hash string) (removed bool, err error) {
	_, err = ValidateHash(hash)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.refs[hash]
	if !ok {
		return false, ErrNotFound
	}
	n--
	if n <= 0 {
		c := s.blocks[hash]
		delete(s.blocks, hash)
		delete(s.refs, hash)
		s.size -= int64(len(c))
		return true, nil
	}
	s.refs[hash] = n
	return false, nil
}

// Stats returns a snapshot of store statistics.
func (s *Store) Stats() Stats {
	refs := 0
	for _, n := range s.refs {
		refs += n
	}
	return Stats{
		Blocks: len(s.blocks),
		Bytes:  s.size,
		Refs:   refs,
		Puts:   s.puts,
	}
}

// List returns the digests of all currently-held blocks, sorted ascending.
func (s *Store) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for h := range s.blocks {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// Audit verifies the content-addressable invariant: every stored block's key
// equals the SHA-256 of its content. It returns the number of blocks checked
// and an error describing the first violation, if any.
func (s *Store) Audit() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, c := range s.blocks {
		if len(c) == 0 {
			continue
		}
		if HashOf(c) != h {
			return len(s.blocks), fmt.Errorf("cas: integrity violation for %s", h)
		}
	}
	return len(s.blocks), nil
}
