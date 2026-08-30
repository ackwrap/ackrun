package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestAlertStoreCRUDCooldownAndDeliveries(t *testing.T) {
	db := openAdvancedTestStore(t)
	channel := &model.AlertChannel{
		Name: "Operations", Type: model.AlertChannelTelegram, Enabled: true,
		Config: model.AlertChannelConfig{TelegramChatID: "-100123"}, Destination: "Telegram -100123",
		SecretContext: "context", SecretCiphertext: []byte("ciphertext"), SecretNonce: []byte("nonce"),
	}
	if err := db.CreateAlertChannel(channel); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.GetAlertChannel(channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.HasSecret || loaded.Config.TelegramChatID != "-100123" || loaded.LastStatus != model.AlertDeliveryNever {
		t.Fatalf("created channel = %+v", loaded)
	}
	loaded.Name = "Operations updated"
	loaded.Enabled = false
	if err := db.UpdateAlertChannel(loaded); err != nil {
		t.Fatal(err)
	}
	channels, err := db.ListAlertChannels()
	if err != nil || len(channels) != 1 || channels[0].Name != "Operations updated" || channels[0].Enabled {
		t.Fatalf("updated channels = %+v, err = %v", channels, err)
	}

	rule := &model.AlertRule{
		Name: "Failures", Enabled: true,
		EventTypes: []string{model.AlertEventCircuitOpen, model.AlertEventSubscriptionFailed},
		ChannelIDs: []int64{channel.ID}, CooldownMinutes: 60,
	}
	if err := db.CreateAlertRule(rule); err != nil {
		t.Fatal(err)
	}
	loadedRule, err := db.GetAlertRule(rule.ID)
	if err != nil || !reflect.DeepEqual(loadedRule.ChannelIDs, []int64{channel.ID}) {
		t.Fatalf("created rule = %+v, err = %v", loadedRule, err)
	}
	const first = int64(1_000_000)
	claimed, err := db.ClaimAlertRuleCooldown(rule.ID, "circuit_open:node:1", first, time.Hour)
	if err != nil || !claimed {
		t.Fatalf("first cooldown claim = %v, err = %v", claimed, err)
	}
	claimed, err = db.ClaimAlertRuleCooldown(rule.ID, "circuit_open:node:1", first+1, time.Hour)
	if err != nil || claimed {
		t.Fatalf("duplicate cooldown claim = %v, err = %v", claimed, err)
	}
	claimed, err = db.ClaimAlertRuleCooldown(rule.ID, "circuit_open:node:1", first+int64(time.Hour/time.Millisecond), time.Hour)
	if err != nil || !claimed {
		t.Fatalf("expired cooldown claim = %v, err = %v", claimed, err)
	}

	ruleID, channelID := rule.ID, channel.ID
	for index, success := range []bool{true, false} {
		item := &model.AlertDelivery{
			RuleID: &ruleID, ChannelID: &channelID, ChannelName: loaded.Name,
			ChannelType: loaded.Type, EventType: model.AlertEventCircuitOpen,
			EventTitle: "Circuit open", Success: success, StatusCode: 200 + index,
			DeliveredAt: first + int64(index),
		}
		if !success {
			item.Error = "request failed"
		}
		if err := db.CreateAlertDelivery(item); err != nil {
			t.Fatal(err)
		}
	}
	page, err := db.ListAlertDeliveries(model.AlertDeliveryFilter{
		ChannelID: channel.ID, EventType: model.AlertEventCircuitOpen,
		Status: model.AlertDeliveryFailed, Limit: 10,
	})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Success {
		t.Fatalf("filtered deliveries = %+v, err = %v", page, err)
	}

	if err := db.DeleteAlertChannel(channel.ID); err != nil {
		t.Fatal(err)
	}
	loadedRule, err = db.GetAlertRule(rule.ID)
	if err != nil || loadedRule.Enabled || len(loadedRule.ChannelIDs) != 0 {
		t.Fatalf("orphaned rule = %+v, err = %v", loadedRule, err)
	}
	page, err = db.ListAlertDeliveries(model.AlertDeliveryFilter{Limit: 10})
	if err != nil || len(page.Items) != 2 || page.Items[0].ChannelID != nil {
		t.Fatalf("delivery snapshots after channel delete = %+v, err = %v", page, err)
	}
	deleted, err := db.PruneAlertDeliveries(1)
	if err != nil || deleted != 1 {
		t.Fatalf("prune deliveries deleted = %d, err = %v", deleted, err)
	}
	if err := db.ClearAlertDeliveries(); err != nil {
		t.Fatal(err)
	}
	page, err = db.ListAlertDeliveries(model.AlertDeliveryFilter{Limit: 10})
	if err != nil || page.Total != 0 {
		t.Fatalf("deliveries after clear = %+v, err = %v", page, err)
	}
}
