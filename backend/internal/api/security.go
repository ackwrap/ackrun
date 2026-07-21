package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
)

const (
	apiTokenCookie       = "ackwrap_api_token"
	apiTokenCookieMaxAge = 30 * 24 * 60 * 60
)

// SecurityMiddleware protects the API when remote listening is enabled.
func SecurityMiddleware(apiToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if apiToken == "" {
			c.Next()
			return
		}

		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			if redirectWithoutAccessToken(c) {
				return
			}
			c.Next()
			return
		}

		bearer := bearerToken(c)
		if tokenMatches(bearer, apiToken) {
			setAPITokenCookie(c, bearer)
			c.Next()
			return
		}
		if cookie, err := c.Cookie(apiTokenCookie); err == nil && tokenMatches(cookie, apiToken) {
			c.Next()
			return
		}
		if allowsAccessTokenQuery(c.Request) && tokenMatches(c.Query("access_token"), apiToken) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{Error: model.APIError{
			Code:    "UNAUTHORIZED",
			Message: "需要有效的 API Token",
		}})
	}
}

func redirectWithoutAccessToken(c *gin.Context) bool {
	query := c.Request.URL.Query()
	if _, found := query["access_token"]; !found {
		return false
	}
	query.Del("access_token")
	target := c.Request.URL.Path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	c.Redirect(http.StatusSeeOther, target)
	c.Abort()
	return true
}

func allowsAccessTokenQuery(request *http.Request) bool {
	if request.Method != http.MethodGet {
		return false
	}
	path := request.URL.Path
	return strings.HasPrefix(path, "/api/v1/rules/subscriptions/") && strings.HasSuffix(path, "/content") ||
		strings.HasPrefix(path, "/api/v1/rules/geo/rule-sets/") && strings.HasSuffix(path, "/content")
}

func setAPITokenCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     apiTokenCookie,
		Value:    token,
		Path:     "/api",
		MaxAge:   apiTokenCookieMaxAge,
		HttpOnly: true,
		Secure:   c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteStrictMode,
	})
}

func bearerToken(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.Fields(authorization)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func tokenMatches(actual, expected string) bool {
	if actual == "" || expected == "" {
		return false
	}
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1
}
