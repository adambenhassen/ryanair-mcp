# Security Policy

## Reporting a vulnerability

Please report security issues privately via GitHub's
[private vulnerability reporting](https://github.com/adambenhassen/ryanair-mcp/security/advisories/new)
rather than opening a public issue. Include reproduction steps and impact.

You can expect an initial response within a few days.

## Scope

This project is a read-only client for Ryanair's public, unauthenticated
endpoints. It handles no credentials, accounts, or payment data. Relevant
concerns include input handling in the MCP tool layer and the HTTP client
(e.g. request construction, response parsing). It is provided "as is" with no
warranty (see [LICENSE](LICENSE)).
