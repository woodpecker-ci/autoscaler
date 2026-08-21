package utils

import (
	"fmt"
	"strings"
)

// CheckReservedTags rejects tags that intrude into a reserved key namespace.
//
// Each tag is treated as a "key=value" pair; only the key part is inspected,
// with surrounding whitespace trimmed. If any key starts with forbiddenPrefix,
// potentialError is returned wrapped with that prefix. Empty tags list, or no
// match, yields nil.
//
// Callers use this to keep a prefix under their own control, so externally
// configured tags cannot impersonate internally managed ones. Example: the
// autoscaler reserves its own prefix, because a configured tag in it could
// claim a foreign pool, making this pool's instances show up in that pool's
// listing and get torn down by it.
func CheckReservedTags(tags []string, forbiddenPrefix string, potentialError error) error {
	for _, tag := range tags {
		key, _, _ := strings.Cut(tag, "=")
		if strings.HasPrefix(strings.TrimSpace(key), forbiddenPrefix) {
			return fmt.Errorf("%w: %s", potentialError, forbiddenPrefix)
		}
	}
	return nil
}
