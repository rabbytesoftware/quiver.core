package v0

type arrowV0 struct {
	Name         string              `yaml:"name"`
	Description  string              `yaml:"description"`
	Version      string              `yaml:"version"`
	License      string              `yaml:"license"`
	URL          string              `yaml:"url"`
	Maintainers  []string            `yaml:"maintainers"`
	Tags         []string            `yaml:"tags"`
	Credits      []creditV0          `yaml:"credits"`
	Requirements requirementsV0      `yaml:"requirements"`
	Dependencies []string            `yaml:"dependencies"`
	Variables    []variableV0        `yaml:"variables"`
	Netbridge    []portV0            `yaml:"netbridge"`
	Lifecycle    lifecycleV0         `yaml:"lifecycle"`
	Methods      map[string]methodV0 `yaml:"methods"`
}

type creditV0 struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	URL   string `yaml:"url"`
}

type requirementsV0 struct {
	CpuCores int      `yaml:"cpu_cores"`
	RamGB    int      `yaml:"ram_gb"`
	DiskGB   int      `yaml:"disk_gb"`
	OS       []string `yaml:"os"`
}

type variableV0 struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Default     string   `yaml:"default"`
	Sensitive   bool     `yaml:"sensitive"`
	Values      []string `yaml:"values"`
	Min         int      `yaml:"min"`
	Max         int      `yaml:"max"`
	Type        string   `yaml:"type"`
}

type portV0 struct {
	Name     string `yaml:"name"`
	Protocol string `yaml:"protocol"`
	Default  int    `yaml:"default"`
	Required bool   `yaml:"required"`
}

type lifecycleV0 struct {
	Install   []stepV0 `yaml:"install"`
	Execute   []stepV0 `yaml:"execute"`
	Stop      []stepV0 `yaml:"stop"`
	Uninstall []stepV0 `yaml:"uninstall"`
}

type methodV0 struct {
	AvailableIn []string `yaml:"available_in"`
	Steps       []stepV0 `yaml:"steps"`
}

type stepV0 struct {
	Type          string `yaml:"type"`
	Command       string `yaml:"command"`
	URL           string `yaml:"url"`
	To            string `yaml:"to"`
	Signal        string `yaml:"signal"`
	Title         string `yaml:"title"`
	Timeout       string `yaml:"timeout"`
	ExitOnFailure bool   `yaml:"exit_on_failure"`
}
