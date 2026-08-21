package server

import (
	"strings"
	"testing"
)

// Gemma4PromptBuilder builds prompts in Gemma 4 chat format.
type Gemma4PromptBuilder struct {
	SystemPrompt string
}

// BuildPrompt constructs a Gemma 4 formatted prompt.
func (b *Gemma4PromptBuilder) BuildPrompt(messages []Gemma4Message) string {
	var sb strings.Builder

	sb.WriteString("<bos>")

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			sb.WriteString("<|turn>system\n")
			sb.WriteString(msg.Content)
			sb.WriteString("<turn|>\n")
		case "user":
			sb.WriteString("<|turn>user\n")
			sb.WriteString(msg.Content)
			sb.WriteString("<turn|>\n")
		case "assistant":
			sb.WriteString("<|turn>model\n<|channel>text\n")
			sb.WriteString(msg.Content)
			sb.WriteString("<turn|>\n")
		}
	}

	// Start model turn with thought channel
	sb.WriteString("<|turn>model\n<|channel>thought")

	return sb.String()
}

// Gemma4Message represents a chat message for Gemma 4 format.
type Gemma4Message struct {
	Role    string
	Content string
}

// CleanGemma4Response removes Gemma 4 channel/thinking markers from response.
func CleanGemma4Response(raw string) string {
	// Remove thinking blocks
	for {
		start := strings.Index(raw, "<|channel>thought")
		if start == -1 {
			break
		}
		end := strings.Index(raw[start:], "<channel|>")
		if end == -1 {
			raw = raw[:start]
			break
		}
		raw = raw[:start] + raw[start+end+len("<channel|>"):]
	}

	// Remove text channel prefix
	raw = strings.ReplaceAll(raw, "<|channel>text\n", "")
	raw = strings.ReplaceAll(raw, "<|channel>text", "")

	// Remove remaining markers
	raw = strings.ReplaceAll(raw, "<channel|>", "")
	raw = strings.ReplaceAll(raw, "<|channel|>", "")

	// Remove think markers (with content between them)
	for {
		start := strings.Index(raw, "<|think|>")
		if start == -1 {
			break
		}
		end := strings.Index(raw[start:], "<|/think|>")
		if end == -1 {
			raw = raw[:start]
			break
		}
		raw = raw[:start] + raw[start+end+len("<|/think|>"):]
	}

	return strings.TrimSpace(raw)
}

func TestGemma4Format(t *testing.T) {
	builder := &Gemma4PromptBuilder{
		SystemPrompt: "Ты — Анафемтрон.",
	}

	messages := []Gemma4Message{
		{Role: "system", Content: "Ты — Анафемтрон."},
		{Role: "user", Content: "Кто ты?"},
	}

	prompt := builder.BuildPrompt(messages)

	// Check required markers
	if !strings.HasPrefix(prompt, "<bos>") {
		t.Error("Prompt should start with <bos>")
	}
	if !strings.Contains(prompt, "<|turn>system\n") {
		t.Error("Missing system turn marker")
	}
	if !strings.Contains(prompt, "<|turn>user\n") {
		t.Error("Missing user turn marker")
	}
	if !strings.Contains(prompt, "<|channel>thought") {
		t.Error("Missing thought channel marker")
	}
	if !strings.Contains(prompt, "Ты — Анафемтрон.") {
		t.Error("Missing system prompt content")
	}
	if !strings.Contains(prompt, "Кто ты?") {
		t.Error("Missing user message content")
	}
}

func TestGemma4Format_WithHistory(t *testing.T) {
	builder := &Gemma4PromptBuilder{}

	messages := []Gemma4Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}

	prompt := builder.BuildPrompt(messages)

	// Should have all turns (system + user + assistant + user + thought)
	if strings.Count(prompt, "<|turn>") < 4 {
		t.Errorf("Expected at least 4 turns, got %d", strings.Count(prompt, "<|turn>"))
	}
	// Assistant turn should have text channel
	if !strings.Contains(prompt, "<|channel>text\n") {
		t.Error("Missing text channel in assistant turn")
	}
}

func TestCleanGemma4Response(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean text",
			input:    "Привет! Как дела?",
			expected: "Привет! Как дела?",
		},
		{
			name:     "remove thought block",
			input:    "<|channel>thoughtДумаю...<channel|>Привет!",
			expected: "Привет!",
		},
		{
			name:     "remove text prefix",
			input:    "<|channel>text\nОтвет здесь",
			expected: "Ответ здесь",
		},
		{
			name:     "remove multiple markers",
			input:    "<|channel>thought思考中...<channel|><|channel>text\nРезультат",
			expected: "Результат",
		},
		{
			name:     "remove think markers",
			input:    "<|think|>Думаю<|/think|>Ответ",
			expected: "Ответ",
		},
		{
			name:     "empty response",
			input:    "",
			expected: "",
		},
		{
			name:     "only markers",
			input:    "<|channel>thought<channel|>",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanGemma4Response(tt.input)
			if result != tt.expected {
				t.Errorf("CleanGemma4Response(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCleanGemma4Response_PreservesContent(t *testing.T) {
	input := "<|channel>thoughtАнализирую код...<channel|>Я — Анафемтрон. Занесено."
	result := CleanGemma4Response(input)

	if !strings.Contains(result, "Анафемтрон") {
		t.Error("Should preserve 'Анафемтрон'")
	}
	if !strings.Contains(result, "Занесено") {
		t.Error("Should preserve 'Занесено'")
	}
	if strings.Contains(result, "Анализирую") {
		t.Error("Should remove thinking content")
	}
}

func TestGemma4Format_NoSystemPrompt(t *testing.T) {
	builder := &Gemma4PromptBuilder{}

	messages := []Gemma4Message{
		{Role: "user", Content: "Привет"},
	}

	prompt := builder.BuildPrompt(messages)

	if !strings.Contains(prompt, "<|turn>user\n") {
		t.Error("Missing user turn")
	}
	if !strings.Contains(prompt, "<|channel>thought") {
		t.Error("Missing thought channel")
	}
}
