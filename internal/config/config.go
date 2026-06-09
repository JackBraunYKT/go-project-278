package config

import "os"

// GetEnv возвращает значение переменной окружения или fallback, если оно пустое.
func GetEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		value = fallback
	}

	return value
}
