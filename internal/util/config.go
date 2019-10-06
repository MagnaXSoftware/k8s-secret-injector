package util

// Configuration holds all the configuration values of the
type Configuration struct {
	ApiserverHost  string
	KubeconfigFile string

	WatchedSecretName  string
	WatchedNamespace   string
	TargetNameTemplate string
	TargetNamespaces   []string
	CleanNamespaces    bool
}
