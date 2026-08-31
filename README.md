# rel

`rel` is a fast, interactive TUI (Terminal User Interface) CLI tool that automates periodic GitHub release processes for **any** organization, team or personal account.

It helps you quickly select repositories, bump versions, check branch synchronization, create release branches, and automatically open pull requests.

## Features

- **`rel init`**: One-time setup wizard. Asks for your organization (or username), an optional team, repository filters, and detects your branch names from a real repository.
- **`rel release`**: Interactive release workflow. Fetches repositories, calculates the next minor version, validates that the release branch is not ahead of the development branch, creates a `release/x.y.z` branch, opens a PR, and writes a markdown file with release notes.
- **`rel sync`**: Post-release utility. Checks whether the release branch is ahead of the development branch and opens sync PRs to keep them up to date.
- **`rel profile`**: Switch between multiple setups (for example your company team and your personal account) without editing any files.
- **Repository cache**: The repository list is cached locally for 30 minutes, per profile, so repeated runs don't re-fetch it from GitHub every time.

## Prerequisites

- [Go](https://golang.org/doc/install) (1.20+)
- A GitHub Personal Access Token with the `repo` scope. Add `read:org` as well if you want to scope repositories to a team.

## Build / Installation

```bash
git clone https://github.com/gozeloglu/rel.git
cd rel
go build -o rel
```

(Optional) Move the `rel` binary to your system's `$PATH` for global access:

```bash
mv rel /usr/local/bin/
```

## Getting Started

Authenticate via the `GH_TOKEN` environment variable:

```bash
export GH_TOKEN="your_github_token_here"
```

Then run the setup wizard:

```bash
rel init
```

The wizard asks for:

1. **Organization or username** — validated against the API, which also detects whether it is an organization or a personal account.
2. **Team slug** (organizations only, optional) — limits the repository list to one team. Leave it empty to use every repository of the owner.
3. **Repository filters** (optional) — comma separated globs, e.g. include `payment-*`, exclude `*-manifests`.
4. **Branch names** — the wizard inspects one of your repositories and suggests the default branch as the release branch, plus `dev`/`develop` as the development branch when they exist.
5. **Profile name** — used to switch between setups later.

## Usage

### Start a Release

```bash
rel release
```

Select the repositories you want to release, confirm or edit the version numbers, and the tool handles branching and PR creation.

Key bindings in the repository list:

| Key | Action |
| --- | --- |
| `↑`/`↓` or `k`/`j` | Move the cursor |
| `space` | Toggle the highlighted repository |
| `ctrl+a` / `a` | Toggle every currently visible repository |
| `/` | Start filtering — what you type is shown live on the `Filter:` line and matches are highlighted |
| `tab` | Toggle a repository without leaving the filter input |
| `ctrl+u` | Clear the filter input while typing |
| `esc` | Stop typing the filter; press again to clear the filter and return to the full list |
| `enter` | Apply the filter while typing, otherwise confirm the selection |
| `ctrl+c` / `q` | Quit |

### Sync Branches

```bash
rel sync
```

Opens `chore: <base> to <dev> sync` pull requests wherever the release branch is ahead. Profiles that use a single branch for both sides skip this step automatically.

### Profiles

```bash
rel profile                # interactive picker: enter use · n new · e edit · d delete
rel profile list           # list profiles, '*' marks the active one
rel profile use <name>     # switch without the TUI
rel profile delete <name>  # remove a profile
rel release --profile <name>   # one-off override for a single run
```

Profiles are stored as JSON in your OS config directory (`~/.config/rel/config.json` on Linux, `~/Library/Application Support/rel/config.json` on macOS):

```json
{
  "current_profile": "acme-payments",
  "profiles": [
    {
      "name": "acme-payments",
      "owner": "acme",
      "owner_type": "org",
      "team": "payments",
      "base_branch": "master",
      "dev_branch": "dev",
      "include": ["payment-*"],
      "exclude": ["*-manifests"]
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `owner` / `owner_type` | GitHub organization or user login, and which one it is |
| `team` | Optional team slug; omit it to use every repository of the owner |
| `base_branch` | Branch that release PRs target |
| `dev_branch` | Branch that release branches are cut from; set it to the same value as `base_branch` for a single branch workflow |
| `include` / `exclude` | Case-insensitive globs applied to repository names; an empty `include` means "everything" and `exclude` always wins |

### Repository Cache

The repository list is cached per profile for 30 minutes and is invalidated automatically when the owner, team or filters change.

```bash
rel release --refresh   # bypass the cache and re-fetch
rel sync --refresh      # same for sync
rel cache status        # show cache age and repo count for the active profile
rel cache clear         # delete the cached list of the active profile
rel cache clear --all   # delete every cached list
```

### Shell Completion

`rel` completes commands, flags and — most usefully — your profile names.

The quickest way to set it up:

```bash
rel completion install          # detects $SHELL and writes the script
rel completion install --shell zsh
rel completion install --print  # only show where the script would go
```

To install it manually, or to inspect the script first:

```bash
rel completion bash > ~/.local/share/bash-completion/completions/rel
rel completion zsh  > "${fpath[1]}/_rel"
rel completion fish > ~/.config/fish/completions/rel.fish
rel completion powershell | Out-String | Invoke-Expression
```

Restart your shell afterwards. What gets completed:

| Input | Suggestion |
| --- | --- |
| `rel profile use <TAB>` | profile names, with the active one marked and a summary of each |
| `rel profile delete <TAB>` | the same list |
| `rel release --profile <TAB>` | the same list (also for `rel sync`) |
| `rel <TAB>` | subcommands and their descriptions |
| `rel release <TAB>` | flags only — no stray file name suggestions |

Completion never calls the GitHub API; it reads only the local config file, so it works offline and without a token.
