package secrethub

import (
	"net/url"

	"github.com/secrethub/secrethub-go/internals/auth"
	"github.com/secrethub/secrethub-go/pkg/secrethub"
)

func newClientFactory(store CredentialStore) *clientFactory {
	return &clientFactory{
		store: store,
	}
}

type clientFactory struct {
	client    secrethub.Client
	ServerURL *url.URL
	store     CredentialStore
}

// Register the flags for configuration on a cli application.
func (f *clientFactory) Register(r FlagRegisterer) {
	r.Flag("api-remote", "The SecretHub API address, don't set this unless you know what you're doing.").Hidden().URLVar(&f.ServerURL)
}

// NewClient returns a new client that is configured to use the remote that
// is set with the flag.
func (f *clientFactory) NewClient() (secrethub.Client, error) {
	if f.client == nil {
		credential, err := f.store.Get()
		if err != nil {
			return nil, err
		}

		f.client = secrethub.NewClient(credential, auth.NewHTTPSigner(credential), f.NewClientOptions())
	}
	return f.client, nil
}

func (f *clientFactory) newUnauthenticatedClient() (secrethub.Client, error) {
	return secrethub.NewClient(nil, nil, f.NewClientOptions()), nil
}

// NewClientOptions returns the client options configured by the flags.
func (f *clientFactory) NewClientOptions() *secrethub.ClientOptions {
	var opts secrethub.ClientOptions

	if f.ServerURL != nil {
		opts.ServerURL = f.ServerURL.String()
	}
	return &opts
}
