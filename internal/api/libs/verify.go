package libs

import (
	"os"
	"regexp"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func IsUrl(val string) bool {
	re := regexp.MustCompile(`^(https?:\/\/)([\w-]+\.)+[\w-]+(:\d+)?(\/[^\s]*)?$`)

	return re.MatchString(val)
}

func IsNamespace(val, schemaType string) bool {
	err := domain.Namespace.Validate(domain.Namespace(val))

	return err == nil
}
