package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
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
	otherSub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "other-sub", URL: "https://example.com/other", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create other subscription: %v", err)
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
	allSubscriptionsGroup, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "all subscriptions", Type: "selector", FilterInclude: ".*", Enabled: true})
	if err != nil {
		t.Fatalf("create all-subscriptions group: %v", err)
	}
	multiSubscriptionGroup, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "multiple subscriptions", Type: "selector", FilterSubscriptions: fmt.Sprintf("%d,%d", sub.ID, otherSub.ID), FilterInclude: ".*", Enabled: true})
	if err != nil {
		t.Fatalf("create multi-subscription group: %v", err)
	}
	refs, _ := json.Marshal([]int64{manualGroup.ID, filterGroup.ID, allSubscriptionsGroup.ID, multiSubscriptionGroup.ID})
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
	if len(groups) != 2 || groups[0].ID != allSubscriptionsGroup.ID || groups[1].ID != multiSubscriptionGroup.ID {
		t.Fatalf("expected only all-subscription and remaining-scoped groups, got %+v", groups)
	}
	if groups[0].FilterSubscriptions != "" || groups[1].FilterSubscriptions != fmt.Sprintf("%d", otherSub.ID) {
		t.Fatalf("unexpected remaining subscription filters: %+v", groups)
	}
	updatedCollection, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get proxy collection: %v", err)
	}
	keptRefs, _ := json.Marshal([]int64{allSubscriptionsGroup.ID, multiSubscriptionGroup.ID})
	if updatedCollection.ReferencedGroupIDs != string(keptRefs) {
		t.Fatalf("expected only deleted group refs removed, got %s", updatedCollection.ReferencedGroupIDs)
	}
}

func TestClearSubscriptionNodesPreservesDynamicGroupAndReference(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub", URL: "https://example.com/sub", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	const uid = "dynamic-node"
	node := model.ParsedNode{UID: uid, Name: "HK-01", Type: "vless", Server: "hk.example.com", ServerPort: 443, Raw: "raw", RawJSON: `{"type":"vless"}`}
	if err := db.ReplaceSubscriptionNodes(sub.ID, []model.ParsedNode{node}); err != nil {
		t.Fatalf("replace subscription nodes: %v", err)
	}
	group, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "dynamic", Type: "selector", FilterSubscriptions: fmt.Sprintf("%d", sub.ID), FilterInclude: "HK", Enabled: true})
	if err != nil {
		t.Fatalf("create dynamic group: %v", err)
	}
	refs, _ := json.Marshal([]int64{group.ID})
	collection := &model.ProxyCollection{Name: "strategy", Type: "selector", SourceType: "node_groups", ReferencedGroupIDs: string(refs), RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatalf("create proxy collection: %v", err)
	}

	if err := db.ClearSubscriptionNodes(sub.ID); err != nil {
		t.Fatalf("clear subscription nodes: %v", err)
	}
	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list groups after clear: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != group.ID || groups[0].MatchedNodeCount != 0 {
		t.Fatalf("dynamic group not preserved empty: %+v", groups)
	}
	updatedCollection, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get collection after clear: %v", err)
	}
	if updatedCollection.ReferencedGroupIDs != string(refs) {
		t.Fatalf("dynamic group reference changed after clear: %s", updatedCollection.ReferencedGroupIDs)
	}

	if err := db.ReplaceSubscriptionNodes(sub.ID, []model.ParsedNode{node}); err != nil {
		t.Fatalf("restore matching node: %v", err)
	}
	groups, err = db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list groups after restore: %v", err)
	}
	if len(groups) != 1 || groups[0].MatchedNodeCount != 1 {
		t.Fatalf("restored node was not counted: %+v", groups)
	}
	matches, err := db.PreviewNodeGroupMatches(group.FilterProtocols, group.FilterSubscriptions, group.FilterInclude, group.FilterExclude)
	if err != nil {
		t.Fatalf("resolve restored dynamic group matches: %v", err)
	}
	if len(matches) != 1 || matches[0].UID != uid {
		t.Fatalf("restored node was not resolved by dynamic filters: %+v", matches)
	}
	resolved, err := db.GetProxyCollectionWithNodes(collection.ID)
	if err != nil {
		t.Fatalf("resolve collection groups: %v", err)
	}
	if len(resolved.ReferencedGroups) != 1 || resolved.ReferencedGroups[0].ID != group.ID {
		t.Fatalf("restored dynamic group was not resolved: %+v", resolved.ReferencedGroups)
	}
}

func TestCleanInvalidNodeUIDsDeletesEmptyGroupsAndUpdatesStrategyRefs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub", URL: "https://example.com/sub", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	const removedUID = "removed-uid"
	const keptUID = "kept-uid"
	if err := db.ReplaceSubscriptionNodes(sub.ID, []model.ParsedNode{
		{UID: removedUID, Name: "TW-01", Type: "vless", Server: "tw.example.com", ServerPort: 443, Raw: "raw-tw", RawJSON: `{"type":"vless","server":"tw.example.com","server_port":443,"uuid":"uuid-tw"}`},
		{UID: keptUID, Name: "HK-01", Type: "vless", Server: "hk.example.com", ServerPort: 443, Raw: "raw-hk", RawJSON: `{"type":"vless","server":"hk.example.com","server_port":443,"uuid":"uuid-hk"}`},
	}); err != nil {
		t.Fatalf("replace subscription nodes: %v", err)
	}

	emptyManual, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "empty manual", Type: "selector", NodeUIDs: []string{removedUID}, Enabled: true})
	if err != nil {
		t.Fatalf("create empty manual group: %v", err)
	}
	emptyDynamic, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "empty dynamic", Type: "selector", FilterSubscriptions: fmt.Sprintf("%d", sub.ID), FilterInclude: "TW", Enabled: true})
	if err != nil {
		t.Fatalf("create empty dynamic group: %v", err)
	}
	keptGroup, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "kept group", Type: "selector", NodeUIDs: []string{keptUID}, Enabled: true})
	if err != nil {
		t.Fatalf("create kept group: %v", err)
	}
	keptDynamic, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "kept dynamic", Type: "selector", FilterSubscriptions: fmt.Sprintf("%d", sub.ID), FilterInclude: "HK", Enabled: true})
	if err != nil {
		t.Fatalf("create kept dynamic group: %v", err)
	}

	refs, _ := json.Marshal([]int64{emptyManual.ID, emptyDynamic.ID, keptGroup.ID, keptDynamic.ID})
	groupCollection := &model.ProxyCollection{Name: "group strategy", Type: "selector", SourceType: "node_groups", ReferencedGroupIDs: string(refs), RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true}
	if err := db.CreateProxyCollection(groupCollection); err != nil {
		t.Fatalf("create group strategy: %v", err)
	}
	manualUIDs, _ := json.Marshal([]string{removedUID, keptUID})
	manualCollection := &model.ProxyCollection{Name: "manual strategy", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: string(manualUIDs), Enabled: true}
	if err := db.CreateProxyCollection(manualCollection); err != nil {
		t.Fatalf("create manual strategy: %v", err)
	}

	if err := db.ReplaceSubscriptionNodes(sub.ID, []model.ParsedNode{
		{UID: keptUID, Name: "HK-01", Type: "vless", Server: "hk.example.com", ServerPort: 443, Raw: "raw-hk", RawJSON: `{"type":"vless","server":"hk.example.com","server_port":443,"uuid":"uuid-hk"}`},
	}); err != nil {
		t.Fatalf("replace updated subscription nodes: %v", err)
	}
	cleanup, err := db.CleanInvalidNodeUIDs([]string{removedUID})
	if err != nil {
		t.Fatalf("clean invalid node UIDs: %v", err)
	}
	if cleanup.UpdatedCollections != 1 || cleanup.DeletedNodeGroups != 1 {
		t.Fatalf("unexpected cleanup result: %+v", cleanup)
	}

	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups: %v", err)
	}
	if len(groups) != 3 || groups[0].ID != emptyDynamic.ID || groups[1].ID != keptGroup.ID || groups[2].ID != keptDynamic.ID {
		t.Fatalf("expected empty dynamic and groups with matches to remain, got %+v", groups)
	}
	updatedGroupCollection, err := db.GetProxyCollection(groupCollection.ID)
	if err != nil {
		t.Fatalf("get group strategy: %v", err)
	}
	var updatedRefs []int64
	if err := json.Unmarshal([]byte(updatedGroupCollection.ReferencedGroupIDs), &updatedRefs); err != nil {
		t.Fatalf("unmarshal group strategy refs: %v", err)
	}
	if len(updatedRefs) != 3 || updatedRefs[0] != emptyDynamic.ID || updatedRefs[1] != keptGroup.ID || updatedRefs[2] != keptDynamic.ID {
		t.Fatalf("expected only empty manual group ref removed, got %+v", updatedRefs)
	}
	updatedManualCollection, err := db.GetProxyCollection(manualCollection.ID)
	if err != nil {
		t.Fatalf("get manual strategy: %v", err)
	}
	var updatedUIDs []string
	if err := json.Unmarshal([]byte(updatedManualCollection.NodeUIDs), &updatedUIDs); err != nil {
		t.Fatalf("unmarshal manual strategy UIDs: %v", err)
	}
	if len(updatedUIDs) != 1 || updatedUIDs[0] != keptUID {
		t.Fatalf("expected invalid manual UID removed, got %+v", updatedUIDs)
	}
}

func TestCleanInvalidNodeUIDsSerializesConcurrentCleanup(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub", URL: "https://example.com/sub", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	const uidA = "removed-a"
	const uidB = "removed-b"
	const keptUID = "kept"
	nodes := []model.ParsedNode{
		{UID: uidA, Name: "A", Type: "vless", Server: "a.example.com", ServerPort: 443, Raw: "raw-a", RawJSON: `{"type":"vless","server":"a.example.com","server_port":443,"uuid":"uuid-a"}`},
		{UID: uidB, Name: "B", Type: "vless", Server: "b.example.com", ServerPort: 443, Raw: "raw-b", RawJSON: `{"type":"vless","server":"b.example.com","server_port":443,"uuid":"uuid-b"}`},
		{UID: keptUID, Name: "K", Type: "vless", Server: "k.example.com", ServerPort: 443, Raw: "raw-k", RawJSON: `{"type":"vless","server":"k.example.com","server_port":443,"uuid":"uuid-k"}`},
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, nodes); err != nil {
		t.Fatalf("replace subscription nodes: %v", err)
	}
	groupA, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "group a", Type: "selector", NodeUIDs: []string{uidA}, Enabled: true})
	if err != nil {
		t.Fatalf("create group a: %v", err)
	}
	groupB, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "group b", Type: "selector", NodeUIDs: []string{uidB}, Enabled: true})
	if err != nil {
		t.Fatalf("create group b: %v", err)
	}
	refs, _ := json.Marshal([]int64{groupA.ID, groupB.ID})
	uids, _ := json.Marshal([]string{uidA, uidB, keptUID})
	collection := &model.ProxyCollection{Name: "strategy", Type: "selector", SourceType: "manual", ReferencedGroupIDs: string(refs), RouteRuleIDs: "[]", NodeUIDs: string(uids), Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, nodes[2:]); err != nil {
		t.Fatalf("replace updated subscription nodes: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, uid := range []string{uidA, uidB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cleanupErr := db.CleanInvalidNodeUIDs([]string{uid})
			errs <- cleanupErr
		}()
	}
	wg.Wait()
	close(errs)
	for cleanupErr := range errs {
		if cleanupErr != nil {
			t.Fatalf("concurrent cleanup: %v", cleanupErr)
		}
	}

	updated, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	if updated.NodeUIDs != `["kept"]` || updated.ReferencedGroupIDs != "[]" {
		t.Fatalf("expected both stale references removed, got node_uids=%s group_refs=%s", updated.NodeUIDs, updated.ReferencedGroupIDs)
	}
	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected zero-match groups deleted, got %+v", groups)
	}
}

func TestCleanInvalidNodeUIDsKeepsUIDReferencedByAnotherSubscription(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	subA, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub-a", URL: "https://example.com/a", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription a: %v", err)
	}
	subB, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub-b", URL: "https://example.com/b", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription b: %v", err)
	}
	const sharedUID = "shared-uid"
	const keptUID = "kept-a"
	sharedNode := model.ParsedNode{UID: sharedUID, Name: "Shared", Type: "vless", Server: "shared.example.com", ServerPort: 443, Raw: "raw-shared", RawJSON: `{"type":"vless","server":"shared.example.com","server_port":443,"uuid":"uuid-shared"}`}
	keptNode := model.ParsedNode{UID: keptUID, Name: "Kept", Type: "vless", Server: "kept.example.com", ServerPort: 443, Raw: "raw-kept", RawJSON: `{"type":"vless","server":"kept.example.com","server_port":443,"uuid":"uuid-kept"}`}
	if err := db.ReplaceSubscriptionNodes(subA.ID, []model.ParsedNode{sharedNode, keptNode}); err != nil {
		t.Fatalf("replace subscription a nodes: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(subB.ID, []model.ParsedNode{sharedNode}); err != nil {
		t.Fatalf("replace subscription b nodes: %v", err)
	}
	group, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "shared group", Type: "selector", NodeUIDs: []string{sharedUID}, Enabled: true})
	if err != nil {
		t.Fatalf("create shared group: %v", err)
	}
	emptyDynamic, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "subscription a shared group", Type: "selector", FilterSubscriptions: fmt.Sprintf("%d", subA.ID), FilterInclude: "Shared", Enabled: true})
	if err != nil {
		t.Fatalf("create subscription-scoped dynamic group: %v", err)
	}
	refs, _ := json.Marshal([]int64{group.ID, emptyDynamic.ID})
	uids, _ := json.Marshal([]string{sharedUID})
	collection := &model.ProxyCollection{Name: "shared strategy", Type: "selector", SourceType: "node_groups", ReferencedGroupIDs: string(refs), RouteRuleIDs: "[]", NodeUIDs: string(uids), Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatalf("create shared strategy: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(subA.ID, []model.ParsedNode{keptNode}); err != nil {
		t.Fatalf("replace updated subscription a nodes: %v", err)
	}

	cleanup, err := db.CleanInvalidNodeUIDs([]string{sharedUID})
	if err != nil {
		t.Fatalf("clean shared UID: %v", err)
	}
	if cleanup.UpdatedCollections != 0 || cleanup.DeletedNodeGroups != 0 {
		t.Fatalf("expected shared UID and empty dynamic group retained, got %+v", cleanup)
	}
	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups: %v", err)
	}
	if len(groups) != 2 || groups[0].ID != group.ID || groups[1].ID != emptyDynamic.ID {
		t.Fatalf("expected shared manual and dynamic groups retained, got %+v", groups)
	}
	updated, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get shared strategy: %v", err)
	}
	if updated.NodeUIDs != string(uids) || updated.ReferencedGroupIDs != string(refs) {
		t.Fatalf("expected shared references retained, got node_uids=%s group_refs=%s", updated.NodeUIDs, updated.ReferencedGroupIDs)
	}
}

func TestUpdateNodeGroupFiltersReturnsNoRowsForMissingGroup(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if err := db.UpdateNodeGroupFilters(9999, "vless", "1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing group error = %v, want sql.ErrNoRows", err)
	}
}

func TestCleanInvalidNodeUIDsSerializesWithSubscriptionClear(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	subA, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub-a", URL: "https://example.com/a", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription a: %v", err)
	}
	subB, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub-b", URL: "https://example.com/b", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription b: %v", err)
	}
	const uidA = "removed-a"
	const keptUID = "kept-a"
	const uidB = "removed-b"
	nodeA := model.ParsedNode{UID: uidA, Name: "A", Type: "vless", Server: "a.example.com", ServerPort: 443, Raw: "raw-a", RawJSON: `{"type":"vless","server":"a.example.com","server_port":443,"uuid":"uuid-a"}`}
	keptNode := model.ParsedNode{UID: keptUID, Name: "K", Type: "vless", Server: "k.example.com", ServerPort: 443, Raw: "raw-k", RawJSON: `{"type":"vless","server":"k.example.com","server_port":443,"uuid":"uuid-k"}`}
	nodeB := model.ParsedNode{UID: uidB, Name: "B", Type: "vless", Server: "b.example.com", ServerPort: 443, Raw: "raw-b", RawJSON: `{"type":"vless","server":"b.example.com","server_port":443,"uuid":"uuid-b"}`}
	if err := db.ReplaceSubscriptionNodes(subA.ID, []model.ParsedNode{nodeA, keptNode}); err != nil {
		t.Fatalf("replace subscription a nodes: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(subB.ID, []model.ParsedNode{nodeB}); err != nil {
		t.Fatalf("replace subscription b nodes: %v", err)
	}
	groupA, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "group a", Type: "selector", NodeUIDs: []string{uidA}, Enabled: true})
	if err != nil {
		t.Fatalf("create group a: %v", err)
	}
	groupB, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "group b", Type: "selector", NodeUIDs: []string{uidB}, Enabled: true})
	if err != nil {
		t.Fatalf("create group b: %v", err)
	}
	refs, _ := json.Marshal([]int64{groupA.ID, groupB.ID})
	uids, _ := json.Marshal([]string{uidA, uidB})
	collection := &model.ProxyCollection{Name: "strategy", Type: "selector", SourceType: "manual", ReferencedGroupIDs: string(refs), RouteRuleIDs: "[]", NodeUIDs: string(uids), Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(subA.ID, []model.ParsedNode{keptNode}); err != nil {
		t.Fatalf("replace updated subscription a nodes: %v", err)
	}

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, cleanupErr := db.CleanInvalidNodeUIDs([]string{uidA})
		errs <- cleanupErr
	}()
	go func() {
		defer wg.Done()
		errs <- db.ClearSubscriptionNodes(subB.ID)
	}()
	wg.Wait()
	close(errs)
	for operationErr := range errs {
		if operationErr != nil {
			t.Fatalf("concurrent cleanup and clear: %v", operationErr)
		}
	}

	updated, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	if updated.NodeUIDs != "[]" || updated.ReferencedGroupIDs != "[]" {
		t.Fatalf("expected all stale references removed, got node_uids=%s group_refs=%s", updated.NodeUIDs, updated.ReferencedGroupIDs)
	}
	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected empty groups deleted, got %+v", groups)
	}
}

func TestCleanInvalidNodeUIDsRollsBackAllChanges(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "sub", URL: "https://example.com/sub", UserAgent: "clash-meta/2.4.0", SyncMode: "off", SyncTimeoutSecs: 60})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	const removedUID = "removed"
	const keptUID = "kept"
	nodes := []model.ParsedNode{
		{UID: removedUID, Name: "R", Type: "vless", Server: "r.example.com", ServerPort: 443, Raw: "raw-r", RawJSON: `{"type":"vless","server":"r.example.com","server_port":443,"uuid":"uuid-r"}`},
		{UID: keptUID, Name: "K", Type: "vless", Server: "k.example.com", ServerPort: 443, Raw: "raw-k", RawJSON: `{"type":"vless","server":"k.example.com","server_port":443,"uuid":"uuid-k"}`},
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, nodes); err != nil {
		t.Fatalf("replace subscription nodes: %v", err)
	}
	group, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "empty group", Type: "selector", NodeUIDs: []string{removedUID}, Enabled: true})
	if err != nil {
		t.Fatalf("create node group: %v", err)
	}
	uids, _ := json.Marshal([]string{removedUID, keptUID})
	collection := &model.ProxyCollection{Name: "strategy", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: string(uids), Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatalf("create strategy: %v", err)
	}
	if _, err := db.DB().Exec(`UPDATE proxy_collections SET referenced_group_ids = ? WHERE id = ?`, "invalid-json", collection.ID); err != nil {
		t.Fatalf("corrupt strategy refs: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, nodes[1:]); err != nil {
		t.Fatalf("replace updated subscription nodes: %v", err)
	}

	if _, err := db.CleanInvalidNodeUIDs([]string{removedUID}); err == nil {
		t.Fatal("expected malformed group references to fail cleanup")
	}
	updated, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatalf("get strategy after rollback: %v", err)
	}
	if updated.NodeUIDs != string(uids) {
		t.Fatalf("expected node UID cleanup rolled back, got %s", updated.NodeUIDs)
	}
	groups, err := db.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups after rollback: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != group.ID {
		t.Fatalf("expected node group deletion rolled back, got %+v", groups)
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
