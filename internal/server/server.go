// Package server builds the Ryanair MCP server and runs it over stdio or
// streamable HTTP.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/adambenhassen/ryanair-mcp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is the server version advertised to MCP clients. It is not a
// hand-maintained constant (which drifts from the release tag): release builds
// stamp the tag via -ldflags "-X .../internal/server.version=<tag>", and
// `go install module@vX` builds fall back to the module version from build
// info. Plain local builds report "dev".
var version = "dev"

// serverVersion resolves the version to advertise, preferring an ldflags-stamped
// value, then the build-info module version, then the "dev" default.
func serverVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

// httpReadHeaderTimeout bounds how long the HTTP server waits for request
// headers, guarding against slow-client resource exhaustion.
const httpReadHeaderTimeout = 10 * time.Second

// New builds an MCP server with all Ryanair tools registered.
func New() (*mcp.Server, error) {
	client, err := ryanair.NewClient()
	if err != nil {
		return nil, fmt.Errorf("server: build ryanair client: %w", err)
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "ryanair-mcp", Version: serverVersion()}, nil)
	tools.Register(srv, client)
	return srv, nil
}

// RunStdio serves the MCP server over stdin/stdout until the client disconnects.
func RunStdio(ctx context.Context, srv *mcp.Server) error {
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server: stdio run: %w", err)
	}
	return nil
}

// RunHTTP serves the MCP server over streamable HTTP on addr until ctx is done.
func RunHTTP(ctx context.Context, srv *mcp.Server, addr string) error {
	base := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	// The SDK stopped applying cross-origin protection by default in v1.6, so
	// restore it with the stdlib middleware it recommends. This guards against
	// cross-origin browser (CSRF / DNS-rebinding) requests; non-browser MCP
	// clients, which send no Origin/Sec-Fetch-Site headers, are unaffected.
	handler := http.NewCrossOriginProtection().Handler(base)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpReadHeaderTimeout)
		defer cancel()
		var shutdownErr error
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			shutdownErr = fmt.Errorf("server: http shutdown: %w", err)
		}
		// The goroutine always sends exactly once, so this read is safe and
		// surfaces a real serve failure that raced with shutdown rather than
		// silently dropping it.
		var serveErr error
		if err := <-errCh; err != nil {
			serveErr = fmt.Errorf("server: http serve: %w", err)
		}
		return errors.Join(serveErr, shutdownErr)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server: http serve: %w", err)
		}
		return nil
	}
}
