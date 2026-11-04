package model

// SSHSFTPProfile is persisted per trusted host, rather than negotiated for every
// upload. Generation prevents a late probe from replacing a newer result.
type SSHSFTPProfile struct {
	HostID      int64
	Fingerprint string
	Username    string
	Generation  string
	PacketSize  int
	Status      string
	ProbedAt    int64
}
