# Repository Guidelines

This repository contains the Content Grid Chain, a Cosmos SDK–based blockchain written in Go. Use this guide to contribute changes safely and consistently.

## Project Structure & Module Organization
- `app/`: Minimal app wiring for Cosmos SDK v0.53 (module basics, encoding, default genesis).
- `cmd/content-grid-d/`: CLI entrypoint for the node daemon (e.g., `version`, `init`).
- `x/`: Placeholder for chain modules (add feature modules here).
- Root files: `README.md`, `whitepaper.md`, `go.mod`, `go.sum`. Built binary: `content-grid-d`.

## Build, Test, and Development Commands
- Build daemon: `go build -o content-grid-d ./cmd/content-grid-d`
- Run locally: `./content-grid-d version` or `./content-grid-d init`
- Unit tests: `go test ./...`
- Vet/format: `go vet ./...` and `gofmt -s -w .`
Requirements: Go version per `go.mod` (Go 1.25+).

## Coding Style & Naming Conventions
- Format: gofmt (required). Run before pushing.
- Imports: standard → third‑party → local; keep groups separated.
- Naming: packages lower_snake (e.g., `x/yourmodule`), exported identifiers in UpperCamelCase, tests in `*_test.go`.
- Errors: wrap with context; prefer `%w` and sentinel errors.

## Testing Guidelines
- Framework: Go `testing` package with table tests where appropriate.
- File names: `foo_test.go`; test funcs `TestXxx` and benchmarks `BenchmarkXxx`.
- Run coverage: `go test ./... -cover`.
- Target meaningful coverage for new/changed code, especially in `app/` and new `x/<module>` packages.

## Commit & Pull Request Guidelines
- Commits: imperative subject, concise body, optional scope, e.g., `app: export DefaultGenesis` or `x/indexer: add query handler`.
- Link issues in the body (`Fixes #123`) when applicable.
- PRs: include purpose, scope of changes, testing notes, and any follow‑ups. Add screenshots only if user‑facing CLI behavior changes.

## Architecture Notes
- The app currently exposes module basics and encoding; full runtime wiring (depinject/runtime.App) is pending. New modules should live under `x/` and register via the app once wiring lands.

