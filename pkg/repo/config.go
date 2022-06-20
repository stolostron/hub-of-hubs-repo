package repo

type RepoConfig struct {
	// Directory to read helm charts from
	ChartDir string
	// Directory to write packaged helm charts
	RepoDir string
	// Version of helm charts
	Version string
	// Port to serve on
	Port int
	// Host to serve on
	Host string
}
