package service

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestReconcileProtectedConfigDataPreservesUnmanagedValues(t *testing.T) {
	data := []byte(`{"route":{"final":"stale"},"dns":{"final":"remote"},"outbounds":[{"type":"direct","tag":"direct"}],"endpoints":[{"type":"wireguard","tag":"obsolete"}],"experimental":{"custom_integer":9007199254740993},"log":{"level":"debug"},"inbounds":[{"type":"mixed","listen_port":9999}],"services":[{"type":"api","secret":"synthetic-secret"}]}`)
	var expected map[string]interface{}
	if err := json.Unmarshal([]byte(`{"route":{"final":"direct"},"dns":{"final":"remote"},"outbounds":[{"type":"direct","tag":"direct"}]}`), &expected); err != nil {
		t.Fatal(err)
	}
	updated, changed, err := reconcileProtectedConfigData(data, expected)
	if err != nil || !reflect.DeepEqual(changed, []string{"route", "endpoints"}) {
		t.Fatalf("reconcile: changes=%v error=%v", changed, err)
	}
	var before, after map[string]json.RawMessage
	_ = json.Unmarshal(data, &before)
	_ = json.Unmarshal(updated, &after)
	for _, key := range []string{"dns", "outbounds", "experimental", "log", "inbounds", "services"} {
		var compact bytes.Buffer
		if err := json.Compact(&compact, after[key]); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before[key], compact.Bytes()) {
			t.Fatalf("unrelated %s was changed", key)
		}
	}
	if _, exists := after["endpoints"]; exists {
		t.Fatal("obsolete endpoints were retained")
	}
	again, changed, err := reconcileProtectedConfigData(updated, expected)
	if err != nil || len(changed) != 0 || !bytes.Equal(again, updated) {
		t.Fatalf("unchanged configuration was rewritten: %v %v", changed, err)
	}
}

func TestReconcileProtectedConfigDataRejectsInvalidDocuments(t *testing.T) {
	for _, input := range []string{"null", "[]", "invalid JSON"} {
		if _, _, err := reconcileProtectedConfigData([]byte(input), nil); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
