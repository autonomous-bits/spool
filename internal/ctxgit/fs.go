package ctxgit

import (
	"os"
	"path/filepath"
)

func osStatJoin(root, rel string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
}
