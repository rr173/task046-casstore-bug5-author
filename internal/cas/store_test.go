package cas

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPutDedup(t *testing.T) {
	s := New()
	h1, stored, err := s.Put([]byte("data"))
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("first put should store a new block")
	}
	h2, stored2, err := s.Put([]byte("data"))
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("dedup hash mismatch: %s != %s", h2, h1)
	}
	if stored2 {
		t.Fatal("second put of identical content should not store a new block")
	}
	st := s.Stats()
	if st.Blocks != 1 || st.Refs != 2 {
		t.Fatalf("stats=%+v, want blocks=1 refs=2", st)
	}
}

func TestEmptyContent(t *testing.T) {
	s := New()
	h, stored, err := s.Put([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("empty content should be stored as a new block")
	}
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if h != want {
		t.Errorf("empty hash=%s want %s", h, want)
	}
	g, err := s.Get(h)
	if err != nil {
		t.Fatalf("get empty: %v", err)
	}
	if len(g) != 0 {
		t.Errorf("get empty len=%d want 0", len(g))
	}
}

func TestGetNotFoundVsInvalid(t *testing.T) {
	s := New()
	s.Put([]byte("x"))

	// Valid hash, absent.
	absent := "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := s.Get(absent); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent: err=%v want ErrNotFound", err)
	}

	// Invalid formats.
	for _, bad := range []string{
		"zzz",   // wrong length
		"short", // wrong length
		strings.Repeat("g", 64), // 64 chars but non-hex
		"nothex" + strings.Repeat("a", 58), // 64 chars but non-hex prefix
	} {
		if _, err := s.Get(bad); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("bad %q: err=%v want ErrInvalidHash", bad, err)
		}
	}
}

func TestUppercaseHashNormalized(t *testing.T) {
	s := New()
	h, _, _ := s.Put([]byte("up"))
	upper := strings.ToUpper(h)
	g, err := s.Get(upper)
	if err != nil {
		t.Fatalf("get uppercase: %v", err)
	}
	if !bytes.Equal(g, []byte("up")) {
		t.Errorf("content mismatch: %q", g)
	}
}

func TestDeleteRefcount(t *testing.T) {
	s := New()
	h, _, _ := s.Put([]byte("rc"))
	s.Put([]byte("rc"))
	s.Put([]byte("rc")) // refs = 3

	rem, err := s.Delete(h)
	if err != nil || rem {
		t.Fatalf("delete #1: rem=%v err=%v (want false nil)", rem, err)
	}
	if has, _ := s.Has(h); !has {
		t.Fatal("absent after delete #1 (refs was 3 -> 2)")
	}
	rem, _ = s.Delete(h)
	if rem {
		t.Fatal("delete #2 should not remove (refs 2 -> 1)")
	}
	rem, _ = s.Delete(h)
	if !rem {
		t.Fatal("delete #3 should remove (refs 1 -> 0)")
	}
	if has, _ := s.Has(h); has {
		t.Fatal("still present after delete #3")
	}
	rem, err = s.Delete(h)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("delete after removal: err=%v want ErrNotFound", err)
	}
}

func TestStatsBytesUnique(t *testing.T) {
	s := New()
	s.Put([]byte("aaaaa")) // 5 bytes
	s.Put([]byte("aaaaa")) // dedup
	s.Put([]byte("bb"))    // 2 bytes
	st := s.Stats()
	if st.Blocks != 2 || st.Bytes != 7 || st.Refs != 3 || st.Puts != 3 {
		t.Fatalf("stats=%+v, want blocks=2 bytes=7 refs=3 puts=3", st)
	}
}

func TestListSortedUnique(t *testing.T) {
	s := New()
	s.Put([]byte("c"))
	s.Put([]byte("a"))
	s.Put([]byte("b"))
	list := s.List()
	if len(list) != 3 {
		t.Fatalf("len=%d want 3", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1] > list[i] {
			t.Fatalf("not sorted: %v", list)
		}
	}
}

func TestAuditIntegrity(t *testing.T) {
	s := New()
	s.Put([]byte("hello"))
	s.Put([]byte("world"))
	n, err := s.Audit()
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if n != 2 {
		t.Errorf("audit n=%d want 2", n)
	}
}

func TestPutDoesNotRetainCallerSlice(t *testing.T) {
	s := New()
	content := []byte("mutable")
	h, _, _ := s.Put(content)
	content[0] = 'X' // mutate caller's copy after Put
	g, _ := s.Get(h)
	if !bytes.Equal(g, []byte("mutable")) {
		t.Errorf("stored content mutated via caller slice: %q", g)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	s := New()
	h, _, _ := s.Put([]byte("orig"))
	g, _ := s.Get(h)
	g[0] = 'X' // mutate returned slice
	g2, _ := s.Get(h)
	if !bytes.Equal(g2, []byte("orig")) {
		t.Errorf("internal block mutated via Get: %q", g2)
	}
}

func TestHashOfKnown(t *testing.T) {
	// SHA-256 of "hello"
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got := HashOf([]byte("hello")); got != want {
		t.Errorf("HashOf(hello)=%s want %s", got, want)
	}
}

func TestValidateHash(t *testing.T) {
	good := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if _, err := ValidateHash(good); err != nil {
		t.Errorf("good: %v", err)
	}
	// uppercase normalizes to lowercase
	if g, err := ValidateHash(strings.ToUpper(good)); err != nil || g != good {
		t.Errorf("uppercase: g=%s err=%v", g, err)
	}
	for _, bad := range []string{"", "short", strings.Repeat("z", 64)} {
		if _, err := ValidateHash(bad); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("bad %q: err=%v want ErrInvalidHash", bad, err)
		}
	}
}
