package service_test

import (
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
