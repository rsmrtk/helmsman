# Homebrew formula template for a tap (a repository named rsmrtk/homebrew-tap).
#
# Put this file in that repository as Formula/helmsman.rb, replace the url,
# version and sha256 with the values of a published release (they are listed in
# checksums.sha256 on the Releases page), and people can install with:
#
#   brew install rsmrtk/tap/helmsman
class Helmsman < Formula
  desc "Terminal UI for Kubernetes that shows what is wrong at a glance"
  homepage "https://github.com/rsmrtk/helmsman"
  version "0.1.0"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/rsmrtk/helmsman/releases/download/v#{version}/helmsman_Darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_OF_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/rsmrtk/helmsman/releases/download/v#{version}/helmsman_Darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_OF_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/rsmrtk/helmsman/releases/download/v#{version}/helmsman_Linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_OF_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/rsmrtk/helmsman/releases/download/v#{version}/helmsman_Linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_OF_LINUX_AMD64"
    end
  end

  def install
    bin.install "helmsman"
  end

  test do
    assert_match "Version", shell_output("#{bin}/helmsman version")
  end
end
