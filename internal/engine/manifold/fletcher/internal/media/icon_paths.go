package media

import "path"

var iconDirs = []string{
	"",
	"assets",
	".github",
	"docs",
	"images",
	"icons",
	"resources",
	"build",
	"public",
}

var iconNames = []string{
	"icon",
	"logo",
	"app-icon",
}

var iconExtensions = []string{
	"png",
	"svg",
}

var iconFixedPaths = []string{
	"src-tauri/icons/icon.png",
	"src-tauri/icons/128x128@2x.png",
}

func iconProbePaths() []string {
	paths := append([]string(nil), iconFixedPaths...)
	for _, dir := range iconDirs {
		for _, name := range iconNames {
			for _, ext := range iconExtensions {
				paths = append(paths, path.Join(dir, name+"."+ext))
			}
		}
	}
	return paths
}
