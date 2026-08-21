package ionos

import "time"

// Configuration holds configuration from environmental variables
type Configuration struct {
	// APIKey is Either the IONOS Cloud Token or the IONOS API Key
	// depending on the API used.
	APIKey string `env:"IONOS_API_KEY"`

	// Username of the bot account, for IONOS Cloud only
	// Ignored if IONOS_API_KEY is set
	Username string `env:"IONOS_USERNAME"`

	// Password of the bot account, for IONOS Cloud only
	// Ignored if IONOS_API_KEY is set
	Password string `env:"IONOS_PASSWORD"`

	// TokenTTL is the TTL used when requesting a token
	// this is only taken into consideration in the case of IONOS Cloud
	// if the Username/Password method is used
	TokenTTL time.Duration `env:"IONOS_TOKEN_TTL" envDefault:"31536000s"`

	// APIEndpointURL is the base API URL
	// if left empty, it set automatically based on the detected provider
	APIEndpointURL string `env:"IONOS_API_URL"`

	// AuthHeader is the Authentication Header name, set automatically based on the detected provider
	AuthHeader string `env:"IONOS_AUTH_HEADER"`

	// Debug toggles the debug mode
	Debug bool `env:"IONOS_DEBUG" envDefault:"false"`

	// DryRun is set to avoid any changes to the DNS records
	// it only prints a message instead of performing the actual operation
	DryRun bool `env:"DRY_RUN" envDefault:"false"`
}
