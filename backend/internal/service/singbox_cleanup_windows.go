//go:build windows

package service

// sing-tun installs Windows routes on its Wintun adapter and strict-route
// filters in a dynamic WFP session. Closing the process releases those
// handles, so enumerating or deleting system routes here would risk touching
// routes owned by other tunnel applications.
func cleanupPlatformSingboxState(string) (platformCleanupResult, error) {
	return platformCleanupResult{}, nil
}

func snapshotPlatformPriorityOneTables(activeTUNState) (platformNetworkBaseline, error) {
	return platformNetworkBaseline{}, nil
}

func recordPlatformSingboxRouteTables(string, <-chan struct{}) error {
	return nil
}

func syncOwnershipStateParentDirectory(string) error {
	return nil
}
