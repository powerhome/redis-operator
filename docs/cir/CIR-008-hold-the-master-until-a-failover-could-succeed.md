# CIR-008: Hold the master until a failover could succeed

## Intent

Stop the rolling update from removing a master that nothing can replace.

## Behavior

- GIVEN a failover of more than one Redis whose master runs an old pod template
- WHEN the operator reaches the master in its rolling update
- THEN it replaces the pod only once every Sentinel reports a replica it could
  promote

- GIVEN a Sentinel that holds no such replica
- WHEN the operator would replace the master
- THEN the master is left running and serving, and the operator says which
  Sentinel is short

- GIVEN a Sentinel that holds a replica it has flagged `s_down`, `o_down` or
  `disconnected`
- WHEN the operator asks what could be promoted
- THEN that replica does not count, because Sentinel will not promote it either

- GIVEN a failover of one Redis
- WHEN its only pod runs an old pod template
- THEN it is replaced without asking, since no replica exists to promote and
  waiting for one would never end

- GIVEN a failover replicating from `bootstrapNode.host`
- WHEN its pods are replaced
- THEN nothing is asked of the Sentinels, which may not be running at all

- GIVEN a replica running an old pod template
- WHEN the operator replaces it
- THEN nothing is asked of the Sentinels, since losing a replica is not a
  failover

## Constraints

- **A replica reports itself synced as soon as it is.** `SlaveIsReady` reads the
  replica's own `INFO replication` for `master_sync_in_progress`,
  `master_link_status` and a pending master. A replica holding no data satisfies
  all three within milliseconds of being told to replicate, in the same reconcile
  that told it, so the existing gate can be met by a replica no Sentinel has
  heard of.
- **A Sentinel learns replicas only by reading the master's replica list**, which
  it can do only while the master answers. `SENTINEL REMOVE` and `SENTINEL RESET`
  both discard what it learned, and the operator issues both during an ordinary
  reconcile.
- **A failover with nothing to promote does not resolve later.** Sentinel answers
  a missing master with `-failover-abort-no-good-slave` and retries on its own
  timer forever. The operator seeds a master only when every Redis reports
  `127.0.0.1`, which a replica still holding the old master's address prevents,
  so neither party acts again.
- **Any Sentinel may be the one elected to carry out a promotion**, so the
  question has to be asked of all of them.

## Decisions

**The master's replacement is held, rather than the whole rolling update.**
Replicas are replaced first and losing one is not a failover, so there is nothing
to gain by stopping earlier. Holding only the master also keeps the failure
visible in the right place: an upgrade that stops with one pod on the old pod
template and a master still serving.

**The Sentinels are asked what they could promote, rather than the operator
working it out.** Sentinel's own rules for a promotable replica cover flags,
priority, link age and last reply, and a second implementation of them in the
operator would be a model of Sentinel that can disagree with Sentinel without
saying so. Reading `SENTINEL replicas` and discarding what Sentinel has flagged
`s_down`, `o_down` or `disconnected` asks Sentinel the part of the question that
decides the outcome.

**The failure direction is a stalled upgrade.** A Sentinel that never sees a
replica leaves the master where it is, so the failover keeps serving and one pod
keeps an old pod template. That is recoverable and visible, where the alternative
is a cluster with no master that neither Sentinel nor the operator will repair.

**Widening the operator's own recovery is the other half, and is not here.** The
operator will seed a master only when every Redis reports `127.0.0.1`; widening
that to "no master, and no Redis with a live master link" would let it recover
this state rather than only avoid it. It is worth having and it is not safe until
the master it seeds is chosen by what the Sentinels remember rather than by pod
order, which is a different piece of work.

## Date

2026-09-30
