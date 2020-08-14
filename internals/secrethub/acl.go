package secrethub

import (
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"
)

// ACLCommand handles operations on access rules.
type ACLCommand struct {
	io        ui.IO
	newClient newClientFunc
}

// NewACLCommand creates a new ACLCommand.
func NewACLCommand(io ui.IO, newClient newClientFunc) *ACLCommand {
	return &ACLCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command and its sub-commands on the provided Registerer.
func (cmd *ACLCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("acl", "Manage access rules on directories.")
	NewACLCheckCommand(cmd.io, cmd.newClient).Register(clause)
	NewACLListCommand(cmd.io, cmd.newClient).Register(clause)
	NewACLRmCommand(cmd.io, cmd.newClient).Register(clause)
	NewACLSetCommand(cmd.io, cmd.newClient).Register(clause)
}
