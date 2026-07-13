package service

var unsupportedNodeTypes = map[string]bool{
	"ssr":   true,
	"mieru": true,
}

func isUnsupportedNodeType(nodeType string) bool {
	return unsupportedNodeTypes[nodeType]
}
