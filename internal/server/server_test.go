package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/adambenhassen/ryanair-mcp/internal/server"
	"github.com/adambenhassen/ryanair-mcp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// wantTools is every tool the server must expose over MCP.
var wantTools = []string{
	"search_one_way", "search_return", "find_anywhere_under",
	"cheapest_per_day", "cheapest_return_per_day", "cheapest_weekend",
	"get_active_dates", "get_schedules", "list_airports", "validate_route",
	"explore_destinations", "airport_info",
}

// connect wires an in-memory MCP client to srv and returns the live client
// session, exercising the real initialize handshake.
func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		if err := cs.Close(); err != nil {
			t.Errorf("close client session: %v", err)
		}
	})
	return cs
}

// TestServerExposesAllToolsOverMCP drives the production server.New through a
// real MCP session (initialize + tools/list), proving the wiring and schema
// generation for every tool. Listing tools makes no upstream calls, so this is
// hermetic and runs in CI.
func TestServerExposesAllToolsOverMCP(t *testing.T) {
	srv, err := server.New()
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	res, err := connect(t, srv).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, name := range wantTools {
		if !got[name] {
			t.Errorf("server is missing tool %q", name)
		}
	}
	if len(res.Tools) != len(wantTools) {
		t.Errorf("tool count = %d, want %d", len(res.Tools), len(wantTools))
	}
}

// TestServerCallToolRoundtrip exercises a full tools/call over MCP against
// fixture data, covering registration, the handler, and structured output as
// the protocol delivers it to a client.
func TestServerCallToolRoundtrip(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "ryanair-mcp", Version: "test"}, nil)
	tools.Register(srv, fixtureClient(t))

	res, err := connect(t, srv).CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_airports",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool list_airports: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	blob, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if !strings.Contains(string(blob), `"iata_code":"DUB"`) {
		t.Errorf("list_airports result missing DUB: %s", blob)
	}
}

// fixtureClient builds a ryanair.Client backed by the recorded testdata
// fixtures, so a tool call resolves without hitting the network.
func fixtureClient(t *testing.T) *ryanair.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/api/views/locate/3/aggregate/all/en"):
			serveFixture(t, w, "network.json")
		case strings.HasPrefix(r.URL.Path, "/farfnd/v4/oneWayFares"):
			serveFixture(t, w, "one_way_fares.json")
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client, err := ryanair.NewClient(
		ryanair.WithHTTPClient(&http.Client{Transport: rewriteHost{base: base}}),
		ryanair.WithNetworkTTL(time.Minute),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func serveFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "ryanair", "testdata", name))
	if err != nil {
		t.Errorf("read fixture %s: %v", name, err)
		http.Error(w, "fixture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(b); err != nil {
		t.Errorf("write fixture: %v", err)
	}
}

// rewriteHost redirects the client's hard-coded hosts to the test server.
type rewriteHost struct{ base *url.URL }

func (rt rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.base.Scheme
	req.URL.Host = rt.base.Host
	return http.DefaultTransport.RoundTrip(req)
}
