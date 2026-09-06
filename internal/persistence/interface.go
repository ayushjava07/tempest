package persistence

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/hex"
	"time"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type DefinitionFilter struct {
	Namespace ttypes.Namespace
	Name      string
	Limit     int
	Offset    int
}

type RunFilter struct {
	Namespace   ttypes.Namespace
	Workflow    string
	State       *ttypes.RunState
	CreatedAfter *time.Time
	Limit       int
	Offset      int
}

type QueueLease struct {
	Item  ttypes.QueueItem
	Token string
}

func NewLeaseToken() string {
	b := make([]byte, 16)
	_, _ = crypto_rand.Read(b)
	return "lease-" + hex.EncodeToString(b)
}

type EventFilter struct {
	Namespace ttypes.Namespace
	RunID     string
	Type      ttypes.EventType
	Limit     int
}

type DeliveryFilter struct {
	Namespace  ttypes.Namespace
	EndpointID string
	Status     ttypes.DeliveryStatus
	Limit      int
	Offset     int
}

type CredentialsFilter struct {
	Namespace ttypes.Namespace
	Limit     int
}

type Store interface {
	CreateDefinition(ctx context.Context, d *ttypes.WorkflowDefinition) error
	GetDefinition(ctx context.Context, id ttypes.WorkflowID) (*ttypes.WorkflowDefinition, error)
	GetDefinitionByName(ctx context.Context, namespace ttypes.Namespace, name string) (*ttypes.WorkflowDefinition, error)
	ListDefinitions(ctx context.Context, f DefinitionFilter) ([]ttypes.WorkflowDefinition, error)
	DeleteDefinition(ctx context.Context, id ttypes.WorkflowID) error

	CreateRun(ctx context.Context, r *ttypes.Run) error
	GetRun(ctx context.Context, namespace ttypes.Namespace, id string) (*ttypes.Run, error)
	ListRuns(ctx context.Context, f RunFilter) ([]ttypes.Run, error)
	UpdateRunState(ctx context.Context, namespace ttypes.Namespace, id string, to ttypes.RunState, at time.Time) error
	UpdateRunError(ctx context.Context, namespace ttypes.Namespace, id string, errMsg string) error
	UpdateStepState(ctx context.Context, namespace ttypes.Namespace, id string, step ttypes.StepRun) error

	Enqueue(ctx context.Context, item ttypes.QueueItem) error
	Dequeue(ctx context.Context, namespace ttypes.Namespace, limit int, now time.Time) ([]QueueLease, error)
	DequeueAny(ctx context.Context, limit int, now time.Time) ([]QueueLease, error)
	CompleteStep(ctx context.Context, namespace ttypes.Namespace, runID, stepID, leaseToken string) error
	ReleaseStep(ctx context.Context, namespace ttypes.Namespace, runID, stepID string) error
	RequeueWithDelay(ctx context.Context, namespace ttypes.Namespace, runID, stepID string, visibleAt time.Time, attempt int) error
	LeaseExpired(ctx context.Context, leaseTimeout time.Duration, now time.Time) ([]ttypes.QueueItem, error)
	ListQueue(ctx context.Context, namespace ttypes.Namespace, limit int) ([]ttypes.QueueItem, error)

	AppendEvent(ctx context.Context, e *ttypes.Event) error
	GetEvent(ctx context.Context, id string) (*ttypes.Event, error)
	ListEvents(ctx context.Context, f EventFilter) ([]ttypes.Event, error)
	CreateWebhookEndpoint(ctx context.Context, e *ttypes.WebhookEndpoint) error
	GetWebhookEndpoint(ctx context.Context, namespace ttypes.Namespace, id string) (*ttypes.WebhookEndpoint, error)
	ListWebhookEndpoints(ctx context.Context, namespace ttypes.Namespace) ([]ttypes.WebhookEndpoint, error)
	UpdateWebhookEndpoint(ctx context.Context, e *ttypes.WebhookEndpoint) error
	DeleteWebhookEndpoint(ctx context.Context, namespace ttypes.Namespace, id string) error

	CreateDelivery(ctx context.Context, d *ttypes.Delivery) error
	UpdateDelivery(ctx context.Context, d *ttypes.Delivery) error
	ListDeliveries(ctx context.Context, f DeliveryFilter) ([]ttypes.Delivery, error)

	CreateToken(ctx context.Context, t *ttypes.APIToken) error
	GetToken(ctx context.Context, namespace ttypes.Namespace, id string) (*ttypes.APIToken, error)
	LookupTokenByHash(ctx context.Context, hash string) (*ttypes.APIToken, error)
	ListTokens(ctx context.Context, f CredentialsFilter) ([]ttypes.APIToken, error)
	DeleteToken(ctx context.Context, namespace ttypes.Namespace, id string) error

	ListExpiredRuns(ctx context.Context, cutoff time.Time, limit int) ([]ttypes.Run, error)
	DeleteRuns(ctx context.Context, ids []string) (int, error)
	CompactRuns(ctx context.Context, before time.Time, limit int) (int, error)

	Close() error
}
