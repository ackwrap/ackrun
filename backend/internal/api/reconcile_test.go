package api

import (
	"net/http"
	"testing"
)

func TestShouldReconcileRequest(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPut, "/api/v1/settings/dns", true},
		{http.MethodPut, "/api/v1/settings/inbound-mode", false},
		{http.MethodPut, "/api/v1/settings/proxy-mode", false},
		{http.MethodPost, "/api/v1/settings/geoip-providers", false},
		{http.MethodDelete, "/api/v1/settings/connectivity-targets/1", false},
		{http.MethodPost, "/api/v1/settings/node-filters", false},
		{http.MethodDelete, "/api/v1/settings/node-filters/1", false},
		{http.MethodPut, "/api/v1/settings/core-restart", false},
		{http.MethodPost, "/api/v1/nodes/import", true},
		{http.MethodPost, "/api/v1/nodes/import/preview", false},
		{http.MethodPost, "/api/v1/nodes/flag", false},
		{http.MethodPost, "/api/v1/nodes/flags", false},
		{http.MethodPut, "/api/v1/collections/1", true},
		{http.MethodPost, "/api/v1/collections/reorder", true},
		{http.MethodPost, "/api/v1/collections/1/test", false},
		{http.MethodPost, "/api/v1/dns/servers/reorder", true},
		{http.MethodPost, "/api/v1/dns/outbound-bindings/reorder", false},
		{http.MethodDelete, "/api/v1/subscriptions/1", true},
		{http.MethodPost, "/api/v1/subscriptions/1/sync", false},
		{http.MethodPost, "/api/v1/nodes/tcping", false},
		{http.MethodPost, "/api/v1/nodes/uid/exit-ip", false},
		{http.MethodPost, "/api/v1/nodes/uid/traceroute", false},
		{http.MethodPost, "/api/v1/config/generate", false},
		{http.MethodPost, "/api/v1/core/restart", false},
		{http.MethodGet, "/api/v1/rules", false},
	}

	for _, test := range tests {
		if got := shouldReconcileRequest(test.method, test.path); got != test.want {
			t.Errorf("shouldReconcileRequest(%q, %q) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}
