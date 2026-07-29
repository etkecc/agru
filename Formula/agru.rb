class Agru < Formula
  desc "Fast, drop-in ansible-galaxy replacement for updating role requirements"
  homepage "https://github.com/etkecc/agru"
  license "AGPL-3.0-only"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/etkecc/agru/releases/download/v0.2.1/agru_Darwin_arm64.tar.gz"
      sha256 "d3ee5398de8c8c43456cad3c294c3120adb72a0308092b0dfdc12dc4093eb481"
    else
      url "https://github.com/etkecc/agru/releases/download/v0.2.1/agru_Darwin_x86_64.tar.gz"
      sha256 "150510256474329cf97666c05b2fad13bd74e657a410d2dae01929f0f3dce649"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/etkecc/agru/releases/download/v0.2.1/agru_Linux_arm64.tar.gz"
      sha256 "cc9c61e43943e6117da6cd5fb0b5ccc493e299153d94758810902ce4c1ca0fcb"
    else
      url "https://github.com/etkecc/agru/releases/download/v0.2.1/agru_Linux_x86_64.tar.gz"
      sha256 "6b8f9cdefe0ca981cc73e344384cbc380a68fd88a42ca7aba7517b2392f9ad42"
    end
  end

  def install
    bin.install "agru"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/agru -v")
  end
end
