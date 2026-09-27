package shelf

import "path/filepath"

type Option func(*shelf)

func WithHomeDir(
	dir string,
) Option {
	return func(s *shelf) { s.homeDir = dir }
}

func WithUserHomeDir(
	dir string,
) Option {
	return func(s *shelf) { s.userHomeDir = dir }
}

func WithAppsDirs(
	dirs []string,
) Option {
	return func(s *shelf) { s.appsDirs = dirs }
}

func WithGOOS(
	goos string,
) Option {
	return func(s *shelf) { s.goos = goos }
}

func WithGOARCH(
	goarch string,
) Option {
	return func(s *shelf) { s.goarch = goarch }
}

func WithCommander(
	c Commander,
) Option {
	return func(s *shelf) { s.commander = c }
}

func WithEnv(
	lookup func(string) string,
) Option {
	return func(s *shelf) { s.env = lookup }
}

func withTagger(
	t bundleTagger,
) Option {
	return func(s *shelf) { s.tagger = t }
}

func withUserPath(
	u userPath,
) Option {
	return func(s *shelf) { s.userPath = u }
}

func WithSandboxHome(
	home string,
) Option {
	return func(s *shelf) {
		s.homeDir = home
		s.userHomeDir = home
		s.sandboxHome = home
		s.appsDirs = []string{filepath.Join(home, "Applications")}
	}
}
