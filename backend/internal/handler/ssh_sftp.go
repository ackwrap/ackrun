package handler

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	maxSSHSFTPRequestBody = 16 << 10
	maxSSHSFTPTextBody    = 13 << 20
	maxSSHSFTPUploadBody  = 512 << 20
	sshSFTPTokenHeader    = "X-SSH-Session-Token"
)

func (h *SSHHostHandler) ListSFTP(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	result, err := h.service.ListSFTP(c.Request.Context(), sessionID, token, c.Query("path"))
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) CreateSFTPDirectory(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPRequestBody)
	var request model.SSHSFTPPathRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	if err := h.service.CreateSFTPDirectory(sessionID, token, request.Path); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SFTP directory created"})
}

func (h *SSHHostHandler) CreateSFTPFile(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPRequestBody)
	var request model.SSHSFTPPathRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	if err := h.service.CreateSFTPFile(sessionID, token, request.Path); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SFTP file created"})
}

func (h *SSHHostHandler) RenameSFTP(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPRequestBody)
	var request model.SSHSFTPRenameRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	if err := h.service.RenameSFTP(sessionID, token, request.OldPath, request.NewPath); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SFTP entry renamed"})
}

func (h *SSHHostHandler) CopySFTP(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPRequestBody)
	var request model.SSHSFTPCopyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	if err := h.service.CopySFTP(c.Request.Context(), sessionID, token, request.SourcePath, request.TargetPath); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SFTP entry copied"})
}

func (h *SSHHostHandler) ReadSFTPText(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	result, err := h.service.ReadSFTPText(sessionID, token, c.Query("path"))
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) WriteSFTPText(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	if c.Request.ContentLength > maxSSHSFTPTextBody {
		c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{Error: model.APIError{
			Code: "SSH_REQUEST_TOO_LARGE", Message: "SSH 文本编辑请求体过大",
		}})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPTextBody)
	var request model.SSHSFTPTextWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	result, err := h.service.WriteSFTPText(sessionID, token, request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) DeleteSFTP(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPRequestBody)
	var request model.SSHSFTPPathRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	if err := h.service.DeleteSFTP(sessionID, token, request.Path, request.Recursive); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SFTP entry deleted"})
}

func (h *SSHHostHandler) UploadSFTP(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	if c.Request.ContentLength > maxSSHSFTPUploadBody {
		writeSSHSFTPUploadTooLarge(c)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSFTPUploadBody)
	written, err := h.service.UploadSFTP(sessionID, token, c.Query("path"), c.Query("overwrite") == "true", c.Request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeSSHSFTPUploadTooLarge(c)
			return
		}
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "SFTP file uploaded", "bytes": written})
}

func (h *SSHHostHandler) DownloadSFTP(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	download, err := h.service.OpenSFTPDownload(sessionID, token, c.Query("path"))
	if err != nil {
		writeSSHError(c, err)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": download.Name})
	if disposition == "" {
		disposition = "attachment"
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Length", strconv.FormatInt(download.Size, 10))
	c.Header("Last-Modified", download.ModifiedAt.UTC().Format(http.TimeFormat))
	c.Status(http.StatusOK)
	_, copyErr := io.Copy(c.Writer, download.Reader)
	closeErr := download.Reader.Close()
	if copyErr != nil || closeErr != nil {
		logging.Error("ssh_sftp.download", "SFTP 下载流中断")
	}
}

func parseSSHSFTPSession(c *gin.Context) (string, string, bool) {
	sessionID := c.Param("sessionID")
	token := c.GetHeader(sshSFTPTokenHeader)
	if sessionID == "" || token == "" {
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: model.APIError{
			Code: "SSH_SESSION_NOT_FOUND", Message: "SSH 会话不存在或授权无效",
		}})
		return "", "", false
	}
	return sessionID, token, true
}

func writeSSHSFTPUploadTooLarge(c *gin.Context) {
	c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{Error: model.APIError{
		Code: "SSH_SFTP_UPLOAD_TOO_LARGE", Message: fmt.Sprintf("上传文件不能超过 %d MiB", maxSSHSFTPUploadBody>>20),
	}})
}
