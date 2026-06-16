// Package server builds the Ryanair MCP server and runs it over stdio or
// streamable HTTP.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
	"github.com/adambenhassen/ryanair-mcp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

// httpReadHeaderTimeout bounds how long the HTTP server waits for request
// headers, guarding against slow-client resource exhaustion.
const httpReadHeaderTimeout = 10 * time.Second

// New builds an MCP server with all Ryanair tools registered.
func New() (*mcp.Server, error) {
	client, err := ryanair.NewClient()
	if err != nil {
		return nil, fmt.Errorf("server: build ryanair client: %w", err)
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "ryanair-mcp", Version: version}, nil)
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
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
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
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: http shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server: http serve: %w", err)
		}
		return nil
	}
}
