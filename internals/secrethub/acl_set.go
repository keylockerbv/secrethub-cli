package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"
	"github.com/secrethub/secrethub-go/internals/api"
	"github.com/spf13/cobra"
)

// ACLSetCommand is a command to set access rules.
type ACLSetCommand struct {
	accountName api.AccountName
	force       bool
	io          ui.IO
	path        api.DirPath
	permission  api.Permission
	newClient   newClientFunc
}

// NewACLSetCommand creates a new ACLSetCommand.
func NewACLSetCommand(io ui.IO, newClient newClientFunc) *ACLSetCommand {
	return &ACLSetCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register adds a CommandClause and it's args and flags to a Registerer.
// Register adds args and flags.
func (cmd *ACLSetCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("set", "Set access rule for an user or service on a path.")
	clause.Args = cobra.ExactValidArgs(3)
	clause.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return AutoCompleter{client: GetClient()}.DirectorySuggestions(cmd, args, toComplete)
		} else if len(args) == 1 {
			return []string{}, cobra.ShellCompDirectiveDefault
		}
		return []string{"read", "write", "admin"}, cobra.ShellCompDirectiveDefault
	}
	//clause.Arg("dir-path", "The path of the directory to set the access rule for").Required().PlaceHolder(optionalDirPathPlaceHolder).SetValue(&cmd.path)
	//clause.Arg("account-name", "The account name (username or service name) to set the access rule for").Required().SetValue(&cmd.accountName)
	//clause.Arg("permission", "The permission to set in the access rule.").Required().SetValue(&cmd.permission)
	registerForceFlag(clause, &cmd.force)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run handles the command with the options as specified in the command.
func (cmd *ACLSetCommand) Run() error {
	if !cmd.force {
		confirmed, err := ui.AskYesNo(
			cmd.io,
			fmt.Sprintf(
				"[WARNING] This gives %s %s rights on all directories and secrets contained in %s. "+
					"Are you sure you want to set this access rule?",
				cmd.accountName,
				cmd.permission,
				cmd.path,
			),
			ui.DefaultNo,
		)
		if err != nil {
			return err
		}

		if !confirmed {
			fmt.Fprintln(cmd.io.Output(), "Aborting.")
			return nil
		}
	}

	fmt.Fprintf(cmd.io.Output(), "Setting access rule for %s at %s with %s\n", cmd.accountName, cmd.path, cmd.permission)

	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	_, err = client.AccessRules().Set(cmd.path.Value(), cmd.permission.String(), cmd.accountName.Value())
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Output(), "Access rule set!")

	return nil
}

func (cmd *ACLSetCommand) argumentRegister(c *cobra.Command, args []string) error {
	var err error
	cmd.path, err = api.NewDirPath(args[0])
	if err != nil {
		return err
	}
	cmd.accountName, err = api.NewAccountName(args[1])
	if err != nil {
		return err
	}
	cmd.permission = api.PermissionNone
	err = cmd.permission.Set(args[2])
	if err != nil {
		return err
	}
	return nil
}
