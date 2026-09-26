package coupon

import "context"

// MemoryStore is a fixed set of valid codes, empty by default. It is
// read-only after construction, so it needs no locking.
type MemoryStore struct {
	codes map[string]struct{}
}

func NewMemoryStore(codes ...string) *MemoryStore {
	set := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		set[c] = struct{}{}
	}
	return &MemoryStore{codes: set}
}

func (s *MemoryStore) Has(_ context.Context, code string) (bool, error) {
	_, ok := s.codes[code]
	return ok, nil
}
