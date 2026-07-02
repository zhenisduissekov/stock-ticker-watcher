package config

import (
	"os"
	"strconv"
)

// Config holds application configuration
type Config struct {
	Port            string
	DatabasePath    string
	FrontendOrigin  string
	DemoUserID      int
	SimulatePrices  bool
	SimulateInterval int // in seconds
}

// Load loads configuration from environment variables
func Load() *Config {
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DATABASE_PATH", "./stocks.db")
	frontendOrigin := getEnv("FRONTEND_ORIGIN", "*")
	demoUserID := getEnvAsInt("DEMO_USER_ID", 1)
	simulatePrices := getEnvAsBool("SIMULATE_PRICES", true)
	simulateInterval := getEnvAsInt("SIMULATE_INTERVAL", 2)

	return &Config{
		Port:            port,
		DatabasePath:    dbPath,
		FrontendOrigin:  frontendOrigin,
		DemoUserID:      demoUserID,
		SimulatePrices:  simulatePrices,
		SimulateInterval: simulateInterval,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
