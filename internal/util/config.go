package util

// Configuration holds all the configuration values of the
type Configuration struct {
	ApiserverHost  string
	KubeconfigFile string

	WatchedNamespace string
	WatchedSecret    string
	TargetNamespaces []string
	TargetTemplate   string
}
