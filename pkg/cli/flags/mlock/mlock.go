package mlock

import (
	"github.com/keylockerbv/secrethub-cli/pkg/cli"
	"strconv"

	"github.com/keylockerbv/secrethub-cli/pkg/mlock"
	"github.com/secrethub/secrethub-go/internals/errio"
)

// mlockFlag configures locking memory.
type mlockFlag bool

// init locks the memory based on the flag value if supported.
func (f mlockFlag) init() error {
	if f {
		if mlock.Supported() {
			err := mlock.LockMemory()
			if err != nil {
				return errio.Error(err)
			}
		}
	}
	return nil
}

// RegisterMlockFlag registers a mlock flag that enables memory locking when set to true.
func RegisterMlockFlag(r cli.FlagRegisterer) {
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
		return errio.Error(err)
	}
	*f = mlockFlag(b)
	return f.init()
}

// IsBoolFlag makes the flag a boolean flag when used in a Kingpin application.
// Thus, the flag can be used without argument (--mlock).
func (f mlockFlag) IsBoolFlag() bool {
	return true
}
