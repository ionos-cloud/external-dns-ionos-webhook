package ionoscloud

import (
	"context"
	"errors"
	"testing"

	"github.com/ionos-cloud/external-dns-ionos-webhook/internal/ionos"
	"github.com/stretchr/testify/assert"
)

type mockTokenProvider struct {
	calls int
	token string
	err   error
}

func (tp *mockTokenProvider) GenerateToken(ctx context.Context) (string, error) {
	tp.calls++
	return tp.token, tp.err
}

func TestCreate(t *testing.T) {
	token := "token"
	generatedToken := "generatedToken"
	for _, tc := range []struct {
		name                       string
		givenConfig                *ionos.Configuration
		givenGenerateTokenError    error
		givenGeneratedToken        string
		expectedError              bool
		expectedErrorMessage       string
		expectedClientToken        string
		expectedGenerateTokenCalls int
	}{
		{
			name: "configuration with token does not call generate token -> happy path",
			givenConfig: &ionos.Configuration{
				APIKey: token,
			},
			expectedClientToken:        token,
			expectedGenerateTokenCalls: 0,
		},
		{
			name: "configuration with no token calls generate token -> generate token error",
			givenConfig: &ionos.Configuration{
				Username: "username",
				Password: "password",
			},
			givenGenerateTokenError:    errors.New("api error"),
			expectedError:              true,
			expectedErrorMessage:       "failed to generate token: api error",
			expectedGenerateTokenCalls: 1,
		},
		{
			name: "configuration with no token calls generate token -> happy path",
			givenConfig: &ionos.Configuration{
				Username: "username",
				Password: "password",
			},
			givenGeneratedToken:        generatedToken,
			expectedClientToken:        generatedToken,
			expectedGenerateTokenCalls: 1,
		},
	} {
		t.Run(tc.name, func(tt *testing.T) {
			tokenProvider := &mockTokenProvider{
				err:   tc.givenGenerateTokenError,
				token: tc.givenGeneratedToken,
			}
			factory := &clientFactory{
				tokenProvider: tokenProvider,
				ionosConfig:   tc.givenConfig,
			}

			apiClient, err := factory.Create(tt.Context())
			if tc.expectedError {
				assert.EqualError(tt, err, tc.expectedErrorMessage)
			} else if err != nil {
				tt.Fatalf("expected no error but got: %s", err.Error())
			} else {
				assert.NotNil(tt, apiClient)
				assert.Equal(tt, tc.expectedClientToken, apiClient.GetConfig().Token)
				assert.Equal(tt, tc.expectedGenerateTokenCalls, tokenProvider.calls)
			}
		})
	}
}
