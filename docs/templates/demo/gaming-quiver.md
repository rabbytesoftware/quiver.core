# Gaming Quiver

A curated collection of game server arrows managed by char2cs.

```collection
schema: "collection@v0"

metadata:
  name: "Gaming Quiver"
  version: "1.0.0"
  description: "Game servers and utilities curated by char2cs"
  url: "https://gaming.quiver.ar"

  maintainers:
    - "char2cs"

  tags:
    - "gaming"
    - "servers"

  media:
    icon: "https://gaming.quiver.ar/icon.png"
    banner: "https://gaming.quiver.ar/banner.png"

arrows:
  # Local arrows (YAML files in this repo under a named subdirectory)
  - path: cs2
  - path: minecraft

  # External arrows (full namespace from another repo)
  - namespace: github.com/valve/steamcmd
```
