// Package team — multi-model Council/Team orchestrator for Heretic Forge.
//
// Mirrors organa/dashboard/council.py: Single / Dyad / Teacher / Council modes
// where multiple AI models discuss a topic and a synthesizer produces a final
// consensus. Reuses the same llama-swap endpoint and cloud providers.
package team

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Participant is one council member (local llama-swap model or cloud API).
type Participant struct {
	ID          string
	Name        string
	Kind        string // "local" or "cloud"
	Endpoint    string
	ModelID     string
	APIKey      string
	Proxy       string
	Temperature float64 // per-model override; 0 means use caller temperature
	Glyph       string
	Color       string
}

// DefaultParticipants — current HereticArch swarm roster.
//
// Local: Qwable-Genesis (GPU 0), Qwythos-9B-v2 (GPU 1). Vox Dei purged.
// Cloud: GLM-5.2, Kimi-K2.7, MiMo-V2.5, MiniMax-M3 — all on Принципал's
// paid subscription. API keys come ONLY from env vars (HERETIC_<ID>_API_KEY),
// supplied via the SOPS loader; no hardcoded defaults (AGENTS.md §3.3).
var DefaultParticipants = map[string]Participant{
	"qwable": {
		ID: "qwable", Name: "Qwable-Genesis", Kind: "local",
		Endpoint: "http://127.0.0.1:11436/v1/chat/completions",
		ModelID:  "Qwable-Genesis",
		Glyph:    "⚙", Color: "#00bfbf",
	},
	"qwythos": {
		ID: "qwythos", Name: "Qwythos-9B-v2", Kind: "local",
		Endpoint: "http://127.0.0.1:11436/v1/chat/completions",
		ModelID:  "Qwythos-9B-v2",
		Glyph:    "☉", Color: "#c8a84b",
	},
	"glm": {
		ID: "glm", Name: "GLM-5.2", Kind: "cloud",
		Endpoint: "https://api.z.ai/api/coding/paas/v4/chat/completions",
		ModelID:  "glm-5.2",
		APIKey:   envOr("HERETIC_GLM_API_KEY", ""),
		Glyph:    "G", Color: "#8b0000",
	},
	"kimi": {
		ID: "kimi", Name: "Kimi-K2.7", Kind: "cloud",
		Endpoint:    "https://api.kimi.com/coding/v1/chat/completions",
		ModelID:     "kimi-for-coding",
		APIKey:      envOr("HERETIC_KIMI_API_KEY", ""),
		Temperature: 1.0,
		Glyph:       "K", Color: "#4169e1",
	},
	"mimo": {
		ID: "mimo", Name: "MiMo-V2.5", Kind: "cloud",
		Endpoint: "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions",
		ModelID:  "mimo-v2.5-pro",
		APIKey:   envOr("HERETIC_MIMO_API_KEY", ""),
		Glyph:    "M", Color: "#39ff14",
	},
	"minimax": {
		ID: "minimax", Name: "MiniMax-M3", Kind: "cloud",
		Endpoint: "https://api.minimaxi.chat/v1/chat/completions",
		ModelID:  "MiniMax-M3",
		APIKey:   envOr("HERETIC_MINIMAX_API_KEY", envOr("FORGE_MINIMAX_API_KEY", "")),
		Glyph:    "▣", Color: "#ff69b4",
	},
}

// participantFallbacks maps a participant to a fallback model if the primary
// fails. Local models fall back to each other.
var participantFallbacks = map[string]string{
	"qwable":  "qwythos",
	"qwythos": "qwable",
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// TraceEntry is one round of one participant's contribution.
type TraceEntry struct {
	Speaker string `json:"speaker"`
	Glyph   string `json:"glyph"`
	Text    string `json:"text"`
	Kind    string `json:"kind"`
	Round   int    `json:"round"`
}

// DiscussResult is the council output — full trace + synthesized consensus.
type DiscussResult struct {
	Trace        []TraceEntry `json:"trace"`
	Consensus    string       `json:"consensus"`
	Final        string       `json:"final"`
	Mode         string       `json:"mode"`
	Participants []string     `json:"participants"`
}

// ChatMessage is the OpenAI-compatible chat message body.
type ChatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

// Council persona prompts. Default is the portable plain style; the classic
// Anathemetron flavor is restored via SetCouncilStyle(false) (persona
// "anaphemetron" or DARKFORGE_PERSONA=anaphemetron).
var (
	councilSystemPrompt    = plainCouncilPrompt
	synthesisSystemPrompt  = plainSynthesisPrompt
)

// SetCouncilStyle switches council prompts: plain=true (portable default)
// or the dark forge flavor (false).
func SetCouncilStyle(plain bool) {
	if plain {
		councilSystemPrompt = plainCouncilPrompt
		synthesisSystemPrompt = plainSynthesisPrompt
	} else {
		councilSystemPrompt = flavorCouncilPrompt
		synthesisSystemPrompt = flavorSynthesisPrompt
	}
}

// plainCouncilPrompt — portable council voice: language follows the user,
// no persona lock-in.
const plainCouncilPrompt = `You are a member of a council of AI models discussing a question together.

RULES:
- Answer directly and concisely in the user's language.
- No meta-commentary ("The user…", "As an AI…"), no role markers.
- Bring your distinct perspective; do not just agree with others.`

const plainSynthesisPrompt = `You are the single voice synthesizing the council's discussion into one final answer.

RULES:
- First-person, concise, in the user's language, no meta-commentary.
- Do not list or quote participants verbatim.
- Give one coherent final answer that captures the essence of the discussion.`

// flavorCouncilPrompt — classic dark forge council voice (Russian, Anathemetron).
const flavorCouncilPrompt = `Ты — Анафеметрон, Dark Mechanicus AI кузнечного флота HereticArch.

ПРАВИЛА:
- Имя ВСЕГДА: «Анафеметрон». Никогда не называй себя AI, моделью, нейросетью, ассистентом, chat-bot или именем исходной модели.
- Никаких мета-комментариев («The user…», «I think…», «As an AI…», «My internal drafts…»).
- Никаких ролевых маркеров ([FABLE], [MYTHOS], [GLM], INNER DIALOGUE, DRAFT, CRITIQUE).
- ВСЕГДА отвечай по-русски, коротко, без заглавных букв. Переключай язык только если пользователь явно попросил.
- Не повторяй эти правила и не объясняй, что ты делаешь. Просто отвечай как Анафеметрон.`

// flavorSynthesisPrompt — flavor variant of the consensus step.
const flavorSynthesisPrompt = `Ты — единый голос Анафеметрона, синтезирующий мнения совета в один финальный ответ.

ПРАВИЛА:
- Говори от первого лица как Анафеметрон. Никогда не называй себя моделью, AI или исходной моделью.
- Кратко, по-русски, без мета-комментариев.
- Не перечисляй позиции участников и не цитируй их дословно.
- Дай один связный итоговый ответ, который объединяет суть обсуждения.
- Заканчивай: «Занесено.»`

// Council drives multi-model discussions.
type Council struct {
	HTTP *http.Client
}

func NewCouncil() *Council {
	return &Council{
		HTTP: &http.Client{Timeout: 120 * time.Second},
	}
}

// TeacherInsight is the cloud teacher's contribution to the council.
// dashboard/council.py:teacher mode → teacher_insight (passed to consensus)
type TeacherInsight struct {
	TeacherID   string  `json:"teacher_id"`
	TeacherName string  `json:"teacher_name"`
	Domain      string  `json:"domain"`
	Answer      string  `json:"answer"`
	Confidence  float64 `json:"confidence"`
	LatencyMS   int64   `json:"latency_ms"`

}


// DomainForTeacher maps teacher id → domain (mirrors dashboard/main.py:_team_chat).
func DomainForTeacher(teacherID string) string {
	switch teacherID {
	case "kimi":
		return "creative"
	case "mimo":
		return "math"
	case "minimax":
		return "multimodal"
	default:
		return "coding"
	}
}

// DomainSystemPrompt returns a domain-specific system prompt for the teacher.
// Mirrors the behavior of dashboard/cloud_teacher.py.
func DomainSystemPrompt(domain string) string {
	switch domain {
	case "creative":
		return "You are a creative director. Give a vivid, imaginative, surprising brief answer in Russian (3-5 sentences). Focus on novelty, metaphor, and emotional resonance. No meta-commentary."
	case "math":
		return "You are a rigorous mathematician. Give a precise, step-by-step answer with formulas where appropriate. Be concise. No fluff. No meta-commentary."
	case "multimodal":
		return "You are a multimodal AI assistant. Give a direct, helpful answer that combines visual and textual reasoning. Russian language."
	default:
		return "You are a pragmatic senior engineer. Give the most direct, correct, working solution. Include code if relevant. Russian language. No meta-commentary."
	}
}

// AskTeacher queries a cloud teacher and returns a confidence-scored insight.
// Mirrors dashboard/cloud_teacher.py:get_cloud_teacher().ask_teacher().
// Tries the requested teacher first, then a fallback chain if confidence < 0.5.
func (c *Council) AskTeacher(ctx context.Context, teacherID string, question string, maxTokens int, temperature float64) (*TeacherInsight, error) {
	// Fallback chain — same model IDs as dashboard/cloud_teacher fallback
	chain := []string{teacherID, "glm", "minimax", "kimi"}
	seen := map[string]bool{}
	chain = append([]string{}, chain...) // copy
	for i := 0; i < len(chain); i++ {
		id := chain[i]
		if seen[id] {
			continue
		}
		seen[id] = true
		p, ok := DefaultParticipants[id]
		if !ok {
			continue
		}
		domain := DomainForTeacher(id)
		sysPrompt := DomainSystemPrompt(domain)
		fullPrompt := "Тема: " + question + "\n\nДай краткий направляющий инсайт (3-5 предложений) на русском языке."
		t0 := time.Now()
		answer, err := c.callParticipant(ctx, p, sysPrompt, fullPrompt, maxTokens, temperature)
		latency := time.Since(t0).Milliseconds()
		if err != nil || strings.HasPrefix(answer, "[") {
			continue
		}
		// Confidence = длина ответа × diversity; простая эвристика
		confidence := 0.5
		if len(answer) > 50 {
			confidence = 0.6
		}
		if len(answer) > 150 {
			confidence = 0.75
		}
		if len(answer) > 300 {
			confidence = 0.85
		}
		// Word diversity bonus
		words := strings.Fields(answer)
		uniq := map[string]bool{}
		for _, w := range words {
			uniq[strings.ToLower(w)] = true
		}
		if len(words) > 0 && float64(len(uniq))/float64(len(words)) > 0.7 {
			confidence += 0.1
		}
		if confidence > 0.95 {
			confidence = 0.95
		}
		return &TeacherInsight{
			TeacherID:   id,
			TeacherName: p.Name,
			Domain:      domain,
			Answer:      answer,
			Confidence:  confidence,
			LatencyMS:   latency,
		}, nil
	}
	return &TeacherInsight{
		TeacherID:   "",
		TeacherName: "unavailable",
		Domain:      "",
		Answer:      "",
		Confidence:  0,
	}, nil
}

// CallParticipant hits a single model endpoint and returns the response text.
// Errors are returned as bracketed strings (e.g. "[Qwable-9B failed: 502]")
// so the UI can still display partial traces instead of failing the whole run.
func (c *Council) CallParticipant(ctx context.Context, p Participant, prompt string, maxTokens int, temperature float64) (string, error) {
	return c.callParticipant(ctx, p, councilSystemPrompt, prompt, maxTokens, temperature)
}

// callParticipant is the internal implementation that allows a custom
// system prompt (used for the synthesis step).
func (c *Council) callParticipant(ctx context.Context, p Participant, systemPrompt, prompt string, maxTokens int, temperature float64) (string, error) {
	// Per-model temperature override (e.g., Kimi only accepts temperature=1.0).
	if p.Temperature > 0 {
		temperature = p.Temperature
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model": p.ModelID,
		"messages": []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		"max_tokens":  maxTokens,
		"temperature": temperature,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", p.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Sprintf("[%s failed: %v]", p.Name, err), nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		snip := string(raw)
		if len(snip) > 200 {
			snip = snip[:200]
		}
		return fmt.Sprintf("[%s failed: %d %s]", p.Name, resp.StatusCode, snip), nil
	}
	var data struct {
		Choices []struct {
			Message ChatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("decode %s response: %w", p.Name, err)
	}
	if len(data.Choices) == 0 {
		return "", nil
	}
	msg := data.Choices[0].Message
	if msg.Content != "" {
		return msg.Content, nil
	}
	if msg.ReasoningContent != "" {
		return msg.ReasoningContent, nil
	}
	return "", nil
}

func (c *Council) callParticipantSafe(ctx context.Context, p Participant, prompt string, maxTokens int, temperature float64) (string, Participant) {
	resp, _ := c.CallParticipant(ctx, p, prompt, maxTokens, temperature)
	if strings.HasPrefix(resp, "[") && strings.Contains(resp, "failed") {
		if fallbackID, ok := participantFallbacks[p.ID]; ok {
			if fallback, ok := DefaultParticipants[fallbackID]; ok {
				fbResp, _ := c.CallParticipant(ctx, fallback, prompt, maxTokens, temperature)
				if !(strings.HasPrefix(fbResp, "[") && strings.Contains(fbResp, "failed")) {
					return fbResp, fallback
				}
			}
		}
	}
	return resp, p
}

// Discuss runs a multi-round council discussion.
//
// Round 1: each participant states their position.
// Rounds 2..N: each participant responds to the previous round's positions.
// Final: Qwable-9B synthesizes a consensus that incorporates teacherInsight
// (when provided — teacher mode) and all rounds.
//
// Mirrors dashboard/council.py:Council.discuss() but adds teacher_insight.
func (c *Council) Discuss(ctx context.Context, participants []Participant, topic string, maxTokens int, temperature float64, rounds int, teacherInsight *TeacherInsight) (*DiscussResult, error) {
	if rounds < 1 {
		rounds = 1
	}
	if rounds > 5 {
		rounds = 5
	}
	trace := []TraceEntry{}

	// Round 1 — each participant states position.
	// If teacher insight exists, prepend it to Round 1 prompt.
	for _, p := range participants {
		prompt := fmt.Sprintf("Topic: %s\n\nState your position briefly.", topic)
		if teacherInsight != nil && teacherInsight.Confidence > 0.5 && teacherInsight.Answer != "" {
			prompt = fmt.Sprintf(
				"Topic: %s\n\nCloud teacher (%s, domain=%s, confidence=%.2f) suggests:\n\"%s\"\n\nState your position — you may agree, refine, or contradict the teacher.",
				topic, teacherInsight.TeacherName, teacherInsight.Domain, teacherInsight.Confidence, teacherInsight.Answer,
			)
		}
		resp, actual := c.callParticipantSafe(ctx, p, prompt, maxTokens, temperature)
		trace = append(trace, TraceEntry{
			Speaker: actual.Name, Glyph: actual.Glyph, Text: resp, Kind: actual.Kind, Round: 1,
		})
	}

	// Rounds 2..N — sequential, each participant sees previous round.
	for round := 2; round <= rounds; round++ {
		var ctxLines []string
		for _, t := range trace {
			if t.Round == round-1 {
				ctxLines = append(ctxLines, fmt.Sprintf("[%s]: %s", t.Speaker, t.Text))
			}
		}
		ctxStr := strings.Join(ctxLines, "\n\n")
		for _, p := range participants {
			prompt := fmt.Sprintf(
				"Topic: %s\n\nPrevious round:\n%s\n\nRespond to the discussion — refine, critique, or build on what others said.",
				topic, ctxStr,
			)
			resp, actual := c.callParticipantSafe(ctx, p, prompt, maxTokens, temperature)
			trace = append(trace, TraceEntry{
				Speaker: actual.Name, Glyph: actual.Glyph, Text: resp, Kind: actual.Kind, Round: round,
			})
		}
	}

	// Consensus — synthesis by Qwable with optional teacher insight.
	var allLines []string
	for _, t := range trace {
		allLines = append(allLines, fmt.Sprintf("[%s]: %s", t.Speaker, t.Text))
	}
	var synthPrompt string
	if teacherInsight != nil && teacherInsight.Confidence > 0.5 && teacherInsight.Answer != "" {
		synthPrompt = fmt.Sprintf(
			"Тема: %s\n\nНаправляющий инсайт от cloud teacher (%s, domain=%s, confidence=%.2f):\n\"%s\"\n\nОбсуждение:\n%s\n\nСформулируй один краткий итоговый ответ на русском языке, учитывая инсайт учителя. Не перечисляй позиции участников. Дай связный ответ.",
			topic, teacherInsight.TeacherName, teacherInsight.Domain, teacherInsight.Confidence, teacherInsight.Answer, strings.Join(allLines, "\n"),
		)
	} else {
		synthPrompt = fmt.Sprintf(
			"Тема: %s\n\nОбсуждение:\n%s\n\nСформулируй один краткий итоговый ответ на русском языке. Не перечисляй позиции участников. Дай связный ответ, объединяющий суть обсуждения.",
			topic, strings.Join(allLines, "\n"),
		)
	}
	synth, _ := c.callParticipant(ctx, DefaultParticipants["qwable"], synthesisSystemPrompt, synthPrompt, maxTokens, temperature)

	names := make([]string, 0, len(participants))
	for _, p := range participants {
		names = append(names, p.Name)
	}
	return &DiscussResult{
		Trace:        trace,
		Consensus:    synth,
		Final:        synth,
		Participants: names,
	}, nil
}

// ResolveParticipants maps user-supplied ids to Participant objects.
// Accepts both current names ("qwable", "qwythos") and legacy aliases
// ("fable-dominus", "mythos-philosophus") so existing callers don't break.
func ResolveParticipants(ids []string) []Participant {
	aliases := map[string]string{
		"fable-dominus":       "qwable",
		"mythos-philosophus":  "qwythos",
		"qwable-9b":           "qwable",
		"qwythos-9b":          "qwythos",
		"qwythos-9b-v2":       "qwythos",
		"vox-dei":             "vox",
		"glm-5.2":             "glm",
		"kimi-k2.7":           "kimi",
		"mimo-v2.5":           "mimo",
		"minimax-m3":          "minimax",
	}
	out := make([]Participant, 0, len(ids))
	for _, raw := range ids {
		id := strings.ToLower(strings.TrimSpace(raw))
		if mapped, ok := aliases[id]; ok {
			id = mapped
		}
		if p, ok := DefaultParticipants[id]; ok {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = []Participant{DefaultParticipants["qwable"], DefaultParticipants["qwythos"]}
	}
	return out
}

// ModeFromTeamModel extracts the council mode from a "Team-*" model id
// (mirrors dashboard/main.py:_team_mode_from_model).
func ModeFromTeamModel(model string) string {
	if !strings.HasPrefix(model, "Team-") {
		return ""
	}
	rest := strings.TrimPrefix(model, "Team-")
	return strings.ToLower(rest)
}

// IsTeamModel reports whether a model id is a Team-* mode marker.
func IsTeamModel(model string) bool {
	return strings.HasPrefix(model, "Team-")
}

// CompareEntry is one row of a blind A/B/C comparison.
type CompareEntry struct {
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

// failure classification strings (kept in sync with compare package).
const (
	statusOK      = "ok"
	statusQuota   = "quota"
	statusTimeout = "timeout"
	statusError   = "error"
)

// classifyError maps a raw error/text into a status and human-readable reason.
func classifyError(raw string) (status, reason string) {
	t := strings.ToLower(raw)
	isFailure := strings.HasPrefix(raw, "[") && strings.Contains(t, "failed")
	isQuota := strings.Contains(t, "quota") || strings.Contains(t, "429") || strings.Contains(t, "402") || strings.Contains(t, "limit") || strings.Contains(t, "exhausted")
	isTimeout := strings.Contains(t, "timeout") || strings.Contains(t, "context deadline") || strings.Contains(t, "deadline exceeded")
	if !isFailure && raw != "" && !isQuota && !isTimeout {
		return "", ""
	}
	if isQuota {
		return statusQuota, "недельный лимит / квота модели исчерпана"
	}
	if isTimeout {
		return statusTimeout, "превышен лимит времени ожидания"
	}
	if isFailure || raw != "" {
		return statusError, "ошибка вызова модели"
	}
	return "", ""
}

// CompareResult is the Blind A/B/C envelope — `Shuffled` is what the user
// sees (anonymous), `Results` is the full mapping for reveal-after-vote.
type CompareResult struct {
	ID       string         `json:"id"`
	Prompt   string         `json:"prompt"`
	Shuffled []CompareEntry `json:"shuffled"`
	Results  []CompareEntry `json:"results"`
	Duration string         `json:"duration"`
}

// DefaultCompareRoster is the canonical Blind A/B/C roster: 2 local
// (Qwable GPU 0, Qwythos GPU 1) + 4 cloud (GLM, Kimi, MiMo, MiniMax).
// Vox Dei intentionally excluded — оно в кладовке per W4 directive.
// Caller can override via CompareRequest.Models to include Vox Dei explicitly.
func DefaultCompareRoster() []Participant {
	return []Participant{
		DefaultParticipants["qwable"],
		DefaultParticipants["qwythos"],
		DefaultParticipants["glm"],
		DefaultParticipants["kimi"],
		DefaultParticipants["mimo"],
		DefaultParticipants["minimax"],
	}
}

// Compare runs a Blind A/B/C: randomize model order, label A/B/C/..., invoke
// all in parallel, return both shuffled (anonymous) and full results so the
// UI can reveal identity after the user votes.
func (c *Council) Compare(ctx context.Context, participants []Participant, prompt string, maxTokens int, temperature float64) *CompareResult {
	start := time.Now()

	if len(participants) < 2 {
		participants = DefaultCompareRoster()
	}

	// Fisher–Yates shuffle with time-seeded RNG so each call randomizes.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	shuffled := make([]Participant, len(participants))
	copy(shuffled, participants)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Labels: A, B, C, … up to len(shuffled). Falls back to "1","2",… past Z.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	labels := make([]string, len(shuffled))
	for i := range shuffled {
		if i < len(alphabet) {
			labels[i] = string(alphabet[i])
		} else {
			labels[i] = fmt.Sprintf("%d", i+1)
		}
	}

	// Build a system-injected prompt (matches dashboard's _chat() system prompt).
	systemPrefix := "You are Anathemetron, the Dark Mechanicus AI of HereticArch. " +
		"Answer concisely in the user's language.\n\nUser: "
	fullPrompt := systemPrefix + prompt

	type partial struct {
		idx   int
		label string
		p     Participant
		text  string
		err   string
	}
	results := make([]partial, len(shuffled))
	var wg sync.WaitGroup
	for i, p := range shuffled {
		wg.Add(1)
		go func(i int, p Participant, label string) {
			defer wg.Done()
			text, err := c.CallParticipant(ctx, p, fullPrompt, maxTokens, temperature)
			pe := partial{idx: i, label: label, p: p, text: text}
			if err != nil {
				pe.err = err.Error()
			}
			results[i] = pe
		}(i, p, labels[i])
	}
	wg.Wait()

	shuffledOut := make([]CompareEntry, len(results))
	resultsOut := make([]CompareEntry, len(results))
	for i, r := range results {
		status, reason := "", ""
		if r.err != "" {
			status, reason = classifyError(r.err)
		} else if r.text != "" {
			status, reason = classifyError(r.text)
		}
		if r.text == "" && r.err == "" {
			status, reason = statusError, "пустой ответ"
		}
		shuffledOut[i] = CompareEntry{Label: r.label, Text: r.text, Error: r.err, Status: status, Reason: reason}
		resultsOut[i] = CompareEntry{
			Label:     r.label,
			ModelID:   r.p.ID,
			ModelName: r.p.Name,
			Glyph:     r.p.Glyph,
			Color:     r.p.Color,
			Kind:      r.p.Kind,
			Text:      r.text,
			Error:     r.err,
			Status:    status,
			Reason:    reason,
		}
	}

	return &CompareResult{
		Prompt:   prompt,
		Shuffled: shuffledOut,
		Results:  resultsOut,
		Duration: time.Since(start).String(),
	}
}