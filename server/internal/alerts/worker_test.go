package alerts

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type fakeWorkerObserver struct {
	worker string
	outcome string
	items int64
	err error
}
func (f *fakeWorkerObserver) StartWorker(ctx context.Context, worker string) (context.Context, func(string, int64, error)) {
	return ctx, func(outcome string, items int64, err error) {
		f.worker, f.outcome, f.items, f.err = worker, outcome, items, err
	}
}

type fakeEvaluationRepository struct {
	acquired bool
	lockErr error
	listErr error
	rules []store.AlertRule
	applied []string
}

func (f *fakeEvaluationRepository) TryAcquireAlertEvaluationLock(context.Context) (func(), bool, error) {
	return func(){}, f.acquired, f.lockErr
}
func (f *fakeEvaluationRepository) ListEnabledWorkspaceAlertRules(context.Context, int) ([]store.AlertRule, error) {
	return f.rules, f.listErr
}
func (f *fakeEvaluationRepository) ApplyWorkspaceAlertSample(_ context.Context, workspaceID, ruleID, serviceName string, value float64, at time.Time) (store.AlertInstance, bool, error) {
	f.applied = append(f.applied, ruleID+":"+serviceName)
	return store.AlertInstance{ID:"instance",WorkspaceID:workspaceID,AlertRuleID:ruleID,ServiceName:serviceName,Value:value,Status:"pending"}, true, nil
}
func (f *fakeEvaluationRepository) ListWorkspaceAlertInstances(context.Context, string, string, string, string, int) ([]store.AlertInstance, error) {
	return nil, nil
}

type fakeMetricsSource struct {
	calls []evaluationGroup
	samples map[evaluationGroup]map[string]float64
	err error
}
func (f *fakeMetricsSource) WorkspaceRequestRates(_ context.Context, workspaceID, service string, window int) (map[string]float64, error) {
	f.calls = append(f.calls, evaluationGroup{workspaceID:workspaceID,windowSeconds:window})
	if f.err != nil { return nil, f.err }
	return f.samples[evaluationGroup{workspaceID:workspaceID,windowSeconds:window}], nil
}

func TestWorkerGroupsPrometheusQueriesAndEvaluatesMatchingServices(t *testing.T) {
	repository := &fakeEvaluationRepository{
		acquired:true,
		rules:[]store.AlertRule{
			{ID:"rule-b",WorkspaceID:"workspace-a",ServiceName:"checkout",WindowSeconds:300},
			{ID:"rule-a",WorkspaceID:"workspace-a",ServiceName:"",WindowSeconds:300},
			{ID:"rule-c",WorkspaceID:"workspace-a",ServiceName:"api-gateway",WindowSeconds:60},
			{ID:"rule-d",WorkspaceID:"workspace-b",ServiceName:"",WindowSeconds:300},
		},
	}
	metrics := &fakeMetricsSource{samples:map[evaluationGroup]map[string]float64{
		{workspaceID:"workspace-a",windowSeconds:300}:{"checkout":2.5,"api-gateway":1.25},
		{workspaceID:"workspace-a",windowSeconds:60}:{"api-gateway":3.5},
		{workspaceID:"workspace-b",windowSeconds:300}:{"worker":0.5},
	}}
	observer := &fakeWorkerObserver{}
	worker := NewWorker(repository,metrics,slog.New(slog.NewTextHandler(discardWriter{},nil)),time.Second,observer)
	worker.now = func() time.Time { return time.Date(2026,11,2,0,0,0,0,time.UTC) }
	worker.evaluate(context.Background())
	if observer.worker != "alert_evaluator" || observer.outcome != "ok" || observer.items != 5 || observer.err != nil {
		t.Fatalf("worker observation = %+v", observer)
	}

	wantCalls:=[]evaluationGroup{
		{workspaceID:"workspace-a",windowSeconds:60},
		{workspaceID:"workspace-a",windowSeconds:300},
		{workspaceID:"workspace-b",windowSeconds:300},
	}
	if !reflect.DeepEqual(metrics.calls,wantCalls) { t.Fatalf("metric query groups = %#v, want %#v",metrics.calls,wantCalls) }
	wantApplied:=[]string{
		"rule-c:api-gateway",
		"rule-b:checkout",
		"rule-a:api-gateway",
		"rule-a:checkout",
		"rule-d:worker",
	}
	if !reflect.DeepEqual(repository.applied,wantApplied) { t.Fatalf("applied samples = %#v, want %#v",repository.applied,wantApplied) }
}

func TestWorkerDoesNothingWhenAnotherReplicaOwnsLock(t *testing.T) {
	repository:=&fakeEvaluationRepository{acquired:false,rules:[]store.AlertRule{{ID:"r",WorkspaceID:"w",WindowSeconds:60}}}
	metrics:=&fakeMetricsSource{samples:map[evaluationGroup]map[string]float64{}}
	NewWorker(repository,metrics,nil,time.Second).evaluate(context.Background())
	if len(repository.applied)!=0 || len(metrics.calls)!=0 { t.Fatalf("work ran while another replica held the lock") }
}

func TestWorkerLeavesAlertStateAloneWhenPrometheusIsUnavailable(t *testing.T) {
	repository:=&fakeEvaluationRepository{acquired:true,rules:[]store.AlertRule{{ID:"r",WorkspaceID:"w",WindowSeconds:60}}}
	metrics:=&fakeMetricsSource{err:errors.New("Prometheus unavailable")}
	NewWorker(repository,metrics,nil,time.Second).evaluate(context.Background())
	if len(repository.applied)!=0 { t.Fatalf("alert state changed without telemetry samples") }
}

type discardWriter struct{}
func (discardWriter) Write(p []byte)(int,error){ return len(p),nil }
