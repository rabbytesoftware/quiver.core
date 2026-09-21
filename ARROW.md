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
  maintainers:
    - name: "Rabbyte Software"
      url: "https://char2cs.net"
  credits:
    - name: "Rabbyte Software"

variables:
  - name: "QUIVER_RELEASE_ASSET_URL"
    description: "Download URL for the resolved quiver.core release binary for this platform."
  - name: "QUIVER_RELEASE_CHECKSUM"
    description: "SHA-256 checksum of the release binary, for fetch-step verification."

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
          url: "${QUIVER_RELEASE_ASSET_URL}"
          to: "${WORKDIR}/quiver-new"
          checksum: "${QUIVER_RELEASE_CHECKSUM}"
          timeout: "120s"
          exit_on_failure: true
```
