# Router Hardening — Least-Privilege Accounts for Looking Glass

Looking Glass authenticates to every managed router as a regular SSH user and runs a **small, fixed set of commands** for each operation. Because the daemon is reachable from the public Internet (WebUI + gRPC), the SSH credential it holds is a high-value target: a leaked key or a successful injection into the LG itself becomes, in the worst case, shell on every device in the registry.

This document tells you how to neuter that risk by configuring a **bespoke, read-only-ish role** on each vendor — one that allows exactly the commands the LG needs and denies everything else.

Each section gives you:

1. **The command surface** the LG runs against that vendor (so you can verify the whitelist is sufficient).
2. **A copy-pasteable config snippet** creating the role/class/ profile/group, the user, and SSH-key authentication.
3. **Verification steps** — positive (LG operations work) and negative (`configure`, `reload`, etc. are denied).
4. **Caveats specific to the vendor**.

> **Operand safety is handled by the LG, not by the device role.** Every user-supplied operand is validated and re-rendered before reaching the SSH channel: IP/CIDR via `net.ParseIP` / `net.ParseCIDR`, communities from integer protobuf fields, AS-path regexes via [`utils.SanitizeASPathRegex`][san] which whitelists `\^?[0-9_]+\$?`. The role definitions below therefore don't need to guard against shell-metacharacter injection at the command-line level — they only need to bound *which commands* are permitted.

[san]: ../pkg/utils/sanitize.go

The recommended username throughout this document is `lookingglass`; substitute whatever fits your naming convention.

> **Always pair these snippets with the matching [`example.config.yaml`](../example.config.yaml) entry** — the looking-glass server-side `username:` and `ssh_key:` must match the user you create here.

---

## Why a bespoke role at all?

Most vendors split their CLI into two privilege strata:

| Vendor          | Lower level (login)              | Upper level (diagnostics)                 |
| --------------- | -------------------------------- | ----------------------------------------- |
| Arista EOS      | `>` exec, priv-1                 | `#` enable, priv-15                       |
| Cisco IOS/XE    | `>` user EXEC, priv-1            | `#` privileged EXEC, priv-15              |
| Juniper JunOS   | login class permissions          | `network` permission for ping/traceroute  |
| Nokia SR OS     | profile match rules              | `permit` entries                          |
| MikroTik 7      | per-policy boolean grants        | `test` policy for ping/traceroute         |
| FRRouting       | Linux user                       | shell access                              |

A naive "just run the LG as admin" deployment hands the upper stratum *and* everything else (configure, copy, reload, file operations) to whoever owns the LG's SSH key. The roles below give the LG **upper-stratum diagnostic access** (ping, traceroute, BGP show) and *nothing else*.

---

## Arista EOS

### Command surface

```
ping vrf <vrf> ip <addr>      | ping vrf <vrf> ipv6 <addr>
traceroute vrf <vrf> ip <addr>| traceroute vrf <vrf> ipv6 <addr>
show ip bgp …                 | show ipv6 bgp …
show ip bgp neighbors <peer> received-routes detail vrf <vrf>
show ip bgp neighbors <peer> routes detail vrf <vrf>
show ip bgp neighbors <peer> filtered-routes detail vrf <vrf>
show ip bgp neighbors <peer> advertised-routes detail vrf <vrf>
```

That is the *complete* surface from [`pkg/routers/arista_eos.yml`](../pkg/routers/arista_eos.yml).

### Configuration

```text
!
! Looking Glass automation user — minimum command surface
!
role lg-readonly
   10 permit mode exec command (ping|traceroute)( |$).*
   20 permit mode exec command show (ip|ipv6) bgp( |$).*
   100 deny mode exec command .*
!
username lookingglass privilege 15 role lg-readonly nopassword
username lookingglass sshkey ssh-ed25519 AAAAC3NzaC1lZDI1NTE5...
!
! REQUIRED #1: without this line every SSH session lands at priv-1
! and the user must type `enable` (which a non-interactive exec
! session cannot satisfy). With it, priv-15 is applied at login.
!
aaa authorization exec default local
!
! REQUIRED #2: without this line the `role lg-readonly` definition
! is parsed but never consulted — every command the user types is
! accepted regardless of the permit/deny entries. This is the line
! that actually enforces the whitelist.
!
aaa authorization commands all default local
!
```

If you also use TACACS+/RADIUS for login authentication, keep the `local` fallback on both `aaa authorization` lines so that an AAA partition does not lock the LG out:

```text
aaa authorization exec     default group tacacs+ local
aaa authorization commands all default group tacacs+ local
```

### Verification

```bash
# Positive checks
ssh lookingglass@arn01-border-a 'show privilege'
# → Current privilege level is 15

ssh lookingglass@arn01-border-a 'ping vrf default ip 1.1.1.1 repeat 1'
# → normal ping output

ssh lookingglass@arn01-border-a 'show ip bgp summary'
# → normal BGP summary

# Negative checks — must all be denied
ssh lookingglass@arn01-border-a 'configure'
# → % Authorization denied for command 'configure'

ssh lookingglass@arn01-border-a 'show running-config'
# → % Authorization denied for command 'show running-config'

ssh lookingglass@arn01-border-a 'reload'
# → % Authorization denied for command 'reload'
```

### Caveats

* **`aaa authorization exec default local` is mandatory.** Without it the `privilege 15` keyword on `username` is silently ignored and SSH exec lands at priv-1. The user then has to `enable`, which cannot be answered over an exec channel — and the LG would fail with `% Invalid input (privileged mode required)`.
* **`aaa authorization commands all default local` is also mandatory.** Without it the `role lg-readonly` rules are accepted by the parser but never enforced — the user gets priv-15 *and* every command on the box. Verify with the negative checks above; they must return `% Authorization denied`, not normal output.
* The `role lg-readonly` definition uses regex match on the *full* command string. The `10 permit … (ping|traceroute)( |$).*` form is permissive enough to allow the source / VRF / repeat flags the LG sends, but tight enough to disallow other top-level commands that happen to embed the word `ping`.
* `nopassword` + an SSH key is the cleanest credential model. If your security policy forbids `nopassword`, use a long random secret and rotate it; the LG never types it (it logs in by key).

---

## Cisco IOS / IOS-XE

### Command surface

```
ping vrf <vrf> <addr>           | ping vrf <vrf> ipv6 <addr>
traceroute vrf <vrf> <addr>     | traceroute vrf <vrf> ipv6 <addr>
show bgp vrf <vrf> ipv4 unicast …
show bgp vrf <vrf> ipv6 unicast …
show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> received-routes
show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> routes
show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> received-routes filtered
show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> advertised-routes
```

That is the *complete* surface from [`pkg/routers/cisco_ios.yml`](../pkg/routers/cisco_ios.yml).

Two equally good ways to lock this down. Pick whichever fits your fleet.

### Option 1 — Parser view (modern RBAC, recommended)

```text
!
aaa new-model
aaa authentication login default local
aaa authorization exec default local
aaa authorization commands 15 default local if-authenticated
!
parser view LG-READONLY
   secret 0 LG-READONLY-secret-unused
   commands exec include ping
   commands exec include traceroute
   commands exec include show bgp
   commands exec include show ip bgp
   commands exec include show ipv6 bgp
!
username lookingglass view LG-READONLY privilege 15 secret 0 <strong-random>
ip ssh pubkey-chain
   username lookingglass
      key-string
         AAAAC3NzaC1lZDI1NTE5...
      exit
   exit
exit
!
```

### Option 2 — Privilege-level demotion (classic, simpler)

```text
!
privilege exec level 5 show bgp
privilege exec level 5 show ip bgp
privilege exec level 5 show ipv6 bgp
privilege exec level 5 ping
privilege exec level 5 traceroute
!
username lookingglass privilege 5 secret 0 <strong-random>
! plus pubkey config as above
!
```

### Verification

```bash
ssh lookingglass@rt 'show privilege'         # → 15  (Option 1) or 5 (Option 2)
ssh lookingglass@rt 'show bgp ipv4 unicast summary'  # works
ssh lookingglass@rt 'configure terminal'     # denied
ssh lookingglass@rt 'show running-config'    # denied
```

### Caveats

* **Option 1** requires you to also configure `aaa authorization commands 15 default local if-authenticated` — without it the `parser view` is created but not enforced.
* **Option 2** is easier to write but uglier to audit: privilege level 5 ends up implicitly granting *every* command at level ≤ 5. Verify against `show parser dump exec` after applying.
* The LG only authenticates by SSH key; the `secret` line is a fallback for console emergencies. Use a high-entropy value and rotate it independently.

---

## Juniper JunOS

### Command surface

```
ping <addr> source <src> count 5 rapid routing-instance <vrf>
traceroute <addr> source <src> routing-instance <vrf>
show bgp summary instance <vrf>
show route protocol bgp … table <vrf>.inet[6].0 detail
show route protocol bgp community <c> table … detail
show route protocol bgp large-community <lc> table … detail
show route protocol bgp aspath-regex "<r>" table … detail
show route receive-protocol bgp <peer>
show route protocol bgp neighbor <peer> table <vrf>.inet[6].0 detail
show route receive-protocol bgp <peer> filtered
show route advertising-protocol bgp <peer>
```

That is the *complete* surface from [`pkg/routers/juniper_junos.yml`](../pkg/routers/juniper_junos.yml).

### Configuration

```junos
system {
    login {
        class lg-readonly {
            permissions [ network ];
            allow-commands "^(show route protocol bgp|show bgp summary|ping|traceroute)( |$)";
            deny-commands "^(configure|edit|set|delete|clear|file|request|start|restart|test|monitor|op|load|save|copy)( |$)";
        }
        user lookingglass {
            class lg-readonly;
            authentication {
                ssh-ed25519 "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5...";
            }
        }
    }
}
```

### Verification

```bash
ssh lookingglass@rt 'show system users'           # → user shown with class lg-readonly
ssh lookingglass@rt 'ping 1.1.1.1 count 1 rapid'  # works
ssh lookingglass@rt 'show bgp summary'            # works
ssh lookingglass@rt 'configure'                   # → error: permission denied
ssh lookingglass@rt 'request system reboot'       # denied
ssh lookingglass@rt 'show configuration'          # denied (no view-configuration permission)
```

### Caveats

* `permissions [ network ]` is the JunOS permission flag that grants `ping` and `traceroute`. Don't omit it or both diagnostics will fail even though `allow-commands` lists them.
* `allow-commands` and `deny-commands` are evaluated as regexes. Anchor with `^` and use word-boundary patterns like `( |$)` to avoid accidentally matching longer commands.
* If you need RPC-over-NETCONF for other automation, add `permissions [ network admin-control ]` — but that broadens the blast radius. The LG does not use NETCONF; keep it off if the LG is your only consumer for this user.

---

## Nokia SR OS (classic CLI)

### Command surface

```
ping <addr> source <src> count 5 router <vrf>
traceroute <addr> source <src> router <vrf>
show router <vrf> bgp summary
show router <vrf> bgp routes <addr> hunt
show router <vrf> bgp routes community <c> detail
show router <vrf> bgp routes community large:<lc> detail
show router <vrf> bgp routes aspath-regex "<r>" detail
show router <vrf> bgp routes neighbor <peer> received hunt
show router <vrf> bgp routes neighbor <peer> hunt
show router <vrf> bgp routes neighbor <peer> rejected hunt
show router <vrf> bgp routes neighbor <peer> advertised hunt
```

That is the *complete* surface from [`pkg/routers/nokia_sros.yml`](../pkg/routers/nokia_sros.yml).

### Configuration

```text
configure system security
    profile "lg-readonly"
        default-action deny-all
        entry 10
            match "show router"
            action permit
        exit
        entry 20
            match "ping"
            action permit
        exit
        entry 30
            match "traceroute"
            action permit
        exit
    exit
    user "lookingglass"
        access console
        console
            member "lg-readonly"
            no member "default"
        exit
        public-keys
            ecdsa
                ecdsa-key 1 create
                    key-value "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5..."
                exit
            exit
        exit
    exit
exit all
```

### Verification

```bash
ssh lookingglass@rt 'show router 1 bgp summary'   # works
ssh lookingglass@rt 'ping 1.1.1.1 count 1'        # works
ssh lookingglass@rt 'admin reboot'                # denied
ssh lookingglass@rt 'configure'                   # denied
ssh lookingglass@rt 'show system version'         # denied
```

### Caveats

* SR OS profile entries are evaluated in numeric order; the first match wins. `default-action deny-all` is essential — without it unmatched commands fall through to a permissive default.
* The MD-CLI variant uses a different config grammar; the snippet above is for the **classic CLI**, which the [`nokia_sros.yml`](../pkg/routers/nokia_sros.yml) template targets.
* `no member "default"` is critical. The implicit `default` profile grants broad operational access; strip it explicitly when the user should be confined to `lg-readonly`.

---

## MikroTik RouterOS 7

### Command surface

```
/ping <addr> src-address=<src> count=5 routing-table=<vrf>
/tool traceroute <addr> src-address=<src> routing-table=<vrf> count=1 duration=10s
/routing bgp session print detail without-paging
/routing bgp advertisements print detail where …
/routing route print detail where peer="<peer>"
/routing route print detail where peer="<peer>" and !filtered
/routing route print detail where peer="<peer>" and filtered
/routing bgp advertisements print detail where peer="<peer>"
```

That is the *complete* surface from [`pkg/routers/mikrotik_routeros.yml`](../pkg/routers/mikrotik_routeros.yml).

### Configuration

```routeros
/user group add name=lg-readonly \
    policy=ssh,read,test,!local,!telnet,!winbox,!web,!ftp,!api,!reboot,!write,!policy,!password,!sniff,!sensitive,!romon,!dude,!tikapp

/user add name=lookingglass group=lg-readonly disabled=no comment="Looking Glass automation"

# Import the LG's public key (file uploaded to the router first)
/user ssh-keys import public-key-file=lookingglass.pub user=lookingglass
```

Required policies, in detail:

* `ssh` — log in over SSH
* `read` — read configuration (needed for `/routing bgp …`)
* `test` — execute `/ping` and `/tool traceroute`

Everything else is **explicitly negated** with `!policy`. RouterOS defaults are permissive; the negations matter.

### Verification

```bash
ssh lookingglass@rt '/ping 1.1.1.1 count=1'                       # works
ssh lookingglass@rt '/routing/bgp/session/print detail'           # works
ssh lookingglass@rt '/system reboot'                              # denied
ssh lookingglass@rt '/file print'                                 # denied
ssh lookingglass@rt '/user print'                                 # denied
```

### Caveats

* RouterOS 7.x renamed several BGP-related paths from 6.x; this role assumes 7.13+. On older 7.x train you may need `/routing/bgp/peer` instead of `/routing/bgp/session`.
* The `test` policy is the surprise. Without it `/ping` and `/tool traceroute` both fail with `not enough permissions`.
* `policy=…` is comma-separated and order-insensitive. Use the `!negation` form for everything you don't grant — relying on implicit defaults is brittle across RouterOS upgrades.

---

## FRRouting (Linux)

FRR is unique among the supported vendors: there is no in-process RBAC. The "role" is enforced at the OpenSSH layer instead, by combining `ForceCommand` with a small whitelist wrapper script.

### Command surface

```
ping -n -4 -c5 -I <src> <addr>           | ping -n -6 -c5 -I <src> <addr>
traceroute -4 -w 1 -q1 -I --back --mtu -e -s <src> <addr>
traceroute -6 -w 1 -q1 -I --back --mtu -e -s <src> <addr>
vtysh -c 'show bgp vrf <vrf> ipv4 unicast …'
vtysh -c 'show bgp vrf <vrf> ipv6 unicast …'
vtysh -c 'show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> received-routes json'
vtysh -c 'show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> routes json'
vtysh -c 'show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> received-routes filtered json'
vtysh -c 'show bgp vrf <vrf> ipv4/ipv6 unicast neighbors <peer> advertised-routes json'
```

That is the *complete* surface from [`pkg/routers/frrouting.yml`](../pkg/routers/frrouting.yml).

### Configuration

**1. Create the Linux user with no interactive shell:**

```bash
sudo adduser --system --group --home /var/lib/lookingglass --shell /usr/sbin/nologin lookingglass
sudo usermod -a -G frrvty lookingglass             # required to read FRR vtysh socket
sudo install -d -o lookingglass -g lookingglass -m 700 /var/lib/lookingglass/.ssh
```

**2. Install the whitelist wrapper as `/usr/local/sbin/lg-shell`:**

```bash
sudo tee /usr/local/sbin/lg-shell <<'EOF' >/dev/null
#!/bin/bash
# /usr/local/sbin/lg-shell — Looking Glass command whitelist.
# Invoked by sshd ForceCommand; the original SSH client command is
# in $SSH_ORIGINAL_COMMAND. Only commands matching the patterns
# below are executed; everything else is rejected with exit 126.
set -eu
cmd="${SSH_ORIGINAL_COMMAND:-}"

# Reject any command containing shell metacharacters as defence in
# depth. The LG renders all operands through strict validators
# (pkg/utils/sanitize.go, net.ParseIP / net.ParseCIDR) so these
# bytes can never legitimately appear; their presence indicates
# either a misconfigured LG or active tampering.
case "$cmd" in
    *';'*|*'|'*|*'&'*|*'`'*|*'$('*|*'>'*|*'<'*|*$'\n'*|*$'\r'*)
        echo "lg-shell: rejected metacharacters" >&2
        exit 126
        ;;
esac

case "$cmd" in
    "ping -n -4 -c5 -I "*\ *|"ping -n -6 -c5 -I "*\ *)
        exec /bin/bash -c -- "$cmd"
        ;;
    "traceroute -4 "*|"traceroute -6 "*)
        exec /bin/bash -c -- "$cmd"
        ;;
    "vtysh -c 'show bgp "*"'")
        exec /bin/bash -c -- "$cmd"
        ;;
    *)
        echo "lg-shell: command not in whitelist: $cmd" >&2
        exit 126
        ;;
esac
EOF
sudo chmod 0755 /usr/local/sbin/lg-shell
```

**3. Constrain the SSH session in `sshd_config`:**

```text
# /etc/ssh/sshd_config.d/40-lookingglass.conf
Match User lookingglass
    ForceCommand /usr/local/sbin/lg-shell
    PermitTTY no
    AllowTcpForwarding no
    AllowAgentForwarding no
    X11Forwarding no
    PermitTunnel no
    PermitUserRC no
    AuthenticationMethods publickey
```

Reload sshd:

```bash
sudo sshd -t && sudo systemctl reload ssh
```

**4. Install the LG public key:**

```bash
sudo -u lookingglass tee -a /var/lib/lookingglass/.ssh/authorized_keys <<EOF
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5... lookingglass@server
EOF
sudo chmod 600 /var/lib/lookingglass/.ssh/authorized_keys
```

### Verification

```bash
# Positive checks
ssh lookingglass@rt 'ping -n -4 -c5 -I 192.0.2.1 1.1.1.1'       # works
ssh lookingglass@rt "vtysh -c 'show bgp vrf default ipv4 unicast summary json'"  # works

# Negative checks
ssh lookingglass@rt 'whoami'                  # → exit 126, "not in whitelist"
ssh lookingglass@rt 'cat /etc/passwd'         # → exit 126
ssh lookingglass@rt 'ls; cat /etc/shadow'     # → exit 126, "rejected metacharacters"
ssh lookingglass@rt -t                        # → no PTY (sshd PermitTTY no)
```

### Caveats

* The wrapper deliberately re-invokes `bash -c -- "$cmd"` *after* the case-pattern accepts the command. The metacharacter check earlier in the script makes injection through the whitelisted prefixes structurally impossible. **Do not** modify the metachar reject without thinking through how the LG renders operands.
* `usermod -a -G frrvty lookingglass` is mandatory — without it `vtysh` falls back to read-only mode that can't talk to bgpd.
* `traceroute` requires raw-socket privileges. On most distros the binary is setuid or has `cap_net_raw=ep`; the LG-side user does not need additional capabilities, but verify with `getcap $(command -v traceroute)`.
* If you also want to harden against a key reuse for unintended hosts, use the `from="ip,ip"` and `restrict` directives in `authorized_keys` instead of (or in addition to) `Match User …`.

---

## Cross-vendor verification checklist

After applying the role on each device, run this from the LG host:

```bash
# Confirm LG operations succeed:
lg-cli ping <instance> <router> 1.1.1.1
lg-cli traceroute <instance> <router> 1.1.1.1
lg-cli bgp summary <instance> <router>
lg-cli bgp route <instance> <router> 1.1.1.1
lg-cli bgp community <instance> <router> 64500:100
lg-cli bgp aspath <instance> <router> 64500

# Confirm escalation is denied (this should *fail*):
ssh lookingglass@<router>-host '<vendor-specific-write-command>'
```

If all positive checks succeed and the negative check is denied, the role is correctly scoped.

## Maintenance notes

* **When a template changes**, audit this document. New commands added to a vendor YAML need new permit rules in the matching role. The diff is the contract.
* **Rotate keys** at least annually. The LG accepts both `password:` and `ssh_key:` in `config.yaml`; prefer the key.
* **Audit logs** server-side: every device should log AAA events for the `lookingglass` user. A surge of denied commands is the first signal that either the LG was upgraded with new commands (benign — update the role) or someone is probing the credential (not benign).

## See also

* [getting-started.md](./getting-started.md) — initial setup, links here in the "Configure your first router" section.
* [router-templates.md](./router-templates.md) — template authoring reference, cross-links to the per-vendor command surface tables above.
* [`example.config.yaml`](../example.config.yaml) — annotated reference for the LG-side `devices[].username` / `ssh_key` fields.
