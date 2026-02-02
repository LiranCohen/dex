package session

// TerminationReason indicates why a session ended
type TerminationReason string

const (
	// Normal completion
	TerminationCompleted     TerminationReason = "completed"
	TerminationHatTransition TerminationReason = "hat_transition"

	// Budget/limit exhaustion
	TerminationMaxIterations TerminationReason = "max_iterations"
	TerminationMaxTokens     TerminationReason = "max_tokens"
	TerminationMaxCost       TerminationReason = "max_cost"
	TerminationMaxRuntime    TerminationReason = "max_runtime"

	// Quality gate exhaustion
	TerminationQualityGateExhausted TerminationReason = "quality_gate_exhausted"

	// Loop health issues
	TerminationLoopThrashing       TerminationReason = "loop_thrashing"
	TerminationConsecutiveFailures TerminationReason = "consecutive_failures"
	TerminationValidationFailure   TerminationReason = "validation_failure"
	TerminationRepetitionLoop      TerminationReason = "repetition_loop"

	// External termination
	TerminationUserStopped TerminationReason = "user_stopped"
	TerminationError       TerminationReason = "error"
)

// TerminationInfo provides detailed information about why a session ended
type TerminationInfo struct {
	Reason              TerminationReason `json:"reason"`
	Message             string            `json:"message"`
	Iterations          int               `json:"iterations"`
	TokensUsed          int64             `json:"tokens_used"`
	CostUSD             float64           `json:"cost_usd"`
	QualityGateAttempts int               `json:"quality_gate_attempts,omitempty"`
}

// IsSuccess returns true if the termination represents a successful completion
func (t TerminationReason) IsSuccess() bool {
	return t == TerminationCompleted || t == TerminationHatTransition
}

// IsExhaustion returns true if the termination was due to resource exhaustion
func (t TerminationReason) IsExhaustion() bool {
	switch t {
	case TerminationMaxIterations, TerminationMaxTokens, TerminationMaxCost, TerminationMaxRuntime,
		TerminationQualityGateExhausted, TerminationLoopThrashing, TerminationConsecutiveFailures,
		TerminationValidationFailure, TerminationRepetitionLoop:
		return true
	default:
		return false
	}
}

// String returns a human-readable description of the termination reason
func (t TerminationReason) String() string {
	switch t {
	case TerminationCompleted:
		return "Task completed successfully"
	case TerminationHatTransition:
		return "Transitioned to different hat"
	case TerminationMaxIterations:
		return "Maximum iterations reached"
	case TerminationMaxTokens:
		return "Token budget exhausted"
	case TerminationMaxCost:
		return "Cost budget exhausted"
	case TerminationMaxRuntime:
		return "Maximum runtime exceeded"
	case TerminationQualityGateExhausted:
		return "Quality gate attempts exhausted"
	case TerminationLoopThrashing:
		return "Loop thrashing detected"
	case TerminationConsecutiveFailures:
		return "Too many consecutive failures"
	case TerminationValidationFailure:
		return "Too many validation failures"
	case TerminationRepetitionLoop:
		return "Tool repetition loop detected"
	case TerminationUserStopped:
		return "Stopped by user"
	case TerminationError:
		return "Error occurred"
	default:
		return string(t)
	}
}
