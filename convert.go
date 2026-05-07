package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// ==================== Anthropic types ====================

type anthropicRequest struct {
	Model         string          `json:"model"`
	Messages      []anthropicMsg  `json:"messages"`
	System        json.RawMessage `json:"system,omitempty"`
	MaxTokens     int             `json:"max_tokens"`
	Stream        bool            `json:"stream"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool `json:"tools,omitempty"`
	ToolChoice    json.RawMessage `json:"tool_choice,omitempty"`
	Metadata      *anthropicMeta  `json:"metadata,omitempty"`
}

type anthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []contentBlock
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicMeta struct {
	UserID string `json:"user_id,omitempty"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // tool_result content
	Source    json.RawMessage `json:"source,omitempty"`  // image source
}

// ==================== OpenAI request types ====================

type openaiRequest struct {
	Model         string       `json:"model"`
	Messages      []openaiMsg  `json:"messages"`
	MaxTokens     int          `json:"max_tokens,omitempty"`
	Stream        bool         `json:"stream"`
	StreamOptions *streamOpts  `json:"stream_options,omitempty"`
	Temperature   *float64     `json:"temperature,omitempty"`
	TopP          *float64     `json:"top_p,omitempty"`
	Stop          []string     `json:"stop,omitempty"`
	Tools         []openaiTool `json:"tools,omitempty"`
	ToolChoice    interface{}  `json:"tool_choice,omitempty"`
	User          string       `json:"user,omitempty"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiMsg struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"`              // string, nil, or []contentPart
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function oaiTCFunc  `json:"function"`
}

type oaiTCFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string      `json:"type"`
	Function oaiToolFunc `json:"function"`
}

type oaiToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ==================== OpenAI response types ====================

type openaiResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   *openaiUsage   `json:"usage,omitempty"`
}

type openaiChoice struct {
	Index        int       `json:"index"`
	Message      openaiMsg `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ==================== OpenAI streaming chunk types ====================

type openaiChunk struct {
	ID      string              `json:"id"`
	Model   string              `json:"model"`
	Choices []openaiChunkChoice `json:"choices"`
	Usage   *openaiUsage        `json:"usage,omitempty"`
}

type openaiChunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type chunkDelta struct {
	Role             string        `json:"role,omitempty"`
	Content          *string       `json:"content,omitempty"`
	ReasoningContent *string       `json:"reasoning_content,omitempty"`
	ToolCalls        []chunkTC     `json:"tool_calls,omitempty"`
}

type chunkTC struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function *chunkTCFunc `json:"function,omitempty"`
}

type chunkTCFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ==================== Helpers ====================

func randMsgID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter", "insufficient_system_resource":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// ==================== Request conversion: Anthropic → OpenAI ====================

func anthropicToOpenAI(aReq *anthropicRequest) (*openaiRequest, error) {
	oReq := &openaiRequest{
		Model:       aReq.Model,
		MaxTokens:   aReq.MaxTokens,
		Stream:      aReq.Stream,
		Temperature: aReq.Temperature,
		TopP:        aReq.TopP,
	}

	if aReq.Stream {
		oReq.StreamOptions = &streamOpts{IncludeUsage: true}
	}
	if len(aReq.StopSequences) > 0 {
		oReq.Stop = aReq.StopSequences
	}
	if aReq.Metadata != nil && aReq.Metadata.UserID != "" {
		oReq.User = aReq.Metadata.UserID
	}

	// System prompt
	if len(aReq.System) > 0 {
		sysText := extractSystemText(aReq.System)
		if sysText != "" {
			oReq.Messages = append(oReq.Messages, openaiMsg{Role: "system", Content: sysText})
		}
	}

	// Messages
	for _, msg := range aReq.Messages {
		converted, err := convertMessage(msg)
		if err != nil {
			return nil, err
		}
		oReq.Messages = append(oReq.Messages, converted...)
	}

	// Tools
	for _, t := range aReq.Tools {
		oReq.Tools = append(oReq.Tools, openaiTool{
			Type: "function",
			Function: oaiToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	// Tool choice
	if len(aReq.ToolChoice) > 0 {
		oReq.ToolChoice = convertToolChoice(aReq.ToolChoice)
	}

	return oReq, nil
}

func extractSystemText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func convertToolChoice(raw json.RawMessage) interface{} {
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name,omitempty"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		return map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": tc.Name},
		}
	default:
		return "auto"
	}
}

func convertMessage(msg anthropicMsg) ([]openaiMsg, error) {
	// Content as plain string
	var contentStr string
	if err := json.Unmarshal(msg.Content, &contentStr); err == nil {
		return []openaiMsg{{Role: msg.Role, Content: contentStr}}, nil
	}

	// Content as array of blocks
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil, fmt.Errorf("cannot parse content for role=%s: %w", msg.Role, err)
	}

	if msg.Role == "assistant" {
		return convertAssistantBlocks(blocks)
	}
	return convertUserBlocks(blocks)
}

func convertAssistantBlocks(blocks []contentBlock) ([]openaiMsg, error) {
	var textParts []string
	var toolCalls []openaiToolCall

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "tool_use":
			argsBytes, _ := json.Marshal(b.Input)
			if argsBytes == nil {
				argsBytes = []byte("{}")
			}
			toolCalls = append(toolCalls, openaiToolCall{
				ID:   b.ID,
				Type: "function",
				Function: oaiTCFunc{Name: b.Name, Arguments: string(argsBytes)},
			})
		// thinking blocks are dropped
		}
	}

	m := openaiMsg{Role: "assistant"}
	if len(textParts) > 0 {
		m.Content = strings.Join(textParts, "")
	}
	if len(toolCalls) > 0 {
		m.ToolCalls = toolCalls
	}
	return []openaiMsg{m}, nil
}

func convertUserBlocks(blocks []contentBlock) ([]openaiMsg, error) {
	// User messages can mix text, image, and tool_result blocks.
	// tool_result → separate role=tool messages
	// text/image → collected into one user message
	var result []openaiMsg
	var userParts []contentBlock // non-tool_result blocks to collect

	for _, b := range blocks {
		if b.Type == "tool_result" {
			// Flush any pending user content before the tool messages
			if len(userParts) > 0 {
				result = append(result, buildUserMsg(userParts))
				userParts = nil
			}
			result = append(result, openaiMsg{
				Role:       "tool",
				Content:    extractToolResultText(b.Content),
				ToolCallID: b.ToolUseID,
			})
		} else {
			userParts = append(userParts, b)
		}
	}

	if len(userParts) > 0 {
		result = append(result, buildUserMsg(userParts))
	}
	return result, nil
}

func buildUserMsg(blocks []contentBlock) openaiMsg {
	// If only one text block, use string content
	if len(blocks) == 1 && blocks[0].Type == "text" {
		return openaiMsg{Role: "user", Content: blocks[0].Text}
	}
	// Otherwise build multimodal array
	var parts []map[string]interface{}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, map[string]interface{}{"type": "text", "text": b.Text})
		case "image":
			// Translate Anthropic image source to OpenAI image_url
			var src struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			}
			if err := json.Unmarshal(b.Source, &src); err == nil && src.Type == "base64" {
				dataURL := "data:" + src.MediaType + ";base64," + src.Data
				parts = append(parts, map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]string{"url": dataURL},
				})
			}
		}
	}
	return openaiMsg{Role: "user", Content: parts}
}

func extractToolResultText(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return ""
	}
	// Try string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content blocks → concatenate text
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

// ==================== Non-streaming response: OpenAI → Anthropic ====================

func openaiToAnthropic(resp *openaiResponse, originalModel string) map[string]interface{} {
	result := map[string]interface{}{
		"id":            randMsgID(),
		"type":          "message",
		"role":          "assistant",
		"model":         originalModel,
		"stop_sequence": nil,
	}

	var content []map[string]interface{}

	if len(resp.Choices) > 0 {
		ch := resp.Choices[0]

		// Text content
		if ch.Message.Content != nil {
			if text, ok := ch.Message.Content.(string); ok && text != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": text})
			}
		}

		// Tool calls
		for _, tc := range ch.Message.ToolCalls {
			var input interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = map[string]interface{}{}
			}
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}

		result["stop_reason"] = mapFinishReason(ch.FinishReason)
	} else {
		result["stop_reason"] = "end_turn"
	}

	if content == nil {
		content = []map[string]interface{}{}
	}
	result["content"] = content

	// Usage
	usage := map[string]int{"input_tokens": 0, "output_tokens": 0}
	if resp.Usage != nil {
		usage["input_tokens"] = resp.Usage.PromptTokens
		usage["output_tokens"] = resp.Usage.CompletionTokens
	}
	result["usage"] = usage

	return result
}

// ==================== Streaming response: OpenAI SSE → Anthropic SSE ====================

type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *sseWriter) writeEvent(event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, jsonData)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func streamOpenAIToAnthropic(resp *http.Response, w http.ResponseWriter, originalModel string) {
	flusher, _ := w.(http.Flusher)
	sw := &sseWriter{w: w, flusher: flusher}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	msgID := randMsgID()

	// message_start
	sw.writeEvent("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         originalModel,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	})

	// ping
	sw.writeEvent("ping", map[string]string{"type": "ping"})

	// State machine
	type blockState struct {
		index     int
		blockType string // "text" or "tool_use"
	}

	var current *blockState
	nextIndex := 0
	var outputTokens int

	// Track seen tool call indices to detect new tool calls
	seenToolIndices := map[int]bool{}

	closeCurrentBlock := func() {
		if current != nil {
			sw.writeEvent("content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": current.index,
			})
			current = nil
		}
	}

	startTextBlock := func() {
		closeCurrentBlock()
		idx := nextIndex
		nextIndex++
		current = &blockState{index: idx, blockType: "text"}
		sw.writeEvent("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]interface{}{
				"type": "text",
				"text": "",
			},
		})
	}

	startToolBlock := func(id, name string) {
		closeCurrentBlock()
		idx := nextIndex
		nextIndex++
		current = &blockState{index: idx, blockType: "tool_use"}
		sw.writeEvent("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]interface{}{
				"type":  "tool_use",
				"id":    id,
				"name":  name,
			},
		})
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var chunk openaiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Capture usage from final chunk
		if chunk.Usage != nil {
			outputTokens = chunk.Usage.CompletionTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]

		// Text content delta
		if ch.Delta.Content != nil && *ch.Delta.Content != "" {
			if current == nil || current.blockType != "text" {
				startTextBlock()
			}
			sw.writeEvent("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": current.index,
				"delta": map[string]string{
					"type": "text_delta",
					"text": *ch.Delta.Content,
				},
			})
		}

		// Reasoning content (DeepSeek R1) — output as regular text
		if ch.Delta.ReasoningContent != nil && *ch.Delta.ReasoningContent != "" {
			if current == nil || current.blockType != "text" {
				startTextBlock()
			}
			sw.writeEvent("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": current.index,
				"delta": map[string]string{
					"type": "text_delta",
					"text": *ch.Delta.ReasoningContent,
				},
			})
		}

		// Tool calls
		for _, tc := range ch.Delta.ToolCalls {
			if !seenToolIndices[tc.Index] {
				// New tool call
				seenToolIndices[tc.Index] = true
				name := ""
				if tc.Function != nil {
					name = tc.Function.Name
				}
				startToolBlock(tc.ID, name)
			}
			// Arguments delta
			if tc.Function != nil && tc.Function.Arguments != "" {
				if current != nil {
					sw.writeEvent("content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": current.index,
						"delta": map[string]string{
							"type":         "input_json_delta",
							"partial_json": tc.Function.Arguments,
						},
					})
				}
			}
		}

		// Finish reason
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			closeCurrentBlock()
			sw.writeEvent("message_delta", map[string]interface{}{
				"type": "message_delta",
				"delta": map[string]interface{}{
					"stop_reason":   mapFinishReason(*ch.FinishReason),
					"stop_sequence": nil,
				},
				"usage": map[string]int{"output_tokens": outputTokens},
			})
		}
	}

	// Ensure blocks are closed even if no explicit finish_reason
	closeCurrentBlock()

	sw.writeEvent("message_stop", map[string]string{"type": "message_stop"})
}

// ==================== Handler ====================

func handleMessagesConvert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var aReq anthropicRequest
	if err := json.Unmarshal(body, &aReq); err != nil {
		http.Error(w, "invalid anthropic request: "+err.Error(), http.StatusBadRequest)
		return
	}

	cfgMu.RLock()
	c := cfg
	cfgMu.RUnlock()

	// Record the original model name the client sent
	originalModel := aReq.Model
	for src, dst := range c.ModelMap {
		if strings.EqualFold(aReq.Model, src) {
			aReq.Model = dst
			log.Printf("[convert] model replaced: %s -> %s", originalModel, dst)
			break
		}
	}

	oReq, err := anthropicToOpenAI(&aReq)
	if err != nil {
		http.Error(w, "conversion error: "+err.Error(), http.StatusBadRequest)
		return
	}

	reqBody, _ := json.Marshal(oReq)

	upstreamURL := strings.TrimRight(c.UpstreamURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, "build upstream request failed", http.StatusInternalServerError)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.UpstreamToken)
	httpReq.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[convert] upstream error: %v", err)
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Upstream error — forward as Anthropic error
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		log.Printf("[convert] upstream %d: %s", resp.StatusCode, string(errBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		errResp, _ := json.Marshal(map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "api_error",
				"message": fmt.Sprintf("upstream returned %d: %s", resp.StatusCode, string(errBody)),
			},
		})
		w.Write(errResp)
		return
	}

	if aReq.Stream {
		streamOpenAIToAnthropic(resp, w, originalModel)
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "read upstream response failed", http.StatusBadGateway)
			return
		}
		var oResp openaiResponse
		if err := json.Unmarshal(respBody, &oResp); err != nil {
			log.Printf("[convert] cannot parse upstream response: %v  body=%s", err, string(respBody))
			http.Error(w, "cannot parse upstream response", http.StatusBadGateway)
			return
		}
		anthropicResp := openaiToAnthropic(&oResp, originalModel)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResp)
	}
}

// handleCountTokensStub 是 /v1/messages/count_tokens 的简单估算实现。
func handleCountTokensStub(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	// Rough estimate: total characters / 4
	tokens := len(body) / 4
	if tokens < 1 {
		tokens = 1
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"input_tokens": tokens})
}
