package picker

const (
	familyLinux   = "linux"
	familyDarwin  = "darwin"
	familyWindows = "windows"

	archNone      = ""
	archARM64     = "arm64"
	archAMD64     = "amd64"
	archUniversal = "universal"
	archOther     = "other"

	boundaryStart = `(?:^|[-_. /])`
	boundaryEnd   = `(?:$|[-_. ])`

	skipExtensionSource = `\.(?:sha256|sha512|sha1|md5|sig|asc|pem|crt|sbom|spdx|json|txt|yml|yaml|blockmap|intoto\.jsonl|bundle|minisig|pub|sum|cdx|xml|html|md|pdf|sh|ps1|whl|jar|aar|apk|ipa|aab|vsix|nupkg|crx|xpi|zsync|pkg\.tar\.zst|sigstore|cosign|pom|gem|dll|so|dylib|lib|a|h|ttf|otf|woff2|png|svg|wasm|cab|nsis\.zip|app\.tar\.gz|msi\.zip|tar\.gz\.sig)$`
	ignoredWordsSource  = `checksums?|sha256sums?|sha512sums?|sums|src|sources?|debug|symbols|dbgsym|pdb|sbom|provenance|headers|devel|dev|sdk|docs?|manual|javadoc|dsyms?|pdbs?`
	archiveSource       = `\.(?:tar\.gz|tgz|tar\.xz|txz|tar\.bz2|tbz|tbz2|tar\.zst|zip|gz|xz|bz2|zst|tar)$`
	installerSource     = `\.(?:pacman|pkg|msix|msixbundle|appx|deb|rpm|snap|flatpak|flatpakref)$`
	stemSource          = `\.(?:tar\.gz|tgz|tar\.xz|txz|tar\.bz2|tbz2|tbz|tar\.zst|zip|gz|xz|bz2|zst|tar|dmg|msi|exe|appimage)$`

	linuxSource   = `linux|linux64|lin64|appimage|musl|gnu|ubuntu|debian|fedora|el[789]|manylinux\w*|unknown-linux`
	darwinSource  = `darwin|macos|macosx|mac|osx|apple|mac64|macos\d+`
	windowsSource = `windows|win|win32|win64|msvc|mingw|mingw64|pc-windows|x64-setup|setup`

	arm64Source     = `arm64|aarch64|aarch_64|armv8|arm64e|apple-silicon|m1|silicon`
	amd64Source     = `x86_64|x86-64|amd64|x64|64bit|64-bit|win64|intel|linux64|mac64|x86_64-v\d`
	universalSource = `universal|universal2|fat`
	otherArchSource = `i386|i586|i686|386|x86|32bit|32-bit|arm32|armv7l?|armv6l?|armv5\w*|arm5|armhf|armel|arm|ppc64le|ppc64|powerpc64le|powerpc64|sparc64|s390x|riscv64|mips\w*|loong\w+|ia32|win32`

	knownSource     = `^(?:v?\d+|linux|linux64|lin64|darwin|macos|macosx|macos\d+|mac|mac64|osx|apple|windows|win|win32|win64|pc|msvc|mingw|mingw64|gnu|musl|unknown|ubuntu|debian|fedora|el[789]|manylinux\w*|x86|x64|x86_64|amd64|arm64|arm64e|armv8|aarch|aarch64|m1|universal|universal2|fat|64|bit|64bit|setup|installer|portable|appimage|exe|zip|tar|gz|tgz|xz|zst|bz2|7z|dmg|pkg|msi|deb|rpm|release|static|intel|silicon|gnueabihf|x86-64|1)$`
	channelSource   = `^(?:nightly|beta|alpha|rc\d*|canary|preview|dev|next|edge|stable)$`
	companionSource = `^(?:cli|api|server|remote|agent|daemon|client|go|py|python|plugin|extension|helper|headless|worker|proxy|bridge|updater)$`

	separatorSource = `[-_. ]+`
	nonAlnumSource  = `[^a-z0-9]+`

	installerExeSource = `desktop|setup|install|nsis|squirrel`
	guiPackageSource   = `\.(?:dmg|appimage)$`

	setupWord     = "setup"
	installerWord = "installer"

	musl         = "musl"
	portableWord = "portable"

	appImageSuffix = ".appimage"
	dmgSuffix      = ".dmg"
	msiSuffix      = ".msi"
	exeSuffix      = ".exe"

	bareBinaryTail = 6
)
