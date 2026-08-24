package ionoscloud

import (
	"context"
	"fmt"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	sdk "github.com/ionos-cloud/sdk-go-dns"
)

type clientFactory struct {
	tokenProvider TokenProvider
	ionosConfig   *ionos.Configuration
	apiClient     *sdk.APIClient
}

func newClientFactory(ionosConfig *ionos.Configuration) *clientFactory {
	return &clientFactory{
		tokenProvider: newCachedTokenProvider(ionosConfig),
		ionosConfig:   ionosConfig,
	}
}

func (cf *clientFactory) Create(ctx context.Context) (*sdk.APIClient, error) {
	if cf.ionosConfig.APIKey != "" {
		if cf.apiClient == nil {
			cf.initClient(cf.ionosConfig.APIKey)
		}
	} else {
		token, err := cf.tokenProvider.GetToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}
		if cf.apiClient == nil || cf.apiClient.GetConfig().Token != token {
			cf.initClient(token)
		}
	}

	return cf.apiClient, nil
}

func (cf *clientFactory) initClient(token string) {
	sdkConfig := sdk.NewConfiguration("", "", token, cf.ionosConfig.APIEndpointURL)
	sdkConfig.Debug = cf.ionosConfig.Debug
	cf.apiClient = sdk.NewAPIClient(sdkConfig)
}
