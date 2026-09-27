package shelf

type PathStatus struct {
	BinDir     string   `json:"bin_dir"`
	OnPath     bool     `json:"on_path"`
	Configured bool     `json:"configured"`
	Files      []string `json:"files"`
}
