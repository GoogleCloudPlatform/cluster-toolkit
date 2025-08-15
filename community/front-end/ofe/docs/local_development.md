# Local Development Guide

This guide explains how to set up a local development environment for the Cluster Toolkit Front End (OFE).

## Quick Start

```bash
# For a fresh setup (removes existing database and environment):
./deploy.sh --local --clean

# Deploy to a specific path:
./deploy.sh --workdir /tmp/ofe-dev-env/

# To continue with existing setup:
./deploy.sh --local
```

The `--local` command will:

1. Copy the project to a 'local-dev-env' subdirectory
2. Create a Python virtual environment (if not exists)
3. Install all required dependencies
4. Set up a SQLite database
5. Create a default admin user (admin/admin)
6. Start the Django development server

The optional `--clean` flag will:

1. Remove existing database file
2. Remove virtual environment
3. Clear Python cache files and dependencies
4. Ensure a completely fresh setup

The optional `--workdir` flag will:

1. Be used as the runtime path
2. Check for existing test OFE instances
3. Deploy to the selected path for testing

## Prerequisites

- Python 3.x
- Git
- virtualenv or venv module

## Development Environment Details

### Configuration

The local development environment uses a simplified configuration:

```yaml
config:
  server:
    gcp_project: "local-dev-project"
    gcs_bucket: "local-dev-bucket"
    c2_topic: "local-dev-topic"
    deployment_name: "local-dev"
    runtime_mode: "local"
    runtime_path: "/choose/a/path"
```

### Default Credentials

- Username: admin
- Password: admin
- URL: http://localhost:8000

### Custom Local Credentials

You can override default admin credentials by setting them via config file
or by environment variables:

```bash
export LOCAL_DJANGO_USERNAME="myuser"
export LOCAL_DJANGO_PASSWORD="mypassword"
export LOCAL_DJANGO_EMAIL="myemail@example.com"
./deploy.sh --local
```

### Database

- Uses SQLite for development
- Located at `website/db.sqlite3`
- Automatically migrated during setup
- Use `--clean` flag to remove existing database and start fresh

### Static Files

- Collected in `website/static/`
- Served directly by Django development server

## Development Workflow

1. Make code changes in the `$workdir/website/` directory
2. Django's development server will automatically reload when you save changes
3. Database migrations:

   ```bash
   python manage.py makemigrations
   python manage.py migrate
   ```

## Testing

For local testing:

1. Activate the virtual environment:

   ```bash
   source venv/bin/activate
   ```

2. Run tests:

   ```bash
   cd website
   python manage.py test
   ```

## Troubleshooting

1. If you want to start fresh:

   ```bash
   ./deploy.sh --local --clean
   ```

2. If the virtual environment is missing:

   ```bash
   python -m venv venv
   source venv/bin/activate
   pip install -r requirements.txt
   ```

3. If database is corrupted or you want to reset it:

   ```bash
   # Option 1: Use the clean flag (recommended)
   ./deploy.sh --local --clean

   # Option 2: Manual cleanup
   rm db.sqlite3
   python manage.py migrate
   python manage.py createsuperuser
   ```

4. If static files are missing:

   ```bash
   python manage.py collectstatic
   ```

## Notes

- The local development environment does not require GCP resources
- Valid credential can be provided but mock data is used as a fallback
- Use this environment for testing and development only
- Can be used to generate and validate blueprints
- For production deployment, refer to the Administrator's Guide
- Recommend using config file and setting `--workdir` for dev environment
