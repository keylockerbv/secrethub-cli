package secrethub

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/secrethub/secrethub-cli/internals/cli"
	"github.com/secrethub/secrethub-cli/internals/cli/filemode"
	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/onepassword"
	"github.com/secrethub/secrethub-go/internals/api"
	"github.com/secrethub/secrethub-go/internals/api/uuid"
	"github.com/secrethub/secrethub-go/pkg/secrethub"
	"github.com/secrethub/secrethub-go/pkg/secrethub/iterator"
	"github.com/secrethub/secrethub-go/pkg/secretpath"

	"gopkg.in/yaml.v2"
)

const (
	defaultPlanPath = "./1password-migration-plan.yml"
)

func newPlan() *plan {
	return &plan{
		dirByVaultName: make(map[string]string),
		vaults:         make(map[string]*vault),
	}
}

type plan struct {
	SignInAddress  string
	dirByVaultName map[string]string
	vaults         map[string]*vault
}

type referenceMapping map[string]string

func newReferenceMapping(p *plan) referenceMapping {
	index := make(map[string]string)
	for _, vault := range p.vaults {
		for _, item := range vault.Items {
			for _, field := range item.Fields {
				opPath := fmt.Sprintf("op://%s/%s/%s", vault.Name, item.Name, field.Name)
				index[field.Reference] = opPath
			}
		}
	}
	return referenceMapping(index)
}

// addVarPossibilities adds variations to the index for all values in the passed in vars map
func (m referenceMapping) addVarPossibilities(vars map[string][]string) error {
	exists := make(map[string]string)
	for varname, possibleValues := range vars {
		varname = strings.ToUpper(varname)
		for _, value := range possibleValues {
			if otherVarname := exists[value]; otherVarname != "" && otherVarname != varname {
				return fmt.Errorf("you've ran into a limitation of the migration tool. You can't have multiple variables with the same value: '%s' now occurs in both '%s' and '%s'", value, varname, otherVarname)
			}
			exists[value] = varname
		}
	}

	for varname, possibleValues := range vars {
		uppercaseVarname := strings.ToUpper(varname)

		for _, value := range possibleValues {
			for secrethubRef, opRef := range m {
				if strings.Contains(secrethubRef, value) && strings.Contains(opRef, value) {
					// Add syntax variations to the index
					variations := map[string]string{
						"$" + varname:                 "$" + uppercaseVarname,
						"$" + uppercaseVarname:        "$" + uppercaseVarname,
						"${" + varname + "}":          "${" + uppercaseVarname + "}",
						"${" + uppercaseVarname + "}": "${" + uppercaseVarname + "}",
					}
					for secretHubVariation, opVariation := range variations {
						m[strings.ReplaceAll(secrethubRef, value, secretHubVariation)] = strings.ReplaceAll(opRef, value, opVariation)
					}
				}
			}
		}
	}

	return nil
}

// stripSecretHubURIScheme removes the secrethub:// prefix from the index keys so it can be
// used for secrethub.env files and config file templates.
func (m referenceMapping) stripSecretHubURIScheme() {
	for secrethubRef, opRef := range m {
		stripped := strings.TrimPrefix(secrethubRef, secretReferencePrefix)
		delete(m, secrethubRef)
		m[stripped] = opRef
	}
}

type vault struct {
	Name  string `yaml:"vault-name"`
	Items []item
}

func (v vault) Validate() error {
	for _, item := range v.Items {
		err := item.Validate()
		if err != nil {
			return fmt.Errorf("item '%s': %s", item.Name, err)
		}
	}
	return nil
}

type item struct {
	Name   string `yaml:"item-name"`
	Fields []field
}

func (i item) Validate() error {
	for _, field := range i.Fields {
		err := field.Validate()
		if err != nil {
			return fmt.Errorf("field '%s': %s", field.Name, err)
		}
	}
	return nil
}

type field struct {
	Name      string `yaml:"field-name"`
	Reference string `yaml:"value"` // Path to a SecretHub secret which value to use for this field. The used format is secrethub://
	Concealed bool
}

func (f field) Validate() error {
	if !strings.HasPrefix(f.Reference, secretReferencePrefix) {
		return fmt.Errorf("value: expected value to be a reference to a SecretHub secret (starting with secrethub://)")
	}
	err := api.ValidateSecretPath(strings.TrimPrefix(f.Reference, secretReferencePrefix))
	if err != nil {
		return fmt.Errorf("value: '%s' is not a valid secret path", f.Reference)
	}

	return nil
}

func (p *plan) addVault(tree *api.Tree, dirID uuid.UUID) (string, error) {
	path, err := tree.AbsDirPath(dirID)
	if err != nil {
		return "", err
	}

	// Drop the namespace from the vault name and replace separators between repo and directories with dashes.
	vaultName := strings.ReplaceAll(strings.SplitN(path.Value(), "/", 2)[1], "/", "-")

	_, exists := p.vaults[vaultName]
	if !exists {
		p.vaults[vaultName] = &vault{
			Name: vaultName,
		}
		p.dirByVaultName[vaultName] = path.Value()
	} else {
		if p.dirByVaultName[vaultName] != path.Value() {
			return "", fmt.Errorf("'%s' and '%s' both resolve to the same vault name: %s", p.dirByVaultName[vaultName], path.Value(), vaultName)
		}
	}
	return vaultName, nil
}

func (p *plan) addItem(vaultName, name string, fields []field) {
	vault := p.vaults[vaultName]
	vault.Items = append(vault.Items, item{
		Name:   name,
		Fields: fields,
	})
}

type planYML struct {
	SignInAddress string `yaml:"sign-in-address"`
	Vaults        []*vault
}

func (p *plan) MarshalYAML() (interface{}, error) {
	res := planYML{
		SignInAddress: p.SignInAddress,
		Vaults:        make([]*vault, len(p.vaults)),
	}

	i := 0
	for _, vault := range p.vaults {
		res.Vaults[i] = vault
		i++
	}

	return res, nil
}

func (p *plan) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var yml planYML
	err := unmarshal(&yml)
	if err != nil {
		return err
	}

	p.SignInAddress = yml.SignInAddress

	p.vaults = make(map[string]*vault, len(yml.Vaults))
	for _, vault := range yml.Vaults {
		p.vaults[vault.Name] = vault
	}

	return nil
}

func (p *plan) Validate() error {
	for _, vault := range p.vaults {
		err := vault.Validate()
		if err != nil {
			return fmt.Errorf("vault '%s': %s", vault.Name, err)
		}
	}
	return nil
}

func (cmd *MigratePlanCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	err = onepassword.EnsureSignedIn()
	if err != nil {
		return err
	}

	plan := newPlan()

	opClient, err := onepassword.GetOPClient()
	if err != nil {
		return err
	}

	if !opClient.IsV2() {
		signInAddress, err := onepassword.GetSignInAddress()
		if err != nil {
			return err
		}
		plan.SignInAddress = signInAddress
	}

	if len(cmd.paths) == 0 {
		err := cmd.addReposToPlan(client, nil, plan)
		if err != nil {
			return err
		}
	}
	for _, path := range cmd.paths {
		path = secretpath.Clean(path)
		if secretpath.Count(path) >= 2 {
			err = cmd.addDirToPlan(client, path, plan)
			if err != nil {
				return err
			}
		} else {
			me, err := client.Accounts().Me()
			if err != nil {
				return err
			}
			if !strings.EqualFold(me.Name.String(), path) {
				orgMember, err := client.Orgs().Members().Get(path, me.Name.Value())
				if err != nil {
					return err
				}
				if orgMember.Role != api.OrgRoleAdmin {
					fmt.Fprintf(os.Stderr, "WARN: You are not an admin on %s. There may be repositories you do not have access to. Ask an admin to verify all secrets are included in the migration.\n", path)
				}
			}

			err = cmd.addReposToPlan(client, &secrethub.RepoIteratorParams{Namespace: &path}, plan)
			if err != nil {
				return err
			}
		}
	}

	out, err := yaml.Marshal(plan)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(cmd.outFile, out, cmd.fileMode.FileMode())
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.io.Stdout())
	fmt.Fprintf(cmd.io.Stdout(), "Plan complete and written to: %s\n", cmd.outFile)
	fmt.Fprintf(cmd.io.Stdout(), "You can edit the plan to your preferences. When you are satisfied, run the migration with:\n")
	fmt.Fprintf(cmd.io.Stdout(), "    secrethub migrate apply --plan-file=%s\n", cmd.outFile)

	return nil
}

func (cmd *MigratePlanCommand) addReposToPlan(client secrethub.ClientInterface, params *secrethub.RepoIteratorParams, plan *plan) error {
	iter := client.Repos().Iterator(params)
	for {
		repo, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		err = cmd.addDirToPlan(client, repo.Path().Value(), plan)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cmd *MigratePlanCommand) addDirToPlan(client secrethub.ClientInterface, path string, plan *plan) error {
	fmt.Fprintf(cmd.io.Output(), "Planning migration for %s\n", path)

	tree, err := client.Dirs().GetTree(path, -1, false)
	if err == api.ErrForbidden || api.IsErrNotFound(err) {
		fmt.Fprintf(os.Stderr, "WARN: Skipping '%s' because you do not have read access. ", path)
		accessLevels, err := client.AccessRules().ListLevels(path)
		if err == nil {
			var usernames []string
			for _, level := range accessLevels {
				if level.Account.AccountType == "user" {
					usernames = append(usernames, level.Account.Name.String())
				}
			}
			fmt.Fprintf(os.Stderr, "Ask any of the following users to migrate the skipped secrets: %s.\n", strings.Join(usernames, ", "))
		} else {
			fmt.Fprint(os.Stderr, "Ask an admin to migrate the skipped secrets.\n")
		}
		return nil
	}
	if err != nil {
		return err
	}

	err = addTreeToPlan(tree, plan)
	if err != nil {
		return err
	}
	return nil
}

func addTreeToPlan(tree *api.Tree, plan *plan) error {
	return walkTree(tree, func(dir *api.Dir) error {
		if len(dir.Secrets) == 0 {
			return nil
		}

		if dir.ParentID != nil && isSecretItem(dir) {
			vault, err := plan.addVault(tree, *dir.ParentID)
			if err != nil {
				return err
			}
			fields := make([]field, len(dir.Secrets))
			for i, secret := range dir.Secrets {
				secretPath, err := tree.AbsSecretPath(secret.SecretID)
				if err != nil {
					return err
				}

				fields[i] = field{
					Name:      secret.Name,
					Reference: secretReferencePrefix + secretPath.Value(),
					Concealed: shouldBeConcealed(secretpath.Base(secretPath.Value())),
				}
			}
			plan.addItem(vault, dir.Name, fields)
		} else {
			vault, err := plan.addVault(tree, dir.DirID)
			if err != nil {
				return err
			}
			for _, secret := range dir.Secrets {
				secretPath, err := tree.AbsSecretPath(secret.SecretID)
				if err != nil {
					return err
				}
				plan.addItem(vault, secret.Name, []field{{Name: "secret", Reference: secretReferencePrefix + secretPath.Value(), Concealed: true}})
			}
		}

		return nil
	})
}

func shouldBeConcealed(secretName string) bool {
	for _, specialSecretName := range []string{
		"user", "username",
		"host", "hostname", "port",
		"name",
		"access-key-id", "client-id", "kms-key-id", "source-id",
		"public.pgp", "fingerprint.pgp",
	} {
		if strings.EqualFold(strings.ReplaceAll(secretName, "_", "-"), specialSecretName) {
			return false
		}
	}
	return true
}

// isSecretItem returns whether the directory itself should be interpreted as a secret item,
// rather than the secrets that are in the directory.
func isSecretItem(dir *api.Dir) bool {
	if len(dir.SubDirs) > 0 {
		return false
	}
	if len(dir.Secrets) < 2 {
		return true
	}
	for _, secret := range dir.Secrets {
		if !shouldBeConcealed(secret.Name) {
			return true
		}

		for _, specialSecretName := range []string{
			"password", "pass", "passphrase",
			"secret-key", "access-key", "secret-access-key", "access-token", "secret-access-token",
			"client-secret",
			"api-key", "api-secret",
			"token",
			"credential", "credential-file", "service-credential",
			"credentials.json",
			"write-key",
			"private.pgp",
		} {
			if strings.EqualFold(strings.ReplaceAll(secret.Name, "_", "-"), specialSecretName) {
				return true
			}
		}
	}
	return false
}

func walkTree(tree *api.Tree, fn func(*api.Dir) error) error {
	return walkTreeRec(tree.RootDir, fn)
}

func walkTreeRec(dir *api.Dir, fn func(*api.Dir) error) error {
	err := fn(dir)
	if err != nil {
		return err
	}
	for _, subDir := range dir.SubDirs {
		err := walkTreeRec(subDir, fn)
		if err != nil {
			return err
		}
	}

	return nil
}

type change interface {
	Vault() string
	Apply() error
	Print(w io.Writer)
}

type vaultCreation struct {
	vault    string
	opClient onepassword.OPCLI
}

func (c vaultCreation) Vault() string {
	return c.vault
}

func (c vaultCreation) Apply() error {
	return c.opClient.CreateVault(c.vault)
}

func (c vaultCreation) Print(w io.Writer) {
	fmt.Fprintf(w, "Create vault '%s'\n", c.vault)
}

type itemCreation struct {
	vault        string
	item         string
	itemTemplate onepassword.ItemTemplate
	opClient     onepassword.OPCLI
}

func (c itemCreation) Vault() string {
	return c.vault
}

func (c itemCreation) Apply() error {
	return c.opClient.CreateItem(c.vault, c.itemTemplate, c.item)
}

func (c itemCreation) Print(w io.Writer) {
	fmt.Fprintf(w, "Create item '%s'\n", c.item)
}

type itemUpdate struct {
	vault       string
	item        string
	fieldValues map[string]string
	opClient    onepassword.OPCLI
}

func (c itemUpdate) Vault() string {
	return c.vault
}

func (c itemUpdate) Apply() error {
	for field, value := range c.fieldValues {
		err := c.opClient.SetField(c.vault, c.item, field, value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c itemUpdate) Print(w io.Writer) {
	fmt.Fprintf(w, "Update item '%s' fields:\n", c.item)
	for field := range c.fieldValues {
		fmt.Fprintf(w, "  '%s'\n", field)
	}
}

func (cmd *MigrateApplyCommand) Run() error {
	plan, err := getPlan(cmd.planFile)
	if err != nil {
		return err
	}

	opClient, err := onepassword.GetOPClient()
	if err != nil {
		return err
	}

	if !opClient.IsV2() {
		err = onepassword.EnsureSignedIn()
		if err != nil {
			return err
		}

		signInAddress, err := onepassword.GetSignInAddress()
		if err != nil {
			return err
		}
		if signInAddress != plan.SignInAddress {
			return fmt.Errorf("op is signed in to a different account than planned. Run `eval $(op signin %s) to login to the desired account or change the sign-in-address in the plan", plan.SignInAddress)
		}
	}

	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	vaultCreateCount := 0
	itemCreateCount := 0
	fieldUpdateCount := 0
	warningCount := 0
	skipCount := 0

	var changes []change

	i := 1
	for _, vault := range plan.vaults {
		fmt.Fprintf(cmd.io.Output(), "[%d/%d] Checking vault: %s\n", i, len(plan.vaults), vault.Name)
		vaultExists, err := opClient.ExistsVault(vault.Name)
		if err != nil {
			return fmt.Errorf("could not check vault existence: %s", err)
		}
		if !vaultExists {
			changes = append(changes, vaultCreation{
				vault:    vault.Name,
				opClient: opClient,
			})
			vaultCreateCount++
		}

		for _, item := range vault.Items {
			itemExists := false
			if vaultExists {
				itemExists, err = opClient.ExistsItemInVault(vault.Name, item.Name)
				if err != nil {
					return err
				}
			}

			if !itemExists {
				template := onepassword.NewItemTemplate(opClient)
				for _, field := range item.Fields {
					value, err := client.Secrets().ReadString(strings.TrimPrefix(field.Reference, secretReferencePrefix))
					if err != nil {
						return err
					}
					template.AddField(field.Name, value, field.Concealed)
				}

				changes = append(changes, itemCreation{
					vault:        vault.Name,
					item:         item.Name,
					itemTemplate: template,
					opClient:     opClient,
				})
				itemCreateCount++
			} else {
				opFields, err := opClient.GetFields(vault.Name, item.Name)
				if err != nil {
					return err
				}
				fieldsToUpdate := map[string]string{}
				for _, field := range item.Fields {
					opValue, hasField := opFields[field.Name]
					if !hasField {
						fmt.Fprintf(os.Stderr, "item %s.%s has missing field %s, please add this field manually to allow the migration tool to update it\n", vault.Name, item.Name, field.Name)
						warningCount++
						skipCount++
						continue
					}

					value, err := client.Secrets().ReadString(strings.TrimPrefix(field.Reference, secretReferencePrefix))
					if err != nil {
						return err
					}
					if value != opValue {
						fieldsToUpdate[field.Name] = value
						fieldUpdateCount++
					}
				}
				if len(fieldsToUpdate) > 0 {
					changes = append(changes, itemUpdate{
						vault:       vault.Name,
						item:        item.Name,
						fieldValues: fieldsToUpdate,
						opClient:    opClient,
					})
				}
			}
		}
		i++
	}

	fmt.Fprintln(cmd.io.Output())
	if len(changes) == 0 {
		fmt.Fprintln(cmd.io.Output(), "Already up to date.")
		return nil
	}
	fmt.Fprintln(cmd.io.Output(), "Detected changes to be made:")
	indentedWriter := indentedWriter{
		w: cmd.io.Output(),
	}
	lastVault := ""
	for _, change := range changes {
		if change.Vault() != lastVault {
			fmt.Fprintf(cmd.io.Output(), "Vault %s:\n", change.Vault())
			lastVault = change.Vault()
		}
		change.Print(indentedWriter)
	}

	fmt.Fprintln(cmd.io.Output())
	fmt.Fprintln(cmd.io.Output(), "Summary:")
	if vaultCreateCount != 0 {
		fmt.Fprintf(cmd.io.Output(), "%d vaults will be created\n", vaultCreateCount)
	}
	if itemCreateCount != 0 {
		fmt.Fprintf(cmd.io.Output(), "%d items will be created\n", itemCreateCount)
	}
	if fieldUpdateCount != 0 {
		fmt.Fprintf(cmd.io.Output(), "%d fields will be updated\n", fieldUpdateCount)
	}

	fmt.Fprintln(cmd.io.Output())
	fmt.Fprint(cmd.io.Output(), "Note: Adding secrets to an existing vault will make them accessible to everyone who has read permission on the vault.\n")

	if !cmd.update {
		fmt.Fprintln(cmd.io.Output())
		confirmed, err := ui.AskYesNo(cmd.io, "Would you like to apply these changes?", ui.DefaultNo)
		if err != nil {
			return errors.New("error prompting for confirmation. Run the command again with --update to skip this prompt")
		}
		if !confirmed {
			fmt.Fprintln(cmd.io.Output(), "Aborting...")
			return nil
		}
	}

	fmt.Fprintln(cmd.io.Output())
	fmt.Fprintf(cmd.io.Output(), "Applying changes:\n")
	for i, change := range changes {
		fmt.Fprintf(cmd.io.Output(), "[%d/%d]\n", i+1, len(changes))
		err := change.Apply()
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(cmd.io.Output(), "\n"+
		"Migration completed successfully.\n"+
		"Your secrets are now available via 1Password.\n"+
		"Learn how to load them using any of the integrations at https://secrethub.io/docs/1password/migration/#integrations")
	return nil
}

type indentedWriter struct {
	w io.Writer
}

func (w indentedWriter) Write(p []byte) (n int, err error) {
	return w.w.Write(append([]byte{' ', ' '}, p...))
}

func getPlan(planFile string) (*plan, error) {
	contents, err := ioutil.ReadFile(planFile)
	if err != nil {
		return nil, err
	}

	var plan plan
	err = yaml.Unmarshal(contents, &plan)
	if err != nil {
		return nil, fmt.Errorf("plan at '%s' is not valid: could not parse as yaml: %s", planFile, err)
	}

	err = plan.Validate()
	if err != nil {
		return nil, fmt.Errorf("plan at '%s' is not valid: %s", planFile, err)
	}

	return &plan, nil
}

type MigrateCommand struct {
	io        ui.IO
	newClient newClientFunc
}

func NewMigrateCommand(io ui.IO, newClient newClientFunc) *MigrateCommand {
	return &MigrateCommand{
		io:        io,
		newClient: newClient,
	}
}

func (cmd *MigrateCommand) Register(r cli.Registerer) {
	clause := r.Command("migrate", "Migrate your secrets to 1Password.")
	clause.HelpLong("Check out https://secrethub.io/docs/1password/migration/ for detailed instructions.")

	NewMigratePlanCommand(cmd.io, cmd.newClient).Register(clause)
	NewMigrateApplyCommand(cmd.io, cmd.newClient).Register(clause)

	NewMigrateConfigCommand(cmd.io).Register(clause)
}

type MigratePlanCommand struct {
	io        ui.IO
	newClient newClientFunc

	outFile  string
	fileMode filemode.FileMode
	paths    cli.StringListValue
}

func NewMigratePlanCommand(io ui.IO, newClient newClientFunc) *MigratePlanCommand {
	return &MigratePlanCommand{
		io:        io,
		newClient: newClient,

		fileMode: filemode.New(0600),
	}
}

func (cmd *MigratePlanCommand) Register(r cli.Registerer) {
	clause := r.Command("plan", "Generate a migration plan file.")
	clause.HelpLong("Generate a YAML file to specify which 1Password vaults and items will be used to store your secrets." +
		" You can review and edit this plan, then apply it with `secrethub migrate apply`.\n" +
		"\n" +
		"Check out https://secrethub.io/docs/1password/migration/ for detailed instructions.")

	clause.Flags().StringVar(&cmd.outFile, "out-file", defaultPlanPath, "The path where to write the YAML file.")
	clause.Flags().Var(&cmd.fileMode, "file-mode", "Set file mode for the output file.")

	clause.BindArgumentsArr(cli.Argument{Value: &cmd.paths, Name: "path", Required: false, Description: "Migrate only secrets in these paths."})

	clause.BindAction(cmd.Run)
}

type MigrateApplyCommand struct {
	io        ui.IO
	newClient newClientFunc

	planFile string
	update   bool
}

func NewMigrateApplyCommand(io ui.IO, newClient newClientFunc) *MigrateApplyCommand {
	return &MigrateApplyCommand{
		io:        io,
		newClient: newClient,
	}
}

func (cmd *MigrateApplyCommand) Register(r cli.Registerer) {
	clause := r.Command("apply", "Execute the planned migration.")
	clause.HelpLong("Create the vaults and items specified in the YAML plan file." +
		" You can generate a plan file using `secrethub migrate plan`.\n" +
		"\n" +
		"Check out https://secrethub.io/docs/1password/migration/ for detailed instructions.")

	clause.Flags().StringVar(&cmd.planFile, "plan-file", defaultPlanPath, "Path to the YAML file specifying what vaults and items to create.")
	clause.Flags().BoolVar(&cmd.update, "update", false, "Perform migration without prompting for confirmation.")

	clause.BindAction(cmd.Run)
}
