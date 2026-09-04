package ports

import (
	"context"

	"webmcp-backend/internal/domain/approval"
	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/commit"
	"webmcp-backend/internal/domain/core"
	"webmcp-backend/internal/domain/planning"
	"webmcp-backend/internal/domain/proposal"
	"webmcp-backend/internal/domain/scenario"
)

type BusinessSnapshotReader interface {
	GetSnapshot(context.Context, core.ID, core.ID) (business.Snapshot, error)
}

type GoalReader interface {
	GetGoal(context.Context, core.ID, core.ID, core.ID) (planning.Goal, error)
}

type ScenarioRepository interface {
	CreateScenario(context.Context, core.ID, core.ID, scenario.Scenario, string) error
	GetScenario(context.Context, core.ID, core.ID, core.ID) (scenario.Scenario, error)
	ListScenarios(context.Context, core.ID, core.ID, []core.ID) ([]scenario.Scenario, error)
	UpdateSimulation(context.Context, core.ID, scenario.Scenario, uint64, core.ID, string) error
}

type ProposalRepository interface {
	CreateProposal(context.Context, core.ID, core.ID, proposal.Proposal, string) error
	GetProposal(context.Context, core.ID, core.ID, core.ID) (proposal.Proposal, error)
	SaveRevision(context.Context, core.ID, core.ID, proposal.Proposal, uint64, string) error
	SubmitReview(context.Context, core.ID, core.ID, core.ID, uint64, string) error
}

type ApprovalRepository interface {
	RecordDecision(context.Context, core.ID, approval.Decision, string) error
}

type CommitInput struct {
	OrganizationID    core.ID
	OperationID       core.ID
	IdempotencyKey    string
	BusinessID        core.ID
	ProposalID        core.ID
	ProposalVersionID core.ID
	ActorID           core.ID
	RequestID         string
	Authorized        bool
}

type CommitOutcome struct {
	OperationID              core.ID
	DecisionID               core.ID
	Status                   commit.Status
	CommittedBusinessVersion uint64
	Replayed                 bool
}

type Committer interface {
	Commit(context.Context, CommitInput) (CommitOutcome, error)
}
