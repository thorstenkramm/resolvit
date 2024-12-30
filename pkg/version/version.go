package version

var ResolvitVersion = "0.0.0-src"

// Set version at compile time with
// go build -ldflags "-X resolvit/pkg/version.ResolvitVersion=1.0.0" -o resolvit

// For a release build with version and optimization flags:
// go build -ldflags "-s -w -X resolvit/pkg/version.ResolvitVersion=1.0.0" -o resolvit
