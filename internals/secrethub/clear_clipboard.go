package secrethub

import (
	"encoding/hex"
	"time"

	"github.com/secrethub/secrethub-cli/internals/cli/clip"
	"github.com/secrethub/secrethub-cli/internals/cli/cloneproc"

	"golang.org/x/crypto/bcrypt"
)

// defaultClearClipboardAfter defines the default TTL for data written to the clipboard.
const defaultClearClipboardAfter = 45 * time.Second

// ClearClipboardCommand is a command to clear the contents of the clipboard after some time passed.
type ClearClipboardCommand struct {
	clipper clip.Clipper
	hash    []byte
	timeout time.Duration
}

// NewClearClipboardCommand creates a new ClearClipboardCommand.
func NewClearClipboardCommand() *ClearClipboardCommand {
	return &ClearClipboardCommand{
		clipper: clip.NewClipboard(),
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *ClearClipboardCommand) Register(r Registerer) {
	clause := r.Command("clipboard-clear", "Removes secret from clipboard.").Hidden()
	clause.Arg("hash", "Hash from the secret to be cleared").Required().HexBytesVar(&cmd.hash)
	clause.Flag("timeout", "Time to wait before clearing in seconds").DurationVar(&cmd.timeout)

	BindAction(clause, cmd.Run)
}

// Run handles the command with the options as specified in the command.
func (cmd *ClearClipboardCommand) Run() error {
	if cmd.timeout > 0 {
		time.Sleep(cmd.timeout)
	}

	read, err := cmd.clipper.ReadAll()
	if err != nil {
		return err
	}

	err = bcrypt.CompareHashAndPassword(cmd.hash, read)
	if err != nil {
		return nil
	}

	err = cmd.clipper.WriteAll(nil)
	if err != nil {
		return err
	}
	return nil
}

// WriteClipboardAutoClear writes data to the clipboard and clears it after the timeout.
func WriteClipboardAutoClear(data []byte, timeout time.Duration, clipper clip.Clipper) error {
	hash, err := bcrypt.GenerateFromPassword(data, bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	err = clipper.WriteAll(data)
	if err != nil {
		return err
	}

	err = cloneproc.Spawn(
		"clipboard-clear", hex.EncodeToString(hash),
		"--timeout", timeout.String())

	return err
}
