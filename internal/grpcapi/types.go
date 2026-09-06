package grpcapi

type SubmitRequest struct {
	Workflow string         `json:"workflow"`
	Version  int32          `json:"version"`
	Input    map[string]any `json:"input"`
}

type SubmitResponse struct {
	RunId string `json:"run_id"`
}

type GetRunRequest struct {
	Namespace string `json:"namespace"`
	RunId     string `json:"run_id"`
}

type RunResponse struct {
	Id        string `json:"id"`
	Namespace string `json:"namespace"`
	Workflow  string `json:"workflow"`
	Version   int32  `json:"version"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
}

type ListRunsRequest struct {
	Namespace string `json:"namespace"`
	Limit     int32  `json:"limit"`
}

type ListRunsResponse struct {
	Runs  []*RunResponse `json:"runs"`
	Total int32          `json:"total"`
}

type CancelRunRequest struct {
	Namespace string `json:"namespace"`
	RunId     string `json:"run_id"`
}

type CancelRunResponse struct {
	Success bool `json:"success"`
}

type CreateDefinitionRequest struct {
	Name    string      `json:"name"`
	Version int32       `json:"version"`
	Steps   []*StepSpec `json:"steps"`
}

type StepSpec struct {
	Id        string   `json:"id"`
	Handler   string   `json:"handler"`
	Depends   []string `json:"depends"`
	TimeoutNs int64    `json:"timeout_ns"`
}

type DefinitionResponse struct {
	Name    string `json:"name"`
	Version int32  `json:"version"`
}

type PublishEventRequest struct {
	Type      string         `json:"type"`
	Namespace string         `json:"namespace"`
	RunId     string         `json:"run_id"`
	StepId    string         `json:"step_id,omitempty"`
	Payload   map[string]any `json:"payload"`
}

type PublishEventResponse struct {
	EventId string `json:"event_id"`
}
