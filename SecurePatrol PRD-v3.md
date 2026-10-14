# Product Requirements Document
## GARDA — Enterprise Security Patrol & Attendance Management System

**Version:** 1.2 Draft  
**Date:** 2026-09-30  
**Status:** Draft — Pending Stakeholder Review

---

## 1. Executive Summary

GARDA is a self-hosted, enterprise-grade platform for managing security guard patrols, daily attendance, incident reporting, and operational oversight across multiple client sites. Field staff use an Android/iOS tablet app; administrators, supervisors, HR, and site owners use a responsive web dashboard. The system operates offline-capable on tablets and syncs to the on-premise server when connectivity is restored.

The entire platform runs as a single Docker Compose stack — designed to be operated by one person without deep DevOps expertise.

**v1 launch target:** 2 client sites, up to 10 security guards.

---

## 2. Goals & Success Metrics

| Goal | Success Metric |
|---|---|
| Eliminate paper patrol logs | 100% of checkpoints recorded digitally within 30 days of go-live |
| Reduce missed patrol SLA breaches | SLA alerts fired within 2 minutes of a missed checkpoint |
| Accurate attendance for payroll | < 1% attendance discrepancy vs. manual HR records after 60 days |
| Faster incident response | SOS alert delivered to supervisor in < 30 seconds |
| Client transparency | Site owner can view live patrol status without contacting ops |
| Low operational overhead | Single operator can manage platform with < 2 h/week admin time |

---

## 3. User Roles & Permissions

| Role | Primary Interface | Key Permissions |
|---|---|---|
| **Security Guard** | Tablet (Android / iOS) | Clock in/out, scan checkpoints, file incident/defect/visitor reports, SOS |
| **Shift Supervisor** | Tablet + Web | All guard permissions + view team status, receive SOS alerts, acknowledge incidents |
| **Admin / Operations Manager** | Web | Full access: sites, schedules, guards, reports, system config |
| **HR / Payroll Team** | Web | Read attendance data, view monthly recap report |
| **Client / Site Owner** | Web (read-only) + scheduled email report | View live patrol map for their site, download attendance & incident reports; also receive monthly summary by email |

---

## 4. Functional Requirements

### 4.1 Authentication & Access

- **FR-AUTH-01** Tablet login via PIN (4–6 digit) or password; session persists until explicit logout or shift end.
- **FR-AUTH-02** Web login via username + password through Keycloak; optional 2FA (TOTP) for admin/HR roles — configured in Keycloak admin UI, no custom code required.
- **FR-AUTH-03** Role-based access control (RBAC) managed in Keycloak; JWT claims carry role; the Go API validates tokens without touching credentials.
- **FR-AUTH-04** Auto-lock tablet after configurable idle timeout; guard must re-enter PIN.
- **FR-AUTH-05** Tablet PIN auth handled by the mobile app locally; full web SSO via Keycloak OIDC. Tablets are shared per site and authenticate to the API with a device token (see OQ-9).
- **FR-AUTH-06** Each client has its own Keycloak realm; user namespaces are fully isolated between clients.

### 4.2 Attendance & Shift Management

- **FR-ATT-01** Admin creates fixed shift schedules: guard, site, start time, end time, repeat pattern.
- **FR-ATT-02** Guard clocks in/out within an admin-configured window (e.g., ±15 min around shift start/end).
- **FR-ATT-03** Clock-in records: timestamp, GPS coordinates, optional selfie photo (photo stored in MinIO; DB stores only the object key).
- **FR-ATT-04** System auto-flags a guard as **Absent** if no clock-in is received within X minutes after shift start (X configurable per site, default 30 min). Notification sent to supervisor.
- **FR-ATT-05** Late arrival and early departure tracked and labeled in reports.
- **FR-ATT-06** Admin can manually override attendance status with a reason (audited).

### 4.3 Patrol & Checkpoint Management

- **FR-PAT-01** Admin defines patrol routes: ordered list of named checkpoints per site. Each checkpoint has a QR code identifier. SLA window per checkpoint (time allowed since previous checkpoint) is **dynamically configurable per route** — admin sets it in the route editor, no system-wide default enforced.
- **FR-PAT-02** Guard scans checkpoint QR code on tablet; app records: checkpoint ID, timestamp, GPS, guard ID.
- **FR-PAT-03** Manual check-in fallback: guard taps checkpoint on app and attaches a required photo (stored in MinIO).
- **FR-PAT-04** If an SLA window is configured for a checkpoint and is exceeded, an alert fires to the supervisor and optionally to admin. Routes with no SLA set produce no timing alerts.
- **FR-PAT-05** Patrol session shows guard real-time progress through the route on a map.
- **FR-PAT-06** Admin can define multiple routes per site (day/night, weekday/weekend).

### 4.4 Incident Reporting

- **FR-INC-01 — Incident Report:** Guard submits a typed description, severity level (Low / Medium / High / Critical), category (theft, trespassing, medical, etc.), timestamp, location, and up to 5 photos (stored in MinIO). Supervisor notified immediately.
- **FR-INC-02 — SOS / Panic Button:** Single-tap button on tablet home screen. Fires an immediate alert to all supervisors at the site with guard's last-known GPS. Alert persists until acknowledged by a supervisor.
- **FR-INC-03 — Equipment / Facility Defect Report:** Guard logs defect type, location, description, and photo. Routed to admin for follow-up; status tracked (Open → In Progress → Resolved).
- **FR-INC-04 — Visitor / Vehicle Log:** Guard records visitor name, ID type & number, vehicle plate (optional), host name, entry/exit time, and purpose. Photo of ID/vehicle stored in MinIO.
- **FR-INC-05** All incident records are immutable once submitted; corrections require a supervisor-approved amendment with reason (audited).

### 4.5 Offline Operation

- **FR-OFF-01** Tablet app stores all pending actions (clock-ins, checkpoint scans, reports) in a local SQLite database when network is unavailable.
- **FR-OFF-02** Sync runs automatically when connectivity is restored; conflicts resolved server-side with last-write-wins on non-critical data and supervisor review queue for attendance conflicts.
- **FR-OFF-03** QR codes and route definitions are pre-cached on device at shift start or last successful sync.
- **FR-OFF-04** SOS attempts are queued and retried every 10 seconds until delivered; guard is shown a visual warning if SOS could not be confirmed delivered.
- **FR-OFF-05** Photos captured offline are uploaded to MinIO during sync; records referencing them show a "pending upload" state until confirmed.

### 4.6 Notifications

- **FR-NOT-01** Notification channels: **Email (SMTP)** and **WhatsApp Business API** only (decision 2026-10-13, D-050: LINE and Telegram are not planned).
- **FR-NOT-02** Admin configures which event types go to which channel per site. SMTP credentials and WhatsApp API keys configured in system settings (not hardcoded).
- **FR-NOT-03** Event triggers: SOS, missed checkpoint SLA, absent auto-flag, late clock-in, incident filed (High/Critical), defect opened.
- **FR-NOT-04** Notification delivery failures are logged; admin can retry or view failure reason.
- **FR-NOT-05** Monthly attendance recap email auto-sent to configured recipients (admin, HR, client site owner) on the 1st of each month covering the prior month.

### 4.7 Web Dashboard — Reports

- **FR-REP-01 — Daily Attendance Summary:** Per-site table of all scheduled guards: Present / Absent / Late / Early-Out, clock-in/out times, total hours. Exportable to CSV/Excel.
- **FR-REP-02 — Patrol Route Completion Report:** Per-route, per-shift view of checkpoints hit, missed, or late. Timeline visualization. Exportable.
- **FR-REP-03 — Incident Log:** Filterable by site, date range, type, severity, status. Full detail view with photo thumbnails (served from MinIO pre-signed URLs). Export to PDF / CSV.
- **FR-REP-04 — Guard Performance / KPI Dashboard:** Per-guard metrics: attendance rate, on-time rate, patrol completion %, incidents filed, SLA breach count. Configurable date range.
- **FR-REP-05 — Monthly Recap Report:** Aggregated per-guard summary for a calendar month: total days present/absent/late, total hours, patrol completion rate. Viewable on web and auto-emailed. This is the primary HR payroll reference — no direct payroll system integration in v1.
- **FR-REP-06** Client / Site Owner sees FR-REP-01 through FR-REP-03 and FR-REP-05 scoped to their site(s) only.

---

## 5. Non-Functional Requirements

| ID | Category | Requirement |
|---|---|---|
| NFR-01 | Deployment | Self-hosted on-premise; Docker Compose single-stack. `docker compose up -d` brings up all services. |
| NFR-02 | Data residency | All data and media stay on-premise; no external cloud storage. MinIO provides S3-compatible local object storage. |
| NFR-03 | Performance | Web dashboard loads core pages in < 2 s on LAN; tablet sync completes < 10 s on 4G. |
| NFR-04 | Availability | Server target 99.5% uptime on LAN; scheduled maintenance windows communicated 24 h in advance. |
| NFR-05 | Security | HTTPS (TLS 1.2+) everywhere; authentication via Keycloak OIDC; JWT validation in Go API; audit log for all admin actions. Keycloak handles brute-force protection and session management. |
| NFR-06 | v1 Capacity | 2 client sites, up to 10 guards. DB and MinIO sized accordingly; architecture scales to 50 sites / 500 guards without re-work. |
| NFR-07 | Backup | Daily automated `pg_dump` to a MinIO bucket (versioned); admin-triggered manual backup via web UI; restore procedure documented. |
| NFR-08 | Accessibility | Web dashboard WCAG 2.1 AA for form inputs, tables, and alerts. |
| NFR-09 | Single-operator ops | All routine tasks (user management, schedule changes, report exports, backups) achievable via web UI without CLI access. |

---

## 6. System Architecture (High-Level)

```
┌─────────────────────────┐        ┌──────────────────────────────────────────┐
│   Tablet App            │        │   On-Premise Server (Docker Compose)     │
│   (iOS / Android)       │◄──────►│                                          │
│                         │ HTTPS  │  ┌─────────────────────────────────────┐ │
│  • Offline SQLite       │        │  │  Go API  (Fiber framework)          │ │
│  • QR scanner           │        │  │  • REST endpoints                   │ │
│  • GPS + Camera         │        │  │  • WebSocket (live patrol updates)  │ │
└─────────────────────────┘        │  │  • JWT validation (Keycloak JWKS)   │ │
                                   │  └──────────────┬──────────────────────┘ │
┌─────────────────────────┐        │                 │                        │
│   Web Dashboard         │◄──────►│  ┌──────────────▼──────────────────────┐ │
│   (React / Vue SPA)     │ HTTPS  │  │  PostgreSQL                         │ │
│                         │        │  │  Structured data: shifts, scans,    │ │
│  • Admin / HR / Client  │        │  │  incidents, attendance, audit logs  │ │
│  • Reports & Config     │        │  └─────────────────────────────────────┘ │
└─────────────────────────┘        │                                          │
           │                       │  ┌──────────────────────────────────────┐ │
           │  OIDC login           │  │  MinIO (S3-compatible object store)  │ │
           ▼                       │  │  Photos, backup dumps                │ │
┌─────────────────────────┐        │  └──────────────────────────────────────┘ │
│   Keycloak              │        │                                          │
│   One realm per client  │◄──────►│  ┌──────────────────────────────────────┐ │
│   • RBAC / OIDC         │        │  │  Notification Worker (Go goroutine) │ │
│   • 2FA (TOTP)          │        │  │  SMTP / WhatsApp                    │ │
│   • Brute-force protect │        │  │                                      │ │
└─────────────────────────┘        │  └──────────────────────────────────────┘ │
                                   │                                          │
                                   │  ┌──────────────────────────────────────┐ │
                                   │  │  Redis                               │ │
                                   │  │  Notification queue, rate limiting   │ │
                                   │  └──────────────────────────────────────┘ │
                                   └──────────────────────────────────────────┘
```

---

## 7. Tech Stack

| Layer | Technology | Rationale |
|---|---|---|
| **API backend** | Go + Fiber | High concurrency for WebSocket patrol updates and SOS fan-out; low memory footprint; single compiled binary simplifies ops |
| **Web frontend** | React (Vite) or Vue 3 | Mature ecosystem; component libraries for tables/charts |
| **Mobile app** | Flutter | Single codebase for iOS + Android tablet; offline SQLite via `sqflite` |
| **Database** | PostgreSQL 16 | ACID, strong query planner for reports, JSONB for flexible config fields |
| **Object storage** | MinIO | S3-compatible, on-premise; stores all photos and backup dumps; no data leaves the site |
| **Identity & Auth** | Keycloak 26 | OIDC/OAuth2, RBAC, TOTP 2FA, brute-force protection — one realm per client, no custom auth code |
| **Job queue / cache** | Redis | Notification queue, rate limiting, WebSocket presence |
| **Notifications** | Go worker | SMTP + WhatsApp Business API only |
| **Reverse proxy** | Nginx (or Caddy) | TLS termination, static file serving, route proxying |
| **Orchestration** | Docker Compose | Single `docker compose up -d`; health-check dependency chain |
| **Monitoring (optional v1)** | Grafana + Prometheus | Pre-built Go and PostgreSQL dashboards for single-operator visibility |

### Startup dependency order (Docker Compose)
```
PostgreSQL → Keycloak → Go API → Notification Worker
MinIO  ──────────────────────────────────────────────► (always up)
Redis  ──────────────────────────────────────────────► (always up)
```

---

## 8. Data Models (Summary)

| Entity | Key Fields |
|---|---|
| **Guard** | id, keycloak_user_id, name, employee_id, site_id, role, pin_hash, status |
| **Site** | id, name, client_id, keycloak_realm, address, timezone, config_json |
| **Shift** | id, guard_id, site_id, start_time, end_time, repeat_rule |
| **AttendanceRecord** | id, shift_id, guard_id, clock_in_at, clock_out_at, status, selfie_object_key, gps |
| **Route** | id, site_id, name, checkpoints (ordered JSONB with per-checkpoint sla_minutes nullable) |
| **CheckpointScan** | id, route_id, checkpoint_id, guard_id, scanned_at, method (QR/manual), photo_object_key, gps |
| **Incident** | id, site_id, guard_id, type, severity, description, photo_object_keys[], location, status |
| **DefectReport** | id, site_id, guard_id, defect_type, location, description, photo_object_key, status |
| **VisitorLog** | id, site_id, guard_id, visitor_name, id_type, id_number, plate, host, entry_at, exit_at, photo_object_key |
| **SosAlert** | id, guard_id, site_id, fired_at, gps, acknowledged_by, acknowledged_at |

> `*_object_key` fields reference MinIO object paths; the API generates pre-signed URLs at read time.  
> Checkpoint SLA is stored as a nullable `sla_minutes` per checkpoint in the route JSONB — null means no SLA enforced for that checkpoint.

---

## 9. Decisions Log

| # | Question | Decision |
|---|---|---|
| OQ-1 | Site and guard capacity? | **v1: 2 sites, 10 guards.** Architecture scales to 50 sites / 500 guards. |
| OQ-2 | Notification channels? | **SMTP + WhatsApp Business API only.** LINE and Telegram dropped (D-050). API keys provided by operator. |
| OQ-3 | Payroll system integration? | **Descoped for v1.** HR uses the monthly recap report (FR-REP-05) as payroll reference. CSV export added if needed in v2. |
| OQ-4 | SLA default for checkpoints? | **No system-wide default.** SLA is optional and set per-checkpoint in the route editor. Routes with no SLA produce no alerts. |
| OQ-5 | Client site owner access model? | **Both:** read-only Keycloak login to web dashboard + automated monthly recap email. |
| OQ-6 | Keycloak realm strategy? | **One realm per client.** Full user namespace isolation between clients. |
| OQ-7 | UI languages? | **v1: English + Bahasa Indonesia** on both web and tablet. |
| OQ-8 | Recurring shifts? | **Shift template + pre-generated instances.** Admin manages a recurring template (guard, site, start/end time, weekdays); the system pre-generates concrete shift rows ahead of time. Attendance, absent auto-flag, and reports run against the generated instances. |
| OQ-9 | Tablet authentication model? | **Shared tablet per site, device token + local PIN.** Each tablet is a Keycloak *device* account bound to one site; it registers itself and stays pending until an admin approves it and assigns the site (admin can revoke at any time). Guards unlock the tablet with their PIN, verified locally against a PIN hash synced from the server, so switching guards works offline. Every API call carries the device token plus `X-Guard-ID`; the API rejects guards who are inactive or not assigned to the device's site. Guard identity is never taken from the request body. |

---

## 10. Out of Scope (v1)

- Biometric authentication (fingerprint / face ID)
- Live video streaming from patrol
- Payroll system integration (monthly recap report is the handoff)
- LINE / Telegram notifications (not planned — e-mail + WhatsApp only, D-050)
- Additional UI languages beyond English + Bahasa Indonesia (i18n hooks in place)
- Mobile push notifications via FCM/APNs
- Kubernetes / multi-node HA deployment (Docker Compose single-node)

---

## 11. Milestones (Suggested)

| Phase | Scope | Target |
|---|---|---|
| **M1 — Infrastructure & Auth** | Docker Compose: PostgreSQL, Redis, MinIO, Keycloak (2 realms), Nginx; Go API skeleton; RBAC roles configured | Week 2 |
| **M2 — Core Attendance** | Shift schedule, clock-in/out, selfie → MinIO, daily attendance report, absent auto-flag | Week 6 |
| **M3 — Patrol** | Route editor with dynamic SLA, QR scan, manual check-in + photo, SLA alerts, route completion report | Week 10 |
| **M4 — Incident & Field Reports** | All 4 incident types, offline sync, SOS with Redis queue + WhatsApp/SMTP alert | Week 14 |
| **M5 — Web Dashboard & Reports** | Full admin web, all 5 report types incl. monthly recap, client read-only view | Week 18 |
| **M6 — Notifications & Email** | SMTP + WhatsApp worker, monthly recap auto-email, per-site channel config | Week 20 |
| **M7 — Hardening & Go-Live** | Security audit, Keycloak hardening, load test (2 sites / 10 guards), backup/restore runbook, install docs | Week 22 |