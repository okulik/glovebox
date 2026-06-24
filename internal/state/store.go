package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/okulik/glovebox/internal/agent"
	"github.com/okulik/glovebox/internal/manifest"
)

// Record is the persisted state for one project: the live manifest plus the
// outcome of the most recent apply, and any pending proposal.
type Record struct {
	Manifest     *manifest.Manifest `json:"manifest"`
	ManifestYAML string             `json:"manifest_yaml,omitempty"`
	Proposed     *manifest.Manifest `json:"proposed,omitempty"`
	ProposedYAML string             `json:"proposed_yaml,omitempty"`
	LastApply    ApplyResult        `json:"last_apply"`
}

// ApplyResult captures the status of the last apply attempt.
type ApplyResult struct {
	Time   time.Time `json:"time"`
	Status string    `json:"status"`
	Reason string    `json:"reason,omitempty"`
}

// Store is a file-backed map from project id to Record. Concurrent calls are
// serialized via a mutex; writes are atomic via tempfile + rename.
type Store struct {
	data map[string]*Record
	path string
	mu   sync.Mutex
}

// Open loads (or creates) a store at the given path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: path, data: map[string]*Record{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	return s, nil
}

// Path returns the path the store was opened from.
func (s *Store) Path() string { return s.path }

// SaveApplied updates the live manifest (parsed + raw YAML) and last_apply for
// a project. Save is the YAML-less wrapper kept for existing callers.
func (s *Store) SaveApplied(pid string, m *manifest.Manifest, yaml, status, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.data[pid]
	if r == nil {
		r = &Record{}
	}
	if m != nil {
		r.Manifest = m
		r.ManifestYAML = yaml
	}
	r.LastApply = ApplyResult{Status: status, Reason: reason, Time: time.Now()}
	s.data[pid] = r
	return s.flush()
}

// Save updates the live manifest (if non-nil) and last_apply for a project.
func (s *Store) Save(pid string, m *manifest.Manifest, status, reason string) error {
	return s.SaveApplied(pid, m, "", status, reason)
}

// SaveProposed stores a pending proposal (parsed + raw YAML) for a project.
func (s *Store) SaveProposed(pid string, m *manifest.Manifest, yaml string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.data[pid]
	if r == nil {
		r = &Record{}
	}
	r.Proposed = m
	r.ProposedYAML = yaml
	s.data[pid] = r
	return s.flush()
}

// ClearProposed removes any pending proposal for a project. A missing record
// is a no-op.
func (s *Store) ClearProposed(pid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.data[pid]
	if r == nil {
		return nil
	}
	r.Proposed = nil
	r.ProposedYAML = ""
	return s.flush()
}

// Delete removes a project entirely.
func (s *Store) Delete(pid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, pid)
	return s.flush()
}

// Get returns a copy of the record for a project, plus an `ok` flag. Callers
// receive a value (not a shared pointer) so concurrent Save calls cannot mutate
// the returned record's fields under them.
func (s *Store) Get(pid string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[pid]
	if !ok {
		return Record{}, false
	}
	return *r, true
}

// All returns a snapshot of all records, by value, for the same reason Get
// returns by value: the caller can't race against Save through a shared pointer.
func (s *Store) All() map[string]Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Record, len(s.data))
	for k, v := range s.data {
		out[k] = *v
	}
	return out
}

func (s *Store) flush() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return agent.WriteAtomic(s.path, b, 0o600)
}
