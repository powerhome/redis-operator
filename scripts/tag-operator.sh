#!/usr/bin/env bash
#
# Tag an operator release.
#
# The tag name comes from `VERSION` in the Makefile, where the operator's
# version is declared. Pushing that tag publishes the image and the chart.
#
# CI checks the tag's name against VERSION. It cannot check which commit the tag
# points at, because VERSION reads the same on every commit after the release
# one, so a tag cut from a later commit passes. This checks the commit.

set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="$(sed -n 's/^VERSION := //p' Makefile | head -1)"
[ -n "${VERSION}" ] || { echo "!! no VERSION in the Makefile" >&2; exit 1; }

# A clone configured not to follow tags, which this one is, can be missing a
# release someone else cut. Ask origin rather than trusting what is local.
#
# One request, named. Asking for every tag transfers the lot, which is slow
# enough on a poor connection to look like a hang, and answers a question this
# does not ask. The exit code says everything: 0 found it, 2 reached origin and
# did not, anything else could not reach origin at all.
found=0
git ls-remote --exit-code --tags origin "${VERSION}" >/dev/null 2>&1 || found=$?
case "${found}" in
  0)
    echo "!! ${VERSION} is already tagged on origin. It has been released." >&2
    echo "   Prepare a new version: set VERSION in the Makefile, then make prepare-release" >&2
    exit 1
    ;;
  2) ;;  # reached origin; the tag is not there
  *)
    echo "!! could not reach origin to check whether ${VERSION} is already released." >&2
    echo "   Try again with a connection, or tag by hand if you are certain." >&2
    exit 1
    ;;
esac
if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null; then
  echo "!! ${VERSION} already exists locally but not on origin." >&2
  echo "   Push it, or delete it and tag again: git tag -d ${VERSION}" >&2
  exit 1
fi

# A release tags the commit that set VERSION, which is not necessarily what
# origin's master points at now. The two are the same commit only when nothing
# merges between the release commit landing and the tag being cut, and a
# release that waits on a person cannot assume that.
#
# CI cannot make this check. It compares the tag's name to VERSION, and
# VERSION reads the same on every commit after the release one, so a tag cut
# from a later commit passes while shipping changes no changelog describes.
git fetch --quiet origin master
head="$(git rev-parse HEAD)"
master="$(git rev-parse FETCH_HEAD)"

if ! git merge-base --is-ancestor "${head}" "${master}"; then
  echo "!! HEAD is not merged into origin's master." >&2
  echo "   HEAD           ${head}" >&2
  echo "   origin/master  ${master}" >&2
  echo "   A release tags a reviewed commit. Merge it first, then pull." >&2
  exit 1
fi

release="$(git log -1 --format=%H -G'^VERSION := ' "${master}" -- Makefile)"
if [ -z "${release}" ]; then
  echo "!! could not find the commit that set VERSION." >&2
  exit 1
fi

if [ "${head}" != "${release}" ]; then
  echo "!! HEAD is not the commit that set VERSION to ${VERSION}." >&2
  echo "   HEAD          ${head}" >&2
  echo "   release       ${release}  $(git log -1 --format=%s "${release}")" >&2
  echo "   Tagging anything later ships changes the changelog does not describe." >&2
  echo "   Tag the release commit:" >&2
  echo "     git checkout ${release} && make tag-operator && git checkout -" >&2
  exit 1
fi

git tag "${VERSION}"
echo "tagged ${VERSION}. Push it to publish: git push origin ${VERSION}"
