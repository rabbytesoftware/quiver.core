<p align="center">
  <img src="doc/logo-header.svg" alt="bat - a cat clone with wings"><br>
  <a href="https://github.com/sharkdp/bat/actions?query=workflow%3ACICD"><img src="https://github.com/sharkdp/bat/workflows/CICD/badge.svg" alt="Build Status"></a>
  <img src="https://img.shields.io/crates/l/bat.svg" alt="license">
  A <i>cat(1)</i> clone with syntax highlighting and Git integration.
</p>

### Syntax highlighting

`bat` supports syntax highlighting for a large number of programming and markup
languages:

![Syntax highlighting example](https://imgur.com/rGsdnDe.png)

### Example manifest fence

Here is what an arrow manifest embedded in a README might look like:

```arrow
schema: "arrow@v0"
metadata:
  name: "not-a-real-manifest"
```

## Installation

<!--

Installation instructions need to:
* be for widely used systems

-->

[![Packaging status](https://repology.org/badge/vertical-allrepos/bat-cat.svg?columns=3&exclude_unsupported=1)](https://repology.org/project/bat-cat/versions)

### On Ubuntu (using `apt`)

`bat` is available on [Ubuntu since 20.04](https://packages.ubuntu.com/search?keywords=bat&exact=1).

```bash
apt install bat
```

## Customization

`bat` supports custom syntaxes and themes, see the wiki for details.
