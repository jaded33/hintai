# Contributing

Thanks for helping improve HintAI. Open an issue for larger changes, then submit a focused pull request with tests and updated documentation for behavior changes.

## Development

Requirements: Go 1.23 or newer and Git.

    git clone https://github.com/jaded33/hintai.git
    cd hintai
    go test ./...
    go vet ./...

History-reader tests use synthetic fixtures. Do not include personal shell history, API keys, private configuration, screenshots, or internal paths in issues, commits, or test fixtures. Use a temporary history file when reproducing a problem.

## Pull requests

- Format Go files with gofmt.
- Add or update focused tests.
- Describe user-visible changes and how you verified them.
- Update README or sample configuration when commands or settings change.
- Never commit credentials or personal history.

GitHub Actions checks formatting, tests, vet, and cross-platform builds.
