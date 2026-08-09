# CANDIDATE FORMULA FOR homebrew-core — not used by the tap.
#
# The tap formula (apispec.rb) installs the pre-built release binary. homebrew-core
# does not accept that: core formulae build from source, so this one compiles with
# Go from the release tarball. Keep both; they serve different places.
#
# Submitting this is what puts apispec in the formulae.brew.sh search — a tap is
# never indexed there. See README.md, "Getting into homebrew-core".
#
# Verified locally before committing:
#   brew install --build-from-source <tap>/apispec   → builds, binary reports 0.5.6
#   brew test <tap>/apispec                          → passes
#   brew audit --new --strict --formula <tap>/apispec → clean
#
# Bump `url` and `sha256` to the release being submitted:
#   curl -sL https://github.com/ehabterra/apispec/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256
class Apispec < Formula
  desc "Generate OpenAPI 3.1 specs from Go source by static analysis"
  homepage "https://github.com/ehabterra/apispec"
  url "https://github.com/ehabterra/apispec/archive/refs/tags/v0.5.6.tar.gz"
  sha256 "8f911576f3284988af11708fcb7c6e3a6e4742a1e39e32b27bdc181aec05da61"
  license "Apache-2.0"
  head "https://github.com/ehabterra/apispec.git", branch: "main"

  depends_on "go" => :build

  def install
    # A core formula builds from a release tarball, which carries no git data, so
    # only the values that are knowable are injected. main.go falls back to
    # runtime/debug build info for the rest.
    ldflags = %W[
      -s -w
      -X main.Version=#{version}
      -X main.BuildDate=#{time.iso8601}
    ]
    system "go", "build", *std_go_args(ldflags: ldflags), "./cmd/apispec"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/apispec --version")

    (testpath/"go.mod").write "module example.com/t\n\ngo 1.21\n"
    (testpath/"main.go").write <<~GO
      package main

      import "net/http"

      func main() {
        http.HandleFunc("/things", func(w http.ResponseWriter, r *http.Request) {})
        _ = http.ListenAndServe(":8080", nil)
      }
    GO
    system bin/"apispec", "--dir", testpath, "--output", testpath/"openapi.yaml"
    assert_match "/things", (testpath/"openapi.yaml").read
  end
end
