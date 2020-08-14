package secrethub

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/secretspec"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"

	"github.com/secrethub/secrethub-cli/internals/cli/validation"
	"github.com/secrethub/secrethub-cli/internals/secrethub/tpl"
	"github.com/secrethub/secrethub-go/internals/api"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type errNameCollision struct {
	name       string
	firstPath  string
	secondPath string
}

func (e errNameCollision) Error() string {
	return fmt.Sprintf("secrets at path %s and %s map to the same environment variable: %s. Rename one of the secrets or source them in a different way", e.firstPath, e.secondPath, e.name)
}

type environment struct {
	io                           ui.IO
	newClient                    newClientFunc
	osEnv                        []string
	readFile                     func(filename string) ([]byte, error)
	osStat                       func(filename string) (os.FileInfo, error)
	envar                        MapValue
	envFile                      string
	templateVars                 MapValue
	templateVersion              string
	dontPromptMissingTemplateVar bool
	secretsDir                   string
	secretsEnvDir                string
}

func newEnvironment(io ui.IO, newClient newClientFunc) *environment {
	return &environment{
		io:           io,
		newClient:    newClient,
		osEnv:        os.Environ(),
		readFile:     ioutil.ReadFile,
		osStat:       os.Stat,
		templateVars: MapValue{stringMap: make(map[string]string)},
		envar:        MapValue{stringMap: make(map[string]string)},
	}
}

func (env *environment) register(clause *cli.CommandClause) {
	clause.VarP(&env.envar, "envar", "e", "Source an environment variable from a secret at a given path with `NAME=<path>`", true, false)
	clause.StringVar(&env.envFile, "env-file", "", "The path to a file with environment variable mappings of the form `NAME=value`. Template syntax can be used to inject secrets.", true, false)
	clause.StringVar(&env.envFile, "template", "", "", true, false)
	clause.Flag("template").Hidden = true
	clause.VarP(&env.templateVars, "var", "v", "Define the value for a template variable with `VAR=VALUE`, e.g. --var env=prod", true, false)
	clause.StringVar(&env.templateVersion, "template-version", "auto", "The template syntax version to be used. The options are v1, v2, latest or auto to automatically detect the version.", true, false)
	_ = clause.RegisterFlagCompletionFunc("template-version", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"v1", "v2", "latest", "auto"}, cobra.ShellCompDirectiveDefault
	})
	clause.BoolVar(&env.dontPromptMissingTemplateVar, "no-prompt", false, "Do not prompt when a template variable is missing and return an error instead.", true, false)
	clause.StringVar(&env.secretsDir, "secrets-dir", "", "Recursively include all secrets from a directory. Environment variable names are derived from the path of the secret: `/` are replaced with `_` and the name is uppercased.", true, false)
	clause.StringVar(&env.secretsEnvDir, "env", "default", "The name of the environment prepared by the set command (default is `default`)", true, false)
	clause.Flag("env").Hidden = true
}

func (env *environment) env() (map[string]value, error) {
	osEnvMap, _ := parseKeyValueStringsToMap(env.osEnv)
	var sources []EnvSource

	sources = append(sources, &osEnv{
		osEnv: osEnvMap,
	})

	// .secretsenv dir (for backwards compatibility)
	envDir := filepath.Join(secretspec.SecretEnvPath, env.secretsEnvDir)
	_, err := os.Stat(envDir)
	if err == nil {
		dirSource, err := NewEnvDir(envDir)
		if err != nil {
			return nil, err
		}
		sources = append(sources, dirSource)
	}

	// --secrets-dir flag
	if env.secretsDir != "" {
		secretsDirEnv := newSecretsDirEnv(env.newClient, env.secretsDir)
		sources = append(sources, secretsDirEnv)
	}

	//secrethub.env file
	if env.envFile == "" {
		_, err := env.osStat(defaultEnvFile)
		if err == nil {
			env.envFile = defaultEnvFile
		} else if !os.IsNotExist(err) {
			return nil, ErrReadDefaultEnvFile(defaultEnvFile, err)
		}
	}

	if env.envFile != "" {
		templateVariableReader, err := newVariableReader(osEnvMap, env.templateVars.stringMap)
		if err != nil {
			return nil, err
		}

		if !env.dontPromptMissingTemplateVar {
			templateVariableReader = newPromptMissingVariableReader(templateVariableReader, env.io)
		}

		raw, err := env.readFile(env.envFile)
		if err != nil {
			return nil, ErrCannotReadFile(env.envFile, err)
		}

		parser, err := getTemplateParser(raw, env.templateVersion)
		if err != nil {
			return nil, err
		}

		envFile, err := ReadEnvFile(env.envFile, bytes.NewReader(raw), templateVariableReader, parser)
		if err != nil {
			return nil, err
		}
		sources = append(sources, envFile)
	}

	// secret references (secrethub://)
	referenceEnv := newReferenceEnv(osEnvMap)
	sources = append(sources, referenceEnv)

	// --envar flag
	// TODO: Validate the flags when parsing by implementing the Flag interface for EnvFlags.
	flagEnv, err := NewEnvFlags(env.envar.stringMap)
	if err != nil {
		return nil, err
	}
	sources = append(sources, flagEnv)

	envs := make([]map[string]value, len(sources))
	for _, source := range sources {
		env, err := source.env()
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}

	return mergeEnvs(envs...), nil
}

func mergeEnvs(envs ...map[string]value) map[string]value {
	result := map[string]value{}
	for _, env := range envs {
		for name, value := range env {
			result[name] = value
		}
	}
	return result
}

// EnvSource defines a method of reading environment variables from a source.
type EnvSource interface {
	// Env returns a map of key value pairs.
	env() (map[string]value, error)
}

type value interface {
	resolve(tpl.SecretReader) (string, error)
	containsSecret() bool
}

type secretValue struct {
	path string
}

func (s *secretValue) resolve(sr tpl.SecretReader) (string, error) {
	return sr.ReadSecret(s.path)
}

func (s *secretValue) containsSecret() bool {
	return true
}

func newSecretValue(path string) value {
	return &secretValue{path: path}
}

// secretsDirEnv sources environment variables from the directory specified with the --secrets-dir flag.
type secretsDirEnv struct {
	newClient newClientFunc
	dirPath   string
}

// env returns a map of environment variables containing all secrets from the specified path.
// The variable names are the relative paths of their corresponding secrets in uppercase snake case.
// An error is returned if two secret paths map to the same variable name.
func (s *secretsDirEnv) env() (map[string]value, error) {
	client, err := s.newClient()
	if err != nil {
		return nil, err
	}

	tree, err := client.Dirs().GetTree(s.dirPath, -1, false)
	if err != nil {
		return nil, err
	}

	paths := make(map[string]string, tree.SecretCount())
	for id := range tree.Secrets {
		secretPath, err := tree.AbsSecretPath(id)
		if err != nil {
			return nil, err
		}
		path := secretPath.String()

		envVarName := s.envVarName(path)
		if prevPath, found := paths[envVarName]; found {
			return nil, errNameCollision{
				name:       envVarName,
				firstPath:  prevPath,
				secondPath: path,
			}
		}
		paths[envVarName] = path
	}

	result := make(map[string]value, len(paths))
	for name, path := range paths {
		result[name] = newSecretValue(path)
	}
	return result, nil
}

// envVarName returns the environment variable name corresponding to the secret on the specified path
// by converting the relative path to uppercase snake case.
func (s *secretsDirEnv) envVarName(path string) string {
	envVarName := strings.TrimPrefix(path, s.dirPath)
	envVarName = strings.TrimPrefix(envVarName, "/")
	envVarName = strings.ReplaceAll(envVarName, "/", "_")
	envVarName = strings.ToUpper(envVarName)
	return envVarName
}

func newSecretsDirEnv(newClient newClientFunc, dirPath string) *secretsDirEnv {
	return &secretsDirEnv{
		newClient: newClient,
		dirPath:   dirPath,
	}
}

// EnvFlags defines environment variables sourced from command-line flags.
type EnvFlags map[string]string

// NewEnvFlags parses a map of flag values.
func NewEnvFlags(flags map[string]string) (EnvFlags, error) {
	for name, path := range flags {
		err := validation.ValidateEnvarName(name)
		if err != nil {
			return nil, err
		}

		err = api.ValidateSecretPath(path)
		if err != nil {
			return nil, err
		}
	}

	return flags, nil
}

// Env returns a map of environment variables sourced from
// command-line flags and set to their corresponding value.
func (ef EnvFlags) env() (map[string]value, error) {
	result := make(map[string]value)
	for name, path := range ef {
		result[name] = newSecretValue(path)
	}
	return result, nil
}

// referenceEnv is an environment with secrets configured with the
// secrethub:// syntax in the os environment variables.
type referenceEnv struct {
	envVars map[string]string
}

// newReferenceEnv returns an environment with secrets configured in the
// os environment with the secrethub:// syntax.
func newReferenceEnv(osEnv map[string]string) *referenceEnv {
	envVars := make(map[string]string)
	for key, value := range osEnv {
		if strings.HasPrefix(value, secretReferencePrefix) {
			envVars[key] = strings.TrimPrefix(value, secretReferencePrefix)
		}
	}
	return &referenceEnv{
		envVars: envVars,
	}
}

// Env returns a map of key value pairs with the secrets configured with the
// secrethub:// syntax.
func (env *referenceEnv) env() (map[string]value, error) {
	envVarsWithSecrets := make(map[string]value)
	for key, path := range env.envVars {
		envVarsWithSecrets[key] = newSecretValue(path)
	}
	return envVarsWithSecrets, nil
}

type envDirSecretValue struct {
	value string
}

func (s *envDirSecretValue) resolve(_ tpl.SecretReader) (string, error) {
	return s.value, nil
}

func (s *envDirSecretValue) containsSecret() bool {
	return true
}

func newEnvDirSecretValue(value string) value {
	return &envDirSecretValue{value: value}
}

// EnvDir defines environment variables sourced from files in a directory.
type EnvDir map[string]value

// NewEnvDir sources environment variables from files in a given directory,
// using the file name as key and contents as value.
func NewEnvDir(path string) (EnvDir, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, ErrReadEnvDir(err)
	}

	env := make(map[string]value)
	for _, f := range files {
		if !f.IsDir() {
			filePath := filepath.Join(path, f.Name())
			fileContent, err := ioutil.ReadFile(filePath)
			if err != nil {
				return nil, ErrReadEnvFile(f.Name(), err)
			}

			env[f.Name()] = newEnvDirSecretValue(string(fileContent))
		}
	}

	return env, nil
}

// Env returns a map of environment variables sourced from the directory.
func (dir EnvDir) env() (map[string]value, error) {
	return dir, nil
}

type templateValue struct {
	filepath  string
	template  tpl.Template
	varReader tpl.VariableReader
}

func (v *templateValue) resolve(sr tpl.SecretReader) (string, error) {
	value, err := v.template.Evaluate(v.varReader, sr)
	if err != nil {
		return "", ErrParsingTemplate(v.filepath, err)
	}
	return value, nil
}

func (v *templateValue) containsSecret() bool {
	return v.template.ContainsSecrets()
}

func newTemplateValue(filepath string, template tpl.Template, varReader tpl.VariableReader) value {
	return &templateValue{
		filepath:  filepath,
		template:  template,
		varReader: varReader,
	}
}

type envTemplate struct {
	filepath          string
	envVars           []envvarTpls
	templateVarReader tpl.VariableReader
}

type envvarTpls struct {
	key    tpl.Template
	value  tpl.Template
	lineNo int
}

// Env injects the given secrets in the environment values and returns
// a map of the resulting environment.
func (t envTemplate) env() (map[string]value, error) {
	result := make(map[string]value)
	for _, tpls := range t.envVars {
		key, err := tpls.key.Evaluate(t.templateVarReader, secretReaderNotAllowed{})
		if err != nil {
			return nil, err
		}

		err = validation.ValidateEnvarName(key)
		if err != nil {
			return nil, templateError(tpls.lineNo, err)
		}

		value := newTemplateValue(t.filepath, tpls.value, t.templateVarReader)

		result[key] = value
	}
	return result, nil
}

func templateError(lineNo int, err error) error {
	if lineNo > 0 {
		return ErrTemplate(lineNo, err)
	}
	return err
}

// ReadEnvFile reads and parses a .env file.
func ReadEnvFile(filepath string, reader io.Reader, varReader tpl.VariableReader, parser tpl.Parser) (EnvFile, error) {
	env, err := NewEnv(filepath, reader, varReader, parser)
	if err != nil {
		return EnvFile{}, ErrParsingTemplate(filepath, err)
	}
	return EnvFile{
		path:      filepath,
		envSource: env,
	}, nil
}

// EnvFile contains an environment that is read from a file.
type EnvFile struct {
	path      string
	envSource EnvSource
}

// Env returns a map of key value pairs read from the environment file.
func (e EnvFile) env() (map[string]value, error) {
	env, err := e.envSource.env()
	if err != nil {
		return nil, ErrParsingTemplate(e.path, err)
	}
	return env, nil
}

// NewEnv loads an environment of key-value pairs from a string.
// The format of the string can be `key: value` or `key=value` pairs.
func NewEnv(filepath string, r io.Reader, varReader tpl.VariableReader, parser tpl.Parser) (EnvSource, error) {
	env, err := parseEnvironment(r)
	if err != nil {
		return nil, err
	}

	secretTemplates := make([]envvarTpls, len(env))
	for i, envvar := range env {
		keyTpl, err := parser.Parse(envvar.key, envvar.lineNumber, envvar.columnNumberKey)
		if err != nil {
			return nil, err
		}

		err = validation.ValidateEnvarName(envvar.key)
		if err != nil {
			return nil, err
		}

		valTpl, err := parser.Parse(envvar.value, envvar.lineNumber, envvar.columnNumberValue)
		if err != nil {
			return nil, err
		}

		secretTemplates[i] = envvarTpls{
			key:    keyTpl,
			value:  valTpl,
			lineNo: envvar.lineNumber,
		}
	}

	return envTemplate{
		filepath:          filepath,
		envVars:           secretTemplates,
		templateVarReader: varReader,
	}, nil
}

type envvar struct {
	key               string
	value             string
	lineNumber        int
	columnNumberKey   int
	columnNumberValue int
}

// parseEnvironment parses envvars from a string.
// It first tries the key=value format. When that returns an error,
// the yml format is tried.
// The default parser to be used with the format is also returned.
func parseEnvironment(r io.Reader) ([]envvar, error) {
	var ymlReader bytes.Buffer
	env, err := parseDotEnv(io.TeeReader(r, &ymlReader))
	if err != nil {
		var ymlErr error
		env, ymlErr = parseYML(&ymlReader)
		if ymlErr != nil {
			return nil, err
		}
	}
	return env, nil
}

// parseDotEnv parses key-value pairs in the .env syntax (key=value).
func parseDotEnv(r io.Reader) ([]envvar, error) {
	vars := map[string]envvar{}
	scanner := bufio.NewScanner(r)

	i := 0
	for scanner.Scan() {
		i++
		line := scanner.Text()

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, ErrTemplate(i, errors.New("template is not formatted as key=value pairs"))
		}

		columnNumberValue := len(parts[0]) + 2 // the length of the key (including spaces and quotes) + one for the = sign and one for the current column.
		for _, r := range parts[1] {
			if !unicode.IsSpace(r) {
				break
			}
			columnNumberValue++
		}

		columnNumberKey := 1 // one for the current column.
		for _, r := range parts[0] {
			if !unicode.IsSpace(r) {
				break
			}
			columnNumberKey++
		}

		key := strings.TrimSpace(parts[0])

		value, isTrimmed := trimQuotes(strings.TrimSpace(parts[1]))
		if isTrimmed {
			columnNumberValue++
		}

		vars[key] = envvar{
			key:               key,
			value:             value,
			lineNumber:        i,
			columnNumberValue: columnNumberValue,
			columnNumberKey:   columnNumberKey,
		}
	}

	i = 0
	res := make([]envvar, len(vars))
	for _, envvar := range vars {
		res[i] = envvar
		i++
	}

	return res, nil
}

const (
	doubleQuoteChar = '\u0022' // "
	singleQuoteChar = '\u0027' // '
)

// trimQuotes removes a leading and trailing quote from the given string value if
// it is wrapped in either single or double quotes.
//
// Rules:
// - Empty values become empty values (e.g. `''`and `""` both evaluate to the empty string ``).
// - Inner quotes are maintained (e.g. `{"foo":"bar"}` remains unchanged).
// - Single and double quoted values are escaped (e.g. `'foo'` and `"foo"` both evaluate to `foo`).
// - Single and double qouted values maintain whitespace from both ends (e.g. `" foo "` becomes ` foo `)
// - Inputs with either leading or trailing whitespace are considered unquoted,
//   so make sure you sanitize your inputs before calling this function.
func trimQuotes(s string) (string, bool) {
	n := len(s)
	if n > 1 &&
		(s[0] == singleQuoteChar && s[n-1] == singleQuoteChar ||
			s[0] == doubleQuoteChar && s[n-1] == doubleQuoteChar) {
		return s[1 : n-1], true
	}

	return s, false
}

func parseYML(r io.Reader) ([]envvar, error) {
	contents, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	pairs := make(map[string]string)
	err = yaml.Unmarshal(contents, pairs)
	if err != nil {
		return nil, err
	}

	vars := make([]envvar, len(pairs))
	i := 0
	for key, value := range pairs {
		vars[i] = envvar{
			key:        key,
			value:      value,
			lineNumber: -1,
		}
		i++
	}
	return vars, nil
}

type plaintextValue struct {
	value string
}

func newPlaintextValue(value string) *plaintextValue {
	return &plaintextValue{value: value}
}

func (v *plaintextValue) resolve(_ tpl.SecretReader) (string, error) {
	return v.value, nil
}

func (v *plaintextValue) containsSecret() bool {
	return false
}

type osEnv struct {
	osEnv map[string]string
}

func (o *osEnv) env() (map[string]value, error) {
	res := map[string]value{}
	for name, value := range o.osEnv {
		res[name] = newPlaintextValue(value)
	}
	return res, nil
}

type MapValue struct {
	stringMap map[string]string
}

func (m MapValue) String() string {
	textRepresentation := ""
	for k, v := range m.stringMap {
		textRepresentation += k + "=" + v + ";"
	}
	return textRepresentation
}

//TODO treat the case when the array does not contain exactly 2 elements
func (m MapValue) Set(s string) error {
	arr := strings.Split(s, "=")
	m.stringMap[arr[0]] = arr[1]
	return nil
}

func (m MapValue) Type() string {
	return "mapValue"
}
