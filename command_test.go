package main

import "testing"

func TestRecentIsFreshByDefaultAndSupportsCachedOverride(t *testing.T) {
	cmd := cmdRecent()
	flag := cmd.Flags().Lookup("cached")
	if flag == nil {
		t.Fatal("recent must expose --cached")
	}
	if flag.DefValue != "false" {
		t.Fatalf("recent --cached default=%s want=false", flag.DefValue)
	}
}

func TestListSupportsExplicitFreshMode(t *testing.T) {
	cmd := cmdList()
	flag := cmd.Flags().Lookup("fresh")
	if flag == nil {
		t.Fatal("list must expose --fresh")
	}
	if flag.DefValue != "false" {
		t.Fatalf("list --fresh default=%s want=false", flag.DefValue)
	}
}

func TestRootIncludesSyncCommand(t *testing.T) {
	root := buildRootCommand()
	cmd, _, err := root.Find([]string{"sync"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd == root || cmd.Name() != "sync" {
		t.Fatalf("sync command missing; found %q", cmd.Name())
	}
}
