# Project Index

_Generated from per-project CLAUDE.md frontmatter. Regenerate with `bin/regen-projects-index.sh`._
_Last generated: 2026-06-02T06:29Z_

## Active — Priority Stack

| # | Project | Purpose | Path |
|---|---------|---------|------|
| 1 | **clearwatch** | Automated EDR/XDR vendor comparison report platform that generates and sells PDF/HTML analysis reports to mid-market IT teams. | `~/projects/clearwatch/` |
| 1 | **clearwatch-research** | Fractional competitive intelligence and sales enablement practice for B2B software companies — battle cards, CI briefs, competitive landscapes, retainer programs. | `~/projects/clearwatch-research/` |
| 2 | **infrastructure** | Runbooks, Terraform, and Ansible configurations for on-premise Proxmox and K8s homelab infrastructure that hosts all other projects. | `~/projects/infrastructure/` |
| 3 | **job-search-system** | Web application for tracking job applications, interviews, contacts, and research across AngelList, Greenhouse, and LinkedIn sources. | `~/projects/job-search-system/` |

## Active — Other

| Project | Purpose | Stack | Path |
|---------|---------|-------|------|
| **3dprint** | Multi-color 3D printing automation for Bambu P1S — parametric OpenSCAD designs and Python 3MF packaging. | python, openscad, pytest | `~/projects/3dprint/` |
| **aifleet** | Declarative GPU inference orchestration — Kubernetes CRDs define which models run on which GPU hosts. | go, kubernetes, docker | `~/projects/aifleet/` |
| **armies** | CLI tool giving AI agents persistent identity, XP, and role constraints via historical-figure profiles. | go | `~/projects/armies/` |
| **engram-go** | Go v2 persistent memory service for AI agents — 19 MCP tools, BM25 + vector + recency + knowledge graph recall. | go, postgresql, docker | `~/projects/engram-go/` |
| **linkedin** | LinkedIn post generation and publishing pipeline — draft scheduling, OAuth publishing, and CISO tracker. | python, docker, kubernetes | `~/projects/linkedin/` |
| **olla-fork** | Fork of the Olla LLM proxy/load balancer — custom patches for model alias enforcement and nil-panic fixes. | go | `~/projects/olla-fork/` |
| **resume-website** | Static resume site at resume.petersimmons.com — Chainguard Nginx on Kubernetes via Cloudflare tunnel. | html, kubernetes, nginx | `~/projects/resume-website/` |
| **substack** | Clearwatch Substack publication — AI/security analysis and vendor-independent EDR market research. | markdown, python, content | `~/projects/substack/` |
| **writers** | Research and tooling for replicating historical journalistic voices (Pyle, Murrow, Orwell, Gellhorn, Didion, Heller) in AI content. | python, markdown | `~/projects/writers/` |
| **www** | Personal site at www.petersimmons.com — magazine-style professional showpiece on Kubernetes via Cloudflare. | html, kubernetes, nginx | `~/projects/www/` |

## Reference

| Project | Purpose | Path |
|---------|---------|------|
| **advice** | Curated, source-cited security guidance knowledge base referenced by clearwatch and infrastructure. | `~/projects/advice/` |
| **quality-auditor** | Maintenance home for the quality-audit Claude skill (workslop detector for AI-generated output). | `~/projects/quality-auditor/` |
| **starter-kit** | Reference template demonstrating five cost-containment patterns for multi-agent AI systems. | `~/projects/starter-kit/` |

## Dormant

| Project | Purpose | Path |
|---------|---------|------|
| **character-study** | Universal LLM prompt that generates a self-contained HTML page matching a person to a fictional character. | `~/projects/character-study/` |
| **family** | Private decision-support and record-keeping system for family communications and strategy. | `~/projects/family/` |
| **karen** | Resume and career-documents workspace for Karen Simmons, including PDF/DOCX generation. | `~/projects/karen/` |
| **luck** | Voice-learning toolkit that scrapes Joe Genovese's blog and posts social content in his style. | `~/projects/luck/` |

## Archived

| Project | Purpose | Path | Notes |
|---------|---------|------|-------|
| **archived-democratic-csi-truenas** | K8s persistent-storage migration tooling — Longhorn to NFS with TrueNAS/Proxmox API scripts. | `~/projects/archived-democratic-csi-truenas/` | Directory prefix "archived-" confirms intentional archival. |
| **bambu** | MCP server exposing Bambu Lab P1S printer control to Claude Code via Docker. | `~/archive/bambu/` | Lives in ~/archive/ — superseded or parked. |
| **bambu** | MCP server exposing Bambu Lab P1S printer control to Claude Code via Docker. | `~/projects/homelab-config/archive/bambu/` | Lives in ~/archive/ — superseded or parked. |
| **engram** | Archived Python v1 of the Engram persistent memory service for AI agents; superseded by engram-go. | `~/projects/engram/` | v1 — no new features. Active work is in engram-go. NEVER run docker compose down -v. |
| **nextcloud-deployment** | Completed Nextcloud production deployment for nextcloud.petersimmons.com (Dec 2025). | `~/projects/archive/nextcloud-deployment/` | FINAL-PROJECT-ARCHIVE.md confirms status "COMPLETE — Production Ready" as of 2025-12-17. |

## Not Indexed
_Active projects deliberately excluded from this index:_
- `generals` and `security-intelligence-business` — deny-listed for direct edit; manually add frontmatter if/when needed.
- `kubernetes` — subsumed by `infrastructure`.
- `locked-shields`, `art-direction-research`, and `aifleet-*` worktree directories — no git remote.

## Parse Warnings
_The following files were skipped — no frontmatter or missing required fields:_
- `/home/psimmons/projects/agentgateway-v2/CLAUDE.md (no frontmatter)`
- `/home/psimmons/projects/generals/CLAUDE.md (no frontmatter)`
- `/home/psimmons/projects/homelab-config/CLAUDE.md (no frontmatter)`
- `/home/psimmons/projects/security-intelligence-business/CLAUDE.md (no frontmatter)`
