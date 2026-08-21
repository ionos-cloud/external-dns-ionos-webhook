package ionoscloud

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ionoscloud_auth "github.com/ionos-cloud/sdk-go-auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestGenerateToken(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"subject": "test", "exp": float64(time.Now().Add(time.Hour).Unix())})
	validToken, err := token.SignedString([]byte("key"))
	assert.NoError(t, err)
	// normally this populated when calling jwt.Parse
	// but since we do not parse here, it's assigned manually
	token.Raw = validToken
	expired := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"subject": "test", "exp": float64(time.Now().Add(-time.Hour).Unix())})
	assert.NoError(t, err)

	for _, tc := range []struct {
		name                 string
		givenGetTokenHandler http.HandlerFunc
		givenCachedToken     *jwt.Token
		expectedError        bool
		expectedErrorMessage string
		expectedToken        string
	}{
		{
			name: "generate token -> no cached token -> ionos api error",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedError:        true,
			expectedErrorMessage: "failed to request token from IONOS Cloud API: 500 Internal Server Error: ",
		},
		{
			name: "generate token -> no cached token -> ionos api ok -> token nil",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{}`))
				w.WriteHeader(http.StatusOK)
			},
			expectedError:        true,
			expectedErrorMessage: "token is nil or empty",
		},
		{
			name: "generate token -> no cached token -> ionos api ok -> token empty",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"token":""}`))
				w.WriteHeader(http.StatusOK)
			},
			expectedError:        true,
			expectedErrorMessage: "token is nil or empty",
		},
		{
			name: "generate token -> no cached token -> ionos api ok -> parse token failure",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"token":"not-a-jwt-token"}`))
				w.WriteHeader(http.StatusOK)
			},
			expectedError:        true,
			expectedErrorMessage: "failed to process token: token is malformed: token contains an invalid number of segments",
		},
		{
			name: "generate token -> no cached token -> happy path",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(fmt.Sprintf(`{"token":"%s"}`, validToken)))
				w.WriteHeader(http.StatusOK)
			},
			expectedToken: validToken,
		},
		{
			name: "generate token -> cached token -> token expired -> ionos api errror",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			givenCachedToken:     expired,
			expectedError:        true,
			expectedErrorMessage: "failed to request token from IONOS Cloud API: 500 Internal Server Error: ",
		},
		{
			name: "generate token -> cached token -> token expired -> token nil",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{}`))
				w.WriteHeader(http.StatusOK)
			},
			givenCachedToken:     expired,
			expectedError:        true,
			expectedErrorMessage: "token is nil or empty",
		},
		{
			name: "generate token -> cached token -> token expired -> token nil",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"token":""}`))
				w.WriteHeader(http.StatusOK)
			},
			givenCachedToken:     expired,
			expectedError:        true,
			expectedErrorMessage: "token is nil or empty",
		},
		{
			name: "generate token -> cached token -> token expired -> parse token error",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"token":"not-a-jwt-token"}`))
				w.WriteHeader(http.StatusOK)
			},
			givenCachedToken:     expired,
			expectedError:        true,
			expectedErrorMessage: "failed to process token: token is malformed: token contains an invalid number of segments",
		},
		{
			name: "generate token -> cached token -> token expired -> happy path",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(fmt.Sprintf(`{"token":"%s"}`, validToken)))
				w.WriteHeader(http.StatusOK)
			},
			givenCachedToken: expired,
			expectedToken:    validToken,
		},
		{
			name: "generate token -> cached token -> token not expired -> happy path",
			givenGetTokenHandler: func(w http.ResponseWriter, r *http.Request) {
				// should not be called, cached token is used
			},
			givenCachedToken: token,
			expectedToken:    validToken,
		},
	} {
		t.Run(tc.name, func(tt *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/auth/v1/tokens/generate", tc.givenGetTokenHandler)
			srv := httptest.NewServer(mux)
			tt.Cleanup(func() {
				srv.Close()
			})

			tokenProvider := &cachedTokenProvider{
				client:    ionoscloud_auth.NewAPIClient(ionoscloud_auth.NewConfiguration("username", "password", "", srv.URL)),
				jwtParser: jwt.NewParser(),
			}
			tokenProvider.cachedToken = tc.givenCachedToken

			token, err := tokenProvider.GenerateToken(tt.Context())
			if tc.expectedError {
				assert.EqualError(tt, err, tc.expectedErrorMessage)
			}
			assert.Equal(t, tc.expectedToken, token)
		})
	}
}
