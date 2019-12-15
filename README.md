# ingress-nginx next

**POC objectives:**
- Use controller-runtime
- Watch ingress objects using informers
- Do not use global informers:
    - Secrets
    - Configmaps
    - Services
    - Endpoints

  The goal is to avoid access to objects not referenced by Ingress objects and narrow the amount of events handled by the controller

- Split updates:
    - Ingress -> usually changes nginx.conf (requires a reload)
    - Services -> upstreams (requires a reload)
    - Endpoints -> upstream servers
    - Secrets -> SSL Certificates
