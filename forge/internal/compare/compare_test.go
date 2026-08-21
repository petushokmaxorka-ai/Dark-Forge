// Package compare tests — preference collection, Elo, and DPO export.
package compare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEloUpdate(t *testing.T) {
	ratings := map[string]*Rating{}
	updatePair(ratings, "kimi", "qwable")

	if ratings["kimi"].Rating <= 1500 {
		t.Fatalf("winner should gain points: got %v", ratings["kimi"].Rating)
	}
	if ratings["qwable"].Rating >= 1500 {
		t.Fatalf("loser should lose points: got %v", ratings["qwable"].Rating)
	}
	if ratings["kimi"].Wins != 1 || ratings["qwable"].Losses != 1 {
		t.Fatalf("win/loss counters wrong: %+v", ratings)
	}
}

func TestRecordSaveAndVote(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	id := GenerateID()
	record := &Record{
		ID: id,
		Prompt: "какой цикл for в Go?",
		TaskType: "code",
		Complexity: 3,
		CreatedAt: time.Now(),
		Participants: []Participant{{ID: "qwable", Name: "Qwable-9B", Kind: "local"}},
		Shuffled: []Entry{
			{Label: "A", Text: "for i := 0; i < n; i++"},
		},
		Results: []Entry{
			{Label: "A", ModelID: "qwable", ModelName: "Qwable-9B", Kind: "local", Text: "for i := 0; i < n; i++"},
		},
	}
	if err := m.Save(record); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := m.Load(id)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Prompt != record.Prompt {
		t.Fatalf("prompt mismatch")
	}

	record.Results = []Entry{
		{Label: "A", ModelID: "qwable", ModelName: "Qwable-9B", Kind: "local", Text: "ok"},
		{Label: "B", ModelID: "kimi", ModelName: "Kimi-K2.7", Kind: "cloud", Text: "better"},
	}
	if err := m.Save(record); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	_, resp, err := m.RecordVote(id, "B", "cloud was clearer", "human")
	if err != nil {
		t.Fatalf("vote failed: %v", err)
	}
	if resp.Winner.ModelID != "kimi" {
		t.Fatalf("winner should be kimi, got %v", resp.Winner.ModelID)
	}
	if resp.EloDelta["kimi"] <= 0 {
		t.Fatalf("kimi elo delta should be positive: %v", resp.EloDelta)
	}

	lb := m.LoadLeaderboard()
	if lb.Models["kimi"].Rating <= 1500 {
		t.Fatalf("leaderboard should record kimi gain")
	}
}

func TestExportDPO(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	id := GenerateID()
	record := &Record{
		ID: id,
		Prompt: "explain recursion",
		TaskType: "code",
		CreatedAt: time.Now(),
		Results: []Entry{
			{Label: "A", ModelID: "qwable", Kind: "local", Text: "local answer"},
			{Label: "B", ModelID: "kimi", Kind: "cloud", Text: "cloud answer"},
		},
		Vote: &Vote{WinningLabel: "B", WinningModelID: "kimi", Judge: "human"},
	}
	_ = m.Save(record)

	resp, err := m.Export(ExportRequest{Format: "dpo"})
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected 1 example, got %d", resp.Count)
	}
	if resp.Examples[0]["model_chosen"] != "kimi" || resp.Examples[0]["model_rejected"] != "qwable" {
		t.Fatalf("wrong models in example: %+v", resp.Examples[0])
	}

	localOnly, err := m.Export(ExportRequest{Format: "dpo", LocalOnly: true})
	if err != nil {
		t.Fatalf("export local-only failed: %v", err)
	}
	if localOnly.Count != 1 {
		t.Fatalf("expected 1 local-only example, got %d", localOnly.Count)
	}

	cloudOnly, err := m.Export(ExportRequest{Format: "dpo", CloudOnly: true})
	if err != nil {
		t.Fatalf("export cloud-only failed: %v", err)
	}
	if cloudOnly.Count != 1 {
		t.Fatalf("expected 1 cloud-only example, got %d", cloudOnly.Count)
	}
}

func TestExportFiltersFailedResponses(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	record := &Record{
		ID: GenerateID(),
		Prompt: "test",
		CreatedAt: time.Now(),
		Results: []Entry{
			{Label: "A", ModelID: "qwable", Kind: "local", Text: "ok", Status: StatusOK},
			{Label: "B", ModelID: "kimi", Kind: "cloud", Text: "bad", Status: StatusQuota, Reason: "quota"},
		},
		Vote: &Vote{WinningLabel: "A", WinningModelID: "qwable", Judge: "human"},
	}
	_ = m.Save(record)

	resp, err := m.Export(ExportRequest{Format: "dpo"})
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("failed loser should be excluded, got %d", resp.Count)
	}
}

func TestParseJudgeResponse(t *testing.T) {
	cases := []struct {
		raw    string
		winner string
		reason string
	}{
		{
			`{"winner": "A", "reason": "more concise"}`,
			"A", "more concise",
		},
		{
			"```json\n{\"winner\": \"b\", \"reason\": \"better\"}\n```",
			"B", "better",
		},
		{
			"Some text {\"winner\": \"C\", \"reason\": \"ok\"} trailing",
			"C", "ok",
		},
	}
	for _, c := range cases {
		w, r, err := parseJudgeResponse(c.raw)
		if err != nil {
			t.Fatalf("parse failed for %q: %v", c.raw, err)
		}
		if w != c.winner || r != c.reason {
			t.Fatalf("parse mismatch for %q: got %q/%q want %q/%q", c.raw, w, r, c.winner, c.reason)
		}
	}
}

func TestSaveDataset(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	resp := &ExportResponse{
		Format: "dpo",
		Examples: []map[string]interface{}{
			{"prompt": "p", "chosen": "c", "rejected": "r"},
		},
	}
	path, err := m.SaveDataset("test", resp)
	if err != nil {
		t.Fatalf("save dataset failed: %v", err)
	}
	if !strings.HasSuffix(path, ".jsonl") {
		t.Fatalf("expected jsonl path, got %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dataset failed: %v", err)
	}
	if !strings.Contains(string(data), `"chosen":"c"`) {
		t.Fatalf("dataset content wrong: %s", string(data))
	}
}

func TestManagerDirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if _, err := os.Stat(filepath.Join(dir, ".forge_compare", "records")); err != nil {
		t.Fatalf("records dir not created: %v", err)
	}
	if m.Path() != filepath.Join(dir, ".forge_compare") {
		t.Fatalf("unexpected path: %s", m.Path())
	}
}

func TestInferTaskAndComplexity(t *testing.T) {
	if InferTaskType("напиши функцию сортировки") != "code" {
		t.Fatalf("expected code task")
	}
	if InferComplexity("напиши функцию сортировки") < 1 {
		t.Fatalf("complexity should be >= 1")
	}
}

func TestGenerateIDDifferent(t *testing.T) {
	a := GenerateID()
	b := GenerateID()
	if a == b {
		t.Fatalf("IDs should be unique")
	}
	if !strings.Contains(a, "-") {
		t.Fatalf("ID should contain separator")
	}
}
