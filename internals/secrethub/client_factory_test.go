package secrethub

import (
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/secrethub/secrethub-go/internals/assert"
	"github.com/secrethub/secrethub-go/pkg/secrethub/credentials"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
)

func TestNewClientFactory_ProxyAddress(t *testing.T) {
	proxyAddress, err := url.Parse("http://127.0.0.1:15555")
	assert.OK(t, err)

	proxyReceivedRequest := false
	go func() {
		err = http.ListenAndServe(proxyAddress.Hostname()+":"+proxyAddress.Port(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyReceivedRequest = true
		}))
		if err != http.ErrServerClosed && err != nil {
			t.Errorf("http server error: %s", err)
		}
	}()

	// Check if the configuration option takes precedence over the global HTTP_PROXY environment variable
	os.Setenv("HTTP_PROXY", "http://test.unknown")

	// Make sure the actual API is not reached if proxying fails
	serverAddress, err := url.Parse("http://test.unknown")
	assert.OK(t, err)

	io := ui.NewUserIO()
	store := NewCredentialConfig(io)
	factory := clientFactory{
		identityProvider: "key",
		store:            store,
		ServerURL:        urlValue{serverAddress},
		proxyAddress:     urlValue{proxyAddress},
	}

	client, err := factory.NewUnauthenticatedClient()
	assert.OK(t, err)

	_, _ = client.Users().Create("test", "test@test.test", "test", credentials.CreateKey())
	assert.OK(t, err)
	assert.Equal(t, proxyReceivedRequest, true)
}
