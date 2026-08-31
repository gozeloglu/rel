# rel

[![CI](https://github.com/gozeloglu/rel/actions/workflows/ci.yml/badge.svg)](https://github.com/gozeloglu/rel/actions/workflows/ci.yml)
[![Release](https://github.com/gozeloglu/rel/actions/workflows/release.yml/badge.svg)](https://github.com/gozeloglu/rel/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/gozeloglu/rel.svg)](https://pkg.go.dev/github.com/gozeloglu/rel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`rel` is a fast, interactive TUI (Terminal User Interface) CLI tool that automates periodic GitHub release processes for **any** organization, team or personal account.

It helps you quickly select repositories, bump versions, check branch synchronization, create release branches, and automatically open pull requests.

## Features

- **`rel init`**: One-time setup wizard. Asks for your organization (or username), an optional team, repository filters, and detects your branch names from a real repository.
- **`rel release`**: Interactive release workflow. Fetches repositories, calculates the next minor version, validates that the release branch is not ahead of the development branch, creates a `release/x.y.z` branch, opens a PR, and writes a markdown file with release notes.
- **`rel sync`**: Post-release utility. Checks whether the release branch is ahead of the development branch and opens sync PRs to keep them up to date. `--auto` detects recently released repositories for you, so bulk deploys don't need hand-picking.
- **`rel profile`**: Switch between multiple setups (for example your company team and your personal account) without editing any files.
- **Repository cache**: The repository list is cached locally for 30 minutes, per profile, so repeated runs don't re-fetch it from GitHub every time.

## Prerequisites

A GitHub Personal Access Token with the `repo` scope. Add `read:org` as well if you want to scope repositories to a team.

## Installation

### Homebrew (recommended)

```bash
brew install gozeloglu/tap/rel
```

This installs a prebuilt binary for macOS and Linux (Intel and Apple Silicon / arm64) and sets up shell completions automatically. Upgrading later is just `brew upgrade rel`.

### Go

If you already have Go installed:

```bash
go install github.com/gozeloglu/rel@latest
```

### Prebuilt binaries

Download an archive for your platform from the [releases page](https://github.com/gozeloglu/rel/releases), then:

```bash
tar -xzf rel_<version>_<os>_<arch>.tar.gz
sudo mv rel /usr/local/bin/
```

On macOS, a manually downloaded binary is quarantined by Gatekeeper. If you see "rel is damaged and cannot be opened", clear the attribute:

```bash
xattr -d com.apple.quarantine /usr/local/bin/rel
```

(The Homebrew installation does this for you.)

### From source

```bash
git clone https://github.com/gozeloglu/rel.git
cd rel
go build -o rel
```

Check what you installed with:

```bash
rel version
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

### Auto Sync

After a bulk deploy, picking every affected repository by hand is slow and easy to get wrong. `--auto` finds them for you:

```bash
rel sync --auto                 # repositories released in the last 2 hours
rel sync --auto --since 6h      # widen the window
rel sync --auto --dry-run       # report only, never opens a pull request
rel sync --auto --yes           # skip the confirmation screen
```

A repository is picked up when **both** are true:

1. its base branch is ahead of its development branch, and
2. it published a release (or pushed a tag) inside the window.

The release is used as the deploy signal because it is what actually moves the base branch, and it needs no assumptions about how your deployment pipeline is wired.

Everything else is reported and skipped, grouped by reason: already in sync, no base/dev branch, never released, released too long ago, or already has an open sync PR. That last check makes repeated runs safe — auto sync never stacks duplicate pull requests.

Detected repositories are shown in a confirmation screen with every entry pre-selected, so you only have to uncheck what you want to leave out. Pass `--yes` for unattended runs.

> `--since`, `--yes` and `--dry-run` only apply together with `--auto`.

#### The scan report

Every auto run prints a full picture of the fleet, so nothing is hidden behind a count:

```
Scan · acme/payments · master → dev · window 2h
──────────────────────────────────────────────────────────────────────────

  ✔  READY TO SYNC · 1
     payment-alpha           3 commits  diverged 4h ago    v1.2.0 · 12m

  ⏭  SYNC PR ALREADY OPEN · 2
     payment-service         4 commits  diverged 180d ago  v1.76.0 · 180d
       └ https://github.com/acme/payment-service/pull/20
     payment-payout-service  2 commits  diverged 101d ago  v1.8.0 · 89d

  ⏳  OUT OF SYNC · RELEASED BEFORE WINDOW · 15
     courier-payback-cron    5 commits  diverged 6y3mo ago  v0.0.0 · 6y3mo
     ...

  ⚠  OUT OF SYNC · NEVER RELEASED · 8
     payment-webhook-service  4 commits  diverged 1y2mo ago  never released
     ...

  ✓  ALREADY IN SYNC · 55

  −  NOT APPLICABLE · no 'master' or 'dev' branch · 6
     payment-load-tests, payment-jira-cases, ...

  waiting  = commits on master not yet in dev
  diverged = age of the newest commit both branches share
  scanned 86 repositories
```

Groups that need attention get a table; the rest collapse into a name list. Stale groups are sorted longest-forgotten first, so the repositories most likely to have been missed float to the top. All of this comes from the comparison call auto sync already makes, so the report costs no extra API requests.

### Opening pull requests in your browser

After `rel release` or `rel sync` creates pull requests, the list is printed with each repository next to its URL:

```
Created 3 pull requests
   payment-alpha   https://github.com/acme/payment-alpha/pull/91
   payment-beta    https://github.com/acme/payment-beta/pull/44
```

Pass `--open` to launch them all straight away:

```bash
rel sync --auto --open
rel release --open
```

Without the flag, `rel` asks once whether to open them, defaulting to no. Piped and CI runs are never prompted.

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

If you installed with Homebrew, completions are already set up — skip to the table below.

Otherwise, the quickest way to set it up:

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

## Contributing

```bash
go test ./...
go vet ./...
gofmt -l cmd pkg main.go
```

To validate the release pipeline without publishing anything:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

### Cutting a release of rel itself

Note the distinction: `rel release` releases *your* repositories, while `rel`'s own version is cut with a git tag.

```bash
git tag v0.2.0
git push origin v0.2.0
```

The tag triggers [`release.yml`](.github/workflows/release.yml), which builds every platform, publishes a GitHub release and updates the [Homebrew tap](https://github.com/gozeloglu/homebrew-tap).

## License

[MIT](LICENSE)
