package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-go/internals/api"
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
func (cmd *OrgSetRoleCommand) Register(r Registerer) {
	clause := r.Command("set-role", "Set a user's organization role.")
	clause.Arg("org-name", "The organization name").Required().SetValue(&cmd.orgName)
	clause.Arg("username", "The username of the user").Required().StringVar(&cmd.username)
	clause.Arg("role", "The role to assign to the user. Can be either `admin` or `member`.").Required().StringVar(&cmd.role)

	BindAction(clause, cmd.Run)
}

// Run updates the role of an organization member.
func (cmd *OrgSetRoleCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Stdout(), "Setting role...\n")

	resp, err := client.Orgs().Members().Update(cmd.orgName.Value(), cmd.username, cmd.role)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Stdout(), "Set complete! The user %s is %s of the %s organization.\n", resp.User.Username, resp.Role, cmd.orgName)

	return nil
}
