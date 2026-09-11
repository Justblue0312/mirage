package checksum

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func Compute(stmts []string) string {
	h := sha256.Sum256([]byte(strings.Join(stmts, "\n")))
	return fmt.Sprintf("sha256:%x", h[:])
}
