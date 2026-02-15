---
name: homelab:production-readiness
description: Use BEFORE deploying any new service to production - ensures PRR checklist is complete
---

# Production Readiness Review (PRR)

**Announce:** "Running Production Readiness Review before deployment..."

## Critical Rule

**No service goes to production without completing this checklist.**

## PRR Checklist

Walk through each category with the user. Mark items as they're completed.

### 1. Monitoring
- [ ] Prometheus metrics exposed
- [ ] Grafana dashboard created
- [ ] Logs configured and accessible

### 2. Alerting
- [ ] Critical alerts defined
- [ ] Alert routing tested
- [ ] Runbook linked to alerts

### 3. Backup & Recovery
- [ ] Automated backup configured
- [ ] Restore procedure tested
- [ ] RTO/RPO defined and documented

### 4. Documentation
- [ ] Runbook created with emergency procedures
- [ ] Architecture documented
- [ ] Configuration documented
- [ ] Failure modes documented

### 5. Security
- [ ] RBAC configured
- [ ] Network policies in place
- [ ] Secrets in proper secret management
- [ ] TLS configured

### 6. High Availability
- [ ] Liveness probe configured
- [ ] Readiness probe configured
- [ ] Resource limits set (CPU/memory)
- [ ] Replicas appropriate for service criticality
- [ ] PodDisruptionBudget if needed

### 7. Dependencies
- [ ] All external dependencies documented
- [ ] Graceful degradation for dependency failures
- [ ] Dependency health checks configured

### 8. Deployment
- [ ] Deployment automation working
- [ ] Rollback procedure tested
- [ ] Smoke tests defined

### 9. Performance
- [ ] Load testing completed (if applicable)
- [ ] Performance baselines established
- [ ] Capacity planning documented

### 10. Compliance
- [ ] Data retention policy defined
- [ ] Privacy requirements met
- [ ] Audit logging configured

### 11. Sign-Off
- [ ] Pre-production review completed
- [ ] Post-deployment verification plan ready

## After Completion

1. Save completed checklist to: `/home/psimmons/projects/<service-name>/PRR-CHECKLIST.md`
2. Update CLAUDE.md:
   - Add service to Failure Mode Catalog
   - Add service to Backup Validation Matrix
   - Add service to Service Dependency Graph
3. Announce: "PRR complete. Service approved for production deployment."

## Template Location

Full template: `/home/psimmons/templates/PRR-CHECKLIST.md`
