# rel

`rel` is a fast, interactive TUI (Terminal User Interface) CLI tool designed to automate periodic GitHub release processes for the `payment-integrations` team at Getir.

It helps developers quickly select repositories, bump versions, check branch synchronizations (`master` vs `dev`), create release branches, and automatically open Pull Requests.

## Features

- **`rel release`**: Interactive release workflow. It fetches repositories, calculates the next minor version, validates if `master` is ahead of `dev`, creates a `release/x.y.z` branch, opens a PR, and generates a markdown file with release notes.
- **`rel sync`**: Post-release utility. Automatically checks if `master` is ahead of `dev` and opens sync PRs (`chore: master to dev sync`) to keep branches up-to-date.
- **Repository cache**: The repository list is cached locally for 30 minutes, so repeated runs don't re-fetch it from GitHub every time.

## Prerequisites

- [Go](https://golang.org/doc/install) (1.20+)
- A GitHub Personal Access Token (PAT) with repository read/write and team access.

## Build / Installation

Clone the repository and build the binary using Go:

```bash
git clone https://github.com/gozeloglu/rel.git
cd rel
go build -o rel
```

(Optional) Move the `rel` binary to your system's `$PATH` for global access:

```bash
mv rel /usr/local/bin/
```

## Usage

The tool authenticates via the `GH_TOKEN` environment variable. Before running any commands, export your token:

```bash
export GH_TOKEN="your_github_token_here"
```

### Start a Release

To start the interactive release process:

```bash
rel release
```

Use your arrow keys and spacebar to select the repositories you want to release, confirm or edit the version numbers, and the tool will automatically handle branching and PR creation.

Key bindings in the repository list:

| Key | Action |
| --- | --- |
| `/` | Start filtering |
| `esc` | Cancel the filter input, then clear the filter and return to the full list |
| `space` | Toggle selection |
| `enter` | Confirm selection |
| `ctrl+c` | Quit the program |

### Repository Cache

The repository list is cached under your OS cache directory (`~/Library/Caches/rel` on macOS) for 30 minutes.

```bash
rel release --refresh   # bypass the cache and re-fetch
rel sync --refresh      # same for sync
rel cache status        # show cache age and repo count
rel cache clear         # delete the cached list
```

### Sync Branches

To sync `master` commits back to `dev`:

```bash
rel sync
```
