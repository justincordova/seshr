package version

// Version is the current build version. Overridden at release time via
// -ldflags "-X github.com/justincordova/seshly/internal/version.Version=v0.1.0".
var Version = "dev"
