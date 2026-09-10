# CIR-006: Address Redis by DNS name

## Intent

Stop Sentinel holding an address that can come to mean a different pod.

Carries out [ADR-002](../adr/ADR-002-instances-are-addressed-by-name.md), which
decides that an address written down somewhere that outlives the moment is a
name and an address used to dial once is an address, and holds the reasoning.

## Behavior

- GIVEN a failover whose Redis pods are running
- WHEN the operator tells Sentinel which node to monitor
- THEN it gives the master's name, `<pod>.<service>.<namespace>.svc`

- GIVEN a replica that Sentinel discovers from the master's replica list
- WHEN it reports where it can be reached
- THEN it gives its own name, which Kubernetes substitutes into the Redis
  command from the pod's environment

- GIVEN a Redis pod replaced at a different address
- WHEN Sentinel next reaches it
- THEN the name it holds resolves to the replacement, and the entry it keeps for
  that instance does not change

- GIVEN a failover with `redis.exporter.enabled` unset
- WHEN the operator reconciles it
- THEN the service governing the Redis StatefulSet exists, carrying no port and
  no scrape annotations, because that service is what publishes a DNS record per
  pod

- GIVEN a running Sentinel that is not taking addresses as names
- WHEN the operator sets which master it should monitor
- THEN `resolve-hostnames` and `announce-hostnames` are set on it first, over the
  wire, and the Sentinel pod is not replaced

- GIVEN a Sentinel that refuses the address it is offered
- WHEN the operator tries to change what it monitors
- THEN the address it was monitoring is put back, together with the password it
  reached that master by, so it is left watching the wrong master rather than
  watching nothing or watching a master it cannot authenticate to

- GIVEN a Sentinel that was watching nothing
- WHEN it refuses the address it is offered
- THEN nothing is put back

- GIVEN an operator upgrade
- WHEN it reconciles an existing failover
- THEN the Sentinel pods SHOULD NOT be replaced, since nothing they read at
  startup has to change for them to take a name

- GIVEN a replica that names the master pod as the node it replicates from
- WHEN the operator checks that every replica follows the master
- THEN it recognises the name and the address as the same pod

- GIVEN a replica that names some other pod
- WHEN the operator runs the same check
- THEN it reports the mismatch and repoints the replica

- GIVEN a failover replicating from `bootstrapNode.host`
- WHEN its Redis pods start
- THEN they announce nothing, since a name from this cluster's DNS describes
  nothing that the external master, or anything reading its replica list, can
  reach

- GIVEN a failover whose `redis.command` runs `redis-server` and does not
  already announce
- WHEN its Redis pods start
- THEN the announcement is added to that command

- GIVEN a failover whose `redis.command` runs something else, or announces
  already
- WHEN its Redis pods start
- THEN the command is used exactly as written

## Constraints

- **Sentinel resolves a name when it is given one, and refuses one it cannot
  resolve.** `SENTINEL MONITOR` with an unresolvable address answers `ERR
  Invalid IP address or hostname specified`. Changing what Sentinel monitors
  means removing the old master first, because Sentinel refuses a second master
  under a name it already holds, so a refusal after that point leaves it
  monitoring nothing at all.
- **A Sentinel monitoring localhost never reports ready**, since its readiness
  check is that it monitors something else. One that is never pointed at a master
  therefore holds up any rollout that created it.
- **Sentinel reads its configuration file only at startup**, but both settings
  can be changed at runtime through `SENTINEL CONFIG SET`, and Sentinel persists
  them into that file itself. They are Sentinel-wide rather than per master, so
  `SENTINEL SET` cannot carry them.
- **`announce-hostnames` applies to what Sentinel already holds.** Turning it on
  after a master has been recorded makes Sentinel report that master by name,
  so there is no order in which an entry becomes stuck as an address.
- **`resolve-hostnames` and `announce-hostnames` need Redis 6.2 or newer**, and
  an unknown directive stops Sentinel starting. This project requires Redis 7.
- **A StatefulSet's `serviceName` is immutable.** The service that names the
  pods has to be the one the set already points at.
- **Sentinel resolves through the cluster's DNS.** A policy that denies these pods
  the DNS port leaves it unable to accept any address it is given. The operator
  writes no policy, so whether that port is open is the cluster's to say.
- **A pod has no DNS record until it is ready unless the service publishes
  addresses that are not.** A Redis reading its dataset from disk is not ready,
  which is when reaching it by name matters most.

## Decisions

**The service governing the StatefulSet is created whether or not the exporter
is.** That service is what gives each pod a name, and it is also what the
exporter is scraped through. Tying its existence to the exporter, which is off
by default, leaves a failover with no name to give Sentinel, and Sentinel
refuses an address it cannot resolve, so the failover runs with no failover
protection while looking healthy. Pointing the StatefulSet at the headless
service created alongside HAProxy serves a new failover equally well and an
existing one not at all, because `serviceName` cannot be changed. The exporter
contributes a port and its scrape annotations to a service that exists either
way.

**A rejected monitor is rolled back rather than predicted.** Sentinel offers no
way to ask whether it would accept an address without setting it, so the only
alternative to trying was not trying: keeping a list of addresses believed
acceptable, which is a second model of Sentinel's rules that can be wrong
without saying so. Reading what Sentinel watches, replacing it, and putting the
old one back when the replacement is refused needs no such model.

**The master is established once and written two ways.** Asking every Redis who
the master is, twice in one pass, gets two answers whenever a failover lands
between the two, and the pass then points Sentinel at a node other than the one
the replicas were just given. The address the operator compares against what a
Redis reports about itself, and the name it hands to Sentinel, describe one
answer: the pod at that address.

**The operator accepts a name or an address rather than resolving names
itself.** A replica reports the master it was last told to follow, which is an
address when the operator told it and a name when Sentinel did. Resolving the
name from the operator's own pod would answer a different question, "what does
this name mean here, now", and can disagree with what the replica is actually
connected to. Treating both the master pod's address and its name as the master
compares what was said against what was asked for, which is what the check is
for.

**The two Sentinel-wide settings are applied over the wire, not through a
restart.** They are in the generated configuration as well, so a new pod starts
correct, but what makes them reach a Sentinel already running is
`SENTINEL CONFIG SET`, issued immediately before it is given a master to watch.

Delivering them by replacing the pod was the alternative, and it costs a great
deal more than it looks. The Sentinel pods roll on every upgrade, and a Redis pod
replaced in the same window can take the master away while enough Sentinels are
unusable that none can elect, at which point the operator seeds the replacement
by pod order, which says nothing about which node holds the most recent writes.
A measured upgrade did exactly that. Keeping it safe then needs a second
mechanism to hold the Redis rolling update back until the Sentinels have
finished, which is two mechanisms and a longer upgrade to deliver two settings
that a command already delivers.

Setting them on every pass, rather than once, is what makes it need no record of
which Sentinel has been told. A Sentinel that restarts from an older
configuration is corrected the next time the operator has a master to give it,
and a Sentinel that already has them is set to what it already is.

**A command of one's own is extended only where that means something.** A
command is an argv, so an appended flag reaches Redis only where `redis-server`
is what runs; a wrapper such as `sh -c` would be handed it instead and ignore it,
leaving a failover that looks configured and is not. Refusing to guess about the
rest is what keeps the silent case from happening, and a command that already
passes `--replica-announce-ip` has said what it wants.

Setting the announcement at runtime instead, through `CONFIG SET`, would have
covered every command and needed no such rule. It does not work: the value
travels in the replication handshake, so a master goes on reporting the address
a replica gave it until that replica reconnects. Measured against
`redis:8.4.0-alpine`, a changed value was invisible to the master for as long as
the link lasted and took effect the moment it was killed. On the command line the
value is in force from the first handshake, with no window and no resync.

## Date

2026-09-29
