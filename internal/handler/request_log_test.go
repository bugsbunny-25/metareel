package handler

import "testing"

func TestRedactURI(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/api/v1/me/venues/nearby?lat=19.1&lon=72.9&limit=5", "/api/v1/me/venues/nearby?lat=REDACTED&limit=5&lon=REDACTED"},
		{"/api/v1/me/venues?q=pvr", "/api/v1/me/venues?q=pvr"},
		{"/api/v1/top10", "/api/v1/top10"},
		{"/x?lat=1;2", "/x?REDACTED"},
	} {
		if got := redactURI(tc.in); got != tc.want {
			t.Errorf("redactURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
