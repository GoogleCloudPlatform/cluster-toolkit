# AI-Powered Pre-commit Fixer Workflow

This guide details how to use the `gcluster ai fix-pre-commits` command to automatically resolve pre-commit failures in the Cluster Toolkit.

## Prerequisites

Before running the tool, ensure you have the following installed and configured:

1. **`pre-commit`**: The core framework for managing and maintaining multi-language pre-commit hooks.

   ```bash
   pip install pre-commit
   # Verify installation
   pre-commit --version
   ```

2. **`gcloud` CLI**: Required for authenticating with Vertex AI.

   ```bash
   # Install Google Cloud SDK if not present
   # Authenticate with your Google Cloud account
   gcloud auth login
   gcloud auth application-default login
   ```

3. **Vertex AI Access**: Ensure the project currently configured in `gcloud` has the Vertex AI API enabled.

   ```bash
   # Check current project
   gcloud config get-value project
   ```

## Installation

If you are running from source, build the `gcluster` binary:

```bash
cd cluster-toolkit
go build -o gcluster .
```

## Usage

### 1. Fix All Failures
To run pre-commit hooks on all files and automatically fix any failures:

```bash
./gcluster ai fix-pre-commits
```

**What happens:**
1. The tool runs `pre-commit run --all-files`.
2. If failures are detected, it identifies the failing files and error messages.
3. It sends the file content and error to Vertex AI (Gemini) to generate a fix.
4. It applies the fix and re-runs the hooks to verify.
5. It repeats this process (up to `--max-retries`) until all hooks pass.

### 2. Fix Specific Files
To limit the scope to specific files (useful for faster iteration):

```bash
./gcluster ai fix-pre-commits pkg/shell/terraform.go modules/vpc/main.tf
```

### 3. Customize Behavior
You can adjust the retry limit using the `--max-retries` flag (default is 3):

```bash
./gcluster ai fix-pre-commits --max-retries 5 --verbose
```

**Options:**
- `-v` or `--verbose`: Enable verbose logging to see detailed errors and AI debug info.
- `--max-retries`: Set the maximum number of retry attempts (default: 3).
- `--model`: Specify the Vertex AI model to use (default: `gemini-2.0-flash-001`).
- `--region`: Specify the Vertex AI region (default: `us-central1`).

### 4. Specifying a Different Model

If the default model (`gemini-2.0-flash-001`) is not available in your project/region, you can specify a different one:

```bash
./gcluster ai fix-pre-commits --model gemini-1.0-pro-001 --verbose
```

### 5. Troubleshooting

| Issue | Cause | Resolution |
| :--- | :--- | :--- |
| `pre-commit is not installed` | Missing dependency | Run `pip install pre-commit`. |
| `failed to get access token` | Not authenticated | Run `gcloud auth login`. |
| `Vertex AI API returned status: 403` | API disabled or no permission | Enable Vertex AI API in your GCP project or switch to a project with access. |
| `Wait, the fix is wrong!` | AI Hallucination | The tool attempts to fix based on the error, but AI isn't perfect. Always review the `git diff` before committing! |

## Best Practices

- **Review Changes**: Always run `git diff` after the tool completes to ensure the fixes are correct.
- **Commit Frequently**: Use the tool to clear logical units of work.
- **Fallback**: If the tool gets stuck in a loop, you can always fix the issue manually and run `pre-commit run --all-files` to verify.
