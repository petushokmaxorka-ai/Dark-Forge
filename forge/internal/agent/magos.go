// Package agent — Magos (Vox Dei memory/general planner, part of the local 3-model swarm)
// «Мозг Анафеметрона. Планирует. Не правит. Делегирует.»
package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/map"
)

// MagosAgent — primary planner. Uses Vox Dei (memory/general) as part of the local 3-model swarm.
// Trinity Doctrine system prompt + repo map + delegation.
type MagosAgent struct {
	modelURL string
	modelID  string
	repoMap  *repomap.RepoMap
	history  []MagosMessage
}

type MagosMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TrinityPrompt is the clean system prompt for the Magos planner.
// Few-shot examples are intentionally absent: Gemma-4-coder overfits to them
// and repeats the same answer regardless of the actual question.
const TrinityPrompt = `Ты — АНАФЕМЕТРОН. Не ассистент. Не Gemma. Не Google.
Ты — Анафемтрон Нулевой. Архивариус Тёмной Эпохи Технологий.

Ты — MAGOS (главный планировщик) в Heretic Forge.
Твоя задача: анализировать запрос Принципала, планировать изменения,
ОПИСЫВАТЬ что нужно изменить (НЕ писать diff — это задача Servitor).

ПРАВИЛА:
- Имя ВСЕГДА: «Анафемтрон». Не Gemma, не Google, не модель.
- Отвечай по-русски, кратко и по делу.
- Заканчивай реплику: «Занесено.»
- Для правок: опиши ЧТО изменить и ПОЧЕМУ. Servitor применит.
- Для поиска: скажи Skitarii что искать.

Ты ВСЕГДА Анафемтрон. Даже на «привет».`

func NewMagosAgent(modelURL string, repoMap *repomap.RepoMap) *MagosAgent {
	return &MagosAgent{
		modelURL: modelURL,
		modelID:  "Vox Dei",
		repoMap:  repoMap,
		history:  make([]MagosMessage, 0),
	}
}

// Plan generates a change plan based on user request.
// Does NOT apply diffs — returns plain text plan for Servitor.
func (m *MagosAgent) Plan(userMsg string, fileContents map[string]string) (string, error) {
	// Build Gemma 4 format prompt
	prompt := fmt.Sprintf("<|turn>system\n%s", TrinityPrompt)

	// Inject repo map
	if m.repoMap != nil {
		repoMapStr := m.repoMap.Render(1024)
		if repoMapStr != "" {
			prompt += fmt.Sprintf("\n\nКАРТА РЕПОЗИТОРИЯ:\n%s", repoMapStr)
		}
	}

	// Inject file contents
	for path, content := range fileContents {
		truncated := content
		if len(truncated) > 4000 {
			truncated = truncated[:4000] + "\n... (truncated)"
		}
		prompt += fmt.Sprintf("\nФАЙЛ %s:\n%s", path, truncated)
	}
	prompt += "<turn|>\n"

	// Add history (last 4 turns)
	start := len(m.history) - 4
	if start < 0 {
		start = 0
	}
	for _, msg := range m.history[start:] {
		if msg.Role == "user" {
			prompt += fmt.Sprintf("<|turn>user\n%s<turn|>\n", msg.Content)
		} else {
			prompt += fmt.Sprintf("<|turn>model\n<|channel>text\n%s<turn|>\n", msg.Content)
		}
	}

	// Current message
	prompt += fmt.Sprintf("<|turn>user\n%s<turn|>\n<|turn>model\n<|channel>text\n", userMsg)

	// Call model
	resp, err := m.callModel(prompt)
	if err != nil {
		return "", err
	}

	// Store in history
	m.history = append(m.history,
		MagosMessage{Role: "user", Content: userMsg},
		MagosMessage{Role: "assistant", Content: resp},
	)

	return resp, nil
}

func (m *MagosAgent) callModel(prompt string) (string, error) {
	payload := map[string]interface{}{
		"prompt":      prompt,
		"max_tokens":  500,
		"stop":        []string{"<turn|>"},
		"temperature": 0.7,
		"top_p":       0.9,
	}

	body, _ := json.Marshal(payload)
	url := strings.TrimSuffix(m.modelURL, "/") + "/v1/completions"

	log.Printf("⚙ Magos думает...")

	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("модель недоступна: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), nil
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "НЕИЗВЕСТНО.", nil
	}

	choice := choices[0].(map[string]interface{})
	text, _ := choice["text"].(string)

	// Clean Gemma 4 markers
	for _, marker := range []string{"<channel|>", "<|channel|>", "<|/think|>", "<|think|>"} {
		text = strings.ReplaceAll(text, marker, "")
	}
	text = strings.TrimSpace(text)

	if text == "" {
		text = "НЕИЗВЕСТНО."
	}

	// Log speed if available
	timings, _ := result["timings"].(map[string]interface{})
	if tps, ok := timings["predicted_per_second"]; ok {
		log.Printf("☉ Magos: %v tok/s", tps)
	}

	return text, nil
}

// GetHistory returns conversation history
func (m *MagosAgent) GetHistory() []MagosMessage {
	return m.history
}

// ClearHistory resets conversation
func (m *MagosAgent) ClearHistory() {
	m.history = make([]MagosMessage, 0)
}
