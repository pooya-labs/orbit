package auth

import (
	"strings"
	"testing"
)

func TestPrincipalRolesAreHierarchicalAndActorIDIsDatabaseSafe(t *testing.T) {
	operator := Principal{Issuer: "https://issuer.example", Subject: "subject|with delimiters", Roles: []string{RoleOperator}}
	if !operator.HasRole(RoleViewer) || !operator.HasRole(RoleOperator) || operator.HasRole(RoleAdministrator) {
		t.Fatalf("unexpected operator role hierarchy: %+v", operator.Roles)
	}
	if strings.ContainsRune(operator.ActorID(), '\x00') {
		t.Fatal("actor ID must be safe for PostgreSQL text columns")
	}
	administrator := Principal{Roles: []string{RoleAdministrator}}
	if !administrator.HasRole(RoleOperator) || !administrator.HasRole(RoleViewer) {
		t.Fatal("administrator must inherit operator and viewer access")
	}
}
