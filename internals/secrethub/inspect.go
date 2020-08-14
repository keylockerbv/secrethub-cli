package secrethub

import (
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"
	"github.com/spf13/cobra"

	"github.com/secrethub/secrethub-go/internals/api"
)

// ErrInspectResourceNotSupported is an error that is thrown when the inspect command is called with
// a path as argument that is not a repository- or secret-path.
var ErrInspectResourceNotSupported = errMain.Code("inspect_resource_not_supported").Error("currently only inspecting repositories or secrets is supported")

// InspectCommand prints information about a repository or a secret.
type InspectCommand struct {
	path          api.Path
	io            ui.IO
	newClient     newClientFunc
	timeFormatter TimeFormatter
}

// NewInspectCommand creates a new InspectCommand.
func NewInspectCommand(io ui.IO, newClient newClientFunc) *InspectCommand {
	return &InspectCommand{
		io:            io,
		newClient:     newClient,
		timeFormatter: NewTimeFormatter(true),
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *InspectCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("inspect", "Print details of a resource.")
	clause.Args = cobra.ExactValidArgs(1)
	clause.ValidArgsFunction = AutoCompleter{client: GetClient()}.SecretSuggestions
	//clause.Arg("repo or secret-path", "Path to the repository or the secret to inspect "+repoPathPlaceHolder+" or "+secretPathOptionalVersionPlaceHolder).Required().SetValue(&cmd.path)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run inspects a repository or a secret
func (cmd *InspectCommand) Run() error {
	repoPath, err := cmd.path.ToRepoPath()
	if err == nil {
		repoInspectCmd := NewRepoInspectCommand(
			cmd.io,
			cmd.newClient,
		)
		repoInspectCmd.path = repoPath
		return repoInspectCmd.Run()
	}

	secretPath, err := cmd.path.ToSecretPath()
	if err == nil {
		if secretPath.HasVersion() {
			return NewInspectSecretVersionCommand(
				secretPath,
				cmd.io,
				cmd.newClient,
			).Run()
		}

		return NewInspectSecretCommand(
			secretPath,
			cmd.io,
			cmd.newClient,
		).Run()
	}

	return ErrInspectResourceNotSupported
}

func (cmd *InspectCommand) argumentRegister(c *cobra.Command, args []string) error {
	var err error
	cmd.path, err = api.NewPath(args[0])
	if err != nil {
		return err
	}
	return nil
}
