package audit

import (
	"fmt"
	"strings"
	"time"

	"webmcp-backend/internal/domain/core"
)

type Event struct {
	ID          core.ID
	ActorType   core.ActorType
	ActorID     core.ID
	Action      string
	Aggregate   string
	AggregateID core.ID
	RequestID   string
	OperationID core.ID
	OccurredAt  time.Time
	Metadata    map[string]string
}

func (e Event) Validate() error {
	for label, id := range map[string]core.ID{"audit_event_id": e.ID, "actor_id": e.ActorID, "aggregate_id": e.AggregateID} {
		if err := id.Validate(label); err != nil {
			return err
		}
	}
	if err := e.ActorType.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.Action) == "" || strings.TrimSpace(e.Aggregate) == "" || strings.TrimSpace(e.RequestID) == "" || e.OccurredAt.IsZero() {
		return fmt.Errorf("audit action, aggregate, request id and timestamp are required")
	}
	return nil
}

// Event has no transition or update method by design. Persistence must append
// a new event instead of mutating historical audit records.
