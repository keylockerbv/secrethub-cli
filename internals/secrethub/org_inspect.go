package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"

	"github.com/secrethub/secrethub-go/internals/api"

	"github.com/spf13/cobra"
)

// OrgInspectCommand handles printing out the details of an organization in a JSON format.
type OrgInspectCommand struct {
	name          api.OrgName
	io            ui.IO
	newClient     newClientFunc
	timeFormatter TimeFormatter
}

// NewOrgInspectCommand creates a new OrgInspectCommand.
func NewOrgInspectCommand(io ui.IO, newClient newClientFunc) *OrgInspectCommand {
	return &OrgInspectCommand{
		io:            io,
		newClient:     newClient,
		timeFormatter: NewTimestampFormatter(),
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *OrgInspectCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("inspect", "Show the details of an organization.")
	clause.Args = cobra.ExactValidArgs(1)
	clause.ValidArgsFunction = AutoCompleter{client: GetClient()}.RepositorySuggestions
	//clause.Arg("org-name", "The organization name").Required().SetValue(&cmd.name)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run prints out the details of an organization.
func (cmd *OrgInspectCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	org, err := client.Orgs().Get(cmd.name.Value())
	if err != nil {
		return err
	}

	members, err := client.Orgs().Members().List(cmd.name.Value())
	if err != nil {
		return err
	}

	repos, err := client.Repos().List(cmd.name.Namespace().Value())
	if err != nil {
		return err
	}

	output, err := cli.PrettyJSON(newOrgInspectOutput(org, members, repos, cmd.timeFormatter))
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Output(), output)

	return nil
}

func (cmd *OrgInspectCommand) argumentRegister(c *cobra.Command, args []string) error {
	err := api.ValidateOrgName(args[0])
	if err != nil {
		return err
	}
	cmd.name = api.OrgName(args[0])
	return nil
}

// OrgInspectOutput is the json format to print out with all the details of an organization.
type OrgInspectOutput struct {
	Name        string
	Description string
	CreatedAt   string
	MemberCount int
	Members     []OrgMemberOutput
	RepoCount   int
	Repos       []api.RepoPath
}

func newOrgInspectOutput(org *api.Org, members []*api.OrgMember, repos []*api.Repo, timeFormatter TimeFormatter) OrgInspectOutput {
	out := OrgInspectOutput{
		Name:        org.Name,
		Description: org.Description,
		CreatedAt:   timeFormatter.Format(org.CreatedAt.Local()),
		MemberCount: len(members),
		Members:     make([]OrgMemberOutput, len(members)),
		RepoCount:   len(repos),
		Repos:       make([]api.RepoPath, len(repos)),
	}

	for i, member := range members {
		out.Members[i] = newOrgMemberOutput(member, timeFormatter)
	}

	for i, repo := range repos {
		out.Repos[i] = repo.Path()
	}

	return out
}

// OrgMemberOutput is the json format to print out an org member.
type OrgMemberOutput struct {
	Username      string
	Role          string
	CreatedAt     string
	LastChangedAt string
}

func newOrgMemberOutput(member *api.OrgMember, timeFormatter TimeFormatter) OrgMemberOutput {
	return OrgMemberOutput{
		Username:      member.User.Username,
		Role:          member.Role,
		LastChangedAt: timeFormatter.Format(member.LastChangedAt.Local()),
		CreatedAt:     timeFormatter.Format(member.CreatedAt.Local()),
	}
}
