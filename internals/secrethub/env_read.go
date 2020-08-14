package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"
	"github.com/spf13/cobra"
)

// EnvReadCommand is a command to read the value of a single environment variable.
type EnvReadCommand struct {
	io          ui.IO
	newClient   newClientFunc
	environment *environment
	key         string
}

// NewEnvReadCommand creates a new EnvReadCommand.
func NewEnvReadCommand(io ui.IO, newClient newClientFunc) *EnvReadCommand {
	return &EnvReadCommand{
		io:          io,
		newClient:   newClient,
		environment: newEnvironment(io, newClient),
	}
}

// Register adds a CommandClause and it's args and flags to a Registerer.
func (cmd *EnvReadCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("read", "[BETA] Read the value of a single environment variable.")
	clause.HelpLong("This command is hidden because it is still in beta. Future versions may break.")
	clause.Args = cobra.MaximumNArgs(1)
	//clause.Arg("key", "the key of the environment variable to read").StringVar(&cmd.key)

	cmd.environment.register(clause)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run executes the command.
func (cmd *EnvReadCommand) Run() error {
	env, err := cmd.environment.env()
	if err != nil {
		return err
	}

	value, found := env[cmd.key]
	if !found {
		return fmt.Errorf("no environment variable with that key is set")
	}

	secretReader := newSecretReader(cmd.newClient)

	res, err := value.resolve(secretReader)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Output(), res)

	return nil
}

func (cmd *EnvReadCommand) argumentRegister(c *cobra.Command, args []string) error {
	if len(args) != 0 {
		cmd.key = args[0]
	}
	return nil
}
