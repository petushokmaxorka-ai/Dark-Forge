// Package compare — Blind A/B/C preference collection, Elo leaderboard,
// and DPO export for Heretic Forge.
// «Опыт — кузница. Каждый голос — удар молота.»
package compare

import "time"

// Participant is the minimal model identity stored with a comparison.
type Participant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Entry is one anonymous or revealed response in a comparison.
type Entry struct {
	Label     string `json:"label"`
	ModelID   string `json:"model_id,omitempty"`
	ModelName string `json:"model_name,omitempty"`
	Glyph     string `json:"glyph,omitempty"`
	Color     string `json:"color,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Text      string `json:"text"`
	Error     string `json:"error,omitempty"`
	Status    string `json:"status,omitempty"` // ok, quota, timeout, error
	Reason    string `json:"reason,omitempty"` // human-readable failure reason
}

// Vote records a human or model judge preference.
type Vote struct {
	WinningLabel   string    `json:"winning_label"`
	WinningModelID string    `json:"winning_model_id"`
	Feedback       string    `json:"feedback,omitempty"`
	Judge          string    `json:"judge,omitempty"` // human or model id
	Timestamp      time.Time `json:"timestamp"`
}

// Record is the full persisted comparison.
type Record struct {
	ID           string                 `json:"id"`
	Prompt       string                 `json:"prompt"`
	TaskType     string                 `json:"task_type,omitempty"`
	Complexity   int                    `json:"complexity,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	Participants []Participant          `json:"participants"`
	Shuffled     []Entry                `json:"shuffled"`
	Results      []Entry                `json:"results"`
	Vote         *Vote                  `json:"vote,omitempty"`
	EloBefore    map[string]float64     `json:"elo_before,omitempty"`
	EloAfter     map[string]float64     `json:"elo_after,omitempty"`
}

// Leaderboard holds Elo ratings per model and per capability.
type Leaderboard struct {
	Models       map[string]*Rating            `json:"models"`
	ByCapability map[string]map[string]*Rating `json:"by_capability"`
	UpdatedAt    time.Time                     `json:"updated_at"`
}

// Rating tracks a single model's competitive history.
type Rating struct {
	Rating float64 `json:"rating"`
	Games  int     `json:"games"`
	Wins   int     `json:"wins"`
	Losses int     `json:"losses"`
}

// VoteRequest is the POST body for /api/compare/vote.
type VoteRequest struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Feedback string `json:"feedback,omitempty"`
}

// VoteResponse is the result of a vote.
type VoteResponse struct {
	ID          string             `json:"id"`
	Winner      Entry              `json:"winner"`
	Reveal      []Entry            `json:"reveal"`
	Leaderboard *Leaderboard       `json:"leaderboard"`
	EloDelta    map[string]float64 `json:"elo_delta"`
	Feedback    string             `json:"feedback,omitempty"`
}

// JudgeRequest is the POST body for /api/compare/judge.
type JudgeRequest struct {
	ID    string `json:"id"`
	Model string `json:"model,omitempty"` // judge model id; defaults to glm
}

// JudgeResponse is the model judge result.
type JudgeResponse struct {
	ID     string `json:"id"`
	Winner string `json:"winner"`
	Reason string `json:"reason"`
	Judge  string `json:"judge"`
}

// ImproveRequest is the POST body for /api/compare/improve.
type ImproveRequest struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`    // label to improve; defaults to winner
	Feedback string `json:"feedback"`
	Model    string `json:"model,omitempty"`    // model to use; defaults to winner
}

// ImproveResponse returns the improved text.
type ImproveResponse struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	ModelID  string `json:"model_id"`
	Original string `json:"original"`
	Improved string `json:"improved"`
	Feedback string `json:"feedback"`
}

// ExportRequest selects the exported dataset slice.
type ExportRequest struct {
	Format      string `json:"format,omitempty"`       // dpo, sft, leaderboard
	LocalOnly   bool   `json:"local_only,omitempty"`   // only pairs where rejected is local
	CloudOnly   bool   `json:"cloud_only,omitempty"`   // only pairs where chosen is cloud
	MinVotes    int    `json:"min_votes,omitempty"`    // minimum number of votes (human or model)
	Capability  string `json:"capability,omitempty"`   // filter by task_type
}

// ExportResponse returns the dataset or leaderboard.
type ExportResponse struct {
	Format   string                   `json:"format"`
	Count    int                      `json:"count"`
	Examples []map[string]interface{} `json:"examples,omitempty"`
	Leaderboard *Leaderboard          `json:"leaderboard,omitempty"`
}

// Summary is the leaderboard summary response.
type Summary struct {
	Leaderboard *Leaderboard `json:"leaderboard"`
	TotalVotes  int          `json:"total_votes"`
	TotalComparisons int     `json:"total_comparisons"`
}

const (
	StatusOK     = "ok"
	StatusQuota  = "quota"
	StatusTimeout = "timeout"
	StatusError  = "error"
	StatusMissing = "missing"
)
