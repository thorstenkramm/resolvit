# Repository Guidelines

## Project Structure & Module Organization
- `main.go` and `main_test.go` provide the CLI entrypoint and top-level tests.
- `pkg/` holds core packages: `config/`, `server/`, `handler/`, `records/`, `forward/`, `dnscache/`, `logger/`, and `version/`.
- `testdata/` contains fixture records used by tests; `records.txt` is a sample local records file.
- `dist/` is used for release artifacts; `.github/` includes CI workflows and helper scripts.

## Build, Test, and Development Commands
- `go build -ldflags "-X resolvit/pkg/version.ResolvitVersion=$VERSION" -o resolvit`: build with an explicit version string.
- `go test -race ./...`: run all unit tests with the race detector.
- `./docker-run-tests.sh`: run golangci-lint, unit tests, jscpd, and the stress test in Docker.
- `npx jscpd --pattern "**/*.go" --ignore "**/*_test.go" --threshold 0 --exitCode 1`: copy/paste detection, mirrors CI.
- `./resolvit --listen 127.0.0.1:5300 --upstream 8.8.8.8:53 --resolve-from records.txt`: run locally (or use `go run .`).

## Coding Style & Naming Conventions
- Go 1.25 module; format with `gofmt` and `goimports` (see `.golangci.yaml`).
- Keep lines around 140 chars (linted by `lll`), and avoid unused exports.
- Package names are lowercase; tests live in `*_test.go`.
- Pay attention to all rules located in `./rules/`.

## Testing Guidelines
- Use the Go `testing` package and keep tests beside the code they cover.
- Store fixtures under `testdata/` and keep tests deterministic.
- Update or add stress-test scenarios when DNS behavior changes (`dns-stress.py` or `.github/scripts/stress-test.sh`).

## Commit & Pull Request Guidelines
- History favors short, sentence-case messages like "Fix ..." or "Bumping ..."; keep them specific.
- PRs should include a brief summary, testing commands run, and links to issues if relevant.
- Update `README.md` when flags, record formats, or usage examples change.
