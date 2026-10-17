# Known issues

The merged implementation includes PostgreSQL migrations, integer-cent money,
atomic admin bootstrap, CSRF integration, method-aware throttling, strict
production configuration, graceful draining, and deployment recovery guidance.

Remaining limitations:

- Cash on delivery is the only implemented payment method and stays pending.
  Online providers need verified callbacks, reconciliation, and refund support.
- Partner accounts require explicit `partner_users` membership. Store settings
  remain platform-wide, and mixed-partner orders require staff fulfillment.
- Shipping and tax are fixed estimates and need business-specific policies.
- Durable email delivery, production monitoring, and external SMTP/TLS validation
  require deployment work beyond the local test suite.

See [the roadmap](roadmap/ROADMAP.md) and [deployment runbook](DEPLOYMENT.md).
