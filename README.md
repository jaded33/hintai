# HintAI

**Search the commands you ran. Get commands for what you need to do.**

![HintAI logo](asset/hintai-logo.png)

![HintAI demo using synthetic history](asset/demo.gif)

HintAI is a Go command-line tool that searches Zsh, Bash, or Fish history with natural-language queries and suggests shell commands using local or cloud language models.

> **History privacy:** `hintai find` sends the query and selected history entries to the configured provider endpoint. `hintai suggest` sends the task description and, by default, recent history; use `--no-history-context` to omit history. The default limits are 300 entries for `find` and 100 for `suggest`; `--limit 0` sends all available entries. A command history can contain secrets. Choose a local provider if you want prompts to stay on your machine, and review provider retention policies before using a cloud endpoint. HintAI never runs suggested commands for you.

## Install

Download a macOS or Linux archive for your system from [Releases](https://github.com/jaded33/hintai/releases). Each archive contains the executable and license notices; a separate SHA256 checksum file is included with each release.

Or install from source with Go 1.26 or newer:

    go install github.com/jaded33/hintai/cmd/hintai@latest

## Quick start

    hintai --version
    hintai configure
    hintai find "show the Git working tree status"
    hintai suggest "find log files changed in the last day"

## Configure

Run the setup wizard:

    hintai configure

Choose a provider, its base URL and model, and the shell history file. The wizard identifies the provider as local or cloud and prints the endpoint that will receive history context. It masks keys entered at the terminal. You can also set a provider key in the environment:

For a local setup, start LM Studio or Ollama, then run `hintai configure` and select that provider. For a cloud setup, export its key first, then run the wizard; for example:

    export OPENAI_API_KEY=your-key
    hintai configure

| Provider | Environment variable |
|---|---|
| LM Studio | `LM_API_TOKEN` (optional; for authenticated servers) |
| OpenAI | `OPENAI_API_KEY` |
| Anthropic | `ANTHROPIC_API_KEY` |
| Gemini | `GEMINI_API_KEY` (`GOOGLE_API_KEY` is also supported) |
| OpenRouter | `OPENROUTER_API_KEY` |
| Grok | `XAI_API_KEY` |

Ollama normally runs locally without an API key. The legacy YAML `api_key` field remains supported. Environment keys take precedence over that field.

Configuration is stored at `~/.config/hintai/config.yaml`; an absolute `XDG_CONFIG_HOME` changes that location. HintAI writes the config with owner-only permissions. See [sample-config.yaml](sample-config.yaml) for provider and endpoint examples.

Each provider has a default base URL, and the wizard lets you change it for compatible gateways, proxies, or self-hosted endpoints. Enter the base URL, not the final request path. Keep API keys in the environment or config field rather than embedding them in the URL. Model catalogs and names change often, so choose a model currently supported by your provider.

## Shell history

HintAI detects the shell from `$SHELL` and supports Zsh extended/plain history, Bash timestamped/plain history, and Fish history files. It honors an exported `HISTFILE` for Zsh and Bash and Fish's `XDG_DATA_HOME`. Configure a different file in the wizard or override it for one command:

    hintai find --shell bash --history-file ./sample-history "show disk usage"

The default files are `~/.zsh_history`, `~/.bash_history`, and `$XDG_DATA_HOME/fish/fish_history` (or `~/.local/share/fish/fish_history` when `XDG_DATA_HOME` is unset). Paths beginning with `~/` are expanded.

## Use it

Find a command by describing it:

    hintai find "the git command I used to undo the last commit but keep the changes"
    hintai find --limit 1000 "the command that listed Docker containers by size"

Get a command suggestion for the configured shell:

    hintai suggest "find log files changed in the last day"
    hintai suggest --no-history-context "convert an MP4 to a GIF"

`--limit 0` reads all available entries; negative limits are rejected. Search results are mapped back to commands that actually appear in the supplied history. Suggestions are generated text. Review them before running.

## Providers and API status

HintAI uses each provider's current text-generation API: LM Studio's OpenAI-compatible chat endpoint with its REST model-list/load API, Ollama's native `/api/chat`, OpenAI's Responses API, Anthropic's Messages API, Gemini's Interactions API (currently beta), and OpenAI-compatible Chat Completions for OpenRouter and xAI. These request formats and response parsers are covered by mock HTTP tests. Live inference has not been verified for this release; local and cloud checks require a loaded local model or working provider credentials.

Provider API references: [LM Studio](https://lmstudio.ai/docs/developer/rest), [Ollama](https://docs.ollama.com/api/chat), [OpenAI](https://developers.openai.com/api/reference/resources/responses), [Anthropic](https://platform.claude.com/docs/en/api/messages/create), [Gemini](https://ai.google.dev/api/interactions-api), [OpenRouter](https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request), and [xAI](https://docs.x.ai/developers/rest-api-reference/inference).

## Troubleshooting

- **Configuration not found:** run `hintai configure`; check `XDG_CONFIG_HOME` if you keep configuration outside `~/.config`.
- **History file not found:** confirm the selected shell and file with `--shell` and `--history-file`. Zsh and Bash also honor exported `HISTFILE`; Fish uses `XDG_DATA_HOME`.
- **Authentication or model error:** verify the provider's environment key, current model name, and configured base URL. The base URL should not include the final API path.
- **Local provider timeout:** confirm LM Studio or Ollama is running at the configured endpoint and that the selected model is loaded or available to load. Model loading can take time and memory.

Release binaries target macOS and Linux on ARM64 and AMD64.

## Development

    go test ./...
    go vet ./...

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution steps. GitHub Actions checks formatting, tests, vet, cross-platform builds, and secret patterns.

## Origin and license

HintAI is a fork of [historai](https://github.com/sanspareilsmyn/historai) by sanspareilsmyn. It retains upstream history and the Apache License 2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE). HintAI adds configurable local/cloud providers, shell history readers, and setup options.
