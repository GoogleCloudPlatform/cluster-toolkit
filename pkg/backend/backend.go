package backend

// Backend interface for writing blueprints to a storage
type Backend interface {
	CreateDirectory(bpDirectoryPath string) error
	CopyFromPath(src string, dst string) error
}

var backends = map[string]Backend{
	"local": new(Local),
}

// GetBackendLocal gets the instance writing blueprints to a local
func GetBackendLocal() Backend {
	return backends["local"]
}
