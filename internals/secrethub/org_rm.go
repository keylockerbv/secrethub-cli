package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-go/internals/api"
)

// OrgRmCommand deletes an organization, prompting the user for confirmation.
// It is not possible to force this command as it will not be scripted.
type OrgRmCommand struct {
	name      api.OrgName
	io        ui.IO
	newClient newClientFunc
}

// NewOrgRmCommand creates a new OrgRmCommand.
func NewOrgRmCommand(io ui.IO, newClient newClientFunc) *OrgRmCommand {
	return &OrgRmCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *OrgRmCommand) Register(r Registerer) {
	clause := r.Command("rm", "Permanently delete an organization and all the repositories it owns.")
	clause.Arg("org-name", "The organization name").Required().SetValue(&cmd.name)

	BindAction(clause, cmd.Run)
}

// Run deletes an organization, prompting the user for confirmation.
func (cmd *OrgRmCommand) Run() error {
	confirmed, err := ui.ConfirmCaseInsensitive(
		cmd.io,
		fmt.Sprintf(
			"[DANGER ZONE] This action cannot be undone. "+
				"This will permanently delete the %s organization, repositories, and remove all team associations. "+
				"Please type in the name of the organization to confirm",
			cmd.name,
		),
		cmd.name.String(),
	)
	if err != nil {
		return err
	}

	if !confirmed {
		fmt.Fprintln(cmd.io.Stdout(), "Name does not match. Aborting.")
		return nil
	}

	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Stdout(), "Deleting organization...")

	err = client.Orgs().Delete(cmd.name.Value())
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Stdout(), "Delete complete! The organization %s has been permanently deleted.\n", cmd.name)

	return nil
}
