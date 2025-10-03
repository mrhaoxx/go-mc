# Repository Guidelines

## Project Structure & Module Organization
- Root binary: `main.go` (server entrypoint; reads `config.toml`).
- Modules/packages: `server`, `game`, `world`, `net`, `nbt`, `level`, `chat`, `client`, `yggdrasil`, `realms`, `registry`, `save`, plus `internal` helpers.
- Examples: `examples/` (runnable samples like `mcping`, `daze`).
- Tests: `_test.go` files across packages.
- Data/assets: `data/`, `overworld/`, `world/` (generated/runtime assets; see `.gitignore`).

## Build, Test, and Development Commands
- Build binary: `go build -o go-mc .` (outputs `./go-mc`).
- Run locally: `go run .` or `./go-mc -debug` (debug logging via Zap).
- Unit tests: `go test ./...` (use `-race -cover` for race/coverage).
- Vet/static checks: `go vet ./...`; format with `gofmt -s -w .`.
- Run examples: `go run ./examples/mcping localhost`.

Requires a recent Go toolchain (1.22+; module targets 1.24).

## Coding Style & Naming Conventions
- Use standard Go style via `gofmt`; 4-space visual indent (tabs).
- Package names are short, lowercase; exported identifiers use `CamelCase`.
- Files: tests end with `_test.go`; example files may use `_example.go`.
- Keep public APIs stable; document changes in `README.md` or package docs.

## Testing Guidelines
- Prefer table-driven tests with `testing`.
- Name tests `TestXxx`; benchmarks `BenchmarkXxx`; examples `ExampleXxx`.
- Run `go test ./... -race -cover` before submitting; add focused tests for new code.

## Commit & Pull Request Guidelines
- Commits: imperative mood (“Add…”, “Fix…”), concise scope in subject; group related changes.
- PRs: include summary, rationale, linked issues, and test evidence (output or coverage). Mention any API changes and migration notes.
- Keep diffs minimal; avoid unrelated refactors.

## Security & Configuration Tips
- Do not commit secrets; `config.toml` is for local development. Document sensitive fields and provide safe defaults.
- Large/generated data stay ignored per `.gitignore` (e.g., `overworld/`, `go-mc`).

## Agent-Specific Instructions
- Respect this guide for any automated edits. Do not alter public APIs or repo layout without prior discussion. Prefer surgical patches with clear rationale.
