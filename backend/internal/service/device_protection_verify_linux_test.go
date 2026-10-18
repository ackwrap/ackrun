//go:build linux

package service

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceProtectionDetectsSameCountKernelEdits(t *testing.T) {
	for _, change := range []string{"accept", "source", "interface", "hook", "priority", "policy", "rule-order", "extra-chain", "missing-record", "wrong-record"} {
		t.Run(change, func(t *testing.T) {
			guard, kernel := testDeviceProtection(t)
			sources := []string{"192.168.50.10/32", "fd12:3456::10/128"}
			if err := guard.Apply(sources, nil); err != nil {
				t.Fatal(err)
			}
			var rules []map[string]any
			var chain map[string]any
			for _, object := range kernel.objects {
				if rule, ok := object["rule"].(map[string]any); ok && rule["chain"] == "forward" {
					rules = append(rules, rule)
				}
				if item, ok := object["chain"].(map[string]any); ok && item["name"] == "forward" {
					chain = item
				}
			}
			switch change {
			case "accept":
				rules[0]["expr"] = []any{map[string]any{"accept": nil}}
			case "source":
				rules[0]["expr"].([]any)[0].(map[string]any)["synthetic_statement"] = "ip saddr 192.168.50.11 counter drop"
			case "interface":
				rules[3]["expr"].([]any)[0].(map[string]any)["synthetic_statement"] = `ip saddr 192.168.50.10 oifname != "wan" counter drop`
			case "hook":
				chain["hook"] = "input"
			case "priority":
				chain["prio"] = 100
			case "policy":
				chain["policy"] = "drop"
			case "rule-order":
				rules[0]["expr"], rules[2]["expr"] = rules[2]["expr"], rules[0]["expr"]
			case "extra-chain":
				kernel.objects = append(kernel.objects, map[string]any{"chain": map[string]any{"family": "inet", "table": deviceProtectionTable, "name": "foreign"}})
			case "missing-record":
				if err := os.Remove(guard.verificationPath()); err != nil {
					t.Fatal(err)
				}
			case "wrong-record":
				if err := os.WriteFile(guard.verificationPath(), []byte(`{"version":1,"policy":{"table":{"comment":"wrong"}}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := guard.Verify(sources, nil); err == nil {
				t.Fatalf("kernel change %s was reported protected", change)
			}
			if err := guard.Apply(sources, nil); err != nil {
				t.Fatalf("explicit recovery failed: %v", err)
			}
		})
	}
}

func TestProtectionKernelPolicyIgnoresOnlyRuntimeState(t *testing.T) {
	guard, kernel := testDeviceProtection(t)
	sources := []string{"192.168.50.10/32"}
	if err := guard.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	// Replace one synthetic statement with real nft JSON including an unordered
	// protocol set and a stateful counter, then record our own install echo.
	for _, object := range kernel.objects {
		if rule, ok := object["rule"].(map[string]any); ok {
			rule["expr"] = []any{
				map[string]any{"match": map[string]any{"op": "==", "left": map[string]any{"meta": map[string]any{"key": "l4proto"}}, "right": map[string]any{"set": []any{"tcp", "udp"}}}},
				map[string]any{"counter": map[string]any{"bytes": 0, "packets": 0}},
				map[string]any{"drop": nil},
			}
		}
	}
	var echo []map[string]any
	for _, object := range kernel.objects {
		echo = append(echo, map[string]any{"add": object})
	}
	data, _ := json.Marshal(map[string]any{"nftables": echo})
	if err := guard.recordKernelPolicy(data, kernel.owner); err != nil {
		t.Fatal(err)
	}
	for _, object := range kernel.objects {
		for _, body := range object {
			body.(map[string]any)["handle"] = 123
		}
		if rule, ok := object["rule"].(map[string]any); ok {
			expr := rule["expr"].([]any)
			expr[0].(map[string]any)["match"].(map[string]any)["right"] = map[string]any{"set": []any{"udp", "tcp"}}
			expr[1] = map[string]any{"counter": map[string]any{"bytes": 5000, "packets": 50}}
		}
	}
	if err := guard.Verify(sources, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceProtectionDoesNotTrustReadbackAsInstallBaseline(t *testing.T) {
	guard, kernel := testDeviceProtection(t)
	guard.run = func(input string, args ...string) ([]byte, error) {
		data, err := kernel.run(input, args...)
		if strings.Join(args, " ") == "-j -e -f -" && err == nil {
			// Simulate a foreign edit after our atomic transaction. Its accept
			// rule must not be captured as the expected policy by Apply.
			for _, object := range kernel.objects {
				if rule, ok := object["rule"].(map[string]any); ok {
					rule["expr"] = []any{map[string]any{"accept": nil}}
					break
				}
			}
		}
		return data, err
	}
	if err := guard.Apply([]string{"192.168.50.10/32"}, nil); err == nil {
		t.Fatal("post-install foreign edit was recorded as healthy")
	}
}

// Like TestNFTGuardPackets, this must run in a disposable network namespace.
func TestDeviceProtectionKernelIntegrity(t *testing.T) {
	if os.Getenv("ACKWRAP_NFT_GUARD_TEST") != "1" {
		t.Skip("requires nft and an isolated network namespace")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Readlink("/proc/1/ns/net")
	if err != nil || self == host {
		t.Fatal("refusing to alter the host network namespace")
	}
	guard := &nftDeviceProtection{path: filepath.Join(t.TempDir(), "guard.nft"), supported: func() bool { return true }, run: func(input string, args ...string) ([]byte, error) {
		command := exec.Command("nft", args...)
		command.Stdin = strings.NewReader(input)
		return command.CombinedOutput()
	}}
	sources := []string{"192.168.50.10/32", "fd12:3456::10/128", "02:00:00:00:00:01"}
	var excluded []string
	if err := guard.Apply(sources, excluded); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ name, old, next string }{
		{"accept", "counter drop", "counter accept"},
		{"source", "ip saddr 192.168.50.10/32", "ip saddr 192.168.50.11/32"},
		{"ipv6-source", "ip6 saddr fd12:3456::10/128", "ip6 saddr fd12:3456::11/128"},
		{"interface", `oifname != "tun0"`, `oifname != "wan"`},
		{"hook", "hook forward", "hook output"},
		{"priority", "priority -5", "priority 5"},
		{"dns-port", "redirect to :1054", "redirect to :1053"},
		{"mac-source", "ether saddr 02:00:00:00:00:01", "ether saddr 02:00:00:00:00:02"},
		{"mac-port", "tproxy to :1055", "tproxy to :1053"},
		{"mac-dns-port", "redirect to :1056", "redirect to :1053"},
		{"mac-route-priority", "priority -90", "priority -95"},
	} {
		t.Run(change.name, func(t *testing.T) {
			policy, err := os.ReadFile(guard.path)
			if err != nil {
				t.Fatal(err)
			}
			mutated := strings.Replace(string(policy), change.old, change.next, 1)
			if mutated == string(policy) {
				t.Fatal("mutation did not change policy")
			}
			if output, err := guard.run(mutated, "-f", "-"); err != nil {
				t.Fatalf("apply synthetic mutation: %v: %s", err, output)
			}
			if err := guard.Verify(sources, excluded); err == nil {
				t.Fatal("kernel content change was reported healthy")
			}
			if err := guard.Apply(sources, excluded); err != nil {
				t.Fatalf("recover kernel policy: %v", err)
			}
		})
	}
	// fw4 recreates rules from the text file, with new handles. The original
	// JSON baseline must still verify after this independent reload.
	if output, err := guard.run("", "-f", guard.path); err != nil {
		t.Fatalf("synthetic fw4 reload: %v: %s", err, output)
	}
	if err := guard.Verify(sources, excluded); err != nil {
		t.Fatal(err)
	}
	if err := guard.Disable(); err != nil {
		t.Fatal(err)
	}
}
