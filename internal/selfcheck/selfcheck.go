// Package selfcheck runs an end-to-end verification of the cas store. It is
// invoked by the --smoke-test flag and exits the process on completion.
package selfcheck

import (
	"errors"
	"fmt"
	"strings"

	"task046-casstore/internal/cas"
)

// Run exercises the content-addressable store across isolated scenarios,
// returning nil if every behavior matches the specification.
func Run() error {
	scenarios := []struct {
		name string
		fn   func() error
	}{
		{"写入与读取", scenarioPutGet},
		{"引用计数去重", scenarioDedup},
		{"空内容作为合法块", scenarioEmpty},
		{"删除引用计数递减", scenarioDeleteRefcount},
		{"格式错误与未找到区分", scenarioErrorSemantics},
		{"大写摘要规范化", scenarioUppercase},
		{"内容寻址不变式自审", scenarioAudit},
		{"列举排序与唯一", scenarioList},
		{"统计唯一物理块", scenarioStats},
		{"写入与读取的副本隔离", scenarioCopyIsolation},
	}
	for _, sc := range scenarios {
		if err := sc.fn(); err != nil {
			return fmt.Errorf("%s: %w", sc.name, err)
		}
	}
	return nil
}

func scenarioPutGet() error {
	s := cas.New()
	h, stored, err := s.Put([]byte("hello"))
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}
	if !stored {
		return fmt.Errorf("first put should create a new block")
	}
	got, err := s.Get(h)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if string(got) != "hello" {
		return fmt.Errorf("get = %q", string(got))
	}
	if has, _ := s.Has(h); !has {
		return fmt.Errorf("Has should be true after Put")
	}
	return nil
}

func scenarioDedup() error {
	s := cas.New()
	h1, stored1, _ := s.Put([]byte("dup"))
	h2, stored2, _ := s.Put([]byte("dup"))
	if h1 != h2 {
		return fmt.Errorf("dedup hash mismatch: %s != %s", h2, h1)
	}
	if !stored1 {
		return fmt.Errorf("first put should store")
	}
	if stored2 {
		return fmt.Errorf("second put of identical content should not store a new block")
	}
	st := s.Stats()
	if st.Blocks != 1 {
		return fmt.Errorf("blocks=%d want 1 (deduplicated)", st.Blocks)
	}
	if st.Refs != 2 {
		return fmt.Errorf("refs=%d want 2", st.Refs)
	}
	if st.Bytes != int64(len("dup")) {
		return fmt.Errorf("bytes=%d want %d (unique only)", st.Bytes, len("dup"))
	}
	return nil
}

func scenarioEmpty() error {
	s := cas.New()
	h, stored, err := s.Put([]byte{})
	if err != nil {
		return fmt.Errorf("put empty: %w", err)
	}
	if !stored {
		return fmt.Errorf("empty content should create a new block")
	}
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if h != want {
		return fmt.Errorf("empty hash=%s want canonical %s", h, want)
	}
	g, err := s.Get(h)
	if err != nil {
		return fmt.Errorf("get empty: %w", err)
	}
	if len(g) != 0 {
		return fmt.Errorf("get empty len=%d want 0", len(g))
	}
	// Empty dedups too.
	h2, stored2, _ := s.Put([]byte{})
	if h2 != h {
		return fmt.Errorf("second empty hash mismatch")
	}
	if stored2 {
		return fmt.Errorf("second empty put should dedup, not store")
	}
	return nil
}

func scenarioDeleteRefcount() error {
	s := cas.New()
	h, _, _ := s.Put([]byte("rc"))
	s.Put([]byte("rc")) // refs = 2

	rem, err := s.Delete(h)
	if err != nil {
		return fmt.Errorf("delete #1: %w", err)
	}
	if rem {
		return fmt.Errorf("delete #1 should not remove (refs 2 -> 1)")
	}
	if has, _ := s.Has(h); !has {
		return fmt.Errorf("block absent after delete #1")
	}
	rem, err = s.Delete(h)
	if err != nil {
		return fmt.Errorf("delete #2: %w", err)
	}
	if !rem {
		return fmt.Errorf("delete #2 should remove (refs 1 -> 0)")
	}
	if has, _ := s.Has(h); has {
		return fmt.Errorf("block still present after full delete")
	}
	if _, err := s.Get(h); !errors.Is(err, cas.ErrNotFound) {
		return fmt.Errorf("get after full delete: err=%v want ErrNotFound", err)
	}
	// Deleting a valid but absent hash yields ErrNotFound.
	absent := "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := s.Delete(absent); !errors.Is(err, cas.ErrNotFound) {
		return fmt.Errorf("delete absent: err=%v want ErrNotFound", err)
	}
	return nil
}

func scenarioErrorSemantics() error {
	s := cas.New()
	s.Put([]byte("x"))
	// Valid but absent.
	absent := "1111111111111111111111111111111111111111111111111111111111111111"
	if _, err := s.Get(absent); !errors.Is(err, cas.ErrNotFound) {
		return fmt.Errorf("absent get: err=%v want ErrNotFound", err)
	}
	// Malformed: wrong length, non-hex chars.
	for _, bad := range []string{"zzz", "short", strings.Repeat("g", 64), "nothex" + strings.Repeat("a", 58)} {
		if _, err := s.Get(bad); !errors.Is(err, cas.ErrInvalidHash) {
			return fmt.Errorf("get %q: err=%v want ErrInvalidHash", bad, err)
		}
		if _, err := s.Has(bad); !errors.Is(err, cas.ErrInvalidHash) {
			return fmt.Errorf("has %q: err=%v want ErrInvalidHash", bad, err)
		}
		if _, err := s.Delete(bad); !errors.Is(err, cas.ErrInvalidHash) {
			return fmt.Errorf("delete %q: err=%v want ErrInvalidHash", bad, err)
		}
	}
	return nil
}

func scenarioUppercase() error {
	s := cas.New()
	h, _, _ := s.Put([]byte("up"))
	upper := strings.ToUpper(h)
	g, err := s.Get(upper)
	if err != nil {
		return fmt.Errorf("get uppercase: %w", err)
	}
	if string(g) != "up" {
		return fmt.Errorf("uppercase get content mismatch: %q", g)
	}
	// A deleted block queried via uppercase still classifies as NotFound,
	// proving uppercase is normalized rather than rejected as invalid.
	s.Delete(h)
	if _, err := s.Get(upper); !errors.Is(err, cas.ErrNotFound) {
		return fmt.Errorf("uppercase of deleted: err=%v want ErrNotFound", err)
	}
	return nil
}

func scenarioAudit() error {
	s := cas.New()
	s.Put([]byte("hello"))
	s.Put([]byte("world"))
	s.Put([]byte{})
	n, err := s.Audit()
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if n != 3 {
		return fmt.Errorf("audit n=%d want 3", n)
	}
	if n != s.Stats().Blocks {
		return fmt.Errorf("audit count %d != blocks %d", n, s.Stats().Blocks)
	}
	return nil
}

func scenarioList() error {
	s := cas.New()
	s.Put([]byte("c"))
	s.Put([]byte("a"))
	s.Put([]byte("b"))
	s.Put([]byte("a")) // dedup, must not appear twice
	list := s.List()
	if len(list) != 3 {
		return fmt.Errorf("len=%d want 3", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1] > list[i] {
			return fmt.Errorf("not sorted: %v", list)
		}
	}
	seen := map[string]bool{}
	for _, h := range list {
		if len(h) != 64 {
			return fmt.Errorf("bad hash len: %s", h)
		}
		if seen[h] {
			return fmt.Errorf("duplicate in list: %s", h)
		}
		seen[h] = true
	}
	return nil
}

func scenarioStats() error {
	s := cas.New()
	s.Put([]byte("aaaaa")) // 5
	s.Put([]byte("aaaaa")) // dedup
	s.Put([]byte("bb"))    // 2
	s.Put([]byte{})        // 0
	st := s.Stats()
	if st.Blocks != 3 {
		return fmt.Errorf("blocks=%d want 3", st.Blocks)
	}
	if st.Bytes != 7 {
		return fmt.Errorf("bytes=%d want 7 (5+2+0, unique)", st.Bytes)
	}
	if st.Refs != 4 {
		return fmt.Errorf("refs=%d want 4", st.Refs)
	}
	if st.Puts != 4 {
		return fmt.Errorf("puts=%d want 4", st.Puts)
	}
	return nil
}

func scenarioCopyIsolation() error {
	s := cas.New()
	content := []byte("mutable")
	h, _, _ := s.Put(content)
	content[0] = 'X' // mutate caller's slice after Put
	g, _ := s.Get(h)
	if string(g) != "mutable" {
		return fmt.Errorf("stored content mutated via caller slice: %q", g)
	}
	// Mutating the returned slice must not affect the stored block.
	g[0] = 'Y'
	g2, _ := s.Get(h)
	if string(g2) != "mutable" {
		return fmt.Errorf("internal block mutated via Get: %q", g2)
	}
	return nil
}
