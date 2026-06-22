# syntax=docker/dockerfile:1

# --- build ---
FROM golang:1.26 AS build
WORKDIR /src

# Cached dependency layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# VERSION is the release tag, stamped into the binary so the MCP server reports
# the real version to clients instead of a hand-maintained constant.
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
	-ldflags="-s -w -X github.com/adambenhassen/ryanair-mcp/internal/server.version=${VERSION}" \
	-o /out/ryanair-mcp ./cmd/ryanair-mcp

# --- runtime ---
# distroless static carries CA certs (needed for HTTPS to Ryanair) and runs as
# a non-root user; the binary is static (CGO disabled).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ryanair-mcp /ryanair-mcp

# stdio is the default transport. For HTTP, override the command, e.g.:
#   docker run -p 8080:8080 ghcr.io/adambenhassen/ryanair-mcp --transport http --addr :8080
EXPOSE 8080
ENTRYPOINT ["/ryanair-mcp"]
