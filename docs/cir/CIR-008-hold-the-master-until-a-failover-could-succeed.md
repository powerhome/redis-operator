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
- THEN it waits on the same condition, because losing a replica takes away the
  candidate the Sentinels would promote

- GIVEN the replica running an old pod template is the only one its Sentinels
  could promote
- WHEN the operator would replace it
- THEN it is left running, because the candidate that would make the replacement
  safe is the pod that would be going away

- GIVEN a replica its Sentinels have flagged unreachable, and another they could
  promote
- WHEN the operator replaces the unreachable one
- THEN it is replaced, because what is left is promotable and what is going was
  never a candidate

- GIVEN a failover of two Redis, so one master and one replica
- WHEN the operator replaces that replica
- THEN it asks only that the Sentinels hold a promotable replica, not that one
  survives the replacement, because the failover has none to spare and requiring
  one would leave the pod never replaced

- GIVEN the operator has just replaced the master
- WHEN the reconcile would carry on
- THEN it ends instead, so nothing further in that pass points a Sentinel at the
  departed master or resets one

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
- **`SENTINEL RESET` discards the replica list**, and a Sentinel rebuilds that
  list only by reading it from a master that answers. The operator resets a
  Sentinel whose counts do not match what the spec says to expect, which during a
  rolling update they do not.
- **A reconcile that finds no master returns before it reaches the Sentinels**,
  so the pass that removes the master is the only one that can reset a Sentinel
  into a state it cannot rebuild from.

## Decisions

**Every replacement waits, not only the master's.** Replacing a replica is not a
failover, which is why holding only the master looked sufficient. It is not:
taking a replica away removes the candidate the Sentinels would promote, and
leaves them with none until the replacement has synced and they have read the
master's replica list again. A rollout that does that while they are already
blind, which is what a pass that has just reset them leaves behind, extends the
blindness by another reconcile and the master that dies inside it is not replaced.

Measured on a two node failover with 20000 keys upgrading from `v4.7.1`, waiting
on one condition in both places halved the window in which no Sentinel could
elect, and the Sentinels went from holding no master to holding a master and a
replica in the same second rather than 31 seconds apart.

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

**The pass that removes the master ends there, rather than the reset being
conditioned.** Asking the gate and then resetting the Sentinels in the same pass
answers the question and destroys the answer: measured on a two node failover,
two of three Sentinels were reset in the reconcile that deleted the master, and
the promotion then waited on an election reaching the one Sentinel that had kept
its replica list, 103 seconds rather than the usual 20. Had the third been reset
too, nothing would have promoted anything.

Conditioning the reset on the master being reachable does not cover it, because
the master has only just been deleted and no Sentinel has noticed yet. What is
true at that point is simpler: the pass read a failover that had a master, the
master is now gone, and every remaining step would be acting on what it read.
The next pass is thirty seconds away and sees what is there.

**A failover with one replica is asked a weaker question.** Leaving the pod out
of the count is only answerable when something else could be promoted, and a
failover of two Redis has one replica: taking it away always leaves zero, so the
check could never pass and the rolling update would stop at that pod and stay
there. The operator defaults to three Redis and every example declares three, so
this is an edge the check has to handle rather than the shape it is written for.
Such a failover is asked the question it can answer, that the Sentinels hold a
promotable replica before the pod is disturbed, and the window while the
replacement syncs is accepted because there is no arrangement that avoids it.

**The pod being replaced is named, rather than counted around.** Requiring two
reachable replicas instead of one needs no identity and no pod lookup, and it is
wrong in a case that arises: a failover with one healthy replica and one
the Sentinels have flagged unreachable would never roll the unreachable pod,
because only one of the two counts towards a promotion. Naming the pod keeps that
replacement possible. A Sentinel holds a replica either by the name the operator
announced for it or by the address it had before it was repointed, so the pod is
named both ways and left out either way.

**Widening the operator's own recovery is the other half, and is not here.** The
operator will seed a master only when every Redis reports `127.0.0.1`; widening
that to "no master, and no Redis with a live master link" would let it recover
this state rather than only avoid it. It is worth having and it is not safe until
the master it seeds is chosen by what the Sentinels remember rather than by pod
order, which is a different piece of work.

## Date

2026-09-30, extended 2026-10-01
