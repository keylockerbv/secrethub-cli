package secrethub

import (
	"net/url"

	"github.com/secrethub/secrethub-go/pkg/secrethub"
	"github.com/secrethub/secrethub-go/pkg/secrethub/credentials"
)

// ClientFactory handles creating a new client with the configured options.
type ClientFactory interface {
	// NewClient returns a new SecretHub client.
	NewClient() (*secrethub.Client, error)
	NewUnauthenticatedClient() (*secrethub.Client, error)
	Register(FlagRegisterer)
}

// NewClientFactory creates a new ClientFactory.
func NewClientFactory(store CredentialStore) ClientFactory {
	return &clientFactory{
		store: store,
	}
}

type clientFactory struct {
	client    *secrethub.Client
	ServerURL *url.URL
	UseAWS    bool
	store     CredentialStore
}

// Register the flags for configuration on a cli application.
func (f *clientFactory) Register(r FlagRegisterer) {
	r.Flag("api-remote", "The SecretHub API address, don't set this unless you know what you're doing.").Hidden().URLVar(&f.ServerURL)
	r.Flag("use-aws", "Use AWS credentials for authentication and account key decryption").BoolVar(&f.UseAWS)
}

// NewClient returns a new client that is configured to use the remote that
// is set with the flag.
func (f *clientFactory) NewClient() (*secrethub.Client, error) {
	if f.client == nil {
		var credentialProvider credentials.Provider
		if f.UseAWS {
			credentialProvider = credentials.UseAWS()
		} else {
			credentialProvider = f.store.Provider()
		}

		options := f.baseClientOptions()
		options = append(options, secrethub.WithCredentials(credentialProvider))

		client, err := secrethub.NewClient(options...)
		if err != nil {
			return nil, err
		}
		f.client = client
	}
	return f.client, nil
}

func (f *clientFactory) NewUnauthenticatedClient() (*secrethub.Client, error) {
	options := f.baseClientOptions()

	client, err := secrethub.NewClient(options...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (f *clientFactory) baseClientOptions() []secrethub.ClientOption {
	options := []secrethub.ClientOption{secrethub.WithConfigDir(f.store.ConfigDir())}

	if f.ServerURL != nil {
		options = append(options, secrethub.WithServerURL(f.ServerURL.String()))
	}
	return options
}
