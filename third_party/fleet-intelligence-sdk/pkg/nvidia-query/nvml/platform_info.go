package nvml

import "github.com/NVIDIA/go-nvml/pkg/dl"

const (
	platformInfoSymbol = "nvmlDeviceGetPlatformInfo"
)

type dynamicLibrary interface {
	Open() error
	Close() error
	Lookup(string) error
}

var (
	platformInfoLibraryName = "libnvidia-ml.so.1"
	newDynamicLibrary       = func(name string, flags int) dynamicLibrary {
		return dl.New(name, flags)
	}
)

// PlatformInfoSupported checks if the NVML library exports the platform info symbol.
// Missing symbols can trigger a hard crash if called via cgo, so we guard the call.
func PlatformInfoSupported() bool {
	lib := newDynamicLibrary(platformInfoLibraryName, dl.RTLD_LAZY|dl.RTLD_LOCAL)
	if err := lib.Open(); err != nil {
		return false
	}
	defer func() {
		_ = lib.Close()
	}()

	if err := lib.Lookup(platformInfoSymbol); err == nil {
		return true
	}
	return false
}
