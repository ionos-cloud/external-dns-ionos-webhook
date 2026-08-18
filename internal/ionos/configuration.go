package ionos

// Configuration holds configuration from environmental variables
type Configuration struct {
	APIKey         string `env:"IONOS_API_KEY"`
	Username       string `env:"IONOS_USERNAME"`
	Password       string `env:"IONOS_PASSWORD"`
	APIEndpointURL string `env:"IONOS_API_URL"`
	AuthHeader     string `env:"IONOS_AUTH_HEADER"`
	Debug          bool   `env:"IONOS_DEBUG" envDefault:"false"`
	DryRun         bool   `env:"DRY_RUN" envDefault:"false"`
}
