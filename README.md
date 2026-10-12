# HintAI

**Search the commands you ran. Get commands for what you need to do.**

![HintAI logo](asset/hintai-logo.png)\n\nHintAI is a Go CLI that searches Zsh, Bash, or Fish history with natural-language queries and suggests shell commands using local or cloud language models.

> **Privacy:** hintai find sends the selected history entries and query to the configured provider. hintai suggest also sends recent history unless you pass --no-history-context. Choose a local provider for local inference, or disable suggestion history context. Review generated commands before running them.

## Install

Download a macOS or Linux archive for your system from [Releases](https://github.com/jaded33/hintai/releases). Archives include the executable and SHA256 checksums.

Or install from source with Go 1.23 or newer:

    go install github.com/jaded33/hintai/cmd/hintai@latest

## Configure

Run the setup wizard:

    hintai configure

It configures a local LM Studio or Ollama server, or one of the cloud providers below. Cloud keys are read from environment variables when set; YAML api_key remains supported.

| Provider | API key environment variable |
|---|---|
| OpenAI | OPENAI_API_KEY |
| Anthropic | ANTHROPIC_API_KEY |
| Gemini | GEMINI_API_KEY (legacy GOOGLE_API_KEY also works) |
| OpenRouter | OPENROUTER_API_KEY |
| Grok | XAI_API_KEY |

The config file lives at ~/.config/hintai/config.yaml and is written with owner-only permissions. See [sample-config.yaml](sample-config.yaml) for options. LM Studio model IDs must match a model available in the local library; an already loaded model can be reused.

History shell is detected from $SHELL. Default paths are ~/.zsh_history, ~/.bash_history, and ~/.local/share/fish/fish_history. Override them per command with --shell and --history-file.

## Use it

Find a command by describing it:

    hintai find "the git command I used to undo the last commit but keep the changes"
    hintai find --limit 1000 "the command that listed Docker containers by size"
    hintai find --history-file ./test-history.txt --shell bash "show disk usage"

Get a new command suggestion:

    hintai suggest "find log files changed in the last day"
    hintai suggest --no-history-context "convert an MP4 to a GIF"

--limit 0 reads all available entries; negative limits are rejected. Results from find are mapped back to commands in the supplied history. Suggestions are generated text and should be reviewed before execution.

## Providers and platform support

The provider clients use OpenAI-compatible chat endpoints where available. LM Studio and Ollama run locally. Cloud providers transmit the query and selected history context to their respective services.

| Provider | Status |
|---|---|
| LM Studio | Mock-tested; local HTTP endpoint responds (inference not verified) |
| Ollama | Mock-tested; local HTTP endpoint responds (inference not verified) |
| OpenAI, Anthropic, Gemini, OpenRouter, Grok | Mock-tested; live checks depend on user credentials |

Shell readers support Zsh extended/plain history, Bash timestamped/plain history, and Fish history files. Release binaries target macOS and Linux on ARM64 and AMD64.

## Development

    go test ./...
    go vet ./...

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution steps. GitHub Actions checks formatting, tests, vet, and cross-platform builds.

## Origin and license

HintAI is a fork of [historai](https://github.com/sanspareilsmyn/historai) by sanspareilsmyn. It retains upstream history and the Apache License 2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE). HintAI adds configurable local/cloud providers, shell history readers, and setup options.
