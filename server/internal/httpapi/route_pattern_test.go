package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"
)

func TestHandlerRoutePatternUsesTemplatesAndBoundsUnknownPaths(t *testing.T) {
	handler := NewHandler(Config{Environment: "development", Telemetry: &telemetry.Client{}}, nil, testLogger())

	tests := []struct {
		name string
		method string
		path string
		want string
	}{
		{name: "static metadata route", method: "GET", path: "/api/v1/meta", want: "GET /api/v1/meta"},
		{name: "workspace membership route", method: "GET", path: "/api/v1/workspaces/00000000-0000-4000-8000-000000000001/membership", want: "GET /api/v1/workspaces/{workspaceID}/membership"},
		{name: "unknown path is one bounded value", method: "GET", path: "/unbounded/user/supplied/path/123", want: "unmatched"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if got := handler.RoutePattern(request); got != test.want {
				t.Fatalf("RoutePattern() = %q, want %q", got, test.want)
			}
		})
	}
}
