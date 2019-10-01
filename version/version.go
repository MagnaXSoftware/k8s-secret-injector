package version

import (
	"fmt"
)

var (
	Release = "UNKNOWN"
	Build = "UNKNOWN"
)

// String returns the textual representation of the version
func String() string {
	return fmt.Sprintf("release %v - build %v", Release, Build)
}