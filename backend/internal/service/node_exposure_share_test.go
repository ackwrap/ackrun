package service

import (
	"errors"
	"net/url"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/parser"
)

func TestNodeExposureShareRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, listen, requestHost, wantHost string
		authenticated                       bool
	}{
		{"local", "127.0.0.1", "panel.example:8081", "127.0.0.1", false},
		{"ipv6-auth", "::1", "panel.example:8081", "::1", true},
		{"wildcard-ipv4", "0.0.0.0", "panel.example:8081", "panel.example", true},
		{"wildcard-ipv6", "::", "[2001:db8::10]:8081", "2001:db8::10", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, subscriptionID, nodeUID := createNodeExposureTestData(t)
			defer db.Close()
			svc := NewNodeExposureService(db)
			req := model.NodeExposureRequest{
				Name: "测试入口 #1", SubscriptionID: subscriptionID, NodeUID: nodeUID,
				InboundType: "mixed", Listen: tc.listen, ListenPort: 18080, Enabled: true,
			}
			if tc.authenticated {
				req.Username, req.Password = "user@name", "synthetic:@/?#%密码"
			}
			item, err := svc.Create(req)
			if err != nil {
				t.Fatal(err)
			}
			shared, err := svc.Share(item.ID, tc.requestHost)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(shared.URI)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme != "socks5" || parsed.Hostname() != tc.wantHost || parsed.Port() != "18080" || parsed.Fragment != req.Name {
				t.Fatal("share link did not preserve the binding address, port, or name")
			}
			if tc.authenticated {
				password, exists := parsed.User.Password()
				if !exists || parsed.User.Username() != req.Username || password != req.Password {
					t.Fatal("share link did not preserve authentication")
				}
			} else if parsed.User != nil {
				t.Fatal("unauthenticated binding unexpectedly included credentials")
			}
			if _, err := parser.ParseProxyURI(shared.URI); err != nil {
				t.Fatalf("generated link cannot be imported: %v", err)
			}
		})
	}
}

func TestNodeExposureShareRejectsMissingBindingAndInvalidWildcardHost(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	svc := NewNodeExposureService(db)
	if _, err := svc.Share(9999, "panel.example:8081"); !errors.Is(err, ErrNodeExposureNotFound) {
		t.Fatalf("missing binding error = %v", err)
	}
	item, err := svc.Create(model.NodeExposureRequest{
		Name: "wildcard", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "mixed", Listen: "0.0.0.0", ListenPort: 18080,
		Username: "synthetic", Password: "synthetic-only", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"", "user@panel.example", "panel.example/path"} {
		if _, err := svc.Share(item.ID, host); !errors.Is(err, ErrNodeExposureInvalid) {
			t.Fatalf("invalid wildcard host error = %v", err)
		}
	}
}
