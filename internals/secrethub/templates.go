package secrethub

var (
	// DefaultUsageTemplate is custom template for displaying usage
	// Changes in comparison to kingpin.DefaultUsageTemplate:
	// 1. Removed * for default commands
	DefaultUsageTemplate = `
{{define "FormatSubCommands"}}\
{{ $managementCommands := .Commands | ManagementCommands }}\
{{ $rootCommands := .Commands | RootCommands }}\

{{ if $managementCommands }}\
Management Commands:
{{ $managementCommands | CommandsToTwoColumns | FormatTwoColumns }}
{{ end }}\

{{ if $rootCommands }}\
{{ if $managementCommands }}\
Commands:
{{ end }}\
{{ $rootCommands | CommandsToTwoColumns | FormatTwoColumns }}
{{end}}\
{{end}}\

{{define "FormatAppUsage"}}\
{{if .FlagSummary}} {{.FlagSummary}}{{end}}\
{{range .Args}} {{if not .Required}}[{{end}}<{{.Name}}>{{if .Value|IsCumulative}}...{{end}}{{if not .Required}}]{{end}}{{end}}\
{{if .Commands}} <command> [<args> ...]{{end}}
{{if .Help}}
{{.Help}}\
{{end}}\

{{if .Flags}}

Flags:
{{.Flags|FlagsToTwoColumns|FormatTwoColumns}}
{{end}}\
{{if .Args}}\
Args:
{{.Args|ArgsToTwoColumns|FormatTwoColumns}}
{{end}}\

{{end}}\


{{define "FormatCommandUsage"}}\
{{if .FlagSummary}} {{.FlagSummary}}{{end}}\
{{range .Args}} {{if not .Required}}[{{end}}<{{.Name}}>{{if .Value|IsCumulative}}...{{end}}{{if not .Required}}]{{end}}{{end}}\
{{if .Commands}} <command> [<args> ...]{{end}}
{{if .HelpLong}}
{{.HelpLong}}\
{{ else if .Help}}
{{.Help}}\
{{end}}\

{{if .Flags}}

Flags:
{{.Flags|FlagsToTwoColumns|FormatTwoColumns}}
{{end}}\
{{if .Args}}\
Args:
{{.Args|ArgsToTwoColumns|FormatTwoColumns}}
{{end}}\

{{end}}\


{{if .Context.SelectedCommand}}\
usage: {{.App.Name}} {{.Context.SelectedCommand}}{{template "FormatCommandUsage" .Context.SelectedCommand}}
{{else}}\
usage: {{.App.Name}}{{template "FormatAppUsage" .App}}
{{end}}\
{{if .Context.SelectedCommand}}\
{{if len .Context.SelectedCommand.Commands}}\
Subcommands:
{{template "FormatSubCommands" .Context.SelectedCommand}}
{{end}}\
{{else if .App.Commands}}\
{{template "FormatSubCommands" .App}}
{{end}}\
{{if not .Context.SelectedCommand}}\
Run '{{ .App.Name }} <command> --help' for more information on a command.
{{ end }}\
`
)
