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

1. Create a **public** repo `ehabterra/homebrew-tap` (any description; no files needed).
2. Create a fine-grained PAT with **Contents: read and write** on that repo only.
3. Add it to *this* repo as the secret `HOMEBREW_TAP_TOKEN`.

Until the secret exists the release workflow skips the step, so releases keep
working — a fork will never fail here either.

## Seeding the tap by hand

Only needed once, or to fix the tap without cutting a release:

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

## Why not homebrew-core?

homebrew-core has notability requirements (roughly: a meaningful number of
stars/forks/watchers, or demonstrable wide use). A tap has none and can be
submitted to core later without changing the install command for anyone who
already used the tap.
