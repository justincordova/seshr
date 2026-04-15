package parser

import (
	"encoding/json"
	"time"
)

// rawRecord mirrors a single JSONL line. Only fields the parser actually
// reads are declared — everything else is preserved implicitly by the
// unmarshaller and ignored. See SPEC §6.2.
type rawRecord struct {
	Type             string          `json:"type"`
	UUID             string          `json:"uuid"`
	ParentUUID       string          `json:"parentUuid"`
	Timestamp        time.Time       `json:"timestamp"`
	SessionID        string          `json:"sessionId"`
	ToolUseID        string          `json:"tool_use_id"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Message          json.RawMessage `json:"message"`
}

// rawMessage is the nested message object. Content is left as RawMessage
// because it can be either a plain string (user turns) or an array of
// content blocks (assistant turns).
type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Usage   struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	IsError bool `json:"is_error"`
}

// rawBlock is one entry in an assistant content array.
type rawBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
}
