# Spark: agent workflow

This repository is developed with Codex-style agents in mind. Follow these rules for every change.

## Branching

- Never commit directly to `main`.
- Create a dedicated branch per task/feature/bugfix (one branch per agent run).
- Multiple agents may work in parallel, but never on the same branch.
- Use a clear branch naming scheme, e.g. `agent/<name>/<topic>` or `feat/<topic>`.
- Keep the branch focused: avoid mixing unrelated changes.
- Merge to `main` only via a reviewed merge (fast-forward or merge commit is fine, but keep history readable).

## Commits

- Prefer many small commits over large batches.
- Each commit should compile and keep `go test ./...` passing.
- Use descriptive commit messages in imperative mood.
- Avoid “drive-by” refactors. If needed, do them in their own commit.

## Validation

Before merging to `main`, run:

- `go test ./...`
- `go build ./...`
- `make tinygo-uf2` (when touching shared code used by TinyGo/baremetal)

## Code style

- Go code must be `gofmt`’d.
- Keep the happy path flat; wrap errors with context; avoid panics in libraries.
- Prefer explicit, simple designs (especially in kernel/IPC).

## Architecture boundaries

- Kernel: mechanism only (IPC + minimal task/endpoint primitives); no policy.
- Everything else is a service/task communicating via IPC.
- No hidden global access “by name”: pass capabilities explicitly.
