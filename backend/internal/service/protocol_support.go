package service

var unsupportedNodeTypes = map[string]bool{
	"mieru": true,
}

func isUnsupportedNodeType(nodeType string) bool {
	return unsupportedNodeTypes[nodeType]
}
