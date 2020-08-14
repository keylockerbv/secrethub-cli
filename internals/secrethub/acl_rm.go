package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"
	"github.com/secrethub/secrethub-go/internals/api"
	"github.com/spf13/cobra"
)

// ACLRmCommand handles removing an access rule.
type ACLRmCommand struct {
	path        api.DirPath
	accountName api.AccountName
	force       bool
	io          ui.IO
	newClient   newClientFunc
}

// NewACLRmCommand creates a new ACLRmCommand.
func NewACLRmCommand(io ui.IO, newClient newClientFunc) *ACLRmCommand {
	return &ACLRmCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *ACLRmCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("rm", "Remove an account's access rules on a given directory. Although the server will deny the account access afterwards, note that removing an access rule does not actually revoke an account and does NOT trigger secret rotation.")
	clause.Alias("remove")
	clause.Args = cobra.ExactValidArgs(2)
	clause.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return AutoCompleter{client: GetClient()}.DirectorySuggestions(cmd, args, toComplete)
		}
		return []string{}, cobra.ShellCompDirectiveDefault
	}
	//clause.Arg("dir-path", "The path of the directory to remove the access rule for").Required().PlaceHolder(optionalDirPathPlaceHolder).SetValue(&cmd.path)
	//clause.Arg("account-name", "The account name (username or service name) whose rule to remove").Required().SetValue(&cmd.accountName)
	registerForceFlag(clause, &cmd.force)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run removes the access rule.
func (cmd *ACLRmCommand) Run() error {
	if !cmd.force {
		confirmed, err := ui.AskYesNo(
			cmd.io,
			fmt.Sprintf(
				"[WARNING] This can impact the account's ability to read and/or modify secrets. "+
					"Are you sure you want to remove the access rule for %s?",
				cmd.accountName,
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

	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Output(), "Removing access rule...")

	err = client.AccessRules().Delete(cmd.path.Value(), cmd.accountName.Value())
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Removal complete! The access rule for %s on %s has been removed.\n", cmd.accountName, cmd.path)

	return nil
}

func (cmd *ACLRmCommand) argumentRegister(c *cobra.Command, args []string) error {
	var err error
	cmd.path, err = api.NewDirPath(args[0])
	if err != nil {
		return err
	}
	cmd.accountName, err = api.NewAccountName(args[1])
	if err != nil {
		return err
	}
	return nil
}
