package services

import "sync"

// syncMap wraps sync.Map to avoid exposing the underlying type.
type syncMap struct {
	m sync.Map
}

func (s *syncMap) LoadOrStore(key, value any) (actual any, loaded bool) {
	return s.m.LoadOrStore(key, value)
}
