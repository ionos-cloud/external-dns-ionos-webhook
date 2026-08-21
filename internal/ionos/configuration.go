package ionos

import "time"

// Configuration holds configuration from environmental variables
type Configuration struct {
	// Either the IONOS Cloud Token or the IONOS API Key
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
	TokenTTL       time.Duration `env:"IONOS_TOKEN_TTL" envDefault:"31536000s"`
	APIEndpointURL string        `env:"IONOS_API_URL"`
	AuthHeader     string        `env:"IONOS_AUTH_HEADER"`
	Debug          bool          `env:"IONOS_DEBUG" envDefault:"false"`
	DryRun         bool          `env:"DRY_RUN" envDefault:"false"`
}
