// Package compare — storage layer for comparison records and leaderboards.
package compare

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager owns persistence for comparison records and Elo leaderboards.
// All data lives under <repoPath>/.forge_compare so it is scoped to the repo.
type Manager struct {
	dir string
	mu  sync.Mutex
}

// NewManager creates the compare storage directory tree.
func NewManager(repoPath string) *Manager {
	dir := filepath.Join(repoPath, ".forge_compare")
	_ = os.MkdirAll(dir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "records"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "datasets"), 0755)
	return &Manager{dir: dir}
}

// Path returns the root storage path for diagnostics / mount checks.
func (m *Manager) Path() string { return m.dir }

// Save persists a record to disk.
func (m *Manager) Save(r *Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir, "records", r.ID+".json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write record: %w", err)
	}
	return nil
}

// Load reads a record by id.
func (m *Manager) Load(id string) (*Record, error) {
	path := filepath.Join(m.dir, "records", id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read record: %w", err)
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode record: %w", err)
	}
	return &r, nil
}

// ListRecords returns all stored records, newest first.
func (m *Manager) ListRecords() ([]*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(m.dir, "records"))
	if err != nil {
		return nil, err
	}
	var records []*Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, "records", e.Name()))
		if err != nil {
			continue
		}
		var r Record
		if err := json.Unmarshal(data, &r); err != nil {
			continue
		}
		records = append(records, &r)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}

// Summary returns leaderboard plus aggregate counts.
func (m *Manager) Summary() (*Summary, error) {
	lb := m.LoadLeaderboard()
	records, err := m.ListRecords()
	if err != nil {
		return nil, err
	}
	votes := 0
	for _, r := range records {
		if r.Vote != nil {
			votes++
		}
	}
	return &Summary{
		Leaderboard:      lb,
		TotalVotes:       votes,
		TotalComparisons: len(records),
	}, nil
}

func leaderboardPath() string { return "leaderboard.json" }

// LoadLeaderboard reads the leaderboard or returns a fresh one.
func (m *Manager) LoadLeaderboard() *Leaderboard {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir, leaderboardPath())
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultLeaderboard()
	}
	var lb Leaderboard
	if err := json.Unmarshal(data, &lb); err != nil {
		return defaultLeaderboard()
	}
	if lb.Models == nil {
		lb.Models = make(map[string]*Rating)
	}
	if lb.ByCapability == nil {
		lb.ByCapability = make(map[string]map[string]*Rating)
	}
	return &lb
}

// SaveLeaderboard persists the leaderboard.
func (m *Manager) SaveLeaderboard(lb *Leaderboard) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lb.UpdatedAt = time.Now()
	if lb.Models == nil {
		lb.Models = make(map[string]*Rating)
	}
	if lb.ByCapability == nil {
		lb.ByCapability = make(map[string]map[string]*Rating)
	}
	path := filepath.Join(m.dir, leaderboardPath())
	data, err := json.MarshalIndent(lb, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal leaderboard: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write leaderboard: %w", err)
	}
	return nil
}

func defaultLeaderboard() *Leaderboard {
	return &Leaderboard{
		Models:       make(map[string]*Rating),
		ByCapability: make(map[string]map[string]*Rating),
		UpdatedAt:    time.Now(),
	}
}

// rating returns a rating, creating it at the default rating if absent.
func rating(ratings map[string]*Rating, id string) *Rating {
	if ratings[id] == nil {
		ratings[id] = &Rating{Rating: defaultElo}
	}
	return ratings[id]
}

// generateID creates a time-sortable, collision-resistant identifier.
func generateID() string {
	b := make([]byte, 8)
	_, _ = randReader.Read(b)
	return fmt.Sprintf("%s-%x", time.Now().UTC().Format("20060102-150405"), b)
}

// ensure that the crypto import is used elsewhere; see manager.go.
