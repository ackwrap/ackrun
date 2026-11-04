package handler

import (
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
)

func (h *SSHMCPHandler) TransferFile(c *gin.Context) {
	h.withAuthorizedRequest(c, func() {
		hostID, err := strconv.ParseInt(c.Param("hostID"), 10, 64)
		if err != nil || hostID <= 0 {
			writeMCPError(c, http.StatusBadRequest, "SSH_MCP_INVALID", "SSH 主机 ID 无效")
			return
		}
		request := model.SSHMCPTransferRequest{HostID: hostID, Path: c.GetHeader("X-SSH-Path"), SHA1: c.GetHeader("X-SSH-SHA1")}
		controller := http.NewResponseController(c.Writer)
		if c.Request.Method == http.MethodPut {
			if c.GetHeader("X-SSH-SHA256") != "" {
				writeMCPError(c, http.StatusBadRequest, "SSH_MCP_INVALID", "文件校验已改用 SHA-1，请使用 X-SSH-SHA1")
				return
			}
			if value := c.GetHeader("X-SSH-Overwrite"); value != "" {
				request.Overwrite, err = strconv.ParseBool(value)
				if err != nil {
					writeMCPError(c, http.StatusBadRequest, "SSH_MCP_INVALID", "X-SSH-Overwrite 必须为布尔值")
					return
				}
			}
			h.receiveUpload(c, request)
			return
		}
		err = h.hosts.DownloadMCPStream(c.Request.Context(), model.SSHMCPFileRequest{HostID: hostID, Path: request.Path}, func(ctx context.Context, content io.Reader, info os.FileInfo) error {
			done := make(chan struct{})
			stop := context.AfterFunc(ctx, func() {
				_ = controller.SetWriteDeadline(time.Now())
				close(done)
			})
			defer func() {
				if !stop() {
					<-done
				}
				_ = controller.SetWriteDeadline(time.Time{})
			}()
			c.Header("Content-Type", "application/octet-stream")
			c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
			c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
			c.Writer.WriteHeaderNow()
			written, err := io.CopyBuffer(struct{ io.Writer }{c.Writer}, content, make([]byte, 64<<10))
			if err == nil && written != info.Size() {
				err = io.ErrUnexpectedEOF
			}
			return err
		})
		if err != nil && !c.Writer.Written() {
			writeSSHError(c, err)
		}
	})
}

func (h *SSHMCPHandler) UploadFile(c *gin.Context) {
	if !allowSSHMCPRequest(c) {
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/octet-stream" {
		writeMCPError(c, http.StatusUnsupportedMediaType, "SSH_MCP_INVALID", "请使用返回的 Content-Type，以原始二进制流上传文件")
		return
	}
	h.withRequestSlot(c, func() {
		request, err := h.service.ClaimUpload(c.Param("uploadID"), sshMCPBearer(c))
		if err != nil {
			writeSSHError(c, err)
			return
		}
		h.receiveUpload(c, *request)
	})
}

func (h *SSHMCPHandler) receiveUpload(c *gin.Context, request model.SSHMCPTransferRequest) {
	if c.GetHeader("Content-Encoding") != "" && c.GetHeader("Content-Encoding") != "identity" {
		writeMCPError(c, http.StatusUnsupportedMediaType, "SSH_MCP_INVALID", "请发送未经压缩编码的原始文件字节")
		return
	}
	controller := http.NewResponseController(c.Writer)
	defer controller.SetReadDeadline(time.Time{})
	body := &sshMCPUploadBody{ReadCloser: c.Request.Body, controller: controller}
	result, err := h.hosts.UploadMCPStream(c.Request.Context(), request, body, c.Request.ContentLength)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

type sshMCPUploadBody struct {
	io.ReadCloser
	controller *http.ResponseController
	once       sync.Once
	complete   atomic.Bool
	err        error
}

func (body *sshMCPUploadBody) Read(data []byte) (int, error) {
	n, err := body.ReadCloser.Read(data)
	if err == io.EOF {
		body.complete.Store(true)
	}
	return n, err
}

func (body *sshMCPUploadBody) Close() error {
	body.once.Do(func() {
		if !body.complete.Load() {
			_ = body.controller.SetReadDeadline(time.Now())
		}
		body.err = body.ReadCloser.Close()
	})
	return body.err
}
