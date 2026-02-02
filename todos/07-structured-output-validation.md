# Structured Output Validation

**Priority**: Low-Medium
**Effort**: Low
**Impact**: Medium

## Problem

Some tasks require structured output (API specs, config files, JSON data). Currently, Dex marks tasks complete based on quality gates (tests pass, lint passes), but doesn't validate that the output matches an expected schema.

This leads to:
- Invalid JSON/YAML in generated config files
- Missing required fields in API responses
- Inconsistent output formats across similar tasks

**Inspired by Goose's FinalOutputTool with JSON Schema validation.**

## Solution Overview

Allow tasks to specify an expected output schema. Before marking complete, validate the output against this schema:

1. **Schema definition**: Task can include a JSON Schema for expected output
2. **Validation tool**: Agent must call `task_output` with its result
3. **Error feedback**: Invalid output returns structured errors
4. **Retry loop**: Agent can fix and retry until schema validates

## Task Schema Definition

Extend the Task model:

```go
// internal/db/models.go

type Task struct {
    // ... existing fields

    // Optional: expected output schema
    OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}
```

Example task with output schema:

```json
{
    "title": "Generate API endpoints documentation",
    "description": "Create OpenAPI spec for the user endpoints",
    "output_schema": {
        "type": "object",
        "properties": {
            "openapi": {"type": "string", "pattern": "^3\\."},
            "info": {
                "type": "object",
                "properties": {
                    "title": {"type": "string"},
                    "version": {"type": "string"}
                },
                "required": ["title", "version"]
            },
            "paths": {
                "type": "object",
                "minProperties": 1
            }
        },
        "required": ["openapi", "info", "paths"]
    }
}
```

## Task Output Tool

```go
// internal/tools/task_output.go

var TaskOutputTool = Tool{
    Name: "task_output",
    Description: `Submit the final output for this task.

This tool validates your output against the task's expected schema.
If validation fails, you'll receive specific error messages to help fix the issues.

You MUST call this tool with your final output before the task can be marked complete.`,
    Parameters: json.RawMessage(`{
        "type": "object",
        "properties": {
            "output": {
                "description": "The final output (JSON object matching the task schema)"
            }
        },
        "required": ["output"]
    }`),
    Annotations: ToolAnnotations{
        ReadOnlyHint:    false,
        DestructiveHint: false,
        IdempotentHint:  true,
    },
}
```

## Validation Engine

```go
// internal/session/output_validator.go

import "github.com/santhosh-tekuri/jsonschema/v5"

type OutputValidator struct {
    schema *jsonschema.Schema
}

func NewOutputValidator(schemaBytes json.RawMessage) (*OutputValidator, error) {
    if len(schemaBytes) == 0 {
        return nil, nil // No schema = no validation
    }

    compiler := jsonschema.NewCompiler()
    if err := compiler.AddResource("schema.json", bytes.NewReader(schemaBytes)); err != nil {
        return nil, fmt.Errorf("invalid schema: %w", err)
    }

    schema, err := compiler.Compile("schema.json")
    if err != nil {
        return nil, fmt.Errorf("failed to compile schema: %w", err)
    }

    return &OutputValidator{schema: schema}, nil
}

func (v *OutputValidator) Validate(output interface{}) *ValidationResult {
    if v == nil || v.schema == nil {
        return &ValidationResult{Valid: true}
    }

    err := v.schema.Validate(output)
    if err == nil {
        return &ValidationResult{Valid: true}
    }

    // Extract detailed errors
    result := &ValidationResult{
        Valid:  false,
        Errors: []ValidationError{},
    }

    if ve, ok := err.(*jsonschema.ValidationError); ok {
        result.Errors = extractValidationErrors(ve)
    } else {
        result.Errors = append(result.Errors, ValidationError{
            Path:    "/",
            Message: err.Error(),
        })
    }

    return result
}

type ValidationResult struct {
    Valid  bool
    Errors []ValidationError
}

type ValidationError struct {
    Path     string `json:"path"`     // JSON pointer to the error location
    Message  string `json:"message"`  // Human-readable error
    Expected string `json:"expected"` // What was expected
    Received string `json:"received"` // What was received
}

func extractValidationErrors(ve *jsonschema.ValidationError) []ValidationError {
    errors := []ValidationError{}

    var extract func(e *jsonschema.ValidationError)
    extract = func(e *jsonschema.ValidationError) {
        if e.Message != "" {
            errors = append(errors, ValidationError{
                Path:    e.InstanceLocation,
                Message: e.Message,
            })
        }
        for _, cause := range e.Causes {
            extract(cause)
        }
    }
    extract(ve)

    return errors
}
```

## Integration with Tool Execution

```go
func (r *RalphLoop) executeTaskOutput(call ToolCall) (string, error) {
    // Check if task has schema
    if len(r.task.OutputSchema) == 0 {
        // No schema - just store the output
        r.taskOutput = call.Params["output"]
        return "Output recorded. You may now complete the task.", nil
    }

    // Parse and validate
    var output interface{}
    if err := json.Unmarshal([]byte(call.Params["output"].(string)), &output); err != nil {
        return "", fmt.Errorf("output is not valid JSON: %w", err)
    }

    validator, err := NewOutputValidator(r.task.OutputSchema)
    if err != nil {
        return "", fmt.Errorf("schema error: %w", err)
    }

    result := validator.Validate(output)

    if result.Valid {
        r.taskOutput = output
        r.taskOutputValidated = true
        return "Output validated successfully. You may now complete the task.", nil
    }

    // Format validation errors for the agent
    return formatValidationErrors(result.Errors), nil
}

func formatValidationErrors(errors []ValidationError) string {
    var sb strings.Builder

    sb.WriteString("## Validation Failed\n\n")
    sb.WriteString("Your output does not match the expected schema:\n\n")

    for i, err := range errors {
        sb.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, err.Path, err.Message))
    }

    sb.WriteString("\nPlease fix these issues and call `task_output` again.")

    return sb.String()
}
```

## Quality Gate Integration

Add output validation to quality gates:

```go
func (qg *QualityGate) runAllGates() []GateResult {
    results := []GateResult{}

    // Existing gates: build, test, lint...

    // Add output validation if task has schema
    if len(qg.task.OutputSchema) > 0 {
        results = append(results, qg.checkOutputValidation())
    }

    return results
}

func (qg *QualityGate) checkOutputValidation() GateResult {
    if !qg.session.taskOutputValidated {
        return GateResult{
            Name:    "output_schema",
            Status:  "failed",
            Message: "Task has output schema but task_output not called or validation failed",
        }
    }

    return GateResult{
        Name:   "output_schema",
        Status: "passed",
    }
}
```

## Storing Validated Output

```go
// internal/db/models.go

type Task struct {
    // ... existing fields

    OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
    ValidatedOutput json.RawMessage `json:"validated_output,omitempty"` // Stored on completion
}

// On task completion
func (r *RalphLoop) completeTask() error {
    if r.taskOutputValidated && r.taskOutput != nil {
        outputJSON, _ := json.Marshal(r.taskOutput)
        r.task.ValidatedOutput = outputJSON
        r.db.UpdateTask(r.task)
    }

    // ... rest of completion logic
}
```

## CLI Integration

```bash
# Create task with output schema
dex task create "Generate user API spec" --output-schema ./schemas/openapi.json

# View task output
dex task output 42

# Export task output
dex task output 42 --format json > output.json
```

## API Endpoints

```
GET /api/tasks/{id}/output        - Get validated output
GET /api/tasks/{id}/output-schema - Get the output schema
```

## Prompt Addition

When a task has an output schema, add to the system prompt:

```go
func (r *RalphLoop) buildOutputSchemaPrompt() string {
    if len(r.task.OutputSchema) == 0 {
        return ""
    }

    return fmt.Sprintf(`
## Required Output Format

This task requires structured output. Before completing, you MUST:

1. Call the %stask_output%s tool with your result
2. Your output must validate against this schema:

%s

If validation fails, you'll receive error messages. Fix the issues and try again.
`, "`", "`", prettyPrintSchema(r.task.OutputSchema))
}
```

## Configuration

```go
type OutputValidationConfig struct {
    Enabled          bool `json:"enabled"`           // Default: true
    StrictMode       bool `json:"strict_mode"`       // Fail on additional properties
    MaxOutputSize    int  `json:"max_output_size"`   // Default: 1MB
    MaxSchemaDepth   int  `json:"max_schema_depth"`  // Default: 10
}
```

## Acceptance Criteria

- [ ] Task model extended with OutputSchema field
- [ ] OutputValidator with JSON Schema support
- [ ] task_output tool available when task has schema
- [ ] Validation errors formatted clearly for agent
- [ ] Agent can retry after validation failure
- [ ] Quality gate includes output validation
- [ ] Validated output stored with completed task
- [ ] CLI can view/export task output
- [ ] API endpoints for output retrieval
- [ ] System prompt includes schema when present

## Future Enhancements

- **YAML schema support**: Validate YAML outputs
- **Custom validators**: Plugin system for domain-specific validation
- **Schema generation**: Auto-generate schema from example output
- **Schema library**: Reusable schemas for common output types (OpenAPI, Terraform, etc.)

## Relationship to Other Docs

| Doc | Relationship |
|-----|-------------|
| **03-loop-resilience** | Output validation is a quality gate |
| **04-hat-system** | task_output tool available to editor hat |
