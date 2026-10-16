//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/paths"
)

func TestDeviceDNSLifecycleOwnsOnlyItsTable(t *testing.T) {
	for _, scenario := range []string{"new", "existing", "foreign", "offload", "apply-failure", "remove-failure"} {
		t.Run(scenario, func(t *testing.T) {
			present := scenario == "existing" || scenario == "foreign"
			mutations := 0
			d := &nftDeviceDNS{statePath: filepath.Join(t.TempDir(), "state")}
			d.run = func(input string, args ...string) ([]byte, error) {
				switch strings.Join(args, " ") {
				case "-j list ruleset":
					if scenario == "offload" {
						return []byte(`{"nftables":[{"flowtable":{"name":"ft"}}]}`), nil
					}
					if present {
						owner := deviceDNSOwner
						if scenario == "foreign" {
							owner = "other"
						}
						return []byte(`{"nftables":[{"table":{"family":"inet","name":"ackwrap_device_dns","comment":"` + owner + `"}}]}`), nil
					}
					return []byte(`{"nftables":[]}`), nil
				case "-f -":
					mutations++
					for _, want := range []string{"ip saddr 192.168.50.10/32 tcp dport 53 redirect to :1054", "ip6 saddr fd12:3456::10/128 udp dport 53 redirect to :1054", "priority -190"} {
						if !strings.Contains(input, want) {
							t.Fatalf("missing %s", want)
						}
					}
					if strings.Contains(input, "flush ruleset") || strings.Contains(input, "snat") || strings.Contains(input, "fw4") {
						t.Fatal("unexpected unrelated firewall change")
					}
					if scenario == "apply-failure" {
						return nil, errors.New("apply failed")
					}
					present = true
					return nil, nil
				case "delete table inet ackwrap_device_dns":
					mutations++
					if scenario == "remove-failure" {
						return nil, errors.New("delete failed")
					}
					present = false
					return nil, nil
				default:
					t.Fatal("unexpected nft invocation")
					return nil, nil
				}
			}
			err := d.Activate([]string{"192.168.50.10/32", "fd12:3456::10/128"})
			wantError := scenario == "foreign" || scenario == "offload" || scenario == "apply-failure"
			if (err != nil) != wantError {
				t.Fatalf("Activate: %v", err)
			}
			if scenario == "foreign" || scenario == "offload" {
				if mutations != 0 {
					t.Fatal("mutated incompatible firewall")
				}
				return
			}
			// A fresh manager recovers durable intent after a process or daemon exit.
			recovered := &nftDeviceDNS{statePath: d.statePath, run: d.run}
			err = recovered.Restore()
			if scenario == "remove-failure" {
				if err == nil {
					t.Fatal("cleanup failure hidden")
				}
				if _, err := os.Stat(d.statePath); err != nil {
					t.Fatal("lost retry marker")
				}
			} else {
				if err != nil || present {
					t.Fatalf("Restore: %v, present %t", err, present)
				}
				if _, err := os.Stat(d.statePath); !os.IsNotExist(err) {
					t.Fatal("cleanup marker remains")
				}
			}
		})
	}
}

func TestDeviceDNSNoRulesDoesNotTouchFirewall(t *testing.T) {
	d := &nftDeviceDNS{statePath: filepath.Join(t.TempDir(), "missing"), run: func(string, ...string) ([]byte, error) { t.Fatal("unexpected firewall access"); return nil, nil }}
	if err := d.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := deviceDNSNFTRules([]string{"bad; flush ruleset"}, false); err == nil {
		t.Fatal("accepted invalid source")
	}
}

func TestDeviceDNSForceRestoreWithoutMarkerStillChecksOwnership(t *testing.T) {
	for _, owner := range []string{deviceDNSOwner, "other application"} {
		t.Run(owner, func(t *testing.T) {
			deleted := false
			d := &nftDeviceDNS{statePath: filepath.Join(t.TempDir(), "missing")}
			d.run = func(_ string, args ...string) ([]byte, error) {
				switch strings.Join(args, " ") {
				case "-j list ruleset":
					return []byte(`{"nftables":[{"table":{"family":"inet","name":"ackwrap_device_dns","comment":"` + owner + `"}}]}`), nil
				case "delete table inet ackwrap_device_dns":
					deleted = true
					return nil, nil
				default:
					t.Fatal("unexpected firewall command")
					return nil, nil
				}
			}
			err := d.restore(true)
			if owner == deviceDNSOwner && (err != nil || !deleted) {
				t.Fatalf("orphan redirect not released: %v", err)
			}
			if owner != deviceDNSOwner && (err == nil || deleted) {
				t.Fatal("reset modified another application's table")
			}
		})
	}
}

type recordingDeviceDNS struct{ calls int }

func (d *recordingDeviceDNS) Activate([]string) error { d.calls++; return nil }
func (*recordingDeviceDNS) Restore() error            { return nil }

func TestDeviceDNSDoesNotActivateAfterCoreExit(t *testing.T) {
	d := &recordingDeviceDNS{}
	svc := &SingboxService{paths: &paths.Paths{DataDir: t.TempDir()}, deviceDNS: d}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.activateDeviceDNS(ctx, []string{"192.168.50.10/32"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("activation error: %v", err)
	}
	if d.calls != 0 {
		t.Fatal("installed redirect after core exit")
	}
}
