package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"

	"github.com/secrethub/secrethub-go/internals/api"

	"github.com/spf13/cobra"
)

// OrgSetRoleCommand handles updating the role of an organization member.
type OrgSetRoleCommand struct {
	orgName   api.OrgName
	username  string
	role      string
	io        ui.IO
	newClient newClientFunc
}

// NewOrgSetRoleCommand creates a new OrgSetRoleCommand.
func NewOrgSetRoleCommand(io ui.IO, newClient newClientFunc) *OrgSetRoleCommand {
	return &OrgSetRoleCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *OrgSetRoleCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("set-role", "Set a user's organization role.")
	clause.Args = cobra.ExactValidArgs(3)
	clause.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return AutoCompleter{client: GetClient()}.RepositorySuggestions(cmd, args, toComplete)
		} else if len(args) == 1 {
			return []string{}, cobra.ShellCompDirectiveDefault
		}
		return []string{"admin", "member"}, cobra.ShellCompDirectiveDefault
	}
	//clause.Arg("org-name", "The organization name").Required().SetValue(&cmd.orgName)
	//clause.Arg("username", "The username of the user").Required().StringVar(&cmd.username)
	//clause.Arg("role", "The role to assign to the user. Can be either `admin` or `member`.").Required().StringVar(&cmd.role)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run updates the role of an organization member.
func (cmd *OrgSetRoleCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Setting role...\n")

	resp, err := client.Orgs().Members().Update(cmd.orgName.Value(), cmd.username, cmd.role)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Set complete! The user %s is %s of the %s organization.\n", resp.User.Username, resp.Role, cmd.orgName)

	return nil
}

func (cmd *OrgSetRoleCommand) argumentRegister(c *cobra.Command, args []string) error {
	err := api.ValidateOrgName(args[0])
	if err != nil {
		return err
	}
	cmd.orgName = api.OrgName(args[0])
	cmd.username = args[1]
	cmd.role = args[2]
	return nil
}
