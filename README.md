# Sokomoko E-Commerce Platform

A lightweight, accessible e-commerce website built with **Go, HTML, CSS, JavaScript, and SQLite3** — no frameworks, just bare languages and best practices.

This project is a learning adventure exploring modern web development techniques with a focus on:
- Clean, maintainable Go code
- Vanilla CSS with mobile-first responsive design
- Accessibility-first UI (WCAG AA/AAA)
- Simple, functional database design
- No external dependencies for core functionality

---

## 📋 Project Structure

Following Go best practices, the project is organized as:

```
sokomoko/
├── cmd/
│   └── sokomoko/              Main application entry point
├── internal/
│   ├── db/                    Database models and operations
│   ├── routes/                HTTP request handlers
│   ├── auth/                  Authentication and authorization
│   └── ui/
│       ├── static/            CSS, JavaScript, images
│       └── templates/         HTML templates
├── db/
│   ├── schema.sql             Database schema
│   └── t.db                   SQLite database (auto-created)
├── bin/                       Compiled binaries
├── go.mod & go.sum            Dependency management
└── README.md                  This file
```

---

## 🛠️ Requirements

- **Go** (version 1.21 or higher)
- **SQLite3** (included with Go; no separate installation needed)
- **Git** (for cloning the repository)
- **Terminal/Command line** (bash, zsh, PowerShell, etc.)

---

## 📥 Setup Instructions

### Step 1: Install Go

**Mac (using Homebrew):**
```bash
brew install go
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt-get update
sudo apt-get install golang-go
```

**Windows:**
Download from [golang.org](https://golang.org/dl) and follow the installer.

**Verify installation:**
```bash
go version
```

### Step 2: Clone the Repository

```bash
git clone https://github.com/kyambuthia/sokomoko.git
cd sokomoko
```

### Step 3: Install Dependencies

The project uses Go modules for dependency management. Install dependencies with:

```bash
go mod tidy
```

This will:
- Download all required dependencies listed in `go.mod`
- Remove any unused dependencies
- Update `go.sum` with checksums

### Step 4: (Optional) Configure Local DNS for Subdomains

If you want to use subdomains locally (e.g., `admin.localhost`), add these entries to your hosts file:

**Mac/Linux:**
```bash
sudo nano /etc/hosts
```

**Add these lines:**
```
127.0.0.1 localhost
127.0.0.1 sokomoko.localhost
127.0.0.1 admin.localhost
127.0.0.1 admin.sokomoko.localhost
```

Save and exit (Ctrl+X, then Y, then Enter on nano).

**Windows (as Administrator):**
1. Open `C:\Windows\System32\drivers\etc\hosts` in Notepad (run as Administrator)
2. Add the same lines above
3. Save the file

> **Note:** This is optional. You can also access the app via `localhost:6969` without subdomain configuration.

### Step 5: Run the Application

**Development mode (with auto-reload recommended):**
```bash
go run ./cmd/sokomoko
```

Or if that doesn't work, try:
```bash
go run ./cmd/sokomoko/*.go
```

**Build for production:**
```bash
go build -o bin/sokomoko ./cmd/sokomoko
./bin/sokomoko
```

### Step 6: Access the Application

Once running, the application will start on:

```
http://localhost:6969
```

**What happens on first run:**
- ✅ Database (`db/t.db`) is automatically created
- ✅ Schema is automatically migrated
- ✅ Application starts and listens on port 6969

---

## 🏪 Using the Application

### Customer Site
Access the main storefront at:
```
http://localhost:6969
```

**Available pages:**
- **Home** – Browse featured products
- **Search** – Find products by keyword or category
- **Product Details** – View full product information
- **Cart** – Review selected items
- **Checkout** – Complete purchase
- **User Account** – Manage account and order history
- **Login/Signup** – Create account or sign in

### Partner Dashboard (Sellers)
Create a seller account to manage your product listings:

```
http://localhost:6969/partner/signup
```

**Partner features:**
- Add and manage product listings
- Set pricing and inventory
- View orders from your products
- Track sales and revenue

---

## 👨‍💼 Admin Dashboard

### Accessing the Admin Site

**Admin Login:**
```
http://localhost:6969/admin/login
```

Or directly navigate to:
```
http://localhost:6969/admin
```

You'll be redirected to login if not authenticated.

### Default Admin Credentials

> ⚠️ **IMPORTANT:** Change these credentials immediately in production!

Default credentials (development only):
```
Username: admin
Password: admin123
```

### Admin Features

Once logged in, you have access to:

1. **Dashboard**
   - Overview of key metrics
   - Total products
   - Orders today
   - Revenue today
   - Quick access to management tools

2. **Product Management**
   - View all products
   - Add new products
   - Edit existing products
   - Manage categories
   - Track inventory
   - Import/export products

3. **Order Management**
   - View all customer orders
   - Track order status
   - Process refunds
   - Export order data

4. **User Management**
   - View registered users
   - Manage user accounts
   - View user details and order history

5. **Reports & Analytics**
   - Sales reports
   - Revenue tracking
   - Product performance
   - Customer insights

6. **Settings**
   - System configuration
   - Payment methods
   - Shipping options
   - Site settings

### Admin Navigation

The admin menu is sticky (always visible) and includes:
- **Dashboard** – Overview and statistics
- **Products** – Manage product catalog
- **Orders** – Process customer orders
- **Reports** – View analytics and reports
- **Deliveries** – Track shipping
- **Back to Store** – Return to customer site

---

## 🎨 Design System

The application uses a **mobile-first, retro UI design** (1990s-2000s aesthetic) with:

- **No decorative effects** (gradients, shadows, animations)
- **Flat, solid colors** only
- **System fonts** (Arial, Helvetica, Courier)
- **Accessible by default** (WCAG AA/AAA)
- **Responsive design** (mobile, tablet, desktop)

### View Design Components

See all UI components and design system documentation:

```
http://localhost:6969/design
```

For detailed design documentation, see:
- `DESIGN_SYSTEM.md` – Complete design specifications
- `DESIGN_QUICK_REFERENCE.md` – Quick class reference
- `TEMPLATE_BEST_PRACTICES.md` – HTML best practices
- `CSS_ARCHITECTURE.md` – CSS organization

---

## 💻 Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./internal/db
go test ./internal/auth

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...
```

### Code Style

The project follows standard Go conventions:
- Use `go fmt` to format code
- Use `go lint` to check for issues
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Project Documentation

Additional documentation files:
- `AGENTS.md` – Development guide for AI agents
- `DESIGN_IMPLEMENTATION_SUMMARY.md` – Design system overview
- `GETTING_STARTED.md` – Quick start guide
- `INDEX.md` – Documentation index

---

## 🚀 Building for Production

### Build the Binary

```bash
go build -o bin/sokomoko ./cmd/sokomoko
```

### Set Environment Variables

```bash
export PORT=80                    # Default: 6969
export DB_PATH=/path/to/t.db     # Default: ./db/t.db
export ENV=production             # Set to 'production'
```

### Run in Production

```bash
./bin/sokomoko
```

### Deployment Checklist

- [ ] Change admin credentials
- [ ] Configure database backup
- [ ] Set up SSL/TLS certificates
- [ ] Configure environment variables
- [ ] Review security settings
- [ ] Test all features
- [ ] Set up monitoring/logging
- [ ] Plan database migration strategy

---

## 🐛 Troubleshooting

### Port Already in Use

If port 6969 is already in use:

```bash
# Kill the process on port 6969 (Mac/Linux)
lsof -ti:6969 | xargs kill -9

# Or change the port (if supported)
PORT=8080 go run ./cmd/sokomoko
```

### Database Locked

If you get "database is locked" error:
1. Stop the application
2. Delete `db/t.db`
3. Restart the application (it will recreate the database)

### Dependencies Not Found

```bash
go mod tidy
go mod download
```

### Can't Connect to Localhost

- Ensure application is running (check terminal output)
- Verify port 6969 is open
- Check firewall settings
- Try `http://127.0.0.1:6969` instead of `localhost`

### Admin Login Not Working

1. Verify you're using correct default credentials (`admin` / `admin123`)
2. Check that application restarted after setup
3. Clear browser cookies/cache and try again
4. Check terminal output for error messages

---

## 📞 Support & Issues

Found a bug or have a feature request?

1. **Check existing issues** on GitHub
2. **Create a new issue** with:
   - Description of the problem
   - Steps to reproduce
   - Expected vs actual behavior
   - Your environment (OS, Go version, etc.)

---

## 📝 License

[Add your license information here]

---

## ✨ Credits

Built with care using:
- **Go** – Backend
- **HTML/CSS/JavaScript** – Frontend
- **SQLite3** – Database
- **No frameworks** – Just good practices

---

## 🎯 Quick Reference

| Task | Command |
|------|---------|
| **Start application** | `go run ./cmd/sokomoko` |
| **Build binary** | `go build -o bin/sokomoko ./cmd/sokomoko` |
| **Run tests** | `go test ./...` |
| **Tidy dependencies** | `go mod tidy` |
| **Format code** | `go fmt ./...` |
| **Access storefront** | `http://localhost:6969` |
| **Access admin** | `http://localhost:6969/admin/login` |
| **View design system** | `http://localhost:6969/design` |

---

**Happy coding! 🚀**
