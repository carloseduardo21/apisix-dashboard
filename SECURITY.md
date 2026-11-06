# Security policy

Lintel is the control plane of an API gateway and is meant for PCI DSS
environments, so security reports are handled with priority.

## Supported versions

Until 1.0, only the **latest release** (and `main`) receives security fixes.

## Reporting a vulnerability

**Do not open a public issue, pull request or discussion for a vulnerability.**

Report it privately through GitHub:
[**Report a vulnerability**](https://github.com/carloseduardo21/apisix-dashboard/security/advisories/new)
(Security tab → *Report a vulnerability*).

Please include:

- the affected version or commit, and how Lintel is deployed (binary, Docker, Swarm,
  TLS at a proxy or native);
- the impact (e.g. authentication bypass, privilege escalation, secret disclosure,
  SSRF through the Admin API) and the role needed to exploit it;
- steps to reproduce or a proof of concept;
- any suggested fix.

Never include real admin keys, passwords, TOTP secrets, certificates or customer data
in a report: use placeholders.

## What to expect

- Acknowledgement within **5 business days**.
- An initial assessment (confirmed or not, severity) within **10 business days**.
- A fix or mitigation plan agreed with you; coordinated disclosure through a GitHub
  security advisory (with a CVE when applicable) once a fixed release is available.
  The default disclosure deadline is **90 days** from the report.
- Credit in the advisory, unless you prefer to stay anonymous.

This is a volunteer-maintained project; the timelines above are best effort.

## Scope

In scope: this repository (Go backend, embedded web UI, container image, the
example Swarm stack).

Out of scope, please report upstream: vulnerabilities in Apache APISIX itself
(<https://apisix.apache.org/docs/general/security/>), etcd, Kong or other
third-party software, unless Lintel makes them exploitable.

Not considered vulnerabilities on their own:

- actions an `operator` or `admin` is allowed to perform by design (e.g. enabling a
  plugin APISIX accepts), unless they cross a privilege boundary Lintel promises;
- findings that require disabling documented security defaults
  (`DASH_COOKIE_SECURE=false`, `DASH_TRUST_PROXY_HEADERS=true` without a trusted
  proxy, no TLS outside `localhost`);
- missing hardening headers on non-`/api` responses with no demonstrated impact;
- reports from automated scanners without a demonstrated impact.

## Hardening guidance

See the *Security controls and PCI DSS 4.0* section of the [README](README.md) for
the controls Lintel provides and the responsibilities left to operators.
