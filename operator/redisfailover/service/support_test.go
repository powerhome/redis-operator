package service_test

import (
	"crypto/sha256"
	"encoding/hex"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// notFound builds the error the Kubernetes API returns for an object that does
// not exist.
//
// A test standing in for "nothing deployed yet" needs this rather than an error
// that merely reads like it. The operator tells an absent object apart from one
// it could not read, and acts differently on each, so a plain error saying "not
// found" stands for the wrong one of the two.
func notFound(resource, name string) error {
	return apierrors.NewNotFound(appsv1.Resource(resource), name)
}

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
