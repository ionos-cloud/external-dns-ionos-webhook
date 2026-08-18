package ionoscloud

import (
	"context"

	ionoscloud_auth "github.com/ionos-cloud/sdk-go-auth"
)

type TokenProvider interface {
	GenerateToken(context.Context) (string, error)
}

type cachedTokenProvider struct {
	client      *ionoscloud_auth.APIClient
	cachedToken any
}

func NewCachedTokenProvider(username, password string) TokenProvider {
	return &cachedTokenProvider{client: ionoscloud_auth.NewAPIClient(ionoscloud_auth.NewConfiguration(username, password, "", ""))}
}

func (c *cachedTokenProvider) GenerateToken(ctx context.Context) (string, error) {
	// TODO: make TTL configurable
	token, _, err := c.client.TokensApi.TokensGenerate(ctx).Ttl(3600).Execute()
	if err != nil {
		return "", err
	}

	// TODO: parse token and store it

	return *token.Token, nil
}
