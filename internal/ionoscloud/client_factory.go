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
}

func newClientFactory(ionosConfig *ionos.Configuration) *clientFactory {
	return &clientFactory{
		tokenProvider: newCachedTokenProvider(ionosConfig),
		ionosConfig:   ionosConfig,
	}
}

func (cf *clientFactory) Create(ctx context.Context) (*sdk.APIClient, error) {
	if cf.ionosConfig.APIKey != "" {
		sdkConfig := sdk.NewConfiguration("", "", cf.ionosConfig.APIKey, cf.ionosConfig.APIEndpointURL)
		sdkConfig.Debug = cf.ionosConfig.Debug
		return sdk.NewAPIClient(sdkConfig), nil
	}

	token, err := cf.tokenProvider.GenerateToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	sdkConfig := sdk.NewConfiguration("", "", token, cf.ionosConfig.APIEndpointURL)
	sdkConfig.Debug = cf.ionosConfig.Debug
	apiClient := sdk.NewAPIClient(sdkConfig)
	return apiClient, nil
}
