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

- GIVEN a failover whose only promotable replica is the pod the operator is
  about to replace
- WHEN the operator asks whether a failover could succeed
- THEN that pod does not count, so the pod is left running and the operator says
  which Sentinel would be left with nothing to promote

- GIVEN a failover of one master and one replica
- WHEN the operator reaches either pod in its rolling update
- THEN it asks only that the Sentinels hold a promotable replica now, so the
  update proceeds and the window while the replacement syncs is accepted

- GIVEN a replica Sentinel reports with `slave-priority 0`
- WHEN the operator asks what could be promoted
- THEN that replica does not count, because Sentinel will not promote it either

- GIVEN a failover whose `redis.customConfig` carries `replica-priority 0`
- WHEN any of its pods runs an old pod template
- THEN none is replaced for as long as that setting stands, because the setting
  applies to every Redis in the failover and leaves nothing to promote

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

**The pod being replaced does not count towards what could be promoted.** The
check runs before the delete, so the pod in hand is still one of the replicas a
Sentinel reports. Counting it answers whether a failover could happen now, when
the question is whether one could happen once that pod is gone. Where that pod is
the only one a Sentinel would promote, the check passes on its own candidacy and
the delete takes the candidate with it. That needs three or more Redis, with
every replica other than the one being replaced unpromotable, whether unreachable
to the Sentinels or carrying `replica-priority 0`.

The caller names the pod it is about to delete, and the count leaves out the
address that pod answers to. Resolving a pod name to an address belongs to the
operator, because Sentinel reports addresses and the rolling update works in pod
names.

The exclusion is a match against the `ip` field Sentinel reports for a replica,
so the operator owes an address in the form Sentinel reports it. An address in
any other form matches nothing, and matching nothing is indistinguishable from
having nothing to exclude: the count includes the departing replica and the gate
answers the question this record exists to stop it answering. Nothing fails, and
no log line says so.

A pod the set does not report, or one with no address yet, refuses the pass
rather than excluding nothing, because a count that silently includes the
departing replica is the fault this prevents. The master's path names no pod at
all: a master is not among its own replicas, so there is nothing to leave out,
and resolving its name would only let a failed read of the pod list hold it.

**A failover of two Redis is asked a weaker question.** One master and one
replica means taking the replica away always leaves zero, so a check that
excluded it could never pass: the rolling update would stop at that pod and stay
there, and such a failover could never be upgraded at all. It is asked only that
the Sentinels hold a promotable replica before the pod is disturbed, and the
window while the replacement syncs is accepted because nothing about that shape
avoids it.

**Whether a replica can be spared is counted from the instances that are live,
not the number the spec is heading towards.** A scale-down from three to two reads
as two while three are still running, so a threshold taken from the spec drops the
exclusion there and puts the departing pod back among its own replacements until
the scale-down finishes. The live count is already in hand at the top of the pass.

**Counting two reachable replicas would avoid the pod lookup, and is wrong where
it matters.** Requiring two rather than excluding one needs no identity and no
call to Kubernetes. A failover with one healthy replica and one the Sentinels
have flagged unreachable would then never roll the unreachable pod, because only
one of the two counts, and that pod is exactly the one that needs replacing.

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

**Repointing stops at the first Sentinel that answers to refuse the master.**
Pointing a Sentinel at a master removes what it holds for `mymaster` before it
monitors the new address, and `MonitorRedisWithPort` discards the result of the
remove, so a refused address costs that Sentinel its master outright rather than
leaving the old one in place. Whatever one Sentinel answers, the rest answer for
the same reason: an address Sentinel will not accept is not specific to one of
them. Carrying on would take the whole quorum's `mymaster` in a single pass,
after which none could elect, `SENTINEL replicas` errors for every pass that
follows, and the rolling-update gate refuses them all. Stopping bounds the loss
to one Sentinel per pass while the settings are still applied below, which is
what the refusal must not skip.

**A Sentinel the operator cannot reach is passed over rather than stopped at.**
The reason for stopping needs a reply to hold: an answer says the command itself
was unacceptable, and silence says nothing about whether the next Sentinel would
take the address. `GetSentinelsIPs` lists every pod in phase `Running`, which
includes one that is hung, and it lists them in a fixed order, so stopping there
would leave the same Sentinels unrepointed on every pass for as long as that pod
stayed up and silent. That is the failure mode recorded below as a guard
composing into never. `redis.IsServerRejection` separates the two cases, by
asking whether the error came back from the instance or from the attempt to reach
it.

A Sentinel whose process is suspended may have lost its master all the same, so
passing over it is not a claim that nothing happened there. The kernel accepts the
connection, so `SENTINEL REMOVE` is written and waits in the socket buffer while
the client gives up on the reply, and the Sentinel can run it when it resumes. A
process that has exited refuses the connection instead, and there the remove never
lands. Neither case is saved by stopping, because what repairs such a Sentinel is
the next pass reading what it monitors.

**Carrying on past a Sentinel that cannot be reached costs time on every pass.**
`checkAndHealSentinels` attempts each Sentinel rather than stopping at the first
failure, so an unreachable one is dialled up to five times a pass, and
`service/redis/client.go` sets no timeouts, leaving go-redis its defaults.
Measured on a kind cluster with one Sentinel suspended: the pass still completed
and restored the other two Sentinels' settings within 53 seconds. The cost buys
the fix, since stopping at the first failure is the fault being corrected, and a
Sentinel that is down is the case where the others most need their timings.

**Carrying on past a refused pod costs a gate evaluation per stale replica.**
Each one reads the Redis pod list and dials every Sentinel, and
`service/redis/client.go` sets no timeouts, so a pass over a five replica
failover whose first Sentinel is unreachable spends five dial timeouts where it
used to spend one. The amplification lands when the failover is already degraded,
which is when a pass most needs to finish. Accepted here because stopping at the
first refusal is the stall this change exists to remove; setting timeouts on the
Redis client is the fix for the cost, and belongs with the client rather than
with this.

**A guard and an early return compose into never, so every guard here owes an
answer for the pod it refuses.** This is the fourth time one failure mode has
appeared in this rolling update, and each instance looked different while being
the same thing.

`CheckAndHeal` stopped as soon as the number of ready Sentinels disagreed with
the spec, and a Sentinel on an old pod template is one reason it disagrees, so
the pod that caused the stop was the pod that needed replacing. A failed heal
returned before the roll, and the Sentinel that failed to configure was again the
one needing replacement. `CheckSentinelsCanSpareOne` under a quorum rule refused
every pass for one and two Sentinel failovers, stranding the pod it was
protecting. And the Redis loop returned at the first pod the gate refused, so
that pod's refusal stood in for every stale pod behind it.

None of these was found by designing the guard. Three came out of review, one out
of a cluster test. The question that finds them is not whether the guard is
correct, which each of them was, but what becomes of the pod it says no to: a
guard that refuses is only safe when something else can still make progress, or
when the refusal is expected to clear on its own. A guard whose refusal is both
permanent and exclusive is a stall wearing the clothes of a safety check.

**Widening the operator's own recovery is the other half, and is not here.** The
operator will seed a master only when every Redis reports `127.0.0.1`; widening
that to "no master, and no Redis with a live master link" would let it recover
this state rather than only avoid it. It is worth having and it is not safe until
the master it seeds is chosen by what the Sentinels remember rather than by pod
order, which is a different piece of work.

## Date

2026-09-30
