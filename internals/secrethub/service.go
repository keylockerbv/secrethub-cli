package secrethub

import (
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/command"
)

// ServiceCommand handles operations on services.
type ServiceCommand struct {
	io        ui.IO
	newClient newClientFunc
}

// NewServiceCommand creates a new ServiceCommand.
func NewServiceCommand(io ui.IO, newClient newClientFunc) *ServiceCommand {
	return &ServiceCommand{
		io:        io,
		newClient: newClient,
	}
}

// Register registers the command and its sub-commands on the provided Registerer.
func (cmd *ServiceCommand) Register(r command.Registerer) {
	clause := r.CreateCommand("service", "Manage service accounts.")
	NewServiceAWSCommand(cmd.io, cmd.newClient).Register(clause)
	NewServiceGCPCommand(cmd.io, cmd.newClient).Register(clause)
	NewServiceDeployCommand(cmd.io).Register(clause)
	NewServiceInitCommand(cmd.io, cmd.newClient).Register(clause)
	NewServiceLsCommand(cmd.io, cmd.newClient).Register(clause)
}
