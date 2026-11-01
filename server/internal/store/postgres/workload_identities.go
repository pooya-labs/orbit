package postgres

import (
 "context"
 "errors"
 "fmt"
 "regexp"
 "strings"
 "time"

 "github.com/jackc/pgx/v5"
 "github.com/pooya16pg-spec/orbit/server/internal/store"
)

var workloadSignalPattern = regexp.MustCompile(\`^(metrics|logs|traces)$\`)

func validateWorkloadPrincipal(issuer, subject string) error {
 if strings.TrimSpace(issuer)=="" || issuer!=strings.TrimSpace(issuer) || len(issuer)>512 { return fmt.Errorf("invalid workload issuer") }
 if strings.TrimSpace(subject)=="" || subject!=strings.TrimSpace(subject) || len(subject)>512 { return fmt.Errorf("invalid workload subject") }
 return nil
}

func (r *Repository) CreateWorkloadIdentity(ctx context.Context, in store.CreateWorkloadIdentityInput) (store.WorkloadIdentity,error) {
 if err:=validateWorkloadPrincipal(in.Issuer,in.Subject); err!=nil{return store.WorkloadIdentity{},err}
 if err:=validateWorkloadPrincipal(in.ActorIssuer,in.ActorSubject); err!=nil{return store.WorkloadIdentity{},err}
 in.DisplayName=strings.TrimSpace(in.DisplayName)
 if len(in.DisplayName)<1 || len(in.DisplayName)>120{return store.WorkloadIdentity{},fmt.Errorf("invalid workload display name")}
 if in.ExpiresAt!=nil && !in.ExpiresAt.After(time.Now().UTC()){return store.WorkloadIdentity{},fmt.Errorf("workload expiry must be in the future")}
 tx,err:=r.pool.Begin(ctx); if err!=nil{return store.WorkloadIdentity{},fmt.Errorf("begin workload creation: %w",err)}
 defer func(){_=tx.Rollback(ctx)}()
 var out store.WorkloadIdentity
 err=tx.QueryRow(ctx,\`
 INSERT INTO workload_identities(issuer,subject,display_name,expires_at,created_by_issuer,created_by_subject)
 VALUES($1,$2,$3,$4,$5,$6)
 RETURNING id::text,issuer,subject,display_name,status,expires_at,created_at,updated_at,revoked_at
 \`,in.Issuer,in.Subject,in.DisplayName,in.ExpiresAt,in.ActorIssuer,in.ActorSubject).Scan(&out.ID,&out.Issuer,&out.Subject,&out.DisplayName,&out.Status,&out.ExpiresAt,&out.CreatedAt,&out.UpdatedAt,&out.RevokedAt)
 if err!=nil{return store.WorkloadIdentity{},fmt.Errorf("insert workload identity: %w",err)}
 if _,err=tx.Exec(ctx,\`
 INSERT INTO workload_identity_audit_events(workload_identity_id,actor_issuer,actor_subject,action,details)
 VALUES($1::uuid,$2,$3,'workload.created',jsonb_build_object('display_name',$4))
 \`,out.ID,in.ActorIssuer,in.ActorSubject,in.DisplayName);err!=nil{return store.WorkloadIdentity{},fmt.Errorf("audit workload creation: %w",err)}
 if err=tx.Commit(ctx);err!=nil{return store.WorkloadIdentity{},fmt.Errorf("commit workload creation: %w",err)}
 return out,nil
}

func (r *Repository) SetWorkloadWorkspaceGrant(ctx context.Context,in store.WorkloadGrantInput,granted bool) error {
 if !workspaceIDPattern.MatchString(in.WorkspaceID)||!workspaceIDPattern.MatchString(in.IdentityID){return fmt.Errorf("workload and workspace IDs must be UUIDs")}
 if !workloadSignalPattern.MatchString(in.Signal){return fmt.Errorf("invalid telemetry signal")}
 if err:=validateWorkloadPrincipal(in.ActorIssuer,in.ActorSubject);err!=nil{return err}
 tx,err:=r.pool.Begin(ctx);if err!=nil{return fmt.Errorf("begin workload grant change: %w",err)}
 defer func(){_=tx.Rollback(ctx)}()
 var status string
 err=tx.QueryRow(ctx,\`SELECT status FROM workload_identities WHERE id=$1::uuid FOR UPDATE\`,in.IdentityID).Scan(&status)
 if errors.Is(err,pgx.ErrNoRows){return store.ErrWorkloadIdentityNotFound};if err!=nil{return fmt.Errorf("read workload identity: %w",err)}
 if granted && status!="active"{return store.ErrWorkloadAuthorizationDenied}
 action:="workload.grant.revoked"
 if granted {
  action="workload.grant.created"
  _,err=tx.Exec(ctx,\`
  INSERT INTO workload_workspace_grants(workload_identity_id,workspace_id,signal,granted_by_issuer,granted_by_subject,granted_at,revoked_at)
  VALUES($1::uuid,$2::uuid,$3,$4,$5,NOW(),NULL)
  ON CONFLICT(workload_identity_id,workspace_id,signal) DO UPDATE SET granted_by_issuer=EXCLUDED.granted_by_issuer,granted_by_subject=EXCLUDED.granted_by_subject,granted_at=NOW(),revoked_at=NULL
  \`,in.IdentityID,in.WorkspaceID,in.Signal,in.ActorIssuer,in.ActorSubject)
 } else {
  _,err=tx.Exec(ctx,\`UPDATE workload_workspace_grants SET revoked_at=COALESCE(revoked_at,NOW()) WHERE workload_identity_id=$1::uuid AND workspace_id=$2::uuid AND signal=$3\`,in.IdentityID,in.WorkspaceID,in.Signal)
 }
 if err!=nil{return fmt.Errorf("write workload grant: %w",err)}
 if _,err=tx.Exec(ctx,\`
 INSERT INTO workload_identity_audit_events(workload_identity_id,workspace_id,actor_issuer,actor_subject,action,details)
 VALUES($1::uuid,$2::uuid,$3,$4,$5,jsonb_build_object('signal',$6))
 \`,in.IdentityID,in.WorkspaceID,in.ActorIssuer,in.ActorSubject,action,in.Signal);err!=nil{return fmt.Errorf("audit workload grant: %w",err)}
 if err=tx.Commit(ctx);err!=nil{return fmt.Errorf("commit workload grant: %w",err)}
 return nil
}

func (r *Repository) AuthorizeWorkloadTelemetry(ctx context.Context,issuer,subject,workspaceID,signal string)(store.WorkloadIdentity,error){
 if err:=validateWorkloadPrincipal(issuer,subject);err!=nil{return store.WorkloadIdentity{},err}
 if !workspaceIDPattern.MatchString(workspaceID)||!workloadSignalPattern.MatchString(signal){return store.WorkloadIdentity{},store.ErrWorkloadAuthorizationDenied}
 var out store.WorkloadIdentity
 err:=r.pool.QueryRow(ctx,\`
 SELECT wi.id::text,wi.issuer,wi.subject,wi.display_name,wi.status,wi.expires_at,wi.created_at,wi.updated_at,wi.revoked_at
 FROM workload_identities wi JOIN workload_workspace_grants wg ON wg.workload_identity_id=wi.id
 WHERE wi.issuer=$1 AND wi.subject=$2 AND wi.status='active'
 AND (wi.expires_at IS NULL OR wi.expires_at>NOW())
 AND wg.workspace_id=$3::uuid AND wg.signal=$4 AND wg.revoked_at IS NULL
 \`,issuer,subject,workspaceID,signal).Scan(&out.ID,&out.Issuer,&out.Subject,&out.DisplayName,&out.Status,&out.ExpiresAt,&out.CreatedAt,&out.UpdatedAt,&out.RevokedAt)
 if errors.Is(err,pgx.ErrNoRows){return store.WorkloadIdentity{},store.ErrWorkloadAuthorizationDenied}
 if err!=nil{return store.WorkloadIdentity{},fmt.Errorf("authorize workload telemetry: %w",err)}
 return out,nil
}

func (r *Repository) SetWorkloadIdentityStatus(ctx context.Context,id,actorIssuer,actorSubject,status string)error{
 if !workspaceIDPattern.MatchString(id){return fmt.Errorf("workload identity ID must be a UUID")}
 if status!="active"&&status!="disabled"&&status!="revoked"{return fmt.Errorf("invalid workload status")}
 if err:=validateWorkloadPrincipal(actorIssuer,actorSubject);err!=nil{return err}
 tx,err:=r.pool.Begin(ctx);if err!=nil{return fmt.Errorf("begin workload status change: %w",err)}
 defer func(){_=tx.Rollback(ctx)}()
 var found string
 err=tx.QueryRow(ctx,\`
 UPDATE workload_identities SET status=$2,revoked_at=CASE WHEN $2='revoked' THEN COALESCE(revoked_at,NOW()) ELSE NULL END,updated_at=NOW()
 WHERE id=$1::uuid RETURNING id::text
 \`,id,status).Scan(&found)
 if errors.Is(err,pgx.ErrNoRows){return store.ErrWorkloadIdentityNotFound};if err!=nil{return fmt.Errorf("update workload status: %w",err)}
 action:="workload.disabled";if status=="revoked"{action="workload.revoked"} else if status=="active"{action="workload.enabled"}
 if _,err=tx.Exec(ctx,\`
 INSERT INTO workload_identity_audit_events(workload_identity_id,actor_issuer,actor_subject,action,details)
 VALUES($1::uuid,$2,$3,$4,jsonb_build_object('status',$5))
 \`,id,actorIssuer,actorSubject,action,status);err!=nil{return fmt.Errorf("audit workload status: %w",err)}
 if err=tx.Commit(ctx);err!=nil{return fmt.Errorf("commit workload status: %w",err)}
 return nil
}

var _ store.WorkloadIdentityRepository = (*Repository)(nil)
