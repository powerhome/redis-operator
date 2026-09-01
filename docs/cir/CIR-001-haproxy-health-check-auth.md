# CIR-001: Authenticate the HAProxy health check

## Intent

Make `auth.secretPath` usable on a `RedisFailover` that also declares `haproxy`.

Setting both took the HAProxy-fronted master endpoint out of service. The
generated `haproxy.cfg` health checked each backend with an unauthenticated
`info replication`; under `requirepass` that answers `-NOAUTH`, so `tcp-check
expect string role:master` never matched, every server stayed `DOWN` (they start
`init-state down`), and `rfrm-haproxy-<name>` had nothing to route to. The
failover reported itself healthy throughout. The direct `rfrm-<name>` master
Service was unaffected.

The effect was that nobody used authentication at all: 122 `RedisFailover`
resources across four clusters and nine applications, staging and production,
every one with `auth.secretPath` unset. That was not taste, it was the only
configuration that worked.

## Behavior

- GIVEN a `RedisFailover` with both `auth.secretPath` and `haproxy` set
- WHEN HAProxy health checks a Redis backend
- THEN it authenticates first, the backend joins the pool, and the
  `rfrm-haproxy-<name>` Service routes to the master

- GIVEN a password containing any character Redis accepts, including spaces,
  quotes, backslashes and non-ASCII
- WHEN HAProxy authenticates with it
- THEN the check passes; the usable password set is Redis's to define, not a
  consequence of how the operator escapes a config string

- GIVEN a `RedisFailover` with no `auth.secretPath`
- WHEN the operator generates its HAProxy config and Deployment
- THEN both are byte-identical to before, so nothing restarts on upgrade

- GIVEN a `RedisFailover` whose `auth.secretPath` names a secret with an empty
  password
- WHEN the operator reconciles it
- THEN the reconcile fails and names the secret, rather than producing a Redis
  with no `requirepass` and an HAProxy check that authenticates against it

- GIVEN a `RedisFailover` that is bootstrapping
- WHEN the operator reconciles it
- THEN no HAProxy resources exist to authenticate, because `HaproxyAllowed()` is
  `!Bootstrapping()`, and this change SHOULD NOT alter that

## Constraints

- **The password must stay out of the ConfigMap.** Anything holding `get
  configmaps` in the namespace can read it. HAProxy has to pull the value from
  its own environment, which is why the config uses a log-format expression
  rather than a literal, and why the Deployment gets `REDIS_PASSWORD` from the
  same Secret the Redis pods use.
- **No new HAProxy image floor.** v4.0.0 already requires v3.1.0 or greater;
  whatever is used has to work there.
- **One source for the password.** Redis, Sentinel and HAProxy must not drift on
  where it comes from, hence the shared `getRedisPasswordEnv`.

## Decisions

Four ways of writing the AUTH step were tried against `haproxy:3.1.0` and `3.4`
with a password-protected `redis:8.10.1-alpine`, because the documentation
proved wrong twice. Results:

| AUTH step | space | `"` | `\` | `'` | non-ASCII |
| --- | --- | --- | --- | --- | --- |
| `tcp-check send AUTH ${REDIS_PASSWORD}` | fails | fails | fails | fails | fails |
| `send-lf AUTH \"%[env(REDIS_PASSWORD)]\"` | works | fails | fails | works | works |
| `send-lf AUTH \"%[env(REDIS_PASSWORD),json]\"` | works | works | works | works | fails |
| `send-lf` with protocol framing | works | works | works | works | works |

- **Rejected: plain `tcp-check send`.** HAProxy does not expand `${VAR}` in its
  argument, it forwards the literal text. The value has to come from a
  log-format expression, which only `send-lf` evaluates.
- **Rejected: quoted inline `AUTH`.** AUTH sent inline is split on whitespace by
  Redis, so the value needs quoting to survive a space, and then `"` and `\`
  break back out of those quotes.
- **Rejected: the `json` converter.** It escapes `"` and `\` exactly as Redis's
  inline parser reads them back, but renders non-ASCII as `\uXXXX`, which that
  parser does not decode. It trades one broken class of password for another
  rather than removing the problem.
- **Rejected: refusing passwords the config cannot express.** Proposed first,
  alongside the `json` converter, and steered away from on review: the objection
  was that the password space should be Redis's to define, not a consequence of
  HAProxy's config language. That objection is what produced the option below.
- **Chosen: the Redis Serialization Protocol (RESP) form.** `*2\r\n$4\r\nAUTH\r\n$<length>\r\n<password>\r\n`
  prefixes each argument with its length in bytes, so Redis reads exactly that
  many bytes and no character is special to anything along the way. HAProxy
  supplies both blanks from the environment via `%[env(REDIS_PASSWORD),length]`
  and `%[env(REDIS_PASSWORD)]`. No password is unusable and no validation is
  needed.

- **Rejected: resolving the password for the HAProxy generators.** An empty
  password splits the two conditions that stand for "this failover has a
  password": the Redis config gates on the resolved value and emits no
  `requirepass`, while the HAProxy config and Deployment gate on the spec field
  and still send an AUTH, which Redis answers `-ERR`. Threading the resolved
  value through both HAProxy generators aligns them, but leaves a failover that
  asked for authentication running without any. Refusing an empty password in
  `GetRedisPassword` makes the split unreachable and surfaces the
  misconfiguration instead.

Also considered and left alone: **a Redis NetworkPolicy.** Reported alongside
this in the same issue, but #42 removed those policies deliberately and #48
recorded the cost of having them, so re-adding one is its own decision with its
own failure mode. Out of scope here.

## Verification

Beyond unit coverage, the operator's own generated config was fed verbatim to
HAProxy against a password-protected Redis, with only the `server-template` line
replaced by a static `server` (SRV discovery needs a cluster). Backend `UP` /
Layer7 check passed, and `SET`/`GET` succeeded through the proxy with the
password `pé 'a"b\c#1%x`.

That the `,length` converter returns a byte count rather than a character count
is the assumption the framing rests on, since RESP counts bytes: a character
count would send a short prefix and Redis would misparse every non-ASCII
password. Confirmed independently on review against `haproxy:3.4-alpine` and
`redis:8.10.1-alpine` with `pässwörd✓`, which is 13 bytes over 9 characters and
comes up `UP`/`L7OK` with a successful write.

The pairing is now covered automatically: the integration test's `RedisFailover`
declares `haproxy` alongside the `auth.secretPath` it already set, and
`testHaproxyMaster` reaches the master *through* the proxy. That assertion fails
on the unfixed code, because an unauthenticated check leaves the pool empty.

## Date

2026-08-31
