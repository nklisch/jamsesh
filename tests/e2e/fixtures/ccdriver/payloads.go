// Package ccdriver simulates Claude Code's hook lifecycle by invoking the
// jamsesh binary's hook subcommands with crafted JSON stdin.
package ccdriver

import "encoding/json"

// SessionStartInput mirrors the jamsesh SessionStart hook input schema.
type SessionStartInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

// SessionStartOutput mirrors the jamsesh SessionStart hook output schema.
type SessionStartOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// UserPromptSubmitInput mirrors the jamsesh UserPromptSubmit hook input schema.
type UserPromptSubmitInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

// UserPromptSubmitOutput mirrors the jamsesh UserPromptSubmit hook output schema.
type UserPromptSubmitOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// PreToolUseInput mirrors the jamsesh PreToolUse hook input schema.
type PreToolUseInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

// PreToolUseOutput mirrors the jamsesh PreToolUse hook output schema.
type PreToolUseOutput struct {
	PermissionDecision string `json:"permissionDecision"`
	Reason             string `json:"reason,omitempty"`
}

// ToolResponse mirrors the CC PostToolUse tool response subset.
type ToolResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// PostToolUseInput mirrors the jamsesh PostToolUse hook input schema.
type PostToolUseInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   ToolResponse    `json:"tool_response"`
}

// PostToolUseOutput mirrors the jamsesh PostToolUse hook output schema.
type PostToolUseOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// StopInput mirrors the jamsesh Stop hook input schema.
type StopInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

// StopOutput mirrors the jamsesh Stop hook output schema.
type StopOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// SessionEndInput mirrors the jamsesh SessionEnd hook input schema.
type SessionEndInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

// SessionEndOutput mirrors the jamsesh SessionEnd hook output schema (v1: empty).
type SessionEndOutput struct{}
