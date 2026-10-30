package service

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestGenerateNodeOutboundNormalizesHysteriaPortRanges(t *testing.T) {
	tests := []struct {
		name  string
		ports string
		want  []string
	}{
		{name: "legacy string", ports: `"20000-30000"`, want: []string{"20000:30000"}},
		{name: "legacy list", ports: `["20000-30000","40000-50000"]`, want: []string{"20000:30000", "40000:50000"}},
		{name: "mixed list", ports: `["20000-30000","40000:50000","443"]`, want: []string{"20000:30000", "40000:50000", "443:443"}},
		{name: "comma separated", ports: `"20000-30000,443"`, want: []string{"20000:30000", "443:443"}},
		{name: "native list", ports: `["20000:30000",":1024","60000:","0:1",":"]`, want: []string{"20000:30000", ":1024", "60000:", "0:1", ":"}},
		{name: "empty list", ports: `[]`, want: []string{}},
	}
	for _, protocol := range []string{"hysteria", "hysteria2"} {
		for _, test := range tests {
			t.Run(protocol+"/"+test.name, func(t *testing.T) {
				node := &model.Node{
					Type:    protocol,
					RawJSON: fmt.Sprintf(`{"type":%q,"server":"hy.example.com","server_port":443,"server_ports":%s,"hop_interval":"30s"}`, protocol, test.ports),
				}
				original := node.RawJSON
				outbound, err := (&ConfigGeneratorService{}).generateNodeOutbound(node, "hy-node", nil)
				if err != nil {
					t.Fatalf("generate Hysteria outbound: %v", err)
				}
				if !reflect.DeepEqual(outbound["server_ports"], test.want) {
					t.Fatalf("server_ports = %v, want %v", outbound["server_ports"], test.want)
				}
				if node.RawJSON != original || outbound["tag"] != "hy-node" || outbound["hop_interval"] != "30s" {
					t.Fatal("port normalization changed the saved node or unrelated options")
				}
			})
		}
	}
}

func TestGenerateNodeOutboundRejectsInvalidHysteriaPortRanges(t *testing.T) {
	for _, protocol := range []string{"hysteria", "hysteria2"} {
		for _, ports := range []string{`"30000-20000"`, `["20000-65536"]`, `["20000-30000","invalid"]`, `"20000-30000,"`, `123`, `[true]`} {
			t.Run(protocol+"/"+ports, func(t *testing.T) {
				_, err := (&ConfigGeneratorService{}).generateNodeOutbound(&model.Node{
					Type:    protocol,
					RawJSON: fmt.Sprintf(`{"type":%q,"server":"hy.example.com","server_port":443,"server_ports":%s}`, protocol, ports),
				}, "invalid-hy-node", nil)
				if err == nil || !strings.Contains(err.Error(), "server_ports") {
					t.Fatalf("expected invalid server_ports error, got %v", err)
				}
			})
		}
	}
}

func TestGenerateOutboundsRejectsInvalidHysteriaPortRanges(t *testing.T) {
	for _, protocol := range []string{"hysteria", "hysteria2"} {
		t.Run(protocol, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "invalid-hysteria", URL: "https://example.com/subscription"})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{{
				Name: "Broken HY", Type: protocol, Server: "hy.example.com", ServerPort: 443,
				RawJSON: fmt.Sprintf(`{"type":%q,"server":"hy.example.com","server_port":443,"server_ports":["30000-20000"]}`, protocol),
			}}); err != nil {
				t.Fatal(err)
			}
			_, _, err = NewConfigGeneratorService(db, nil).generateOutbounds()
			if err == nil || !strings.Contains(err.Error(), "Broken HY") || !strings.Contains(err.Error(), "server_ports") {
				t.Fatalf("expected an actionable generation error instead of silently skipping the node, got %v", err)
			}
		})
	}
}
