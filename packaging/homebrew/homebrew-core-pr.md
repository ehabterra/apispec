# homebrew-core PR body — apispec (new formula)

Copy everything below the line into the PR body at
<https://github.com/Homebrew/homebrew-core/compare>.

**The boxes are deliberately unticked.** The template says *"Do not tick a
checkbox if you haven't performed its action. Honesty is indispensable for a
smooth review process."* Tick each one yourself, in your own checkout, after
running the command — see [Before you tick](#before-you-tick).

---

## First: strip the header

`apispec-homebrew-core.rb` starts with ~20 lines of comments written for THIS
repo — "not used by the tap", "see README.md", "verified from a scratch tap".
They must not go to homebrew-core: they reference a tap and a README that do not
exist there, and they read as notes-to-self in a file maintainers are reviewing.

Keep the file from `class Apispec < Formula` down:

```bash
sed -i '' '/^class Apispec/,$!d' Formula/a/apispec.rb   # macOS
git commit --amend -a --no-edit && git push -f
```

The stripped file has been built and audited as-is: `brew audit --new --strict
--online` clean, `HOMEBREW_NO_INSTALL_FROM_API=1 brew install
--build-from-source` builds in ~4s, `brew test` passes, binary reports 0.5.6.

## Before you tick

Run these in the `homebrew/core` checkout, not from a scratch tap. The
`HOMEBREW_NO_INSTALL_FROM_API=1` is required: without it brew ignores your local
file and installs from the API, so a green run proves nothing.

```bash
brew tap --force homebrew/core
cd "$(brew --repository homebrew/core)"
git checkout -b apispec
cp /path/to/apispec/packaging/homebrew/apispec-homebrew-core.rb Formula/a/apispec.rb

# update url + sha256 to the version you are submitting:
#   curl -sL https://github.com/ehabterra/apispec/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256

HOMEBREW_NO_INSTALL_FROM_API=1 brew install --build-from-source apispec   # box 4
brew test apispec                                                        # box 5
brew audit --strict apispec                                              # box 6
brew audit --new --strict --online apispec                               # box 6 (new formula)
```

What has already been checked, and when — re-verify anything time-sensitive
before you submit:

| box | status |
|---|---|
| 3 — no other open PR for this formula | verified: 0 open PRs mentioning apispec in homebrew-core, and no `Formula/a/apispec.rb` exists. Re-check on the day you submit |
| 4, 5, 6 | the **header-stripped** formula passed `--build-from-source`, `brew test` and `brew audit --new --strict --online` — but from a scratch tap, which is always read from disk. That is not the run the boxes ask about. Run them again in the core checkout, with `HOMEBREW_NO_INSTALL_FROM_API=1` |
| commit message | `apispec 0.5.6 (new formula)` on the branch matches the core convention, and carries no AI co-author trailer — both verified |

## About the AI checkbox

This formula was written with AI assistance, so the honest options are:

- **tick it** — permitted by the "…**or** I disclosed the tool/model below and
  reviewed its output" clause, provided all of the following are true for you:
  you have read and understood the formula, no commit is attributed to an AI, and
  you will answer maintainer questions yourself; or
- **leave it unticked** and let the maintainers ask.

Disclosure text is filled in below. Two things to get right:

1. **Do not carry an AI co-author trailer into the homebrew-core commit.** Commits
   in the apispec repo carry `Co-Authored-By: Claude …`; the homebrew-core commit
   is a new one you author, and must not. Check with `git log -1` before pushing.
2. **Non-maintainers may have only one AI-assisted PR open at a time** — see
   <https://docs.brew.sh/Responsible-AI-Usage>.

---

<!-- Do not tick a checkbox if you haven't performed its action. Honesty is indispensable for a smooth review process. -->
<!-- Use [x] to mark item done before creation, or just click the checkboxes with device pointer after creation -->
<!-- In the following questions `<formula>` is the name of the formula you're editing. -->

- [ ] Have you followed the [guidelines for contributing](https://github.com/Homebrew/homebrew-core/blob/HEAD/CONTRIBUTING.md)?
- [ ] Have you ensured that your commits follow the [commit style guide](https://docs.brew.sh/Formula-Cookbook#commit)?
- [ ] Have you checked that there aren't other open [pull requests](https://github.com/Homebrew/homebrew-core/pulls) for the same formula update/change?
- [ ] Have you built your formula locally with `HOMEBREW_NO_INSTALL_FROM_API=1 brew install --build-from-source apispec`?
- [ ] Is your test running fine `brew test apispec`?
- [ ] Does your build pass `brew audit --strict apispec` (after doing `HOMEBREW_NO_INSTALL_FROM_API=1 brew install --build-from-source apispec`)? If this is a new formula, does it pass `brew audit --new apispec`?

-----

- [ ] I did not use AI/LLM to create this PR, or I disclosed the tool/model below and reviewed its output; I did not attribute commits to AI and will answer maintainer questions and review comments myself without AI/LLM.

<!-- If AI was used, explain below how it was used and how you verified the changes. Non-maintainers may only have one AI-assisted PR open at a time. See https://docs.brew.sh/Responsible-AI-Usage for guidance. -->

**AI disclosure.** The formula was drafted with Claude (Anthropic), then reviewed
and verified by me. It is a short, conventional Go formula: `url` on the release
tarball, `depends_on "go" => :build`, `std_go_args` with version ldflags, and a
test that runs `--version` and generates a spec from a throwaway module. I have
read every line, built and tested it locally with the commands above, and will
handle review comments myself.

-----

**apispec** generates OpenAPI 3.1 specifications from Go source by static
analysis — it reads route registrations, handlers and types for gin, echo, chi,
fiber, gorilla/mux and net/http, and emits a spec without annotations or a
running server.

- Homepage / source: <https://github.com/ehabterra/apispec>
- Licence: Apache-2.0
- Stable, signed releases; no dependencies beyond Go at build time
