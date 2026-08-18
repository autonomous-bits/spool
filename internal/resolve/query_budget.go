package resolve

import "time"

const (
	defaultMaxRows          = 1_000
	defaultMaxResponseBytes = 1 << 20
	defaultMaxDepth         = 32
	defaultMaxVisited       = 10_000
	defaultTimeout          = 10 * time.Second
)

// QueryBudget bounds result size and traversal resources for resolution tools.
type QueryBudget struct {
	// MaxRows limits diff changes and context entries.
	MaxRows int `json:"maxRows"`
	// MaxResponseBytes limits JSON-encoded diff responses.
	MaxResponseBytes int `json:"maxResponseBytes"`
	// MaxDepth limits impact traversal edge distance.
	MaxDepth int `json:"maxDepth"`
	// MaxVisited limits impact traversal nodes.
	MaxVisited int `json:"maxVisited"`
	// Timeout is the maximum caller-supplied operation duration.
	Timeout time.Duration `json:"timeout"`
}

// QueryBudgetRequest optionally narrows configured query limits.
type QueryBudgetRequest struct {
	// MaxRows narrows the maximum diff rows.
	MaxRows *int `json:"maxRows,omitempty"`
	// MaxResponseBytes narrows the maximum diff response size.
	MaxResponseBytes *int `json:"maxResponseBytes,omitempty"`
	// MaxDepth narrows the maximum impact depth.
	MaxDepth *int `json:"maxDepth,omitempty"`
	// MaxVisited narrows the maximum impact traversal size.
	MaxVisited *int `json:"maxVisited,omitempty"`
	// Timeout narrows the maximum operation duration.
	Timeout *time.Duration `json:"timeout,omitempty"`
}

// DefaultQueryBudget returns the built-in upper bounds for query execution.
func DefaultQueryBudget() QueryBudget {
	return QueryBudget{
		MaxRows:          defaultMaxRows,
		MaxResponseBytes: defaultMaxResponseBytes,
		MaxDepth:         defaultMaxDepth,
		MaxVisited:       defaultMaxVisited,
		Timeout:          defaultTimeout,
	}
}

// NormalizeQueryBudget applies configured limits then only request values that narrow them.
func NormalizeQueryBudget(request QueryBudgetRequest, configured *QueryBudget) QueryBudget {
	effective := DefaultQueryBudget()
	if configured != nil {
		effective = *configured
	}
	if request.MaxRows != nil && *request.MaxRows < effective.MaxRows {
		effective.MaxRows = *request.MaxRows
	}
	if request.MaxResponseBytes != nil && *request.MaxResponseBytes < effective.MaxResponseBytes {
		effective.MaxResponseBytes = *request.MaxResponseBytes
	}
	if request.MaxDepth != nil && *request.MaxDepth < effective.MaxDepth {
		effective.MaxDepth = *request.MaxDepth
	}
	if request.MaxVisited != nil && *request.MaxVisited < effective.MaxVisited {
		effective.MaxVisited = *request.MaxVisited
	}
	if request.Timeout != nil && *request.Timeout < effective.Timeout {
		effective.Timeout = *request.Timeout
	}
	return effective
}
