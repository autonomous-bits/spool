// Package version provides application release and build metadata.
package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
)

var (
	// Version is the application release version. Set via -ldflags at build time.
	Version = "dev"
	// Commit is the git commit SHA. Set via -ldflags at build time.
	Commit = "none"
	// BuildDate is the RFC3339 build timestamp. Set via -ldflags at build time.
	BuildDate = "unknown"
)

// Info contains build and runtime version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// Get returns the current version and build information, falling back to
// runtime build info if build-time variables are unset.
func Get() Info {
	v := Version
	c := Commit
	d := BuildDate

	if bi, ok := debug.ReadBuildInfo(); ok {
		if v == "" || v == "dev" {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				v = bi.Main.Version
			}
		}
		if c == "" || c == "none" {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					c = setting.Value
					break
				}
			}
		}
		if d == "" || d == "unknown" {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.time" && setting.Value != "" {
					d = setting.Value
					break
				}
			}
		}
	}

	return Info{
		Version:   v,
		Commit:    c,
		BuildDate: d,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// JSON returns the Info marshaled as indented JSON.
func (i Info) JSON() ([]byte, error) {
	return json.MarshalIndent(i, "", "  ")
}

// String returns a human-readable one-line representation of the version info.
func (i Info) String() string {
	return fmt.Sprintf("spl version %s (%s) built at %s (%s %s)", i.Version, i.Commit, i.BuildDate, i.GoVersion, i.Platform)
}
