package apilibs

import (
	"os"
	"regexp"

	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

type ApiLib struct{}

func NewApiLib() *ApiLib {
	return &ApiLib{}
}

func (al *ApiLib) IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (al *ApiLib) IsUrl(val string) bool {
	re := regexp.MustCompile(`^(https?:\/\/)([\w-]+\.)+[\w-]+(:\d+)?(\/[^\s]*)?$`)

	return re.MatchString(val)
}

func (al *ApiLib) IsNamespace(val shared.Namespace, schemaType string) bool {
	err := shared.Namespace.Validate(val)

	if err != nil {
		return false
	}

	return true
}
