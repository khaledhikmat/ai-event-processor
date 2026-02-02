package config

const (
	DefaultModelName = "gemini-2.5-flash"
	DefaultPort      = "8081"
)

type Config struct {
	Env          string
	Project      string
	ModelName    string
	GeminiAPIKey string
	Port         string
}

func New(getEnv func(string) string) (*Config, error) {
	modelName := getEnv("MODEL")
	if modelName == "" {
		modelName = DefaultModelName
	}

	// geminiKey := getEnv("GEMINI_API_KEY")
	// if geminiKey == "" {
	// 	return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	// }

	port := getEnv("PORT")
	if port == "" {
		port = DefaultPort
	}

	project := "default"

	cfg := &Config{
		Env:       "default",
		Project:   project,
		ModelName: modelName,
		//GeminiAPIKey: geminiKey,
		Port: port,
	}

	return cfg, nil
}
