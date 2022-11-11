package secrethub

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/cli/clip"
	"github.com/secrethub/secrethub-cli/internals/cli/filemode"
	"github.com/secrethub/secrethub-cli/internals/cli/posix"
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/tpl"

	"github.com/docker/go-units"
)

// Errors
var (
	ErrUnknownTemplateVersion = errMain.Code("unknown_template_version").ErrorPref("unknown template version: '%s' supported versions are 1, 2 and latest")
	ErrReadFile               = errMain.Code("in_file_read_error").ErrorPref("could not read the input file %s: %s")
)

// InjectCommand is a command to read a secret.
type InjectCommand struct {
	outFile                       string
	inFile                        string
	fileMode                      filemode.FileMode
	force                         bool
	io                            ui.IO
	useClipboard                  bool
	clipWriter                    ClipboardWriter
	osEnv                         []string
	newClient                     newClientFunc
	templateVars                  map[string]string
	templateVersion               string
	dontPromptMissingTemplateVars bool
}

// NewInjectCommand creates a new InjectCommand.
func NewInjectCommand(io ui.IO, newClient newClientFunc) *InjectCommand {
	return &InjectCommand{
		clipWriter: &ClipboardWriterAutoClear{
			clipper: clip.NewClipboard(),
		},
		osEnv:        os.Environ(),
		io:           io,
		newClient:    newClient,
		templateVars: make(map[string]string),
		fileMode:     filemode.New(0600),
	}
}

// Register adds a CommandClause and it's args and flags to a cli.App.
// Register adds args and flags.
func (cmd *InjectCommand) Register(r cli.Registerer) {
	clause := r.Command("inject", "Inject secrets into a template.")
	clause.Flags().BoolVarP(&cmd.useClipboard,
		"clip", "c", false,
		fmt.Sprintf(
			"Copy the injected template to the clipboard instead of stdout. The clipboard is automatically cleared after %s.",
			units.HumanDuration(clearClipboardAfter),
		))
	clause.Flags().StringVarP(&cmd.inFile, "in-file", "i", "", "The filename of a template file to inject.")
	clause.Flags().StringVarP(&cmd.outFile, "out-file", "o", "", "Write the injected template to a file instead of stdout.")
	clause.Flags().StringVar(&cmd.outFile, "file", "", "") // Alias of --out-file (for backwards compatibility)
	clause.Cmd.Flag("file").Hidden = true
	clause.Flags().Var(&cmd.fileMode, "file-mode", "Set filemode for the output file if it does not yet exist. It is ignored without the --out-file flag.")
	clause.Flags().StringToStringVarP(&cmd.templateVars, "var", "v", nil, "Define the value for a template variable with `VAR=VALUE`, e.g. --var env=prod")
	clause.Flags().StringVar(&cmd.templateVersion, "template-version", "auto", "Do not prompt when a template variable is missing and return an error instead.")
	clause.Flags().BoolVar(&cmd.dontPromptMissingTemplateVars, "no-prompt", false, "Do not prompt when a template variable is missing and return an error instead.")
	clause.Flags().BoolVarP(&cmd.force, "force", "f", false, "Overwrite the output file if it already exists, without prompting for confirmation. This flag is ignored if no --out-file is supplied.")

	clause.BindAction(cmd.Run)
	clause.BindArguments(nil)
}

// Run handles the command with the options as specified in the command.
func (cmd *InjectCommand) Run() error {
	if cmd.useClipboard && cmd.outFile != "" {
		return ErrFlagsConflict("--clip and --file")
	}

	var err error
	var raw []byte

	if cmd.inFile != "" {
		raw, err = os.ReadFile(cmd.inFile)
		if err != nil {
			return ErrReadFile(cmd.inFile, err)
		}
	} else {
		if !cmd.io.IsInputPiped() {
			return ErrNoDataOnStdin
		}

		raw, err = io.ReadAll(cmd.io.Input())
		if err != nil {
			return err
		}
	}

	osEnv, _ := parseKeyValueStringsToMap(cmd.osEnv)

	var templateVariableReader tpl.VariableReader
	templateVariableReader, err = newVariableReader(osEnv, cmd.templateVars)
	if err != nil {
		return err
	}

	if !cmd.dontPromptMissingTemplateVars {
		templateVariableReader = newPromptMissingVariableReader(templateVariableReader, cmd.io)
	}

	parser, err := getTemplateParser(raw, cmd.templateVersion)
	if err != nil {
		return err
	}

	template, err := parser.Parse(string(raw), 1, 1)
	if err != nil {
		return err
	}

	injected, err := template.Evaluate(templateVariableReader, newSecretReader(cmd.newClient))
	if err != nil {
		return err
	}

	out := []byte(injected)
	if cmd.useClipboard {
		err = cmd.clipWriter.Write(out)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(cmd.io.Output(), "Copied injected template to clipboard. It will be cleared after %s.\n", units.HumanDuration(clearClipboardAfter))
		if err != nil {
			return err
		}
	} else if cmd.outFile != "" {
		_, err := os.Stat(cmd.outFile)
		if err == nil && !cmd.force {
			if cmd.io.IsOutputPiped() {
				return ErrFileAlreadyExists
			}

			confirmed, err := ui.AskYesNo(
				cmd.io,
				fmt.Sprintf(
					"File %s already exists, overwrite it?",
					cmd.outFile,
				),
				ui.DefaultNo,
			)
			if err != nil {
				return err
			}

			if !confirmed {
				fmt.Fprintln(cmd.io.Output(), "Aborting.")
				return nil
			}
		}

		err = os.WriteFile(cmd.outFile, posix.AddNewLine(out), cmd.fileMode.FileMode())
		if err != nil {
			return ErrCannotWrite(cmd.outFile, err)
		}

		absPath, err := filepath.Abs(cmd.outFile)
		if err != nil {
			return ErrCannotWrite(err)
		}

		fmt.Fprintf(cmd.io.Output(), "%s\n", absPath)
	} else {
		fmt.Fprintf(cmd.io.Output(), "%s", posix.AddNewLine(out))
	}

	return nil
}
