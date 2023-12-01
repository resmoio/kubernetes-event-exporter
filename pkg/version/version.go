package version

import (
	"runtime"
	"runtime/debug"
)

// Build information. Populated at build-time.
var (
	Version   = "unknown"
	GoVersion = runtime.Version()
	GoOS      = runtime.GOOS
	GoArch    = runtime.GOARCH
)

func Revision() string {
	bi, ok := debug.ReadBuildInfo()

	if ok {
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				return kv.Value
			}
		}
	}

	return "unknown"
}
