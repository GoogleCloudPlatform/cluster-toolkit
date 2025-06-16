# Cluster Toolkit Frontend Changelog

## [Unreleased] - OAuth/IAP Integration

### Added
- **OAuth/IAP Integration**: Automatic OAuth authentication setup when DNS hostname is provided
  - New `oauth_client.sh` script for managing IAP brands and clients
  - Conditional IAP brand and client creation in Terraform
  - OAuth client credentials automatically injected into Django configuration
  - Safety mechanism to prevent conflicts with existing IAP brands

### New Configuration Variables
- `oauth_attach_existing`: Required flag when IAP brand already exists in project
- `oauth_project_id`: Project ID for OAuth/IAP resources (enables cross-project OAuth)
- `oauth_support_email`: Support email for OAuth brand (defaults to django_su_email)
- `oauth_application_title`: Application title for OAuth brand (defaults to "deployment_name - hostname")
- `oauth_client_display_name`: Display name for OAuth client (defaults to "deployment_name OAuth Client")

### Enhanced Features
- **Automatic OAuth Detection**: When `dns_hostname` is provided, OAuth is automatically configured
- **Interactive OAuth Configuration**: Prompts users when OAuth conflicts are detected, allowing real-time resolution
- **Cross-Project OAuth Support**: Use OAuth from a different project for shared services scenarios
- **Config File Support**: All OAuth variables work with both interactive and config file deployments
- **Safety Checks**: Prevents accidental conflicts with existing OAuth deployments
- **Terraform Integration**: OAuth resources managed alongside other infrastructure
- **IAP Brand Type Detection**: Automatically detects Internal vs External brand types and provides guidance
- **Internal Brand Creation**: New IAP brands are automatically created as "Internal" type to support OAuth clients
- **OAuth Customization**: Interactive prompts for customizing OAuth application details

### Technical Changes
- Added `data.google_client_openid_userinfo` data source for IAP brand creation
- Updated `server_config_file` template to conditionally include real OAuth credentials
- Enhanced `deploy.sh` with OAuth checking logic for both interactive and config modes
- Added OAuth outputs for client ID and secret access
- Implemented cross-project OAuth support with project validation
- Enhanced `oauth_client.sh` with cross-project guidance and validation
- Fixed terraform.tfvars generation to handle pre-quoted YAML values correctly
- Fixed IAP brand creation logic to properly handle existing brands when `oauth_attach_existing` is true
- Added IAP brand application type detection to `oauth_client.sh` with specific guidance for External brands
- Updated Terraform IAP brand resource to automatically create Internal brands for OAuth client support
- Enhanced interactive deployment with real-time OAuth conflict resolution and customization prompts
- Added OAuth configuration details to deployment summaries for both interactive and config file modes

### Files Modified
- `script/oauth_client.sh` - New OAuth management script
- `tf/variables.tf` - Added OAuth configuration variables
- `tf/main.tf` - Added conditional IAP resources and OAuth logic
- `tf/outputs.tf` - Added OAuth credential outputs
- `deploy.sh` - Enhanced with OAuth integration and checking

### Usage
When deploying with a DNS hostname:
```yaml
deployment_name: MyApp
dns_hostname: myapp.example.com
# OAuth is automatically enabled

# If IAP brand already exists:
oauth_attach_existing: true

# For cross-project OAuth (shared services):
oauth_project_id: shared-oauth-project-123
oauth_attach_existing: true

# Optional customization:
oauth_support_email: admin@example.com
oauth_application_title: "My Custom App"
oauth_client_display_name: "My OAuth Client"
```

### Cross-Project OAuth Use Cases
- **Shared Services**: Use a dedicated OAuth project for multiple deployments
- **Enterprise Security**: Centralized OAuth management by security teams  
- **Cost Optimization**: One IAP brand serving multiple projects
- **Multi-Environment**: Development, staging, and production using shared OAuth

### Breaking Changes
None - all OAuth functionality is optional and only activated when hostname is provided.

### Security Improvements
- OAuth credentials are marked as sensitive in Terraform outputs
- Safety checks prevent accidental IAP brand conflicts
- Clear error messages guide users through OAuth setup conflicts 