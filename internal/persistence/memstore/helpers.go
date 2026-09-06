package memstore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	ttypes "github.com/tempest-io/tempest/pkg/types"
)

func cloneDefinition(d *ttypes.WorkflowDefinition) *ttypes.WorkflowDefinition {
	if d == nil {
		return nil
	}
	cp := *d
	cp.Steps = append([]ttypes.StepDefinition(nil), d.Steps...)
	if d.Tags != nil {
		cp.Tags = make(map[string]string, len(d.Tags))
		for k, v := range d.Tags {
			cp.Tags[k] = v
		}
	}
	return &cp
}

func cloneRun(r *ttypes.Run) *ttypes.Run {
	if r == nil {
		return nil
	}
	cp := *r
	cp.Steps = make([]ttypes.StepRun, len(r.Steps))
	for i, s := range r.Steps {
		cp.Steps[i] = s
		if s.Output != nil {
			cp.Steps[i].Output = make(map[string]any, len(s.Output))
			for k, v := range s.Output {
				cp.Steps[i].Output[k] = v
			}
		}
		if s.Metadata != nil {
			cp.Steps[i].Metadata = make(map[string]string, len(s.Metadata))
			for k, v := range s.Metadata {
				cp.Steps[i].Metadata[k] = v
			}
		}
	}
	if r.Input.Payload != nil {
		cp.Input.Payload = make(map[string]any, len(r.Input.Payload))
		for k, v := range r.Input.Payload {
			cp.Input.Payload[k] = v
		}
	}
	if r.Labels != nil {
		cp.Labels = make(map[string]string, len(r.Labels))
		for k, v := range r.Labels {
			cp.Labels[k] = v
		}
	}
	return &cp
}

func cloneRunForList(r *ttypes.Run) ttypes.Run {
	return *r
}

func cloneEvent(e *ttypes.Event) *ttypes.Event {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

func cloneEndpoint(e *ttypes.WebhookEndpoint) *ttypes.WebhookEndpoint {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Types = append([]ttypes.EventType(nil), e.Types...)
	return &cp
}

func cloneToken(t *ttypes.APIToken) *ttypes.APIToken {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

func cloneDelivery(d *ttypes.Delivery) *ttypes.Delivery {
	if d == nil {
		return nil
	}
	cp := *d
	return &cp
}

func newLeaseToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("lease-%d", time.Now().UnixNano())
	}
	return "lease-" + hex.EncodeToString(buf)
}

func sprint(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
