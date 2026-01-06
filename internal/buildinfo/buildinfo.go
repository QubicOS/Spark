package buildinfo

// Version is set at build time via -ldflags.
var Version = "dev"

// Commit is set at build time via -ldflags.
var Commit = "unknown"

// Date is set at build time via -ldflags.
var Date = "unknown"

// Short returns a compact build identifier for UI/logging.
func Short() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if Commit != "" && Commit != "unknown" {
		return Commit
	}
	return "dev"
}
