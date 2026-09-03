package service_test

import (
	"crypto/sha256"
	"encoding/hex"
)

// redisPasswordDigest mirrors the operator's own digest, so a test can build a
// pod that matches the configured password or one that deliberately does not.
//
// What goes into the digest is pinned by TestRedisStatefulSetPasswordChecksum,
// which reads the value back off a generated pod template instead of
// recomputing it. This helper only has to agree with the operator well enough
// for the comparisons around it to mean something.
func redisPasswordDigest(namespace, name, password string) string {
	sum := sha256.Sum256([]byte(namespace + "/" + name + "\x00" + password))
	return hex.EncodeToString(sum[:])
}
