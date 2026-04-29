package bdynd

const (
	DirName       = ".bdynd"
	DefaultBranch = "main"
	DefaultRemote = "origin"
)

type Config struct {
	DefaultBranch string            `json:"default_branch"`
	Remotes       map[string]string `json:"remotes,omitempty"`
}

type InitOptions struct {
	RemoteName string
	RemoteRoot string
}

type Repo struct {
	Root   string
	Dir    string
	Config Config
}
