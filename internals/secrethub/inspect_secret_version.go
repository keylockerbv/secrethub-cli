package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/cli/ui"

	"github.com/secrethub/secrethub-go/internals/api"
)

// InspectSecretVersionCommand prints out the details of a secret version in JSON format.
type InspectSecretVersionCommand struct {
	path          api.SecretPath
	io            ui.IO
	newClient     newClientFunc
	timeFormatter TimeFormatter
}

// NewInspectSecretVersionCommand creates a new InspectSecretVersionCommand.
func NewInspectSecretVersionCommand(path api.SecretPath, io ui.IO, newClient newClientFunc) *InspectSecretVersionCommand {
	return &InspectSecretVersionCommand{
		path:          path,
		io:            io,
		newClient:     newClient,
		timeFormatter: NewTimeFormatter(true),
	}
}

// Run prints out the details of a secret version.
func (cmd *InspectSecretVersionCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	version, err := client.Secrets().Versions().GetWithoutData(cmd.path.Value())
	if err != nil {
		return err
	}

	output, err := cli.PrettyJSON(newSecretVersionOutput(version, cmd.timeFormatter))
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Stdout(), output)

	return nil
}

func newSecretVersionOutput(secret *api.SecretVersion, timeFormatter TimeFormatter) secretVersionOutput {
	return secretVersionOutput{
		Version:   secret.Version,
		CreatedAt: timeFormatter.Format(secret.CreatedAt.Local()),
		Status:    secret.Status,
	}
}

// secretVersionOutput is the printable JSON format of a secret version.
type secretVersionOutput struct {
	Version   int
	CreatedAt string
	Status    string
}
