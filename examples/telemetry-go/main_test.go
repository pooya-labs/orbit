package main

import "testing"

func TestSLOOutcomeForHTTPStatus(t *testing.T) {
	for _, test := range []struct {
		status int
		want string
	}{
		{status: 199, want: "bad"},
		{status: 200, want: "good"},
		{status: 202, want: "good"},
		{status: 302, want: "good"},
		{status: 399, want: "good"},
		{status: 400, want: "bad"},
		{status: 404, want: "bad"},
		{status: 405, want: "bad"},
		{status: 500, want: "bad"},
	} {
		if got := sloOutcomeForHTTPStatus(test.status); got != test.want {
			t.Errorf("sloOutcomeForHTTPStatus(%d) = %q, want %q", test.status, got, test.want)
		}
	}
}
