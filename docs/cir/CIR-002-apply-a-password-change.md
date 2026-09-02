# CIR-002: Apply a password change to a running RedisFailover

## Intent

Let a `RedisFailover` change the password it runs with, without anyone deleting
pods by hand.

Adding `auth.secretPath` to a running failover, or changing the value in the
secret it names, left Redis serving the password it started with. Redis reads
`requirepass` only at startup and the Redis StatefulSet uses the `OnDelete`
update strategy, so nothing restarted the pods. The operator meanwhile had
already taken the new password, so every check it makes was refused,
`CheckAndHeal` returned on the first of them, and `UpdateRedisesPods`, the one
thing that would have applied the change, sits behind all of them and was never
reached.

Rotation was the worse of the two because nothing looked wrong. Pods stayed
`Running`, HAProxy backends stayed `UP`, and data kept flowing on the old
password, since HAProxy also keeps the password its pod started with. Any client
that re-read the rotated secret was locked out with `WRONGPASS`, and the
operator had silently lost control of the failover: no healing, no failover
management, no sentinel reconfiguration, for as long as it lasted.

## Behavior

- GIVEN a `RedisFailover` whose secret has a new password
- WHEN the operator reconciles it
- THEN the Redis pods restart and the failover returns with one master,
  replication up, and every pod accepting the new password

- GIVEN a running `RedisFailover` that gains `auth.secretPath`
- WHEN the operator reconciles it
- THEN the same repair runs, though Redis answers that no password is
  configured rather than `WRONGPASS`

- GIVEN a running `RedisFailover` that loses `auth.secretPath`
- WHEN the operator reconciles it
- THEN the same repair runs in reverse, and the pods restart without
  `requirepass`

- GIVEN Redis refusing the configured password while every pod already carries
  the current pod template
- WHEN the operator reconciles it
- THEN it reports the refusal and restarts nothing, because the secret itself is
  wrong and repeating the restart would not end

- GIVEN a `RedisFailover` with `haproxy` configured
- WHEN a credential change is applied
- THEN the running HAProxy Deployment is left alone until every Redis pod that is
  up and answering agrees with the configured password, whether that password is
  being taken or given up

- GIVEN a Redis pod that is shutting down, or one that has yet to start
- WHEN the operator decides whether HAProxy may restart
- THEN that pod is not waited for, since it is being replaced by one built for
  the current password and waiting would hold the proxy away from its replacement

- GIVEN a `RedisFailover` with no `auth.secretPath`
- WHEN the operator is upgraded
- THEN its pod template is unchanged and nothing restarts

- GIVEN any pod template change that is not a credential change
- WHEN the operator reconciles it
- THEN the existing one-at-a-time rolling update applies it exactly as before

- GIVEN anyone holding `get pods` in the namespace
- WHEN they read the pod template
- THEN they find a digest and never the password

## Constraints

- **Redis reads `requirepass` at startup only.** A running pod keeps the
  password it began with however many times the secret changes, so restarting is
  the only way to apply one.
- **The secret reaches the pod by reference.** Rotating its value leaves the pod
  template byte for byte identical, so the StatefulSet computes no new revision
  and a rolling update has nothing to roll to.
- **The Redis StatefulSet uses `OnDelete`.** Nothing restarts a pod on its own.
- **Pod templates are readable by anything that can get pods**, a wider audience
  than those trusted with the secret.
- **HAProxy is ensured before the Redis StatefulSet** in the same reconcile, so
  anything HAProxy reads about Redis state describes the state from before the
  password changed.
- **Every check in `CheckAndHeal` authenticates.** Whatever recognises a refused
  credential has to sit ahead of all of them.

## Decisions

- **Rejected: rolling the pods one at a time.** This was the first attempt, and
  it produced a worse failure than doing nothing. A pod that has restarted has
  the new `requirepass` and `masterauth` while one that has not has the old, so
  replication between them fails for as long as they disagree. The pod that
  restarted first cannot sync, so it never reports ready, and the wait for it to
  become ready is what would drive the next restart. Nothing arrives to end it.
  Observed on a cluster: a master on one password, an unreachable replica on the
  other, both pods labelled `slave`, and therefore no endpoints at all on the
  `rfrm-<name>` master service. It sat there for nine minutes and was not going
  to recover. The unfixed operator wedged but left the cluster consistent; that
  version half-rotated into a split.
- **Chosen: restarting the stale pods together.** It costs a short window with
  no Redis, which sentinel and the clients already handle, instead of a split
  that persists. While bootstrapping the pods replicate from the external node
  rather than from each other, so the split cannot arise there; they still go
  together, because the authoritative data is on the bootstrap node and drawing
  the restart out buys nothing.
- **Rejected: comparing pods against the StatefulSet's `Status.UpdateRevision`.**
  HAProxy is ensured before the Redis StatefulSet, so that revision still
  describes the state from before the password changed and the pods trivially
  match it. It reports a rotation finished before it has started. Measured on a
  kind cluster: the HAProxy digest advanced 6 seconds after the rotation rather
  than 66, the proxy restarted in the same second as Redis, and routing through
  it was dead for around 60 seconds. That version passed its unit tests.
- **Chosen: the pod carries a digest of the password it was built for.** Both
  the restart and the HAProxy ordering compare against the digest on the pod
  itself, which no ordering within a reconcile can make stale.
- **Rejected: a digest of the password alone.** The same value in every cluster,
  so one table of precomputed hashes reads passwords back out of any annotation
  anywhere. The digest is keyed to the failover's namespace and name, which
  confines such a table to a single failover and removes the amortisation that
  makes building one worthwhile.
- **Rejected: a slow password hash such as bcrypt or argon2.** The value's job
  is change detection, not credential storage: it is recomputed on every
  reconcile and has to be deterministic and cheap. Keying it to the failover
  addresses precomputation, which is the exposure that matters here. It is not a
  defence against someone attacking one particular failover, since the prefix is
  public and SHA-256 is fast; a weak password still falls to a targeted search.
- **Rejected: treating a refused credential as a node that is not ready.**
  `GetNumberMasters` skipped past any node it could not query, which is right for
  a node that is still starting and wrong for one refusing a credential. It
  reported zero masters and no error, hiding the one fault the caller could
  repair.
- **Chosen: reporting the refusal only when no master answered.** Mid-rotation
  some pods hold the new password and some the old; if one of them is still a
  reachable master there is nothing to repair.
- **Rejected: treating any failure to read the HAProxy Deployment as "no proxy
  running yet".** Found on review. A failed read says nothing about whether a
  proxy exists, and answering with the current digest restarts HAProxy ahead of
  Redis, which is the ordering the guard exists to prevent. Only a `NotFound`
  means absent; any other failure abandons the pass.
- **Rejected: ordering HAProxy by holding back its password digest.** This was
  the first approach and it only ever covered a rotation, where the pod spec is
  genuinely unchanged because the secret is referenced. Adding or removing
  `auth.secretPath` puts `REDIS_PASSWORD` into the HAProxy pod spec or takes it
  out, so the Deployment differs whatever the digest says. A cluster run caught
  the proxy being rewritten with the password env removed while the digest was
  still correctly held at the old value: the guard worked and was bypassed. The
  rewrite landed in the same second the `RedisFailover` was patched, with nothing
  tying it to Redis at all.
- **Chosen: gating the write.** Writing the Deployment is what restarts the
  proxy, so that is what waits. This covers every way the Deployment can differ,
  including ones not yet thought of, rather than the one the digest happens to
  describe. It makes the ordering defined rather than incidental; it is not a
  latency improvement, since a normal change has the Redis pods entering
  termination within about a second and the proxy follows them.
- **Rejected: waiting for pods that are shutting down.** They are being replaced
  by pods built for the current password. Holding the proxy on the old password
  for a pod in graceful shutdown keeps it misaligned with the new pods that have
  already started, which is worse than being misaligned with one that is leaving.
- **Rejected: an empty password as a special case.** Found on review. The
  guard originally returned early when no password was configured, which skipped
  the ordering entirely and restarted HAProxy ahead of Redis whenever
  `auth.secretPath` was removed. Separately, the digest of an empty password
  hashed the empty string while a pod built without a password carries no
  annotation at all, so every such pod read as permanently stale and the
  restart-loop guard never fired. The digest of no password is now the absence of
  a digest, and with both ends of that comparison agreeing the ordering needs no
  special case.
- **Rejected: stopping at the first pod that fails to restart.** Found on
  review. It leaves behind exactly the half-restarted failover the
  together-or-not-at-all rule exists to prevent. Every stale pod is attempted and
  the failures are reported together.
- **Rejected: two separate answers to "which pods count".** Found on review.
  The restart side skipped pods that were not running or were already being
  deleted; the HAProxy side skipped nothing, so a pod stuck terminating on a lost
  node held the proxy on a password its backends had already replaced. Both read
  one filtered list.

## Verification

Measured on a kind cluster with the operator deployed in it, driving all three
directions and reading pod annotations, pod start times and ReplicaSet history
through the API server. Sampling the transition rather than checking the end
state is load bearing: an earlier version of the ordering passed its unit tests,
stamped the right annotation and ended healthy while restarting the proxy in the
same second as Redis.

Second-resolution timestamps cannot settle ordering between events that land in
the same second, which is the usual case here. The evidence for the gate is
therefore the operator's own log: with debug logging on, it reports holding the
HAProxy deployment exactly once per transition, and for adding and removing a
password that hold lands in the same second as the spec change, which is the
pass that previously rewrote the Deployment.

A full rotation completes in a minute or two with no intervention, and the
failover returns with one master, replication up, correct role labels and the
master service endpoint restored.

All three directions are covered by the integration test, which asserts on
reaching a master with the expected password rather than on any generated
resource: an assertion over configuration passes just as well while the pods are
still serving the previous one.

## Date

2026-09-02
