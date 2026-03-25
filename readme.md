# 🗺️ Ministry Mapper Backend

> Self-hosted territory management system built on PocketBase.

<p align="center">
  <a href="https://go.dev/"><img alt="Go Version" src="https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white"></a>
  <a href="https://pocketbase.io/"><img alt="PocketBase" src="https://img.shields.io/badge/PocketBase-0.36.7-B8DBE4?style=for-the-badge&logo=data:image/svg%2bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZmlsbD0iI2ZmZiIgZD0iTTEyIDJDNi40OCAyIDIgNi40OCAyIDEyczQuNDggMTAgMTAgMTAgMTAtNC40OCAxMC0xMFMxNy41MiAyIDEyIDJ6bTAgMThjLTQuNDEgMC04LTMuNTktOC04czMuNTktOCA4LTggOCAzLjU5IDggOC0zLjU5IDgtOCA4eiIvPjwvc3ZnPg=="></a>
  <a href="https://www.sqlite.org/"><img alt="SQLite" src="https://img.shields.io/badge/SQLite-embedded-003B57?style=for-the-badge&logo=sqlite&logoColor=white"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-22c55e?style=for-the-badge"></a>
</p>

---

## 📋 Table of Contents

- [✨ Features](#-features)
- [🏗️ Architecture](#️-architecture)
- [🚀 Quick Start](#-quick-start)
- [🐳 Docker Deployment](#-docker-deployment)
- [⚙️ Configuration](#️-configuration)
- [⏰ Scheduled Jobs](#-scheduled-jobs)
- [🛠️ Development](#️-development)
- [📡 API Integration](#-api-integration)
- [🔒 Security](#-security)
- [📚 Documentation](#-documentation)

---

## ✨ Features

<table>
  <tr>
    <td>🔐 <b>Authentication</b></td>
    <td>User auth & role-based access control</td>
    <td>🌍 <b>Territory Management</b></td>
    <td>Organize maps, addresses & coordinates</td>
  </tr>
  <tr>
    <td>📍 <b>Smart Assignment</b></td>
    <td>Proximity-based map-to-user matching</td>
    <td>📊 <b>Real-time Updates</b></td>
    <td>Server-Sent Events (SSE) for live sync</td>
  </tr>
  <tr>
    <td>📈 <b>Aggregation Engine</b></td>
    <td>Automated territory progress tracking</td>
    <td>⏰ <b>Scheduled Jobs</b></td>
    <td>Background tasks for reports & processing</td>
  </tr>
  <tr>
    <td>📧 <b>Email Reports</b></td>
    <td>Monthly Excel reports via MailerSend</td>
    <td>🤖 <b>AI Summaries</b></td>
    <td>LLM-generated content summaries</td>
  </tr>
  <tr>
    <td>👤 <b>User Lifecycle</b></td>
    <td>Inactivity warnings & auto-deprovisioning</td>
    <td>📋 <b>Analytics</b></td>
    <td>Address logs, audit views & data exports</td>
  </tr>
  <tr>
    <td>🔍 <b>Error Tracking</b></td>
    <td>Sentry integration for monitoring</td>
    <td>🎛️ <b>Feature Flags</b></td>
    <td>LaunchDarkly for controlled rollouts</td>
  </tr>
</table>

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## 🏗️ Architecture

```mermaid
graph TD
    Client["🖥️ Frontend Client\n(React 19 PWA)"]
    SDK["PocketBase JS SDK"]
    API["🗄️ PocketBase API\n:8090"]
    Custom["⚙️ Custom Handlers\n/map/* /territory/*"]
    Auth["🔐 Auth Middleware\nRequireAuth()"]
    DB[("💾 SQLite\npb_data/")]
    Jobs["⏰ Job Scheduler\n(LaunchDarkly-gated)"]
    Sentry["🔍 Sentry\nError Tracking"]
    LD["🎛️ LaunchDarkly\nFeature Flags"]
    Email["📧 MailerSend\nEmail Reports"]
    AI["🤖 OpenAI\nAI Summaries"]

    Client --> SDK --> API
    API --> Auth --> Custom
    API --> DB
    Custom --> DB
    Jobs --> DB
    Jobs --> Email
    Jobs --> AI
    API --> Sentry
    Jobs --> LD
    Custom --> Sentry
```

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## 🚀 Quick Start

### Prerequisites

| Tool | Version |
|------|---------|
| [Go](https://go.dev/dl/) | 1.25+ |
| Git | any |

### Installation

```bash
# Clone repository
git clone git@github.com:rimorin/ministry-mapper-be.git
cd ministry-mapper-be

# Install dependencies
./scripts/install.sh

# Configure environment
cp .env.sample .env
# Edit .env with your settings

# Start development server
./scripts/start.sh
```

> [!NOTE]
> The server starts at **http://localhost:8090**
> Admin UI is available at **http://localhost:8090/\_/**

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## 🐳 Docker Deployment

```bash
# Build image
docker build -t ministry-mapper .

# Run container
docker run -d \
  --name ministry-mapper \
  -p 8080:8080 \
  -v /path/to/pb_data:/app/pb_data \
  --env-file .env \
  ministry-mapper
```

> [!IMPORTANT]
> Always map `/app/pb_data` to a **persistent volume** to preserve:
> - SQLite database
> - User uploads
> - Configuration files

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## ⚙️ Configuration

### Environment Variables

Key variables (see `.env.sample` for the complete list):

| Variable | Description | Required |
|----------|-------------|:--------:|
| `PB_APP_URL` | Frontend application URL | ✅ |
| `PB_ALLOW_ORIGINS` | CORS origins (comma-separated) | ✅ |
| `MAILERSEND_API_KEY` | Email service API key | ✅ |
| `LAUNCHDARKLY_SDK_KEY` | Feature flags SDK key | ✅ |
| `LAUNCHDARKLY_CONTEXT_KEY` | LaunchDarkly environment context key | ✅ |
| `SENTRY_DSN` | Error tracking DSN | ✅ |
| `SENTRY_ENV` | Environment (`development`/`staging`/`production`) | ✅ |
| `OPENAI_API_KEY` | OpenAI API key for AI-generated summaries | ⚠️ AI only |

### Default Ports

| Environment | Port |
|-------------|------|
| Development | `8090` |
| Docker | `8080` _(configurable)_ |

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## ⏰ Scheduled Jobs

All jobs are gated by **LaunchDarkly feature flags** and can be toggled without redeployment.

| Job | Schedule | Flag | Description |
|-----|----------|------|-------------|
| `cleanUpAssignments` | Every 5 min | `enable-assignments-cleanup` | Remove expired map assignments |
| `updateTerritoryAggregates` | Every 10 min | `enable-territory-aggregations` | Recalculate territory progress stats |
| `processMessages` | Every 30 min | `enable-message-processing` | Process pending message queue |
| `processInstructions` | Every 30 min | `enable-instruction-processing` | Process territory assignment instructions |
| `processNotes` | Every hour | `enable-note-processing` | Process updated congregation notes |
| `generateMonthlyReport` | 1st of month | `enable-monthly-report` | Generate & email Excel report to admins |
| `processUnprovisionedUsers` | Daily 01:00 UTC | `enable-unprovisioned-user-processing` | Warn/disable users with no role |
| `processInactiveUsers` | Daily 01:30 UTC | `enable-inactive-user-processing` | Warn/disable inactive accounts |

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## 🛠️ Development

### Update Dependencies

```bash
./scripts/update.sh
```

### Project Structure

```
ministry-mapper-be/
├── internal/
│   ├── handlers/      # API endpoint handlers
│   ├── jobs/          # Background job schedulers & LLM client
│   └── middleware/    # Request middleware
├── migrations/        # Database migrations
├── templates/         # Email templates (reports, user lifecycle)
├── scripts/           # Development scripts
└── pb_data/           # PocketBase data (gitignored)
```

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## 📡 API Integration

Use the [PocketBase JavaScript SDK](https://github.com/pocketbase/js-sdk) to interact with the backend.

### Authentication Example

```javascript
import PocketBase from "pocketbase";

const pb = new PocketBase("http://localhost:8090");
await pb.collection("users").authWithPassword("user@example.com", "password");
```

### Custom Endpoints

> [!NOTE]
> All custom routes require a valid auth token (`Authorization: Bearer <token>`).

| Endpoint | Description |
|----------|-------------|
| `POST /map/codes` | Get address codes for a map |
| `POST /map/code/add` | Add one or more address codes |
| `POST /map/code/delete` | Delete an address code |
| `POST /map/codes/update` | Reorder address codes |
| `POST /map/floor/add` | Add a floor to a multi-level map |
| `POST /map/floor/remove` | Remove a floor from a map |
| `POST /map/reset` | Reset all addresses in a map |
| `POST /map/add` | Create a new map |
| `POST /map/territory/update` | Move a map to another territory |
| `POST /territory/reset` | Reset all maps in a territory |
| `POST /territory/link` | Smart map assignment (Quicklink) |
| `POST /options/update` | Update congregation address options |
| `POST /report/generate` | Trigger on-demand report generation |

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>

---

## 🔒 Security

> [!WARNING]
> Never commit `.env` files or API keys to version control.

- ✅ Always use **HTTPS** in production
- ✅ Store all secrets in environment variables
- ✅ Keep dependencies updated with `./scripts/update.sh`

---

## 📚 Documentation

| Resource | Description |
|----------|-------------|
| **[Official Docs](https://doc.ministry-mapper.com)** | Complete user and developer guides |
| **[Frontend Repo](https://github.com/rimorin/ministry-mapper-v2)** | Ministry Mapper v2 — React 19 + TypeScript PWA |
| [PocketBase Docs](https://pocketbase.io/docs/) | PocketBase platform documentation |
| [Go API Reference](https://pkg.go.dev/github.com/pocketbase/pocketbase) | Go package reference |

---

## 📄 License

[MIT License](LICENSE)

---

## 🙏 Acknowledgments

Built with [PocketBase](https://pocketbase.io/) — an open-source BaaS platform with built-in admin dashboard, real-time subscriptions, SQLite, file storage, and user authentication.

<p align="right"><a href="#ministry-mapper-backend">↑ back to top</a></p>
