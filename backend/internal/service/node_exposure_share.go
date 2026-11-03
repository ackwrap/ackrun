package service

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

func (s *NodeExposureService) Share(id int64, requestHost string) (*model.NodeShareResponse, error) {
	item, err := s.store.GetNodeExposure(id)
	if err != nil {
		return nil, normalizeNodeExposureStoreError(err)
	}

	scheme := "socks5"
	switch item.InboundType {
	case "mixed", "socks":
	case "http":
		scheme = "http"
	default:
		return nil, fmt.Errorf("%w: 不支持复制此代理协议的链接", ErrNodeExposureInvalid)
	}

	server := item.Listen
	if ip := net.ParseIP(server); ip != nil && ip.IsUnspecified() {
		requestURL, err := url.Parse("http://" + requestHost)
		if err != nil || requestURL.Hostname() == "" || requestURL.User != nil ||
			requestURL.Path != "" || requestURL.RawQuery != "" || requestURL.Fragment != "" {
			return nil, fmt.Errorf("%w: 无法确定代理入口地址", ErrNodeExposureInvalid)
		}
		server = requestURL.Hostname()
	}

	shareURL := url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort(server, strconv.Itoa(item.ListenPort)),
		Fragment: item.Name,
	}
	if item.Username != "" || item.Password != "" {
		shareURL.User = url.UserPassword(item.Username, item.Password)
	}
	logging.Info("node_exposure.share", "生成端口绑定链接: %d", id)
	return &model.NodeShareResponse{URI: shareURL.String()}, nil
}
