package cas

import "testing"

func TestProbeAuditDetectsCorruptEmptyBlock(t *testing.T) {

	s := New()
	const badHash = "not-the-sha256-of-an-empty-block"
	s.blocks[badHash] = []byte{}
	s.refs[badHash] = 1
	if _, err := s.Audit(); err == nil {
		t.Fatal("Audit must reject a stored empty block under a non-content hash")
	}
}
