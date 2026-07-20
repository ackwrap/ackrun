package service

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSystemDNSFlushCommands(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "ubus" {
			return "/usr/bin/" + name, nil
		}
		return "", fmt.Errorf("not found")
	}
	commands := systemDNSFlushCommands("linux", lookPath)
	if len(commands) != 1 {
		t.Fatalf("Linux DNS flush commands = %+v", commands)
	}
	if commands[0].name != "OpenWrt dnsmasq" || commands[0].path != "/usr/bin/ubus" || !reflect.DeepEqual(commands[0].args, []string{"call", "service", "signal", `{"name":"dnsmasq","signal":1}`}) {
		t.Fatalf("OpenWrt DNS flush command = %+v", commands[0])
	}
	windows := systemDNSFlushCommands("windows", lookPath)
	if len(windows) != 1 || windows[0].name != "ipconfig" || !reflect.DeepEqual(windows[0].args, []string{"/flushdns"}) {
		t.Fatalf("Windows DNS flush commands = %+v", windows)
	}
	if unsupported := systemDNSFlushCommands("darwin", lookPath); len(unsupported) != 0 {
		t.Fatalf("unsupported DNS flush commands = %+v", unsupported)
	}
	if unsupported := systemDNSFlushCommands("linux", func(string) (string, error) { return "", fmt.Errorf("not found") }); len(unsupported) != 0 {
		t.Fatalf("Linux without ubus commands = %+v", unsupported)
	}
}
