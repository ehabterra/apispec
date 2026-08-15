# Homebrew formula for apispec: installs the pre-built release binary, so
# `brew install` compiles nothing. Go is still a RUNTIME dependency (below):
# apispec shells out to `go list` and cannot analyse anything without it.
#
# packaging/homebrew/apispec.rb.tmpl is the SOURCE; apispec.rb is it rendered at
# the current release. .github/workflows/release.yml renders the template with
# the checksums of the artifacts it just built and pushes the result to the tap
# (ehabterra/homebrew-tap, Formula/apispec.rb). TestHomebrewFormulaMatchesTemplate
# keeps the two from drifting.
class Apispec < Formula
  desc "Generate OpenAPI 3.1 specs from Go source by static analysis"
  homepage "https://github.com/ehabterra/apispec"
  license "Apache-2.0"

  # apispec analyses a project by loading its packages through go/packages, which
  # shells out to `go list`. The pre-built binary still needs the go toolchain on
  # PATH at RUNTIME — without it every run exits with "go command required".
  depends_on "go"

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
