package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
)

const apiTokenCookie = "ackwrap_api_token"

// SecurityMiddleware protects the API when remote listening is enabled. A token
// supplied on the UI URL is exchanged for an HttpOnly same-site session cookie.
func SecurityMiddleware(apiToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if apiToken == "" {
			c.Next()
			return
		}

		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			bootstrapBrowserSession(c, apiToken)
			return
		}

		if tokenMatches(requestToken(c), apiToken) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{Error: model.APIError{
			Code:    "UNAUTHORIZED",
			Message: "需要有效的 API Token",
		}})
	}
}

func bootstrapBrowserSession(c *gin.Context, apiToken string) {
	token := c.Query("access_token")
	if token == "" {
		c.Next()
		return
	}
	if !tokenMatches(token, apiToken) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{Error: model.APIError{
			Code:    "UNAUTHORIZED",
			Message: "API Token 无效",
		}})
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     apiTokenCookie,
		Value:    token,
		Path:     "/api",
		HttpOnly: true,
		Secure:   c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteStrictMode,
	})
	query := c.Request.URL.Query()
	query.Del("access_token")
	target := c.Request.URL.Path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	c.Redirect(http.StatusSeeOther, target)
	c.Abort()
}

func requestToken(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.Fields(authorization)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	if cookie, err := c.Cookie(apiTokenCookie); err == nil {
		return cookie
	}
	// WebSocket clients that cannot set headers may authenticate on the upgrade URL.
	return c.Query("access_token")
}

func tokenMatches(actual, expected string) bool {
	if actual == "" || expected == "" {
		return false
	}
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1
}
