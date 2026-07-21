package application

import (
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/webui"
)

type accessTokenRedactingWriter struct {
	io.Writer
}

func (writer accessTokenRedactingWriter) Write(value []byte) (int, error) {
	if _, err := writer.Writer.Write([]byte(logging.RedactAccessToken(string(value)))); err != nil {
		return 0, err
	}
	return len(value), nil
}

func registerWebUI(router *gin.Engine) error {
	dist, err := fs.Sub(webui.Files, "dist")
	if err != nil {
		return fmt.Errorf("open embedded UI: %w", err)
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return fmt.Errorf("read embedded UI index: %w", err)
	}
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "not found"}})
			return
		}
		fileName := strings.TrimPrefix(c.Request.URL.Path, "/")
		if fileName != "" && fs.ValidPath(fileName) {
			if data, readErr := fs.ReadFile(dist, fileName); readErr == nil {
				contentType := mime.TypeByExtension(path.Ext(fileName))
				if contentType == "" {
					contentType = "application/octet-stream"
				}
				if strings.HasPrefix(fileName, "assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				c.Data(http.StatusOK, contentType, data)
				return
			}
		}
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
	return nil
}
