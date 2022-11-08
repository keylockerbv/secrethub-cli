package secretspec_test

import (
	"os"
	"testing"

	"github.com/secrethub/secrethub-cli/internals/secretspec"

	"github.com/secrethub/secrethub-go/internals/api"
	"github.com/secrethub/secrethub-go/internals/assert"
)

var (
	testInjectConf = `
---
secrets:
   - inject:
      source: "test-config.json"
      target: "test-config-injected.json"
      filemode: "0644"
`

	testConfigJSONToInject = `{
		"field1": "${ danny/example-repo/test_secret }",
		"field2": "${ danny/example-repo/test_secret2 }"
	}`
	testConfigJSONExpected = `{
		"field1": "test_secret_content",
		"field2": "test_secret_content2"
	}`

	testSecret1 = api.SecretVersion{
		Secret: &api.Secret{
			Name: "test_secret",
		},
		Version: 0,
		Data:    []byte("test_secret_content"),
	}

	testSecret2 = api.SecretVersion{
		Secret: &api.Secret{
			Name: "test_secret2",
		},
		Version: 0,
		Data:    []byte("test_secret_content2"),
	}

	testRootPath = ""
)

func TestInjectSetClear(t *testing.T) {
	parsers := []secretspec.Parser{
		secretspec.InjectParser{},
	}

	presenter, err := secretspec.NewPresenter(testRootPath, false, parsers...)
	if err != nil {
		t.Fatalf("cannot create new Presenter: %s", err)
	}

	// write a config to inject
	err = os.WriteFile("test-config.json", []byte(testConfigJSONToInject), 0644)
	if err != nil {
		t.Fatalf("could not write test config to inject: %s", err)
	}
	defer func() {
		err := os.Remove("test-config.json")
		assert.OK(t, err)
	}()

	err = presenter.Parse([]byte(testInjectConf))
	if err != nil {
		t.Fatalf("cannot initialize Presenter: %s", err)
	}

	expected := map[string]api.SecretVersion{
		"danny/example-repo/test_secret":  testSecret1,
		"danny/example-repo/test_secret2": testSecret2,
	}

	err = presenter.Set(expected)
	if err != nil {
		t.Fatalf("cannot set presenter: %s", err)
	}

	actual, err := os.ReadFile("test-config-injected.json")
	if err != nil {
		t.Fatalf("cannot read from consumable file: %s", err)
	}

	if string(actual) != testConfigJSONExpected {
		t.Errorf(
			"unexpected consumable data:\n\t%s (actual) != %s (expected)",
			string(actual),
			testConfigJSONExpected,
		)
	}

	err = presenter.Clear()
	if err != nil {
		t.Errorf("cannot clear presenter: %s", err)
	}

	_, err = os.Stat("test-config-injected.json")
	if err == nil {
		t.Error("file still exists after presenter.Clear()")
	}
}
