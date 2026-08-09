# Homebrew packaging

`brew install ehabterra/tap/apispec` installs the **pre-built release binary**,
so it needs no Go toolchain and takes seconds.

## Files here

| file | role |
|---|---|
| `apispec.rb` | the current formula, readable in this repo — a copy of what the tap serves |
| `apispec.rb.tmpl` | the same formula with `__VERSION__` / `__SHA_*__` placeholders |

The formula users install from lives in a separate repository, because Homebrew
requires a tap repo to be named `homebrew-<name>`:

    ehabterra/homebrew-tap
    └── Formula/
        └── apispec.rb        → brew install ehabterra/tap/apispec

## Why one tap, not one per tool

A tap is a repo with a `Formula/` directory, and it holds **any number** of
formulae. Adding a second tool means adding a second file, not a second repo:

    ehabterra/homebrew-tap
    └── Formula/
        ├── apispec.rb        → brew install ehabterra/tap/apispec
        └── apidiag.rb        → brew install ehabterra/tap/apidiag

A repo per tool (`homebrew-apispec`, `homebrew-apidiag`) would mean one PAT,
one secret and one workflow step each, users tapping every one separately, and
an install string that stutters — `brew install ehabterra/apispec/apispec`,
because the tap name sits between the owner and the formula. The single-tap
layout is what goreleaser, hashicorp and charmbracelet use.

A tool that needs its own release cadence can still get its own tap later
without changing the command for anyone already on this one.

`.github/workflows/release.yml` renders the template and pushes it there on every
tag. The checksums are computed from the artifacts that job just built, so the
formula cannot point at a digest the release does not have.

## One-time setup

The tap repo already exists and is seeded. What is left is the token that lets
the release workflow push to it.

### 1. Create the token

Go to **<https://github.com/settings/personal-access-tokens/new>**
(GitHub → your avatar → Settings → Developer settings → Personal access tokens →
Fine-grained tokens → *Generate new token*), then set:

| field | value |
|---|---|
| Token name | `apispec-homebrew-tap` |
| Expiration | your call — a dated token is a dead release step on the day it expires; 1 year with a calendar reminder is a reasonable trade |
| Resource owner | `ehabterra` |
| Repository access | **Only select repositories** → `ehabterra/homebrew-tap` |
| Permissions → Repository permissions → **Contents** | **Read and write** |

`Metadata: Read-only` is added automatically and is required — leave it.

Grant nothing else. This token only needs to commit one file to one repo; it
should not be able to touch `apispec` itself.

Click **Generate token** and copy it. GitHub shows it once.

### 2. Add it to the apispec repo

Paste it into
**<https://github.com/ehabterra/apispec/settings/secrets/actions/new>**
with the name `HOMEBREW_TAP_TOKEN`.

Or from a terminal, which avoids the clipboard:

```bash
gh secret set HOMEBREW_TAP_TOKEN --repo ehabterra/apispec
# paste the token at the prompt, then press Ctrl-D
```

Verify it registered (this prints the name and date, never the value):

```bash
gh secret list --repo ehabterra/apispec
```

### 3. Nothing else

The next tag pushes an updated `Formula/apispec.rb` automatically. Until the
secret exists the step is skipped, so releases and forks keep working.

## Seeding the tap by hand

The tap is already seeded with 0.5.6. This is how to re-seed it, or to fix it
without cutting a release:

```bash
git clone https://github.com/ehabterra/homebrew-tap.git
mkdir -p homebrew-tap/Formula
cp packaging/homebrew/apispec.rb homebrew-tap/Formula/apispec.rb
cd homebrew-tap && git add Formula/apispec.rb && git commit -m "apispec 0.5.6" && git push
```

## Verifying a change

```bash
brew install --build-from-source ./packaging/homebrew/apispec.rb
brew test apispec
brew audit --strict --formula ./packaging/homebrew/apispec.rb
```

`brew audit` is worth running before a release: it catches a stale `version`,
an unreachable `url`, and a `sha256` that does not match what the URL serves.

## Getting into homebrew-core (the formulae.brew.sh search)

<https://formulae.brew.sh/formula/> indexes **homebrew-core only**. A tap is never
listed there, however it is configured — so appearing in that search means getting
the formula accepted into core.

### Where apispec stands

| requirement | status |
|---|---|
| Notable enough — Homebrew's heuristic is 75 stars **or** 30 forks **or** 30 watchers | **83 stars** — clears it |
| Open-source with an OSI licence | Apache-2.0 |
| Stable, versioned releases | v0.5.6, tagged and signed |
| Maintained, not a duplicate of an existing formula | yes |
| **Builds from source** | this is the work — see below |

### The one blocking difference

core formulae **build from source**; the tap formula downloads a pre-built
binary, which core does not accept. `apispec-homebrew-core.rb` in this directory
is the source-building version, verified locally:

```bash
brew install --build-from-source <tap>/apispec    # compiles with Go, binary reports its version
brew test <tap>/apispec                           # passes
brew audit --new --strict --formula <tap>/apispec # clean
```

### Submitting (once)

This is a one-time submission — see [Do I resubmit every release?](#do-i-resubmit-every-release).

```bash
brew tap --force homebrew/core
cd "$(brew --repository homebrew/core)"
git checkout -b apispec

cp /path/to/apispec/packaging/homebrew/apispec-homebrew-core.rb Formula/a/apispec.rb
# update url + sha256 to the release you are submitting:
#   curl -sL https://github.com/ehabterra/apispec/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256

brew audit --new --strict --online --formula apispec

# HOMEBREW_NO_INSTALL_FROM_API=1 is REQUIRED when the formula is in your local
# homebrew/core checkout — without it brew ignores your file and uses the API.
HOMEBREW_NO_INSTALL_FROM_API=1 brew install --build-from-source apispec
brew test apispec

gh repo fork Homebrew/homebrew-core --remote
git commit -am "apispec 0.5.6 (new formula)"     # the REAL version, not a placeholder
git push -u origin apispec
gh pr create --repo Homebrew/homebrew-core
```

`--online` matters: it checks the URL resolves and the checksum matches, which
the offline audit cannot.

`HOMEBREW_NO_INSTALL_FROM_API=1` matters more. Homebrew installs from its API by
default, and its own FAQ is blunt about the consequence:

> if you are editing a core formula or cask you must set
> `HOMEBREW_NO_INSTALL_FROM_API=1` before using `brew install` or `brew update`
> otherwise they will ignore your local changes and default to the API

Without it the install step quietly tests something other than the file you are
about to submit — the worst kind of green. It is only needed for a formula inside
the **homebrew/core** checkout; a third-party tap like `ehabterra/tap` is always
read from disk, which is why the tap instructions above do not use it.

**The commit message is a convention, not a formality.** homebrew-core expects
`<formula> <version> (new formula)` with the real version, and it becomes the PR
title:

    apispec 0.5.6 (new formula)      ✅
    apispec X.Y.Z (new formula)      ❌ placeholder left in
    Add apispec formula              ❌ wrong shape

Recent core merges look exactly like this — `oxvg 0.0.7 (new formula)`,
`network-doctor 1.10.7 (new formula)`. Later version bumps drop the suffix and
are just `apispec 0.5.7`, which is what BrewTestBot will open on your behalf.

### Do I resubmit every release?

**No.** The sequence above is for the **initial** new-formula submission, once.
After that core keeps itself up to date:

- **Autobump is the default.** `Formula#autobump?` is true unless a formula opts
  out with `no_autobump!` (`Library/Homebrew/formula.rb`), so apispec would be on
  the autobump list automatically. Homebrew's scheduled job notices a new
  upstream release and BrewTestBot opens the version-bump PR itself.
- **Detection already works with no `livecheck` block** — the default strategy
  reads tags from the GitHub URL. Checked against the candidate formula:
  `brew livecheck` reports `apispec: 0.5.6 ==> 0.5.6`, i.e. it found the current
  release and agreed with it. When 0.5.7 is tagged that becomes
  `0.5.6 ==> 0.5.7`, and the bump PR follows.
- **To push a release in yourself** — a bot outage, or wanting it in core the
  same day — it is one command, not the fork sequence:

  ```bash
  brew bump-formula-pr --version=0.5.7 apispec
  ```

  It forks, branches, updates `url` and `sha256`, commits and opens the PR.

So the ongoing cost of being in core is close to zero: the one-time submission,
plus reviewing the occasional bot PR when something breaks.

### What changes if it is accepted

- apispec appears in the formulae.brew.sh search, and `brew install apispec`
  works with no tap
- **Homebrew maintainers own the formula.** Their bots open version-bump PRs when
  they notice a new tag; you no longer control when an update ships, and a
  breaking change to flags is their problem to discover
- the tap keeps working, so anyone already on `ehabterra/tap/apispec` is
  unaffected. Keeping both is normal: the tap can ship a release the same day,
  core lands when it lands

Nothing about the tap has to change to try this, and a rejection costs only the
PR.
