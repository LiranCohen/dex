// Package tools provides shared tool infrastructure for Dex
// Used by both Quest chat (read-only) and RalphLoop (read-write)
package tools

// Tool represents a tool that can be executed by an AI agent
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	ReadOnly    bool           `json:"-"` // Not sent to Claude, used for filtering
}

// Result represents the outcome of executing a tool
type Result struct {
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// Call represents a recorded tool invocation with its result
type Call struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input"`
	Output     string         `json:"output"`
	IsError    bool           `json:"is_error"`
	DurationMs int64          `json:"duration_ms"`
}

// Set represents a collection of tools for a specific use case
type Set struct {
	tools map[string]Tool
}

// NewSet creates a new tool set from a slice of tools
func NewSet(tools []Tool) *Set {
	s := &Set{
		tools: make(map[string]Tool),
	}
	for _, t := range tools {
		s.tools[t.Name] = t
	}
	return s
}

// Get returns a tool by name, or nil if not found
func (s *Set) Get(name string) *Tool {
	if t, ok := s.tools[name]; ok {
		return &t
	}
	return nil
}

// Has returns true if the tool set contains a tool with the given name
func (s *Set) Has(name string) bool {
	_, ok := s.tools[name]
	return ok
}

// All returns all tools in the set as a slice
func (s *Set) All() []Tool {
	result := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		result = append(result, t)
	}
	return result
}

// Names returns the names of all tools in the set
func (s *Set) Names() []string {
	result := make([]string, 0, len(s.tools))
	for name := range s.tools {
		result = append(result, name)
	}
	return result
}

// ToAnthropicFormat converts tools to the format expected by Anthropic's API
func (s *Set) ToAnthropicFormat() []map[string]any {
	result := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		result = append(result, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		})
	}
	return result
}
