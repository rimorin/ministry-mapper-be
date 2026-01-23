# 🗺️ Ministry Mapper Backend

> Self-hosted territory management system built on PocketBase.

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![PocketBase](https://img.shields.io/badge/PocketBase-0.35.0-B8DBE4)](https://pocketbase.io/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## ✨ Features

- 🔐 **Authentication** - User auth & role-based access control
- 🌍 **Territory Management** - Organize maps, addresses, and territories
- 📍 **Smart Assignment** - Intelligent map-to-user proximity matching
- 📊 **Real-time Updates** - Server-Sent Events (SSE) for live data sync
- 📈 **Aggregation Engine** - Automated territory progress tracking
- ⏰ **Scheduled Jobs** - Background tasks for reports & data processing
- 📧 **Email Reports** - Monthly Excel reports via MailerSend
- 🔍 **Error Tracking** - Sentry integration for monitoring
- 🎛️ **Feature Flags** - LaunchDarkly for controlled rollouts

---

## 🚀 Quick Start

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- Git

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

The server starts at **http://localhost:8090**  
Admin UI: **http://localhost:8090/\_/**

---

## 🐳 Docker Deployment

### Build Image

```bash
docker build -t ministry-mapper .
```

### Run Container

```bash
docker run -d \
  --name ministry-mapper \
  -p 8080:8080 \
  -v /path/to/pb_data:/app/pb_data \
  --env-file .env \
  ministry-mapper
```

### Important: Persistent Storage

**Always map `/app/pb_data`** to a persistent volume to preserve:

- SQLite database
- User uploads
- Configuration files

---

## ⚙️ Configuration

### Environment Variables

Key variables (see `.env.sample` for complete list):

| Variable               | Description                                  | Required |
| ---------------------- | -------------------------------------------- | -------- |
| `PB_APP_URL`           | Frontend application URL                     | ✅       |
| `PB_ALLOW_ORIGINS`     | CORS origins (comma-separated)               | ✅       |
| `MAILERSEND_API_KEY`   | Email service API key                        | ✅       |
| `LAUNCHDARKLY_SDK_KEY` | Feature flags SDK key                        | ✅       |
| `SENTRY_DSN`           | Error tracking DSN                           | ✅       |
| `SENTRY_ENV`           | Environment (development/staging/production) | ✅       |

### Default Ports

- **Development**: 8090
- **Docker**: 8080 (configurable)

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
│   ├── jobs/          # Background job schedulers
│   └── middleware/    # Request middleware
├── migrations/        # Database migrations
├── templates/         # Email templates
├── scripts/           # Development scripts
└── pb_data/          # PocketBase data (gitignored)
```

---

## 📡 API Integration

Use the [PocketBase JavaScript SDK](https://github.com/pocketbase/js-sdk) to interact with the backend.

### Example: Authentication

```javascript
import PocketBase from "pocketbase";

const pb = new PocketBase("http://localhost:8090");
await pb.collection("users").authWithPassword("user@example.com", "password");
```

### Custom Endpoints

All custom routes require authentication:

- `POST /map/codes` - Get address codes
- `POST /map/code/add` - Add new address
- `POST /territory/link` - Smart map assignment
- `POST /options/update` - Update congregation options

---

## 🔒 Security Best Practices

- ✅ Always use **HTTPS** in production
- ✅ Never commit secrets to version control
- ✅ Use environment variables for sensitive data
- ✅ Keep dependencies updated with `./scripts/update.sh`

---

## 📚 Documentation

- **[Official Documentation](https://doc.ministry-mapper.com)** - Complete user and developer guides
- **[Frontend Repository](https://github.com/rimorin/ministry-mapper-v2)** - Ministry Mapper v2 web application
- [PocketBase Documentation](https://pocketbase.io/docs/)
- [Go API Reference](https://pkg.go.dev/github.com/pocketbase/pocketbase)

---

## 📄 License

[MIT License](LICENSE)

---

## 🙏 Acknowledgments

Built with [PocketBase](https://pocketbase.io/) - an open-source backend-as-a-service platform featuring:

- Built-in admin dashboard
- Real-time subscriptions
- SQLite database
- File storage
- User authentication
