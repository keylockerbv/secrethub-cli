package secrethub

import (
	"fmt"
	"os"
	"regexp"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
)

func (cmd *MigrateConfigEnvfileCommand) Run() error {
	plan, err := getPlan(cmd.planFile)
	if err != nil {
		return err
	}

	vars := parseVarPossibilities(cmd.vars)
	refMapping := newReferenceMapping(plan)
	err = refMapping.addVarPossibilities(vars)
	if err != nil {
		return err
	}
	refMapping.stripSecretHubURIScheme()

	filepath := cmd.inFile.Value
	if filepath == "" {
		filepath = "secrethub.env"
	}

	inFileContents, err := os.ReadFile(filepath)
	if err != nil {
		return ErrReadFile(filepath, err)
	}

	err = checkForCompositeSecrets(inFileContents)
	if err != nil {
		return err
	}

	output, replaceCount, err := migrateTemplateTags(string(inFileContents), refMapping, "%s")
	if err != nil {
		return err
	}

	inFileInfo, err := os.Stat(filepath)
	if err != nil {
		return ErrReadFile(filepath, err)
	}

	err = os.WriteFile(".env", []byte(output), inFileInfo.Mode())
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Output(), "Created new .env file with %d op:// references\n", replaceCount)

	return nil
}

var regexpCompositeSecrets = regexp.MustCompile(`{{.+?}}[^\s]+`)

func checkForCompositeSecrets(inFileContents []byte) error {
	if match := regexpCompositeSecrets.Find(inFileContents); match != nil {
		return fmt.Errorf("composite environment variables are not supported anymore with Dotenv: %s\nMake sure one environment variable corresponds to just a single secret", match)
	}
	return nil
}

type MigrateConfigEnvfileCommand struct {
	io ui.IO

	inFile   cli.StringValue
	planFile string
	vars     map[string]string
}

func NewMigrateConfigEnvfileCommand(io ui.IO) *MigrateConfigEnvfileCommand {
	return &MigrateConfigEnvfileCommand{
		io: io,
	}
}

func (cmd *MigrateConfigEnvfileCommand) Register(r cli.Registerer) {
	clause := r.Command("envfile", "Migrate secrethub.env file by turning SecretHub paths into 1Password op:// references, resulting in a new Dotenv (.env) file.")
	clause.Flags().StringVar(&cmd.planFile, "plan-file", defaultPlanPath, "Path to the file used to migrate your secrets.")
	clause.Flags().StringToStringVarP(&cmd.vars, "var", "v", nil, "Define the possible values for a template variable, e.g. --var env=dev,staging,prod --var region=us-east-1,eu-west-1")
	clause.BindArguments([]cli.Argument{{Value: &cmd.inFile, Name: "in-file", Required: false, Placeholder: "<path to secrethub.env>", Description: "The path to the secrethub.env file you'd like to migrate."}})

	clause.BindAction(cmd.Run)
}
