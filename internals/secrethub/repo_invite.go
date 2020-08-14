package secrethub

import (
	"fmt"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"

	"github.com/secrethub/secrethub-go/internals/api"

	"github.com/spf13/cobra"
)

// RepoInviteCommand handles inviting a user to collaborate on a repository.
type RepoInviteCommand struct {
	path      api.RepoPath
	username  string
	force     bool
	io        ui.IO
	newClient newClientFunc
}

// NewRepoInviteCommand creates a new RepoInviteCommand.
func NewRepoInviteCommand(io ui.IO, newClient newClientFunc) *RepoInviteCommand {
	return &RepoInviteCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *RepoInviteCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("invite", "Invite a user to collaborate on a repository.")
	clause.Args = cobra.ExactValidArgs(2)
	clause.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return AutoCompleter{client: GetClient()}.RepositorySuggestions(cmd, args, toComplete)
		}
		return []string{}, cobra.ShellCompDirectiveDefault
	}
	//clause.Arg("repo-path", "The repository to invite the user to").Required().PlaceHolder(repoPathPlaceHolder).SetValue(&cmd.path)
	//clause.Arg("username", "username of the user").Required().StringVar(&cmd.username)
	registerForceFlag(clause, &cmd.force)

	command.BindAction(clause, cmd.argumentRegister, cmd.Run)
}

// Run invites the configured user to collaborate on the repo.
func (cmd *RepoInviteCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	if !cmd.force {
		user, err := client.Users().Get(cmd.username)
		if err != nil {
			return err
		}

		msg := fmt.Sprintf("Are you sure you want to add %s to the %s repository?",
			user.PrettyName(),
			cmd.path)

		confirmed, err := ui.AskYesNo(cmd.io, msg, ui.DefaultNo)
		if err != nil {
			return err
		}

		if !confirmed {
			fmt.Fprintln(cmd.io.Output(), "Aborting.")
			return nil
		}
	}
	fmt.Fprintln(cmd.io.Output(), "Inviting user...")

	_, err = client.Repos().Users().Invite(cmd.path.Value(), cmd.username)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Invite complete! The user %s is now a member of the %s repository.\n", cmd.username, cmd.path)

	return nil
}

func (cmd *RepoInviteCommand) argumentRegister(c *cobra.Command, args []string) error {
	var err error
	cmd.path, err = api.NewRepoPath(args[0])
	if err != nil {
		return err
	}
	cmd.username = args[1]
	return nil
}
