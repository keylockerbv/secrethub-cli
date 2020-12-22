package cli

import "fmt"

func (c *CommandClause) argumentError(args []string) error {
	if len(args) >= getRequired(c.Args) && len(args) <= len(c.Args) {
		return nil
	}
	errorText, minimum, maximum := "", getRequired(c.Args), len(c.Args)

	if minimum == maximum {
		errorText += fmt.Sprintf("`secrethub "+c.fullCommand()+"` requires exactly %d argument(s).", minimum)
	} else {
		errorText += fmt.Sprintf("`secrethub "+c.fullCommand()+"` requires between %d and %d arguments.", minimum, maximum)
	}
	errorText += "\n\nSee `secrethub " + c.fullCommand() + " --help` for help.\n\n" + c.usage()
	errorText += "\n\n" + c.Cmd.Short

	return fmt.Errorf(errorText)
}

func (c *CommandClause) usage() string {
	usage := "Usage: " + "secrethub " + c.fullCommand() + " [FLAGS] "
	for _, args := range c.Args {
		usage += "<" + args.Name + "> "
	}
	return usage
}

func (c *CommandClause) Help() string {
	return ""
}