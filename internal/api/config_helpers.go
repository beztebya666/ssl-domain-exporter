package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

func writeConfigValidationError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if validationErr, ok := err.(*config.ValidationError); ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   validationErr.Error(),
			"details": validationErr.Issues,
		})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func redactLegacySecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return config.RedactedSecret
}

func parseCompatBool(field, raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", field)
	}
}

func parseCompatInt(field, raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	return n, nil
}
