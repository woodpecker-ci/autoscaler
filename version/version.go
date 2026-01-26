package version

// Version of Woodpecker Autoscaler, set with ldflags, from Git tag.
var Version string

// String returns the Version set at build time or "dev".
func String() string {
	if Version == "" {
		return "dev"
	}

	return Version
}
