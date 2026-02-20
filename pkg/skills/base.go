package skills

type ToolResult struct {
	ForLLM  string
	ForUser string
	Silent  bool
	IsError bool
	Async   bool
	Err     error
}

func ErrorResult(message string) *ToolResult {
	return &ToolResult{
		ForLLM:  message,
		ForUser: message,
		IsError: true,
	}
}

func (r *ToolResult) WithError(err error) *ToolResult {
	r.Err = err
	return r
}
