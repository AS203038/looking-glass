# Deployment

This guide covers production deployment of Looking Glass. The server is stateless and intentionally simple to deploy: one binary, one YAML file, one port. Everything else is optional.

For a from-zero walkthrough use [getting-started.md](./getting-started.md). Read this page once you're ready to ship.

## Deployment targets

| Target                  | Best for                                           | See section                              |
| ----------------------- | -------------------------------------------------- | ---------------------------------------- |
| Bare binary + systemd   | A single host, no orchestration                    | [Bare metal](#bare-metal-systemd)        |
| Docker / Podman         | A single host, simple isolation                    | [Container](#container-docker--podman)   |
| Kubernetes              | Multiple replicas, ingress, shared cache           | [Kubernetes](#kubernetes)                |
| Behind a reverse proxy  | TLS termination, auth, rate limiting               | [Reverse proxy](#reverse-proxy)          |

The binary is the same in every case; only the wrapper differs.

## Build & release artefacts

* **GitHub Releases** publish prebuilt `looking-glass` and `lg-cli` binaries for several `GOOS/GOARCH` combinations along with `example.config.yaml`.
* **GitHub Container Registry** publishes the multi-arch image as `ghcr.io/as203038/looking-glass:<tag>`. `:latest` floats with main; tagged releases get an immutable tag.
* **Source builds** via `make build` produce identical binaries provided you pin the same Go toolchain and pass `VERSION=v…` so the embedded version string matches the release tag.

## Bare metal (systemd)

The server runs as a regular user-mode process. It does not need root, does not need any capabilities, and should not have any.

```bash
sudo useradd --system --home /var/lib/looking-glass --shell /usr/sbin/nologin lg
sudo install -d -o lg -g lg -m 0750 /var/lib/looking-glass

# Place the binary somewhere on PATH
sudo install -m 0755 looking-glass /usr/local/bin/

# Configuration & secrets
sudo install -o lg -g lg -m 0640 config.yaml /var/lib/looking-glass/config.yaml
sudo install -o lg -g lg -m 0400 lg-id_ed25519 /var/lib/looking-glass/lg-id_ed25519
```

`/etc/systemd/system/looking-glass.service`:

```ini
[Unit]
Description=Looking Glass
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=lg
Group=lg
WorkingDirectory=/var/lib/looking-glass
ExecStart=/usr/local/bin/looking-glass
Restart=on-failure
RestartSec=5

# Hardening (none of these break Looking Glass)
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
RestrictNamespaces=true
LockPersonality=true
SystemCallArchitectures=native
SystemCallFilter=@system-service

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now looking-glass
sudo journalctl -u looking-glass -f
```

The `WorkingDirectory` is critical: the server loads `config.yaml` from CWD.

## Container (Docker / Podman)

The bundled `Dockerfile` builds three isolated stages (`buf` → `pnpm` → `go`) and ships a `scratch` image. The final image contains exactly:

* `/looking-glass`        — the static binary, ENTRYPOINT
* `/etc/ssl/certs/ca-certificates.crt` — Mozilla CA bundle

There is no shell, no package manager, no other binaries.

```bash
# Run, mounting your config read-only
docker run -d --name lg \
  --restart=unless-stopped \
  -p 8080:8080 \
  -v /etc/looking-glass/config.yaml:/config.yaml:ro \
  -v /etc/looking-glass/ssh-key:/ssh-key:ro \
  ghcr.io/as203038/looking-glass:latest
```

If you reference router credentials via `ssh_key:`, the path inside the container must match the volume mount path (`/ssh-key` in the example above). Use Docker `secrets` or Podman `--secret` for the key file in production.

## Kubernetes

Looking Glass scales horizontally; deploy it as a regular `Deployment` with `replicas > 1` and a `Service` in front. The manifest below uses a `Secret` for sensitive material and a `ConfigMap` for the rest of `config.yaml`.

### Manifest pattern

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: looking-glass
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: lg-config
  namespace: looking-glass
data:
  # The non-sensitive part of config.yaml. Devices that need SSH
  # keys reference paths under /run/secrets/ which the Secret
  # below projects in.
  config.yaml: |
    devices:
      - name: "rt1.sto1"
        type: "frrouting"
        location: "Stockholm"
        hostname: "10.0.0.1:22"
        username: "lg"
        ssh_key: "/run/secrets/lg-key"
        source4: "192.0.2.1"
        source6: "2001:db8::1"
        vrf: "default"

    grpc:
      enabled: true
      listen: ":8080"
      tls:
        enabled: false                 # ingress terminates TLS

    redis:
      enabled: true
      ttl: 5m
      uri: "redis://redis.looking-glass.svc.cluster.local:6379/0?protocol=3"

    web:
      enabled: true
      title: "AS65000 Looking Glass"
---
apiVersion: v1
kind: Secret
metadata:
  name: lg-secrets
  namespace: looking-glass
type: Opaque
stringData:
  lg-key: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    …
    -----END OPENSSH PRIVATE KEY-----
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: looking-glass
  namespace: looking-glass
spec:
  replicas: 3
  selector:
    matchLabels: { app: looking-glass }
  template:
    metadata:
      labels: { app: looking-glass }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        seccompProfile: { type: RuntimeDefault }
      containers:
      - name: looking-glass
        image: ghcr.io/as203038/looking-glass:latest
        imagePullPolicy: IfNotPresent
        # The binary reads ./config.yaml — make / its workdir.
        workingDir: /
        ports:
          - { name: http, containerPort: 8080 }
        readinessProbe:
          # Native Kubernetes gRPC probe (>=1.24, GA 1.27). The
          # endpoint speaks the grpc.health.v1.Health protocol and
          # is POST-only — an `httpGet:` probe would issue GET and
          # the server would correctly reply 405 Method Not Allowed.
          # The mux marks the service Serving immediately after
          # startup.
          grpc:
            port: 8080
            service: lookingglass.v0.LookingGlassService
          initialDelaySeconds: 2
          periodSeconds: 10
        livenessProbe:
          tcpSocket: { port: 8080 }
          initialDelaySeconds: 10
          periodSeconds: 30
        resources:
          requests: { cpu: 50m,  memory: 64Mi }
          limits:   { cpu: 500m, memory: 256Mi }
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities: { drop: ["ALL"] }
        volumeMounts:
          - { name: config,  mountPath: /config.yaml, subPath: config.yaml, readOnly: true }
          - { name: secrets, mountPath: /run/secrets, readOnly: true }
      volumes:
        - name: config
          configMap: { name: lg-config }
        - name: secrets
          secret:
            secretName: lg-secrets
            defaultMode: 0400
---
apiVersion: v1
kind: Service
metadata:
  name: looking-glass
  namespace: looking-glass
spec:
  selector: { app: looking-glass }
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

A few things worth pointing out:

* **`workingDir: /`** is essential. `config.yaml` is mounted at `/config.yaml` via `subPath`, and the binary reads from CWD.
* **`readOnlyRootFilesystem: true`** works because nothing the server writes outside `/dev/stdout` — there is no on-disk state.
* **`replicas: 3`** is sized to your fault tolerance, not your load: even a single replica handles thousands of requests per minute.
* **`redis` is required for >1 replica** in any deployment that cares about router login pressure. The shared Redis is not just a response cache: the background health-check loop also uses it to elect a per-router prober every 60 s. Without Redis, every replica probes every router every minute (legacy uncoordinated behaviour); with Redis, exactly one replica per tick opens an SSH session to a given router and the rest read the published outcome. See [Multi-replica health coordination](#multi-replica-health-coordination) below.

### Ingress

Looking Glass speaks HTTP/2 for gRPC, so the ingress must too. The WebUI works fine on HTTP/1.1 but gRPC-Web requires HTTP/2 or gRPC-Web-over-HTTP/1.1 (which ConnectRPC supports out of the box).

NGINX-Ingress example:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: looking-glass
  namespace: looking-glass
  annotations:
    # NGINX needs an explicit hint that the upstream is HTTP/2.
    nginx.ingress.kubernetes.io/backend-protocol: "HTTP"
    # If using TLS:
    cert-manager.io/cluster-issuer: "letsencrypt"
spec:
  ingressClassName: nginx
  tls:
    - hosts: [lg.example.net]
      secretName: lg-tls
  rules:
    - host: lg.example.net
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: looking-glass
                port:
                  name: http
```

Behind Cloudflare or another HTTP/2-capable proxy, set `grpc.tls.enabled: false` and let the proxy terminate. ConnectRPC's h2c mode will accept HTTP/2 cleartext between the proxy and the pod.

## Multi-replica health coordination

Looking Glass does **not** perform Kubernetes-native leader election. It does not talk to the Kubernetes API, hold a `coordination.k8s.io/Lease`, or require any cluster RBAC.

What it does do, when `redis.enabled: true`, is gate every per-router health probe behind a Redis `SET … NX PX 70000` lease keyed on the router name. The replica that wins the `SETNX` race for a given tick opens the SSH session to that router; it then publishes the outcome (`{healthy, checked, source}`) under a second Redis key with a five-minute TTL. The losing replicas read that key and apply the outcome to their own in-memory `HealthCheck` record and gRPC health status without re-probing the router.

Concretely, for `N` replicas and `M` routers:

| Topology                  | Logins to each router  |
| ------------------------- | ---------------------- |
| 1 replica, no Redis       | 1 every 60 s           |
| N replicas, no Redis      | **N every 60 s**       |
| N replicas, shared Redis  | 1 every 60 s           |

The startup behaviour matches: there is no boot-time per-router probe goroutine. The first probe for a given router fires on the first 60-second tick after the leader-of-the-moment elects itself via Redis, which prevents an N×M login storm during rolling updates.

Operational implications:

* **Always point all replicas at the same Redis instance / DB.** Different `/N` slot per *deployment*, but the same slot within one deployment.
* **Redis outages degrade gracefully.** A lease-acquire or state-read error is logged at `WARN` on the `health` component and the replica probes locally for that tick. Health is never wrong; it can only fall back to "every replica probes every router" for the duration of the Redis outage.
* **gRPC health status is still per-process.** Each replica answers `/grpc.health.v1.Health/Check` from its own in-memory state, which is updated by the loop above. A replica that has only just started will report `NOT_SERVING` for each router until it has either won a lease or read a peer's state — typically within one 60-second tick.
* **Kubernetes `livenessProbe` and `readinessProbe` are unaffected.** The service-level health endpoint goes `SERVING` immediately at startup; only the per-router sub-statuses depend on the coordination loop.

If you need leader-election semantics stronger than a SETNX lease (e.g. fencing, leader monitoring, fairness), put a different prober binary in front and have the looking-glass replicas read state only. For the typical "N stateless replicas behind a Service" topology the Redis lease is sufficient.

## Reverse proxy

Looking Glass deliberately ships **no authentication, no rate limiting, and no IP allowlisting**. If you need any of these, put a reverse proxy in front.

### Caddy

```caddyfile
lg.example.net {
    encode zstd gzip

    # Hand HTTP/2 to the upstream (h2c).
    reverse_proxy http://lg.internal:8080 {
        transport http {
            versions h2c 2
        }
        header_up X-Forwarded-For {remote_host}
    }

    # Optional basic-auth example.
    @restricted not path /.well-known/*
    basicauth @restricted {
        ops $2a$14$abcdef…
    }
}
```

The `X-Forwarded-For` header is read by the access-log middleware, so the log lines will show real client IPs.

### NGINX

```nginx
upstream looking_glass {
    server lg.internal:8080;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name lg.example.net;
    ssl_certificate     /etc/ssl/certs/lg.crt;
    ssl_certificate_key /etc/ssl/private/lg.key;

    location / {
        proxy_pass http://looking_glass;
        proxy_http_version 1.1;
        proxy_set_header   Host             $host;
        proxy_set_header   X-Forwarded-For  $remote_addr;
        proxy_set_header   Upgrade          $http_upgrade;
        proxy_set_header   Connection       $connection_upgrade;

        # Generous timeouts: full BGP-table queries are slow.
        proxy_read_timeout    600s;
        proxy_send_timeout    600s;
    }

    # Optional rate limiting (10 req/s/IP, burst 20)
    limit_req_zone $binary_remote_addr zone=lg:10m rate=10r/s;
    location /lookingglass.v0.LookingGlassService/ {
        limit_req zone=lg burst=20 nodelay;
        proxy_pass http://looking_glass;
    }
}
```

NGINX **does not natively proxy gRPC over h2c**; for that, use the `grpc_pass` directive against a TLS upstream or rely on ConnectRPC's HTTP/1.1 fallback (the WebUI uses `@connectrpc/connect-web` which works fine over HTTP/1.1 too).

## TLS

You have three options, in increasing order of "let someone else worry about it":

1. **Server-terminated TLS** — set `grpc.tls.enabled: true` with `cert` + `key` paths. Useful when there is no reverse proxy.
2. **Self-signed certs** — set `grpc.tls.self_signed: true`. A fresh in-memory cert is generated at startup. Useful only when an upstream proxy re-terminates and you need *something* on the internal hop.
3. **Reverse-proxy / ingress termination** — `grpc.tls.enabled: false` and let the proxy do it. Recommended for any non-trivial deployment.

When using server-terminated TLS, both the certificate and key files must remain accessible to the running process; the server reads them at `Server.ListenAndServeTLS` time.

## Redis cache

The cache is optional but recommended for any deployment serving the public internet. It absorbs duplicate requests (which are common — humans paste the same prefix multiple times) and protects your routers from request fan-out across replicas.

Quick Kubernetes recipe (single replica, ephemeral, dev-grade):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: { name: redis, namespace: looking-glass }
spec:
  replicas: 1
  selector: { matchLabels: { app: redis } }
  template:
    metadata: { labels: { app: redis } }
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports: [{ name: redis, containerPort: 6379 }]
        args: ["--maxmemory", "256mb", "--maxmemory-policy", "allkeys-lru"]
        resources:
          requests: { cpu: 50m, memory: 64Mi }
          limits:   { cpu: 200m, memory: 320Mi }
---
apiVersion: v1
kind: Service
metadata: { name: redis, namespace: looking-glass }
spec:
  selector: { app: redis }
  ports: [{ port: 6379, targetPort: 6379 }]
```

Production deployments should use a managed Redis or a replicated sentinel setup; the cache only ever stores ephemeral data so the durability requirements are very weak.

Tuning notes:

* **TTL** is set per-instance via `redis.ttl`. 5 minutes is a sane default. Lower it if you have very dynamic BGP tables; raise it for pure-public-LG workloads.
* **`maxmemory-policy allkeys-lru`** is the right Redis policy: cache entries have no "must keep" property, evicting the LRU is safe.
* **Sharing across instances** — multiple Looking Glass replicas pointed at the same Redis share *both* the response cache and the per-router health-probe lease (see [Multi-replica health coordination](#multi-replica-health-coordination)). Multiple unrelated deployments must *not* share the same Redis DB — the lease keys are namespaced as `lg:health:lease:<router>` and `lg:health:state:<router>`, so two deployments sharing a DB will fight over each other's leases. Use different `/N` slots in the URI (`redis://host:6379/0` vs `…/1`) per deployment.

## Sentry

The Go server and the WebUI both report to Sentry when `web.sentry.enabled: true`. The same DSN, environment, and sample rate apply to both. Distributed traces propagate automatically via the `sentry-trace` and `baggage` headers, which are already in the CORS allow-list.

```yaml
web:
  sentry:
    enabled: true
    dsn: "https://abc123@o0.ingest.sentry.io/0"
    environment: "production"
    sample_rate: 0.1     # 10% of traces; errors are always captured
```

The server's release tag is `utils.Version()` minus the nanosecond suffix, so Sentry groups events by the `-X .../release=` value the binary was built with.

## Observability without Sentry

Even without Sentry the server is reasonably observable through logs alone:

* **Access log** — Structured `slog` JSON format on stdout (under the `httpaccess` component), including the `X-Cache` value (HIT / empty) and the request duration. This is enough to compute cache hit rate, p95 latency, and request volume from logs alone.
* **SSH log** — every dial / auth / exec failure is logged with a router tag (`router=… host=… user=…`) and any captured stderr (truncated to 512 bytes).
* **Health-check log** — state transitions only: `Router X is healthy` / `Router X is unhealthy: <reason>`. Steady-state probes are silent.

The standard `grpc.health.v1.Health/Check` endpoint is also wired up; per-router sub-statuses live at `<service>/<router name>`. It is a real gRPC service, so **every call must be `POST`** — the three supported wire formats (native gRPC over HTTP/2, gRPC-Web, and Connect / Connect-JSON) are all POST-only. A bare `GET` returns `405 Method Not Allowed`.

Use one of:

* **Kubernetes' native `grpc:` readiness probe** (>=1.24, GA 1.27), as in the manifest above. It speaks the gRPC health protocol over HTTP/2 directly.
* **`grpc_health_probe`** — the standard CLI tool, e.g. from a sidecar or `exec:` probe in older clusters:
  ```bash
  grpc_health_probe -addr=:8080 -service=lookingglass.v0.LookingGlassService
  ```
* **A POST'ed Connect-JSON call** — handy for `docker HEALTHCHECK`, monitoring scripts, or curl-based sanity checks:
  ```bash
  curl -fsS -X POST -H 'Content-Type: application/json' -d '{}' \
       http://localhost:8080/grpc.health.v1.Health/Check \
       | jq -e '.status == "SERVING_STATUS_SERVING"'
  ```

## Scaling

| Bottleneck                | Mitigation                                                                                                  |
| ------------------------- | ----------------------------------------------------------------------------------------------------------- |
| SSH session establishment | Enable Redis cache (deduplicates repeated requests).                                                         |
| Slow routers              | Reduce BGP table fetches via cache; increase TTL; serve cached/stale data is acceptable for a public LG.    |
| HTTPS termination         | Move TLS off the Looking Glass binary to an ingress.                                                         |
| Public abuse              | Add rate limiting at the reverse proxy. The server itself does not rate-limit.                               |
| Large fleets of routers   | Health-check goroutines are O(routers) but minute-scale; even hundreds of routers are fine on a small pod.   |
| Health-probe login storm  | Point every replica at the **same** Redis. The probe-lease coordinator collapses N×M logins/min to M/min.   |

The server is stateless: every replica can serve any request and the only cross-replica state is the (optional) shared Redis cache.

## Upgrading

1. Build / pull the new image. If you build, pass `VERSION=v1.2.3` so `GetInfo`, the WebUI footer, and Sentry releases all agree.
2. Roll the deployment. In Kubernetes:
   ```bash
   kubectl -n looking-glass set image deploy/looking-glass \
     looking-glass=ghcr.io/as203038/looking-glass:v1.2.3
   ```
3. The health endpoint goes `Serving` immediately after start, so the rolling update is fast.

There is no migration step. Configuration is forward-compatible: unknown keys are silently ignored, removed keys are silently defaulted, the protobuf service version stays at `v0` and is field-additive.

## Backup & restore

Looking Glass has no persistent state. The only thing to back up is `config.yaml` (plus any SSH keys it references). Keep them in version control (with the secrets encrypted, e.g. via `sops`) and you're done.

## Troubleshooting deployment issues

| Symptom                                              | Probable cause / fix                                                                                  |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Pod CrashLoopBackOff, log says "failed to parse config" | `config.yaml` not present in CWD. Check `workingDir` and `volumeMounts` paths.                       |
| All routers stuck "unhealthy"                        | First probe fires on the first 60-second tick (no boot-time probe). With shared Redis the lease holder probes; followers adopt the state. Past that, check `SSH:` log lines for the real reason. |
| Ingress 502 on gRPC requests                         | Reverse proxy not speaking HTTP/2 / h2c. See [Reverse proxy](#reverse-proxy).                         |
| `X-Cache: HIT` never appears                         | Redis URL parse error logged at startup; cache silently disabled.                                     |
| Looking Glass restarts every minute                  | The binary doesn't watch `config.yaml`. Restarts come from your supervisor — check liveness probes.   |
| WebUI shows "loading…" forever                       | `web.grpc_url` set to the wrong origin or CORS blocked by upstream proxy.                             |
