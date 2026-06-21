package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestSubscriptionStoreCRUDAndSyncResult(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	created, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name:            "test-sub",
		URL:             "https://example.com/sub",
		UserAgent:       "clash-meta/2.4.0",
		SyncMode:        "weekly",
		SyncTime:        "03:04:05",
		SyncWeekday:     3,
		SyncTimeoutSecs: 90,
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected created id")
	}
	if created.UserAgent != "clash-meta/2.4.0" || created.SyncMode != "weekly" || created.SyncTime != "03:04:05" || created.SyncWeekday != 3 || created.SyncTimeoutSecs != 90 {
		t.Fatalf("unexpected schedule fields: %+v", created)
	}
	if created.SyncStatus != "updated" || created.SyncProgress != 100 {
		t.Fatalf("unexpected default sync state: %+v", created)
	}

	updated, err := db.UpdateSubscription(created.ID, &model.SubscriptionRequest{
		Name:            "updated-sub",
		URL:             "https://example.com/updated",
		UserAgent:       "v2rayN/6.0",
		SyncMode:        "daily",
		SyncTime:        "01:02:03",
		SyncTimeoutSecs: 60,
	})
	if err != nil {
		t.Fatalf("update subscription: %v", err)
	}
	if updated.Name != "updated-sub" || updated.UserAgent != "v2rayN/6.0" || updated.SyncMode != "daily" || updated.SyncWeekday != 0 {
		t.Fatalf("unexpected updated subscription: %+v", updated)
	}

	if err := db.SetSubscriptionSyncState(created.ID, "syncing", 35); err != nil {
		t.Fatalf("set sync state: %v", err)
	}
	syncing, err := db.GetSubscription(created.ID)
	if err != nil {
		t.Fatalf("get syncing subscription: %v", err)
	}
	if syncing.SyncStatus != "syncing" || syncing.SyncProgress != 35 {
		t.Fatalf("unexpected syncing state: %+v", syncing)
	}

	result, err := db.UpdateSubscriptionSyncResult(created.ID, 2, 30, 100, 2000000000000)
	if err != nil {
		t.Fatalf("update sync result: %v", err)
	}
	if result.NodeCount != 2 || result.TrafficUsedBytes != 30 || result.TrafficTotalBytes != 100 || result.ExpireAt != 2000000000000 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if result.SyncStatus != "updated" || result.SyncProgress != 100 || result.LastSyncAt == 0 {
		t.Fatalf("unexpected completed sync state: %+v", result)
	}

	if err := db.ReplaceSubscriptionNodes(created.ID, []model.ParsedNode{
		{Name: "HK-01", Type: "trojan", Server: "hk.example.com", ServerPort: 443, Raw: "raw1", RawJSON: `{"name":"HK-01","type":"trojan","server":"hk.example.com","port":443}`},
		{Name: "JP-01", Type: "vmess", Server: "jp.example.com", ServerPort: 8443, Raw: "raw2", RawJSON: `{"name":"JP-01","type":"vmess","server":"jp.example.com","port":8443}`},
	}); err != nil {
		t.Fatalf("replace subscription nodes: %v", err)
	}
	nodes, err := db.ListNodesBySubscription(created.ID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Name != "HK-01" || nodes[1].ServerPort != 8443 {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
	if nodes[0].UID == "" || nodes[1].UID == "" || nodes[0].UID == nodes[1].UID {
		t.Fatalf("expected unique stable node uids: %+v", nodes)
	}
	firstUID := nodes[0].UID

	if err := db.ReplaceSubscriptionNodes(created.ID, []model.ParsedNode{
		{Name: "HK-Renamed", Type: "trojan", Server: "hk.example.com", ServerPort: 443, Raw: "raw-changed", RawJSON: `{"name":"HK-Renamed","type":"trojan","server":"hk.example.com","port":443}`},
	}); err != nil {
		t.Fatalf("replace subscription nodes second time: %v", err)
	}
	nodes, err = db.ListNodesBySubscription(created.ID)
	if err != nil {
		t.Fatalf("list nodes second time: %v", err)
	}
	if len(nodes) != 1 || nodes[0].UID != firstUID {
		t.Fatalf("expected uid to survive rename/raw changes, got %+v want %s", nodes, firstUID)
	}
	if err := db.SetNodeEnabled(firstUID, false); err != nil {
		t.Fatalf("set node enabled: %v", err)
	}
	if err := db.SetNodePreferred(firstUID, true); err != nil {
		t.Fatalf("set node preferred: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(created.ID, []model.ParsedNode{
		{Name: "HK-Again", Type: "trojan", Server: "hk.example.com", ServerPort: 443, Raw: "raw-again", RawJSON: `{"name":"HK-Again","type":"trojan","server":"hk.example.com","port":443}`},
	}); err != nil {
		t.Fatalf("replace subscription nodes third time: %v", err)
	}
	nodes, err = db.ListNodesBySubscription(created.ID)
	if err != nil {
		t.Fatalf("list nodes third time: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Enabled || !nodes[0].Preferred {
		t.Fatalf("expected node state to survive resync, got %+v", nodes)
	}

	items, err := db.ListSubscriptions()
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(items))
	}

	if err := db.DeleteSubscription(created.ID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	deleted, err := db.GetSubscription(created.ID)
	if err != nil {
		t.Fatalf("get deleted subscription: %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected deleted subscription to be nil, got %+v", deleted)
	}
}

func TestDeleteSubscriptionCascadesNodesGroupsAndStrategyRefs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub", URL: "https://example.com/sub", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, []model.ParsedNode{
		{Name: "TW-01", Type: "vless", Server: "tw.example.com", ServerPort: 443, Raw: "raw-tw", RawJSON: `{"type":"vless","server":"tw.example.com","server_port":443,"uuid":"uuid-tw"}`},
	}); err != nil {
		t.Fatalf("replace subscription nodes: %v", err)
	}
	nodes, err := db.ListNodesBySubscription(sub.ID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected one node, got %+v", nodes)
	}

	manualGroup, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "manual group", Type: "selector", NodeUIDs: []string{nodes[0].UID}, Enabled: true})
	if err != nil {
		t.Fatalf("create manual group: %v", err)
	}
	filterGroup, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "filter group", Type: "selector", FilterSubscriptions: fmt.Sprintf("%d", sub.ID), FilterInclude: ".*", Enabled: true})
	if err != nil {
		t.Fatalf("create filter group: %v", err)
	}
	refs, _ := json.Marshal([]int64{manualGroup.ID, filterGroup.ID})
	collection := &model.ProxyCollection{Name: "策略", Type: "selector", SourceType: "node_groups", ReferencedGroupIDs: string(refs), RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatalf("create proxy collection: %v", err)
	}

	if err := db.DeleteSubscription(sub.ID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	nodes, err = db.ListNodesBySubscription(sub.ID)
	if err != nil {
		t.Fatalf("list deleted subscription nodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected subscription nodes deleted, got %+v", nodes)
	}
	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected empty node groups deleted, got %+v", groups)
	}
	updatedCollection, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get proxy collection: %v", err)
	}
	if updatedCollection.ReferencedGroupIDs != "[]" {
		t.Fatalf("expected strategy group refs removed, got %s", updatedCollection.ReferencedGroupIDs)
	}
}

func TestStableNodeUIDUsesOnlyCoreConnectionFields(t *testing.T) {
	base := model.ParsedNode{
		Name:       "HK-01",
		Type:       "vless",
		Server:     "example.com",
		ServerPort: 443,
		Raw:        "raw-a",
		RawJSON:    `{"name":"HK-01","tag":"HK-01","type":"vless","server":"example.com","server_port":443,"uuid":"uuid-1","flow":"xtls-rprx-vision","tls":{"enabled":true,"server_name":"sni.example.com","utls":{"fingerprint":"chrome"}},"transport":{"type":"ws","path":"/ws","headers":{"Host":"host.example.com"}},"latency_ms":10,"extra_display":"a"}`,
	}
	rename := base
	rename.Name = "HK-Renamed"
	rename.Raw = "raw-b"
	rename.RawJSON = `{"name":"HK-Renamed","tag":"HK-Renamed","type":"vless","server":"example.com","server_port":443,"uuid":"uuid-1","flow":"xtls-rprx-vision","tls":{"enabled":true,"server_name":"sni.example.com","utls":{"fingerprint":"chrome"}},"transport":{"type":"ws","path":"/ws","headers":{"Host":"host.example.com"}},"latency_ms":999,"extra_display":"b"}`
	if StableNodeUID(base) != StableNodeUID(rename) {
		t.Fatalf("expected uid to ignore name/tag/raw/display fields")
	}

	changedFlow := base
	changedFlow.RawJSON = `{"name":"HK-01","type":"vless","server":"example.com","server_port":443,"uuid":"uuid-1","flow":"changed","tls":{"enabled":true,"server_name":"sni.example.com","utls":{"fingerprint":"chrome"}},"transport":{"type":"ws","path":"/ws","headers":{"Host":"host.example.com"}}}`
	if StableNodeUID(base) == StableNodeUID(changedFlow) {
		t.Fatalf("expected uid to change when flow changes")
	}
}

func TestNodeFilterStoreCRUD(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	created, err := db.CreateNodeFilter(&model.NodeFilterRequest{Name: "filter hk", Target: "name", Pattern: "HK|香港", Enabled: true})
	if err != nil {
		t.Fatalf("create node filter: %v", err)
	}
	if created.ID == 0 || !created.Enabled {
		t.Fatalf("unexpected created filter: %+v", created)
	}

	updated, err := db.UpdateNodeFilter(created.ID, &model.NodeFilterRequest{Name: "filter jp", Target: "server", Pattern: "jp\\.", Enabled: false})
	if err != nil {
		t.Fatalf("update node filter: %v", err)
	}
	if updated.Target != "server" || updated.Enabled {
		t.Fatalf("unexpected updated filter: %+v", updated)
	}

	items, err := db.ListNodeFilters()
	if err != nil {
		t.Fatalf("list node filters: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(items))
	}

	enabled, err := db.ListEnabledNodeFilters()
	if err != nil {
		t.Fatalf("list enabled filters: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("expected no enabled filters, got %+v", enabled)
	}

	if err := db.DeleteNodeFilter(created.ID); err != nil {
		t.Fatalf("delete node filter: %v", err)
	}
}
