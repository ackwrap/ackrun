package model

type SSHSFTPEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir"`
	IsSymlink  bool   `json:"is_symlink"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	ModifiedAt int64  `json:"modified_at"`
}

type SSHSFTPListResponse struct {
	Path    string         `json:"path"`
	Parent  string         `json:"parent"`
	Home    string         `json:"home"`
	Entries []SSHSFTPEntry `json:"entries"`
}

type SSHSFTPPathRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type SSHSFTPRenameRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}
