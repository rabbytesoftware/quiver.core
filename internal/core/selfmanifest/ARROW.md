# Quiver Core

<p align="center">
  <img src="https://raw.githubusercontent.com/rabbytesoftware/quiver.core/develop/.github/quiver.svg" alt="Quiver" width="450" />
  <br/>
  <em>The engine running quietly underneath everything else.</em>
</p>

This is Quiver's own engine: the background service that actually installs, runs, and manages every application in your library, including the desktop app you're browsing this catalog with. It's listed here so it can report its own version alongside everyone else's, the same way it tracks updates for anything else on your machine.

There's nothing to install here: it's already running, since it's what's showing you this page.

```arrow
schema: "arrow@v0"

metadata:
  name: "Quiver Core"
  description: "The Quiver package manager's own engine daemon."
  version: "0.1"
  license: "GPL-3.0"
  url: "https://github.com/rabbytesoftware/quiver.core"
  media:
    icon: "https://raw.githubusercontent.com/rabbytesoftware/quiver.core/develop/docs/quiver-icon.svg"
    banner: "https://raw.githubusercontent.com/rabbytesoftware/quiver.core/develop/docs/quiver-banner.svg"
  maintainers:
    - name: "Rabbyte Software"
      url: "https://char2cs.net"
  credits:
    - name: "Rabbyte Software"

targets:
  "*":
    requirements:
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 1
    lifecycle:
      update:
        - type: fetch
          title: "Download the new quiver.core binary"
          url:
            darwin/arm64: "https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/quiver-darwin-arm64"
            darwin/amd64: "https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/quiver-darwin-amd64"
            linux/amd64: "https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/quiver-linux-amd64"
            linux/arm64: "https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/quiver-linux-arm64"
            "windows/*": "https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/quiver-windows-amd64.exe"
          to:
            default: "${WORKDIR}/quiver-new"
            "windows/*": "${WORKDIR}\\quiver-new.exe"
          checksum: "sha256sums:https://github.com/rabbytesoftware/quiver.core/releases/download/${REF}/checksums.txt"
          timeout: "120s"
          exit_on_failure: true
        - type: run
          title: "Hand over to the new binary"
          command:
            default: 'chmod +x "${WORKDIR}/quiver-new" && "${WORKDIR}/quiver-new" self-update "${WORKDIR}/quiver-new"'
            "windows/*": '"${WORKDIR}\quiver-new.exe" self-update "${WORKDIR}\quiver-new.exe"'
          timeout: "30s"
          exit_on_failure: true
```
