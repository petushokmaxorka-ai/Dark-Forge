// Package compare — high-level operations: voting, judging, improving, exporting.
package compare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/model"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/team"
)

// RecordVote records a human preference, updates Elo, and persists everything.
// Returns the updated record and the public vote response.
func (m *Manager) RecordVote(recordID, winningLabel, feedback, judge string) (*Record, *VoteResponse, error) {
	record, err := m.Load(recordID)
	if err != nil {
		return nil, nil, err
	}
	if record.Vote != nil {
		return nil, nil, fmt.Errorf("comparison already voted")
	}

	var winner *Entry
	for i := range record.Results {
		if record.Results[i].Label == winningLabel {
			w := record.Results[i]
			winner = &w
			break
		}
	}
	if winner == nil {
		return nil, nil, fmt.Errorf("label %s not found", winningLabel)
	}
	if winner.Status != "" && winner.Status != StatusOK {
		return nil, nil, fmt.Errorf("cannot vote for failed response (%s: %s)", winner.Status, winner.Reason)
	}

	record.Vote = &Vote{
		WinningLabel:   winner.Label,
		WinningModelID: winner.ModelID,
		Feedback:       feedback,
		Judge:          judge,
		Timestamp:      timeNow(),
	}

	lb := m.LoadLeaderboard()
	eloBefore := make(map[string]float64)
	eloAfter := make(map[string]float64)
	for _, e := range record.Results {
		eloBefore[e.ModelID] = rating(lb.Models, e.ModelID).Rating
	}

	// Update overall ratings for every OK opponent.
	for _, e := range record.Results {
		if e.Label == winner.Label {
			continue
		}
		if e.Status != "" && e.Status != StatusOK {
			continue
		}
		updatePair(lb.Models, winner.ModelID, e.ModelID)
	}

	// Update capability-specific ratings if task_type is known.
	if record.TaskType != "" {
		capTable := lb.ByCapability[record.TaskType]
		if capTable == nil {
			capTable = make(map[string]*Rating)
			lb.ByCapability[record.TaskType] = capTable
		}
		for _, e := range record.Results {
			if e.Label == winner.Label {
				continue
			}
			if e.Status != "" && e.Status != StatusOK {
				continue
			}
			updatePair(capTable, winner.ModelID, e.ModelID)
		}
	}

	for _, e := range record.Results {
		eloAfter[e.ModelID] = rating(lb.Models, e.ModelID).Rating
	}
	record.EloBefore = eloBefore
	record.EloAfter = eloAfter

	if err := m.Save(record); err != nil {
		return nil, nil, err
	}
	if err := m.SaveLeaderboard(lb); err != nil {
		return nil, nil, err
	}

	delta := make(map[string]float64)
	for id := range eloBefore {
		delta[id] = eloAfter[id] - eloBefore[id]
	}

	resp := &VoteResponse{
		ID:          record.ID,
		Winner:      *winner,
		Reveal:      record.Results,
		Leaderboard: lb,
		EloDelta:    delta,
		Feedback:    feedback,
	}
	return record, resp, nil
}

// Judge asks a judge model to pick a winner and then records the vote.
func (m *Manager) Judge(ctx context.Context, recordID, judgeModelID string) (*JudgeResponse, error) {
	record, err := m.Load(recordID)
	if err != nil {
		return nil, err
	}
	if record.Vote != nil {
		return nil, fmt.Errorf("comparison already voted")
	}

	if judgeModelID == "" {
		judgeModelID = "glm"
	}
	judge, ok := team.DefaultParticipants[judgeModelID]
	if !ok {
		return nil, fmt.Errorf("judge model %s not found", judgeModelID)
	}

	prompt := buildJudgePrompt(record)
	council := team.NewCouncil()
	council.HTTP = &http.Client{Timeout: 60 * time.Second}
	raw, err := council.CallParticipant(ctx, judge, prompt, 512, 0.3)
	if err != nil {
		return nil, fmt.Errorf("judge call failed: %w", err)
	}

	winner, reason, err := parseJudgeResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("judge parse failed: %w", err)
	}

	_, _, err = m.RecordVote(recordID, winner, reason, judgeModelID)
	if err != nil {
		return nil, err
	}

	return &JudgeResponse{
		ID:     recordID,
		Winner: winner,
		Reason: reason,
		Judge:  judgeModelID,
	}, nil
}

// Improve takes a label and feedback, asks a model to refine the response,
// and returns the improved text. It does NOT record a vote.
func (m *Manager) Improve(ctx context.Context, recordID, label, feedback, improverModelID string) (*ImproveResponse, error) {
	record, err := m.Load(recordID)
	if err != nil {
		return nil, err
	}

	var entry *Entry
	for i := range record.Results {
		if record.Results[i].Label == label {
			entry = &record.Results[i]
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("label %s not found", label)
	}
	if entry.Status != "" && entry.Status != StatusOK {
		return nil, fmt.Errorf("cannot improve a failed response (%s)", entry.Status)
	}

	if improverModelID == "" {
		improverModelID = entry.ModelID
	}
	improver, ok := team.DefaultParticipants[improverModelID]
	if !ok {
		return nil, fmt.Errorf("improver model %s not found", improverModelID)
	}

	prompt := fmt.Sprintf("Исходный ответ:\n%s\n\nОбратная связь пользователя:\n%s\n\nУлучши ответ, учитывая обратную связь. Отвечай кратко и по делу.", entry.Text, feedback)
	council := team.NewCouncil()
	council.HTTP = &http.Client{Timeout: 60 * time.Second}
	improved, err := council.CallParticipant(ctx, improver, prompt, 512, 0.7)
	if err != nil {
		return nil, fmt.Errorf("improve call failed: %w", err)
	}

	return &ImproveResponse{
		ID:       recordID,
		Label:    label,
		ModelID:  improverModelID,
		Original: entry.Text,
		Improved: improved,
		Feedback: feedback,
	}, nil
}

// Export returns a preference dataset or leaderboard snapshot.
func (m *Manager) Export(req ExportRequest) (*ExportResponse, error) {
	if req.Format == "" {
		req.Format = "dpo"
	}
	if req.Format == "leaderboard" {
		return &ExportResponse{
			Format:      "leaderboard",
			Count:       0,
			Leaderboard: m.LoadLeaderboard(),
		}, nil
	}

	records, err := m.ListRecords()
	if err != nil {
		return nil, err
	}

	examples := []map[string]interface{}{}
	for _, r := range records {
		if r.Vote == nil {
			continue
		}
		if req.Capability != "" && r.TaskType != req.Capability {
			continue
		}
		exs := buildExamples(r, req.Format, req.LocalOnly, req.CloudOnly)
		examples = append(examples, exs...)
	}

	return &ExportResponse{
		Format:   req.Format,
		Count:    len(examples),
		Examples: examples,
	}, nil
}

// SaveDataset writes an exported dataset to disk for Python training.
func (m *Manager) SaveDataset(name string, resp *ExportResponse) (string, error) {
	path := filepath.Join(m.dir, "datasets", name+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ex := range resp.Examples {
		if err := enc.Encode(ex); err != nil {
			return "", err
		}
	}
	return path, nil
}

// buildExamples creates DPO or SFT examples from one record.
func buildExamples(r *Record, format string, localOnly, cloudOnly bool) []map[string]interface{} {
	var winner *Entry
	for i := range r.Results {
		if r.Results[i].Label == r.Vote.WinningLabel {
			w := r.Results[i]
			winner = &w
			break
		}
	}
	if winner == nil || (winner.Status != "" && winner.Status != StatusOK) {
		return nil
	}

	examples := []map[string]interface{}{}
	for _, e := range r.Results {
		if e.Label == winner.Label {
			continue
		}
		if e.Status != "" && e.Status != StatusOK {
			continue
		}
		if localOnly && e.Kind != "local" {
			continue
		}
		if cloudOnly && winner.Kind != "cloud" {
			continue
		}

		ex := map[string]interface{}{
			"prompt":          r.Prompt,
			"chosen":          winner.Text,
			"rejected":        e.Text,
			"model_chosen":    winner.ModelID,
			"model_rejected":  e.ModelID,
			"task_type":       r.TaskType,
			"complexity":      r.Complexity,
			"judge":           r.Vote.Judge,
			"feedback":        r.Vote.Feedback,
			"comparison_id":   r.ID,
		}
		if format == "sft" {
			ex = map[string]interface{}{
				"instruction": r.Prompt,
				"output":      winner.Text,
				"model":       winner.ModelID,
				"task_type":   r.TaskType,
				"feedback":    r.Vote.Feedback,
				"comparison_id": r.ID,
			}
		}
		examples = append(examples, ex)
	}
	return examples
}

// buildJudgePrompt creates a deterministic prompt for the model judge.
func buildJudgePrompt(r *Record) string {
	var sb strings.Builder
	sb.WriteString("Ты — экспертный судья. Сравни ответы моделей на запрос пользователя и выбери лучший.\n")
	sb.WriteString("Ответь строго в JSON формате: {\"winner\": \"A\", \"reason\": \"краткая причина\"}\n\n")
	sb.WriteString("Запрос: ")
	sb.WriteString(r.Prompt)
	sb.WriteString("\n\n")
	for _, e := range r.Shuffled {
		sb.WriteString(fmt.Sprintf("=== Вариант %s ===\n%s\n\n", e.Label, e.Text))
	}
	sb.WriteString("Выбери лучший вариант (winner — одна буква A–F).")
	return sb.String()
}

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
var looseJSONRe = regexp.MustCompile("(?s)\\{.*\\}")

// parseJudgeResponse extracts {"winner": "A", "reason": "..."} from raw text.
func parseJudgeResponse(raw string) (winner, reason string, err error) {
	body := raw
	if m := jsonBlockRe.FindStringSubmatch(raw); len(m) > 1 {
		body = m[1]
	} else if m := looseJSONRe.FindString(raw); m != "" {
		body = m
	}
	var v struct {
		Winner string `json:"winner"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return "", "", fmt.Errorf("no JSON object in judge response: %w", err)
	}
	v.Winner = strings.ToUpper(strings.TrimSpace(v.Winner))
	if v.Winner == "" {
		return "", "", fmt.Errorf("judge returned empty winner")
	}
	return v.Winner, v.Reason, nil
}

// InferTaskType uses the Forge task detector.
func InferTaskType(prompt string) string {
	return model.DetectTaskType(prompt)
}

// InferComplexity uses the Forge complexity estimator.
func InferComplexity(prompt string) int {
	return model.EstimateComplexity(prompt)
}

// unused import guard (team is used via DefaultParticipants and NewCouncil).
var _ = team.DefaultParticipants
