//go:build !darwin && !linux

package hotspot

import "log"

// StubManager is a no-op hotspot manager for unsupported platforms and dev mode.
type StubManager struct{}

func NewManager() Manager {
	return &StubManager{}
}

func (s *StubManager) Start(cfg Config) error {
	log.Println("Hotspot: stub manager (no-op)")
	return nil
}

func (s *StubManager) Stop() error {
	return nil
}

func (s *StubManager) Status() (Status, error) {
	return Status{Active: false}, nil
}
