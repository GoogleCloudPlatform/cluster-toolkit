# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging
import os
import requests
import subprocess
from github import Github
from google import genai
from google.genai import types
from googleapiclient.discovery import build

class GitHubClient:
    def __init__(self):
        self.pat = os.getenv("GITHUB_PAT")
        self.repo_name = os.getenv("GITHUB_REPO", "rahimkhan19/cluster-toolkit")
        if self.pat and self.pat != "your_github_personal_access_token":
            self.g = Github(self.pat)
            try:
                self.repo = self.g.get_repo(self.repo_name)
            except Exception as e:
                print(f"Warning: Could not fetch GitHub repo '{self.repo_name}'. Make sure it exists and your PAT has access. Error: {e}")
                self.repo = None
        else:
            self.g = None
            self.repo = None

    def create_rc_and_version_branches(self):
        script_path = os.path.join(os.getcwd(), "tools", "create-release-candidate.sh")
        if not os.path.exists(script_path):
            logging.error(f"Error: Script {script_path} not found.")
            return None
            
        logging.info("Running create-release-candidate.sh...")
        import time
        branch_suffix = str(int(time.time()))
        rc_branch = f"release-candidate-{branch_suffix}"
        
        try:
            env = os.environ.copy()
            if self.pat:
                env["GITHUB_TOKEN"] = self.pat
            env["BRANCH_SUFFIX"] = branch_suffix
            
            # Remove VS Code git askpass to avoid ECONNREFUSED errors during git operations
            env.pop("GIT_ASKPASS", None)
            env.pop("VSCODE_GIT_IPC_HANDLE", None)
            
            result = subprocess.run(["bash", script_path], env=env, capture_output=True, text=True)
            if result.returncode != 0:
                logging.error(f"Error running script (code {result.returncode}):\nStdout:\n{result.stdout}\nStderr:\n{result.stderr}")
                
            for line in result.stdout.splitlines() + result.stderr.splitlines():
                if "https://github.com/" in line and "/pull/" in line:
                    parts = line.split("/pull/")
                    if len(parts) > 1:
                        pr_num = parts[1].strip()
                        if pr_num.isdigit():
                            logging.info(f"Successfully created Version PR: {pr_num}")
                            return int(pr_num), rc_branch
                            
            logging.error(f"Could not find PR URL in output.\nStdout:\n{result.stdout}\nStderr:\n{result.stderr}")
            return None
        except Exception as e:
            logging.error(f"Error executing script: {e}")
            return None

    def is_pr_merged(self, pr_number):
        if not self.repo or not pr_number: return False
        try:
            pr = self.repo.get_pull(int(pr_number))
            return pr.merged
        except Exception as e:
            print(f"Error checking if PR {pr_number} is merged: {e}")
            return False

    def is_pr_closed(self, pr_number):
        if not self.repo or not pr_number: return False
        try:
            pr = self.repo.get_pull(int(pr_number))
            return pr.state == "closed" and not pr.merged
        except Exception as e:
            print(f"Error checking if PR {pr_number} is closed: {e}")
            return False

    def reopen_pr(self, pr_number):
        if not self.repo or not pr_number: return False
        try:
            pr = self.repo.get_pull(int(pr_number))
            pr.edit(state="open")
            print(f"Successfully reopened PR {pr_number}!")
            return True
        except Exception as e:
            print(f"Error reopening PR {pr_number}: {e}")
            return False

    def close_pr(self, pr_number):
        if not self.repo or not pr_number: return False
        try:
            pr = self.repo.get_pull(int(pr_number))
            pr.edit(state="closed")
            print(f"Successfully closed PR {pr_number}!")
            return True
        except Exception as e:
            print(f"Error closing PR {pr_number}: {e}")
            return False

    def open_draft_pr(self, head, base, title, body=""):
        if not self.repo: return 123
        try:
            pr = self.repo.create_pull(title=title, body=body, head=head, base=base, draft=True)
            print(f"Created Draft PR: {pr.html_url}")
            return pr.number
        except Exception as e:
            print(f"Error creating Draft PR: {e}")
            return 123

    def get_pr_comments(self, pr_number):
        if not self.repo or not pr_number: return []
        try:
            pr = self.repo.get_pull(int(pr_number))
            # Combine issue comments and review comments, and sort chronologically
            all_comments = list(pr.get_issue_comments()) + list(pr.get_review_comments())
            all_comments.sort(key=lambda c: c.created_at)
            
            formatted_comments = []
            for c in all_comments:
                if hasattr(c, "path"):
                    file_path = getattr(c, "path", "unknown")
                    line = getattr(c, "line", None) or getattr(c, "original_line", "unknown")
                    commit = getattr(c, "commit_id", None) or getattr(c, "original_commit_id", "unknown")
                    formatted = f"Comment: {c.body}\nContext -> File: {file_path}, Line: {line}, Commit: {commit}"
                    formatted_comments.append(formatted)
                else:
                    formatted_comments.append(c.body)
            return formatted_comments
        except Exception as e:
            print(f"Error fetching PR {pr_number} comments: {e}")
            return []

    def convert_pr_to_active(self, pr_number):
        import subprocess
        import os
        try:
            env = os.environ.copy()
            if self.pat:
                env["GH_TOKEN"] = self.pat
            subprocess.run(["gh", "pr", "ready", str(pr_number), "--repo", self.repo_name], env=env, check=True)
            print(f"Converted PR {pr_number} from Draft to Active!")
            if self.repo:
                pr = self.repo.get_pull(int(pr_number))
                pr.enable_automerge(merge_method="merge")
                print(f"Enabled auto-merge for PR {pr_number}")
        except Exception as e:
            print(f"Error converting PR to Active: {e}")
        
    def assign_reviewer(self, pr_number, github_handle):
        if not self.repo or not pr_number: return
        try:
            pr = self.repo.get_pull(int(pr_number))
            pr.create_review_request(reviewers=[github_handle])
            print(f"Assigned {github_handle} to PR {pr_number}")
        except Exception as e:
            print(f"Error assigning reviewer: {e}")

    def is_pr_approved(self, pr_number):
        if not self.repo or not pr_number: return False
        try:
            pr = self.repo.get_pull(int(pr_number))
            reviews = pr.get_reviews()
            for r in reviews:
                if r.state == "APPROVED":
                    return True
            return False
        except Exception as e:
            print(f"Error checking PR approval: {e}")
            return False

    def merge_pr(self, pr_number):
        if not self.repo or not pr_number: return False
        import time
        try:
            pr = self.repo.get_pull(int(pr_number))
            if pr.merged:
                print(f"PR {pr_number} is already merged!")
                return True
            pr.merge(commit_title=f"Merge PR {pr_number}", merge_method="merge")
            print(f"Merged PR {pr_number}")
            
            # Wait for GitHub to process the merge before returning
            for _ in range(10):
                pr.update()
                if pr.merged:
                    print(f"Confirmed PR {pr_number} is merged!")
                    return True
                time.sleep(2)
            return False
        except Exception as e:
            print(f"Error merging PR {pr_number}: {e}")
            return False

    def create_backport_pr(self, pr_number):
        if not self.repo or not pr_number: return None
        import subprocess
        import os
        try:
            # Check if backport PR already exists
            existing_prs = self.repo.get_pulls(state='open', head=f"{self.repo_name.split('/')[0]}:main", base="develop")
            if existing_prs.totalCount > 0:
                pr = existing_prs[0]
                print(f"Found existing Backport PR: {pr.html_url}")
            else:
                # Simple backport by creating a PR from main -> develop
                pr = self.repo.create_pull(
                    title=f"Backport Release to Develop",
                    body=f"Backporting release changes from PR {pr_number} to develop.",
                    head="main",
                    base="develop",
                    draft=False
                )
                print(f"Created Backport PR: {pr.html_url}")
            
            # Ensure it is ready for review
            env = os.environ.copy()
            if self.pat:
                env["GH_TOKEN"] = self.pat
            subprocess.run(["gh", "pr", "ready", str(pr.number), "--repo", self.repo_name], env=env)
            
            # Enable auto-merge on approval without squashing
            try:
                pr.enable_automerge(merge_method="merge")
                print(f"Enabled auto-merge for Backport PR {pr.number}")
            except Exception as e:
                print(f"Warning: Could not enable auto-merge for Backport PR {pr.number}: {e}")
                
            return pr.number
            
        except Exception as e:
            print(f"Error creating backport PR: {e}")
            return None

    def run_inactive_pr_reminder(self):
        import subprocess
        import os
        import json
        try:
            state_file = os.path.join(os.path.dirname(__file__), "state.json")
            pr_number = None
            if os.path.exists(state_file):
                try:
                    with open(state_file, 'r') as f:
                        state = json.load(f)
                        pr_number = state.get("pr_number") or state.get("version_pr_number") or state.get("backport_pr_number")
                except Exception as e:
                    print(f"Could not read state file inside reminder: {e}")
            
            if not pr_number:
                print("No active PR in state.json. Skipping reminder check.")
                return

            script_path = os.path.join(os.path.dirname(__file__), "inactive-pr-reminder.sh")
            if not os.path.exists(script_path):
                print(f"Error: {script_path} not found.")
                return
            env = os.environ.copy()
            if self.pat:
                env["GH_TOKEN"] = self.pat
            env.pop("GIT_ASKPASS", None)
            env.pop("VSCODE_GIT_IPC_HANDLE", None)
            print(f"Running inactive-pr-reminder.sh for PR #{pr_number}...")
            subprocess.run(["bash", script_path, str(pr_number)], env=env)
        except Exception as e:
            print(f"Error running inactive-pr-reminder.sh: {e}")

class OnCallClient:
    def fetch_on_call_from_api(self, rotation_name="cluster-toolkit"):
        """
        Fetches the actual on-call engineer from the corporate on-call API/rotation tool.
        """
        import subprocess
        import json
        try:
            # Use the Stubby API directly as recommended by the Oncallator JSON API g3doc
            cmd = ["stubby", "call", "--output_json", "blade:oncallator-prod", "Oncallator.GetOncall", f"rotation: '{rotation_name}'"]
            result = subprocess.run(cmd, capture_output=True, text=True)
            
            if result.returncode == 0 and result.stdout:
                try:
                    data = json.loads(result.stdout)
                    return data.get("person", "unknown_ldap")
                except json.JSONDecodeError:
                    return f"Invalid JSON response from stubby: {result.stdout[:50]}"
            else:
                return f"Stubby call failed: {result.stderr}"
        except Exception as e:
            return f"API Error: {e}"

    def get_current_on_call(self):
        print("Fetching current on-call engineer from rotation...")
        ldap = self.fetch_on_call_from_api()
        
        # DEMO: Hardcode the GitHub handle to Neelabh94 so the PR assignment works for the demo
        github_handle = "Neelabh94"
            
        return {"ldap": ldap, "github": github_handle}

class GeminiClient:
    def __init__(self):
        project_id = os.getenv("GOOGLE_CLOUD_PROJECT", "hpc-toolkit-dev")
        location = os.getenv("GOOGLE_CLOUD_LOCATION", "us-central1")
        self.client = genai.Client(vertexai=True, project=project_id, location=location)

    def triage_comments(self, comments, pr_number=None):
        pr_context = f" via the RC to Main PR #{pr_number}" if pr_number else " via the RC to Main PR"
        prompt = f"""
        You are 'ReleaseBot', an autonomous Principal Release Engineer.
        Your goal is to parse raw code review comments (from both humans and bots) and output strict, actionable JSON to automate bug filing.
        Ignore minor stylistic nits. Only output bugs for structural, logic, or security issues that block a release.
        All identified bugs MUST be classified as either P0 (critical blocker) or P1 (major issue). Do not create P2+ tickets for release blockers.
        
        Return a JSON array of objects with 'title', 'description', and 'priority' (P0 or P1).
        
        CRITICAL: In the description for each bug, you MUST first provide a clear, short technical explanation of the issue based on the comment. Then, you MUST include the following details (extract them from the comment context if available):
        1. "This bug was created{pr_context}"
        2. "The bug was introduced in the commit <commit_hash>" (if present in context)
        3. "The issue is in this file <file_path> at line <line_number>" (if present in context)
        
        Comments to parse:
        """ + str(comments)

        response = self.client.models.generate_content(
            model='gemini-2.5-flash',
            contents=prompt,
            config=types.GenerateContentConfig(
                temperature=0.7,
                response_mime_type="application/json",
            )
        )
        import json
        try:
            return json.loads(response.text)
        except Exception:
            return []

class BuganizerClient:
    def __init__(self):
        self.component_id = os.getenv("BUGANIZER_COMPONENT_ID", "123456")
        self.citc_path = os.getenv("CITC_WORKSPACE_PATH", "/google/src/cloud/rahimkh/demo/google3")

    def create_ticket(self, title, description, priority, assignee=None):
        priority_map = {"P0": "P0", "P1": "P1", "P2": "P2"}
        bug_priority = priority_map.get(priority, "P2")
        
        full_title = f"TEST [HACKATHON-2026] {title}"

        cmd = [
            "/google/bin/releases/issues-cli/issues", "create", 
            "--title", full_title, 
            "--component_id", self.component_id, 
            "--priority", bug_priority, 
            "--description", description
        ]
        if assignee:
            cmd.extend(["--assignee", assignee])
        
        try:
            print(f"Running issues-cli to create real ticket...")
            result = subprocess.run(cmd, capture_output=True, text=True)
            
            if result.returncode != 0:
                print(f"CLI error: {result.stderr}")
                
            for line in result.stdout.splitlines() + result.stderr.splitlines():
                # Attempt to extract an ID from the line (URL or raw ID)
                words = line.replace("/", " ").split()
                for word in words:
                    if word.strip(':,').isdigit() and len(word.strip(':,')) > 4:
                        return word.strip(':,')
                            
            print(f"Could not parse ticket ID from output. Assuming dummy_ticket_123.\nOutput was: {result.stdout}")
            return "dummy_ticket_123"
        except Exception as e:
            print(f"Error creating buganizer ticket via CLI: {e}")
            return "dummy_ticket_123"

    def get_ticket_status(self, ticket_id):
        if ticket_id.startswith("dummy"):
            print(f"Demo Mode: Automatically marking {ticket_id} as Closed so pipeline can proceed.")
            return {"status": "Closed", "priority": "P0"}
            
        cmd = [
            "/google/bin/releases/issues-cli/issues", "render", "--fields", "status,priority", ticket_id
        ]
        
        try:
            result = subprocess.run(cmd, capture_output=True, text=True)
            status = "NEW"
            priority = "P2"
            
            # Parse output looking for Status: and Priority:
            for line in result.stdout.splitlines() + result.stderr.splitlines():
                if "status:" in line.lower() or "state:" in line.lower():
                    parts = line.split(":")
                    if len(parts) > 1:
                        status = parts[1].strip()
                elif "priority:" in line.lower():
                    parts = line.split(":")
                    if len(parts) > 1:
                        priority = parts[1].strip()
                        
            return {"status": status, "priority": priority}
        except Exception as e:
            print(f"Error fetching buganizer ticket {ticket_id}: {e}")
            return {"status": "NEW", "priority": "P0"}

class TestRunnerClient:
    def trigger_tests(self, pr_number):
        print(f"Triggering real integration tests for PR {pr_number}...")
        # Actually execute a test script via subprocess
        try:
            result = subprocess.run(["bash", "run_tests.sh"], capture_output=True, text=True)
            if result.returncode == 0:
                print("Tests passed successfully.")
                return "SUCCESS"
            else:
                print(f"Tests failed. Output:\n{result.stderr}")
                return "FAILURE"
        except Exception as e:
            print(f"Failed to execute run_tests.sh: {e}")
            return "FAILURE"

    def check_test_status(self, run_id):
        # We are running synchronously above, so run_id IS the status
        return run_id 

    def get_failure_logs(self, run_id):
        return "Integration tests failed when executing run_tests.sh. Check terminal output."

class ChatClient:
    def __init__(self):
        self.webhook_url = os.getenv("GCHAT_WEBHOOK_URL")

    def send_message(self, text):
        print(f"[GChat Notification]: {text}")
        if self.webhook_url and "googleapis.com" in self.webhook_url:
            try:
                requests.post(self.webhook_url, json={"text": text}, timeout=5)
            except Exception as e:
                print(f"Failed to send GChat message: {e}")

class EmailClient:
    def send_email(self, to_email, subject, body):
        import smtplib
        from email.mime.text import MIMEText
        
        from email.utils import make_msgid, formatdate
        
        print(f"[Email Notification] Sending email to {to_email}: Subject='{subject}'")
        logging.info(f"Sending email to {to_email}...")
        
        try:
            msg = MIMEText(body)
            msg['Subject'] = subject
            msg['From'] = 'release-bot@hpc-toolkit.internal'
            msg['To'] = to_email
            msg['Message-ID'] = make_msgid()
            msg['Date'] = formatdate(localtime=True)
            
            with smtplib.SMTP('smtp.google.com', 25) as server:
                server.send_message(msg)
            logging.info("Email sent successfully!")
        except Exception as e:
            logging.warning(f"Could not send email (using mock fallback): {e}")
