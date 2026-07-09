package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStdioSmoke builds the binary and drives a real MCP session through it as
// a subprocess over stdio (the default transport): initialize + tools/list.
// This covers cmd/main, RunStdio, and the stdio transport that the in-process
// server test cannot. Listing tools makes no network calls, so it is hermetic.
func TestStdioSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bin := filepath.Join(t.TempDir(), "ryanair-mcp")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".") //nolint:gosec // G204: fixed args, temp output path
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build binary: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "smoke", Version: "0"}, nil)
	transport := &mcp.CommandTransport{Command: exec.CommandContext(ctx, bin)} //nolint:gosec // G204: our own freshly built binary
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect to subprocess: %v", err)
	}
	// Deferred (not t.Cleanup) so it runs before cancel() above: closing the
	// session lets the server exit on stdin EOF, rather than cancel() SIGKILLing
	// it out from under us.
	defer func() {
		if err := cs.Close(); err != nil {
			t.Errorf("close session: %v", err)
		}
	}()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 9 {
		t.Errorf("tool count = %d, want 9", len(res.Tools))
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, name := range []string{"search_one_way", "list_airports", "explore_destinations"} {
		if !got[name] {
			t.Errorf("subprocess missing tool %q", name)
		}
	}
}
