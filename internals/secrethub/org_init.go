package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"

	"github.com/secrethub/secrethub-go/internals/api"
)

// OrgInitCommand handles creating an organization.
type OrgInitCommand struct {
	name        orgNameValue
	description string
	force       bool
	io          ui.IO
	newClient   newClientFunc
}

// NewOrgInitCommand creates a new OrgInitCommand.
func NewOrgInitCommand(io ui.IO, newClient newClientFunc) *OrgInitCommand {
	return &OrgInitCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *OrgInitCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("init", "Initialize a new organization account.")
	clause.ValidArgsFunction = AutoCompleter{client: GetClient()}.RepositorySuggestions
	clause.Var(&cmd.name, "name", "The name you would like to use for your organization. If not set, you will be asked for it.", true, false)
	clause.StringVar(&cmd.description, "description", "", "A description (max 144 chars) for your organization so others will recognize it. If not set, you will be asked for it.", true, false)
	clause.StringVar(&cmd.description, "descr", "", "", true, false)
	clause.Flag("descr").Hidden = true
	clause.StringVar(&cmd.description, "desc", "", "", true, false)
	clause.Flag("desc").Hidden = true
	registerForceFlag(clause, &cmd.force)

	command.BindAction(clause, nil, cmd.Run)
}

// Run creates an organization.
func (cmd *OrgInitCommand) Run() error {
	var err error

	incompleteInput := cmd.name.orgName == "" || cmd.description == ""
	if cmd.force && incompleteInput {
		return ErrMissingFlags
	} else if !cmd.force && incompleteInput {
		fmt.Fprintf(
			cmd.io.Output(),
			"Before initializing a new organization, we need to know a few things about your organization. "+
				"Please answer the questions below, followed by an [ENTER]\n\n",
		)

		if cmd.name.orgName == "" {
			name, err := ui.AskAndValidate(cmd.io, "The name you would like to use for your organization: ", 2, api.ValidateOrgName)
			if err != nil {
				return err
			}
			cmd.name = orgNameValue{orgName: api.OrgName(name)}
		}

		if cmd.description == "" {
			cmd.description, err = ui.AskAndValidate(cmd.io, "A short description so your teammates will recognize the organization (max. 144 chars): ", 2, api.ValidateOrgDescription)
			if err != nil {
				return err
			}
		}

		// Print a whitespace line here for readability.
		fmt.Fprintln(cmd.io.Output(), "")
	}

	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Creating organization...\n")

	resp, err := client.Orgs().Create(cmd.name.orgName.Value(), cmd.description)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Creation complete! The organization %s is now ready to use.\n", resp.Name)

	return nil
}

type orgNameValue struct {
	orgName api.OrgName
}

func (o orgNameValue) String() string {
	return o.orgName.String()
}

func (o orgNameValue) Set(s string) error {
	return o.orgName.Set(s)
}

func (o orgNameValue) Type() string {
	return "orgNameValue"
}
