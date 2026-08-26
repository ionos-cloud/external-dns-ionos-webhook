package ionoscloud

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	ionoscloud_auth "github.com/ionos-cloud/sdk-go-auth"
)

var errTokenIsNilOrEmpty = errors.New("token is nil or empty")

type TokenProvider interface {
	GetToken(context.Context) (string, error)
}

type cachedTokenProvider struct {
	client      *ionoscloud_auth.APIClient
	jwtParser   *jwt.Parser
	cachedToken *jwt.Token
	tokenTTL    time.Duration
}

func newCachedTokenProvider(cfg *ionos.Configuration) TokenProvider {
	return &cachedTokenProvider{
		client:    ionoscloud_auth.NewAPIClient(ionoscloud_auth.NewConfiguration(cfg.Username, cfg.Password, "", cfg.AuthAPIEndpointURL)),
		jwtParser: jwt.NewParser(),
		tokenTTL:  cfg.TokenTTL,
	}
}

func (c *cachedTokenProvider) GetToken(ctx context.Context) (string, error) {
	if c.cachedToken == nil {
		if err := c.refresh(ctx); err != nil {
			return "", err
		}
	} else {
		// test if the token has expired
		expiry, err := c.cachedToken.Claims.GetExpirationTime()
		if err != nil {
			return "", fmt.Errorf("error checking token expiry: %w", err)
		}

		// the .Add(-1 * time.Minute) was suggested by copilot
		// to make sure the token does not expire between the checks
		// we substract a 1 minute as a safety window
		if expiry.Before(time.Now().Add(-1 * time.Minute)) {
			if err := c.refresh(ctx); err != nil {
				return "", err
			}
		}
	}

	return c.cachedToken.Raw, nil
}

func (c *cachedTokenProvider) refresh(ctx context.Context) error {
	tokenRawResponse, _, err := c.client.TokensApi.TokensGenerate(ctx).Ttl(int32(c.tokenTTL.Seconds())).Execute()
	if err != nil {
		return fmt.Errorf("failed to request token from IONOS Cloud API: %w", err)
	}

	if tokenRawResponse.Token == nil || *tokenRawResponse.Token == "" {
		return errTokenIsNilOrEmpty
	}

	c.cachedToken, _, err = c.jwtParser.ParseUnverified(*tokenRawResponse.Token, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("failed to process token: %w", err)
	}

	return nil
}
