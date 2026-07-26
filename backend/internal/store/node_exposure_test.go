package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestReplaceSubscriptionNodesDisablesMissingNodeExposure(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "test", URL: "https://subscription.invalid/test"})
	if err != nil {
		t.Fatal(err)
	}
	first := model.ParsedNode{Name: "first", Type: "socks", Server: "192.0.2.10", ServerPort: 1080, RawJSON: `{"type":"socks","server":"192.0.2.10","server_port":1080}`}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{first}); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes = %+v, err = %v", nodes, err)
	}
	exposure := &model.NodeExposure{
		Name:           "first exposure",
		SubscriptionID: subscription.ID,
		NodeUID:        nodes[0].UID,
		InboundType:    "socks",
		Listen:         "127.0.0.1",
		ListenPort:     18080,
		Enabled:        true,
	}
	if err := db.CreateNodeExposure(exposure); err != nil {
		t.Fatal(err)
	}

	second := model.ParsedNode{Name: "second", Type: "socks", Server: "192.0.2.11", ServerPort: 1080, RawJSON: `{"type":"socks","server":"192.0.2.11","server_port":1080}`}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{second}); err != nil {
		t.Fatal(err)
	}
	updated, err := db.GetNodeExposure(exposure.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.NodeExists {
		t.Fatalf("stale exposure was not disabled: %+v", updated)
	}
}

func TestSetNodeEnabledRollsBackWhenExposureDisableFails(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "test", URL: "https://subscription.invalid/test"})
	if err != nil {
		t.Fatal(err)
	}
	node := model.ParsedNode{Name: "node", Type: "socks", Server: "192.0.2.10", ServerPort: 1080, RawJSON: `{"type":"socks","server":"192.0.2.10","server_port":1080}`}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{node}); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes = %+v, err = %v", nodes, err)
	}
	exposure := &model.NodeExposure{Name: "exposure", SubscriptionID: subscription.ID, NodeUID: nodes[0].UID, InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18120, Enabled: true}
	if err := db.CreateNodeExposure(exposure); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		CREATE TRIGGER reject_exposure_disable
		BEFORE UPDATE OF enabled ON node_exposures
		BEGIN SELECT RAISE(ABORT, 'blocked'); END
	`); err != nil {
		t.Fatal(err)
	}

	if err := db.SetNodeEnabled(nodes[0].UID, false); err == nil {
		t.Fatal("expected node disable to fail")
	}
	remaining, err := db.ListEnabledNodes()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := db.GetNodeExposure(exposure.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || !updated.Enabled {
		t.Fatalf("node/exposure update was not atomic: nodes=%d exposure=%+v", len(remaining), updated)
	}
}

func TestUpdateNodeExposureCannotEnableMissingTarget(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "test", URL: "https://subscription.invalid/test"})
	if err != nil {
		t.Fatal(err)
	}
	exposure := &model.NodeExposure{
		Name: "missing", SubscriptionID: subscription.ID, NodeUID: "missing-node",
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18130, Enabled: false,
	}
	if err := db.CreateNodeExposure(exposure); err != nil {
		t.Fatal(err)
	}
	exposure.Enabled = true
	if err := db.UpdateNodeExposure(exposure.ID, exposure); !errors.Is(err, ErrNodeExposureTargetUnavailable) {
		t.Fatalf("update error = %v", err)
	}
	updated, err := db.GetNodeExposure(exposure.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled {
		t.Fatalf("missing target exposure was enabled: %+v", updated)
	}
}
