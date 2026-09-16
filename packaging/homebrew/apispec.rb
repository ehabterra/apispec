# Homebrew formula for apispec: installs the pre-built release binary, so
# `brew install` compiles nothing. Go is still needed at RUNTIME (below):
# apispec shells out to `go list` and cannot analyse anything without it.
#
# packaging/homebrew/apispec.rb.tmpl is the SOURCE; apispec.rb is it rendered at
# the current release. .github/workflows/release.yml renders the template with
# the checksums of the artifacts it just built and pushes the result to the tap
# (ehabterra/homebrew-tap, Formula/apispec.rb). TestHomebrewFormulaMatchesTemplate
# keeps the two from drifting.
# ANY go on PATH satisfies apispec, so this is a requirement and not
# `depends_on "go"`.
#
# A formula dependency can only ever be satisfied by Homebrew's own keg. So
# `brew install apispec` downloaded the newest Go — hundreds of megabytes — onto
# machines that already had a working toolchain from go.dev, gvm, asdf or a
# distro package, and then did not even use it: apispec shells out to whatever
# `go` PATH resolves to, which on those machines is still the one that was
# already there. The keg was pure download.
#
# `satisfy` checks for that binary instead, and `fatal` means a machine with no
# Go at all is told so up front rather than discovering it on the first run.
#
# apispecui.rb declares this class too, WORD FOR WORD. A tap loads its formulae
# into one Ruby process, so the second definition reopens the first class rather
# than defining another — keeping them identical is what makes that a no-op, and
# is why the text below names no tool.
class GoRequirement < Requirement
  fatal true

  # build_env: false — the check runs in the user's own environment, so a Go
  # installed outside Homebrew's PATH is seen.
  satisfy(build_env: false) { which("go") }

  def display_s
    "go (any installation on PATH)"
  end

  def message
    <<~EOS
      This tool needs the Go toolchain on PATH at RUNTIME: it analyses a project
      by loading its packages through go/packages, which shells out to `go list`.
      Without it every run exits with "go command required, not found".

      Any Go installation will do. If you have none:
        brew install go
      or install one from https://go.dev/dl/
    EOS
  end
end

class Apispec < Formula
  desc "Generate OpenAPI 3.1 specs from Go source by static analysis"
  homepage "https://github.com/ehabterra/apispec"
  license "Apache-2.0"

  depends_on GoRequirement

  on_macos do
    on_arm do
      url "https://github.com/ehabterra/apispec/releases/download/v0.5.6/apispec-darwin-arm64"
      sha256 "04f9e3a6abc957bc8300c0f0225084c68f609cb1225d751e5d81a373c870414c"
    end
    on_intel do
      url "https://github.com/ehabterra/apispec/releases/download/v0.5.6/apispec-darwin-amd64"
      sha256 "245ff783ee542f077dd944eb79f9cdb8e3206795970ef37493d29d308b301f80"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/ehabterra/apispec/releases/download/v0.5.6/apispec-linux-arm64"
      sha256 "31af9982727b85f6ab60dbf178d00dca03be6a6763527e63c9a29eea34556bfc"
    end
    on_intel do
      url "https://github.com/ehabterra/apispec/releases/download/v0.5.6/apispec-linux-amd64"
      sha256 "f7ad52b8d1e19ca9499e41d1d5394a5ea24649dd4f0658daac28e1e60c5dd1c2"
    end
  end

  def install
    # The download keeps its asset name; install it under the short one.
    bin.install Dir["apispec-*"].first => "apispec"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/apispec --version")
  end
end
