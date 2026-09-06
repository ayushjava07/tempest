package grpcapi

import (
	"context"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/tempest-io/tempest/internal/auth"
	"github.com/tempest-io/tempest/internal/persistence"
	"github.com/tempest-io/tempest/internal/scheduler"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type Server struct {
	store    persistence.Store
	engine   *scheduler.Engine
	resolver *auth.Resolver
	grpc     *grpc.Server
}

type GRPCServerOptions struct {
	Store    persistence.Store
	Engine   *scheduler.Engine
	Resolver *auth.Resolver
	Addr     string
}

func NewGRPCServer(opts GRPCServerOptions) (*Server, error) {
	s := &Server{
		store:    opts.Store,
		engine:   opts.Engine,
		resolver: opts.Resolver,
	}
	s.grpc = grpc.NewServer(
		grpc.UnaryInterceptor(UnaryInterceptor(opts.Resolver)),
	)
	healthpb.RegisterHealthServer(s.grpc, health.NewServer())
	reflection.Register(s.grpc)
	return s, nil
}

func (s *Server) Serve(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.grpc.Serve(lis)
}

func (s *Server) GracefulStop() {
	s.grpc.GracefulStop()
}

func (s *Server) Submit(ctx context.Context, req *SubmitRequest) (*SubmitResponse, error) {
	if req.Workflow == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow is required")
	}
	input := ttypes.RunInput{
		Workflow: ttypes.WorkflowID{Name: req.Workflow, Version: int(req.Version)},
		Payload:  req.Input,
	}
	runID, err := s.engine.Submit(ctx, input)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &SubmitResponse{RunId: runID}, nil
}

func (s *Server) GetRun(ctx context.Context, req *GetRunRequest) (*RunResponse, error) {
	run, err := s.store.GetRun(ctx, ttypes.Namespace(req.Namespace), req.RunId)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toRunResponse(run), nil
}

func (s *Server) ListRuns(ctx context.Context, req *ListRunsRequest) (*ListRunsResponse, error) {
	ns := ttypes.Namespace(req.Namespace)
	runs, err := s.store.ListRuns(ctx, persistence.RunFilter{Namespace: ns, Limit: int(req.Limit)})
	if err != nil {
		return nil, toGRPCError(err)
	}
	resp := &ListRunsResponse{Total: int32(len(runs))}
	resp.Runs = make([]*RunResponse, len(runs))
	for i := range runs {
		resp.Runs[i] = toRunResponse(&runs[i])
	}
	return resp, nil
}

func (s *Server) CancelRun(ctx context.Context, req *CancelRunRequest) (*CancelRunResponse, error) {
	if err := s.engine.Cancel(ctx, ttypes.Namespace(req.Namespace), req.RunId); err != nil {
		return nil, toGRPCError(err)
	}
	return &CancelRunResponse{Success: true}, nil
}

func (s *Server) CreateDefinition(ctx context.Context, req *CreateDefinitionRequest) (*DefinitionResponse, error) {
	steps := make([]ttypes.StepDefinition, len(req.Steps))
	for i, st := range req.Steps {
		steps[i] = ttypes.StepDefinition{
			ID: st.Id, Handler: st.Handler, DependsOn: st.Depends,
			Timeout: time.Duration(st.TimeoutNs),
		}
	}
	d := &ttypes.WorkflowDefinition{
		ID:    ttypes.WorkflowID{Name: req.Name, Version: int(req.Version)},
		Steps: steps,
	}
	if err := s.store.CreateDefinition(ctx, d); err != nil {
		return nil, toGRPCError(err)
	}
	return &DefinitionResponse{Name: d.ID.Name, Version: int32(d.ID.Version)}, nil
}

func (s *Server) PublishEvent(ctx context.Context, req *PublishEventRequest) (*PublishEventResponse, error) {
	ev := ttypes.Event{
		ID: ttypes.NewID(), Type: ttypes.EventType(req.Type), Version: 1,
		Namespace: ttypes.Namespace(req.Namespace), RunID: req.RunId, StepID: req.StepId,
		CreatedAt: time.Now(), Payload: req.Payload,
	}
	if err := s.store.AppendEvent(ctx, &ev); err != nil {
		return nil, toGRPCError(err)
	}
	return &PublishEventResponse{EventId: ev.ID}, nil
}

func toGRPCError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "not_found") || strings.Contains(msg, "not found") {
		return status.Error(codes.NotFound, msg)
	}
	if strings.Contains(msg, "already_exists") || strings.Contains(msg, "already exists") {
		return status.Error(codes.AlreadyExists, msg)
	}
	if strings.Contains(msg, "conflict") {
		return status.Error(codes.AlreadyExists, msg)
	}
	if strings.Contains(msg, "unauthenticated") {
		return status.Error(codes.Unauthenticated, msg)
	}
	if strings.Contains(msg, "permission") {
		return status.Error(codes.PermissionDenied, msg)
	}
	return status.Error(codes.Internal, msg)
}

func toRunResponse(r *ttypes.Run) *RunResponse {
	return &RunResponse{
		Id: r.ID, Namespace: string(r.Namespace),
		Workflow: r.Workflow.Name, Version: int32(r.Workflow.Version),
		State: string(r.State), CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}
