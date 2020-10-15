package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	// "github.com/spf13/cobra"
)

// EnvReadCommand is a command to read the value of a single environment variable.
type EnvReadCommand struct {
	io          ui.IO
	newClient   newClientFunc
	environment *environment
	key         cli.StringArgValue
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
func (cmd *EnvReadCommand) Register(r cli.Registerer) {
	clause := r.Command("read", "[BETA] Read the value of a single environment variable.")
	clause.HelpLong("This command is hidden because it is still in beta. Future versions may break.")
	// // clause.Cmd.Args = cobra.MaximumNArgs(1)
	//clause.Arg("key", "the key of the environment variable to read").StringVar(&cmd.key)

	cmd.environment.register(clause)

	clause.BindAction(cmd.Run)
	clause.BindArguments([]cli.ArgValue{&cmd.key})
}

// Run executes the command.
func (cmd *EnvReadCommand) Run() error {
	env, err := cmd.environment.env()
	if err != nil {
		return err
	}

	value, found := env[cmd.key.Param]
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
