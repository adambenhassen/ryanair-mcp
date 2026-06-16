// Command ryanair-mcp runs an MCP server exposing Ryanair's read APIs over
// stdio (default) or streamable HTTP.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/adambenhassen/ryanair-mcp/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ryanair-mcp: %v", err)
	}
}

func run() error {
	transport := flag.String("transport", "stdio", "transport to serve: stdio or http")
	addr := flag.String("addr", ":8080", "listen address for http transport")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv, err := server.New()
	if err != nil {
		return err
	}

	switch *transport {
	case "stdio":
		return server.RunStdio(ctx, srv)
	case "http":
		log.Printf("ryanair-mcp: serving streamable HTTP on %s", *addr)
		return server.RunHTTP(ctx, srv, *addr)
	default:
		return fmt.Errorf("unknown transport %q (want stdio or http)", *transport)
	}
}
