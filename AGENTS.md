# Pino: an interactive terminal editor for JSON files.

## Architecture

- Respect the dependency boundaries enforced by `depguard` in `.golangci.yml`.
- `cli` is the only package that may depend on concrete infrastructure adapters.

## Development

- Run `go tool task check` to lint, test, and build changes. See `Taskfile.yml`
  for defined tasks.
- Use `go tool task tools` for the pinned tools.

## Testing

- Before adding or changing tests, read `docs/testing.md`.
