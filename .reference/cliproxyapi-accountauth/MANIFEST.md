# CLIProxyAPI Account Auth Reference

- Source repository: `/opt/project/CLIProxyAPI`
- Copied at: 2026-05-21
- Scope: auth, account runtime, access, and provider dependency code used as a migration reference.
- Purpose: reference-only source for migrating official account pool capabilities into NexusTok.

This directory is intentionally kept under `.reference` so Go package discovery does not compile it during `go list ./...` or `go test ./...`.
Do not import code from this directory directly. Migrate the required behavior into NexusTok-native packages and follow the project rules in `AGENTS.md`.
