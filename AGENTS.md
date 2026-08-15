# Pino: an interactive terminal editor for JSON files.

## Architecture

- Respect the dependency boundaries enforced by `depguard` in `.golangci.yml`.
- `cli` is the only package that may depend on concrete infrastructure adapters.

## Development

- Run `go tool task check` to lint, test, and build changes.
- The parts run on their own too: `go tool task lint`, `test`, `build`, `fmt`.
  `Taskfile.yml` defines them and `go tool task --list` lists them.
- Use `go tool task tools` for the pinned tools. Invoke them through the tasks.

## Testing

- Before adding or changing tests, read `docs/testing.md`.
