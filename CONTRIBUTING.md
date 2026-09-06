# Contributing to Tempest

## Development Setup
```bash
git clone https://github.com/tempest-io/tempest.git
cd tempest
go mod download
go test ./...
```

## Code Style
- Follow standard Go conventions
- Run `gofmt -s -w .` before committing
- Run `go vet ./...` and ensure no warnings
- All exported functions must have doc comments

## Testing
- Write table-driven tests
- Use injected clocks for time-dependent tests
- Run `go test -race ./...` to verify no data races
- Aim for >80% coverage on new code

## Commit Messages
- Use imperative mood
- Keep subject line under 72 characters
- Reference issues when applicable

## Pull Request Process
1. Fork the repository
2. Create a feature branch
3. Write tests for your changes
4. Ensure all tests pass
5. Submit a pull request
