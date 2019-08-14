package secrethub

import (
	"strconv"

	"github.com/secrethub/secrethub-cli/internals/cli/mlock"
)

// mlockFlag configures locking memory.
type mlockFlag bool

// init locks the memory based on the flag value if supported.
func (f mlockFlag) init() error {
	if f {
		if mlock.Supported() {
			err := mlock.LockMemory()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// RegisterMlockFlag registers a mlock flag that enables memory locking when set to true.
func RegisterMlockFlag(r FlagRegisterer) {
	flag := mlockFlag(false)
	r.Flag("mlock", "Enable memory locking").SetValue(&flag)
}

// String implements the flag.Value interface.
func (f mlockFlag) String() string {
	return strconv.FormatBool(bool(f))
}

// Set enables mlock when the given value is true.
func (f *mlockFlag) Set(value string) error {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*f = mlockFlag(b)
	return f.init()
}

// IsBoolFlag makes the flag a boolean flag when used in a Kingpin application.
// Thus, the flag can be used without argument (--mlock).
func (f mlockFlag) IsBoolFlag() bool {
	return true
}
