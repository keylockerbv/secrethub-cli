package onepassword

import (
	"encoding/json"
	"fmt"
	"os"
)

type OPV2CLI struct{}

func (op *OPV2CLI) IsV2() bool {
	return true
}

func (op *OPV2CLI) CreateVault(name string) error {
	_, err := execOP("vault", "create", name)
	if err != nil {
		return fmt.Errorf("could not create vault '%s': %s", name, err)
	}
	return nil
}

func (op *OPV2CLI) CreateItem(vault string, template ItemTemplate, title string) error {
	jsonTemplate, err := json.Marshal(template)
	if err != nil {
		return err
	}

	tempJSONFile, err := os.CreateTemp(os.TempDir(), "jsonTemplate-")
	if err != nil {
		return err
	}
	defer os.Remove(tempJSONFile.Name())

	if _, err = tempJSONFile.Write(jsonTemplate); err != nil {
		return err
	}

	_, err = execOP("item", "create", "--category=apicredential", "--vault="+vault, "--template="+tempJSONFile.Name(), "--title="+title)
	if err != nil {
		return err
	}

	err = tempJSONFile.Close()
	return err
}

func (op *OPV2CLI) SetField(vault, item, field, value string) error {
	_, err := execOP("item", "edit", item, fmt.Sprintf(`%s=%s`, field, value), "--vault="+vault)
	if err != nil {
		return fmt.Errorf("could not set field '%s'.'%s'.'%s'", vault, item, field)
	}
	return nil
}

// GetFields returns a title-to-value map of the fields from the first section of the given 1Password item.
// The rest of the fields are ignored as the migration tool only stores information in the first
// section of each item.
func (op *OPV2CLI) GetFields(vault, item string) (map[string]string, error) {
	opItem := struct {
		Fields []v2ItemFieldTemplate `json:"fields"`
	}{}
	opItemJSON, err := execOP("item", "get", item, "--vault="+vault, "--format=json")
	if err != nil {
		return nil, fmt.Errorf("could not get item '%s'.'%s' from 1Password: %s", vault, item, err)
	}
	err = json.Unmarshal(opItemJSON, &opItem)
	if err != nil {
		return nil, fmt.Errorf("unexpected format of 1Password item in `op get item` command output: %s", err)
	}

	fields := make(map[string]string, len(opItem.Fields))
	for _, field := range opItem.Fields {
		fields[field.Label] = field.Value
	}
	return fields, nil
}

type v2ItemTemplate struct {
	Sections []v2SectionTemplate   `json:"sections"`
	Fields   []v2ItemFieldTemplate `json:"fields"`
}

type v2SectionTemplate struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type v2ItemFieldTemplate struct {
	ID      string            `json:"id"`
	Section v2SectionTemplate `json:"section"`
	Type    string            `json:"type"`
	Label   string            `json:"label"`
	Value   string            `json:"value"`
}

func (tpl *v2ItemTemplate) AddField(name, value string, concealed bool) {
	fieldType := "CONCEALED"
	if !concealed {
		fieldType = "STRING"
	}

	tpl.Fields = append(tpl.Fields, v2ItemFieldTemplate{
		ID:    name,
		Type:  fieldType,
		Label: name,
		Value: value,
	})
}

func (op *OPV2CLI) ExistsVault(vaultName string) (bool, error) {
	vaultsBytes, err := execOP("vault", "list", "--format=json")
	if err != nil {
		return false, fmt.Errorf("could not list vaults: %s", err)
	}

	vaultsJSON := make([]struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}, 0)

	err = json.Unmarshal(vaultsBytes, &vaultsJSON)
	if err != nil {
		return false, fmt.Errorf("unexpected format of `op list vaults`: %s", vaultsBytes)
	}

	for _, vault := range vaultsJSON {
		if vault.Name == vaultName {
			return true, nil
		}
	}

	return false, nil
}

func (op *OPV2CLI) ExistsItemInVault(vault string, itemName string) (bool, error) {
	itemsBytes, err := execOP("item", "list", "--vault="+vault, "--format=json")
	if err != nil {
		return false, fmt.Errorf("could not list items in vault %s: %s", vault, err)
	}

	itemsJSON := make([]struct {
		Title string `json:"title"`
	}, 0)

	err = json.Unmarshal(itemsBytes, &itemsJSON)
	if err != nil {
		return false, fmt.Errorf("unexpected format of `op list items`: %s", itemsBytes)
	}

	for _, item := range itemsJSON {
		if item.Title == itemName {
			return true, nil
		}
	}

	return false, nil
}
