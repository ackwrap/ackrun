package service

import "github.com/ackwrap/ackrun/internal/parser"

func isUnsupportedNodeType(nodeType string) bool {
	return !parser.IsSupportedNodeProtocol(nodeType)
}
