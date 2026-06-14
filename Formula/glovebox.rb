class Glovebox < Formula
  desc "Isolated multi-agent AI coding harness with proxy egress control"
  homepage "https://github.com/okulik/glovebox"
  license "AGPL-3.0-or-later"

  head "https://github.com/okulik/glovebox.git", branch: "main"

  url "https://github.com/okulik/glovebox/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "0019dfc4b32d63c1392aa264aed2253c1e0c2fb09216f8e2cc269bbfb8bb49b5"
  version "0.1.0"

  depends_on "go" => :build
  depends_on "go-md2man" => :build

  def install
    # Build the host gbx binary from cmd/gbx, then install the read-only
    # package files into libexec. The Go source (go.mod / go.sum / cmd /
    # internal / version.go / version.txt) is also shipped so that the
    # agent Dockerfile, which uses ${GBX_LIBEXEC} as its build context and
    # compiles cmd/gbxa for the container, has the inputs it needs at
    # `gbx new` time.
    system "go", "build", "-o", "bin/gbx", "./cmd/gbx"
    libexec.install "bin", "docker", "defaults", ".env.example",
                    "go.mod", "go.sum",
                    "cmd", "internal",
                    "version.go", "version.txt"
    # Write_env_script must target a specific file path. Calling it on `bin`
    # directly would write the wrapper to <keg>/bin as a single file (not a
    # directory), making the link step a silent no-op.
    (bin/"gbx").write_env_script libexec/"bin/gbx", GBX_LIBEXEC: libexec.to_s

    # Generate the man page from docs/gbx.1.md (the source-of-truth Markdown).
    # share/man/man1/gbx.1 is a build artifact and isn't checked into git;
    # go-md2man is declared as a brew build dep above so it's on PATH here.
    mkdir_p "share/man/man1"
    system "go-md2man", "-in", "docs/gbx.1.md", "-out", "share/man/man1/gbx.1"
    man1.install "share/man/man1/gbx.1"
  end

  def caveats
    <<~EOS
      The CLI is `gbx`:
        gbx --help
        man gbx

      Config and per-project state live in ~/.config/glovebox/
      (override with GBX_CONFIG_DIR=/path/to/dir).

      Requires Docker or OrbStack to be running.
    EOS
  end

  test do
    assert_match "gbx", shell_output("#{bin}/gbx --help")
  end
end
