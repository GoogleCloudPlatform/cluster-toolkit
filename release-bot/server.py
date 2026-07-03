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

import http.server
import json
import os
import urllib.parse
import sys
from pathlib import Path

# Resolve paths
BASE_DIR = Path(__file__).parent.resolve()
STATE_FILE = BASE_DIR / "state.json"
HISTORY_FILE = BASE_DIR / "history.json"

# Load local .env file if present
dotenv_path = BASE_DIR / ".env"
if dotenv_path.exists():
    with open(dotenv_path, "r") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if "=" in line:
                key, val = line.split("=", 1)
                os.environ[key.strip()] = val.strip()

class ControlPlaneHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # Suppress logging to keep console output clean
        pass

    def do_GET(self):
        parsed_url = urllib.parse.urlparse(self.path)
        path = parsed_url.path
        
        if path == "/api/state":
            self.handle_get_state()
        elif path == "/api/history":
            self.handle_get_history()
        elif path == "/api/aborted-history":
            self.handle_get_aborted_history()
        elif path == "/api/calendar":
            self.handle_get_calendar()
        elif path == "/" or path == "/index.html":
            self.serve_file(BASE_DIR / "index.html", "text/html")
        else:
            # Try to serve static file from current directory
            file_path = BASE_DIR / path.lstrip("/")
            if file_path.is_file() and not file_path.name.endswith(".py") and not file_path.name.endswith(".json"):
                ext = file_path.suffix
                content_type = "text/plain"
                if ext == ".html": content_type = "text/html"
                elif ext == ".css": content_type = "text/css"
                elif ext == ".js": content_type = "application/javascript"
                elif ext == ".png": content_type = "image/png"
                self.serve_file(file_path, content_type)
            else:
                self.send_error(404, "File Not Found")

    def do_POST(self):
        parsed_url = urllib.parse.urlparse(self.path)
        path = parsed_url.path
        
        if path == "/api/pause":
            self.handle_toggle_pause()
        elif path == "/api/start":
            self.handle_start_release()
        elif path == "/api/stop":
            self.handle_stop_release()
        elif path == "/api/resume":
            self.handle_resume_release()
        else:
            self.send_error(404, "Endpoint Not Found")

    def serve_file(self, path, content_type):
        try:
            with open(path, "rb") as f:
                content = f.read()
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(content)))
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            self.wfile.write(content)
        except Exception as e:
            self.send_error(500, f"Internal Server Error: {e}")

    def handle_get_state(self):
        state = {}
        if STATE_FILE.exists():
            try:
                with open(STATE_FILE, "r") as f:
                    state = json.load(f)
            except Exception:
                pass
        
        # If no state file, supply defaults
        if not state:
            state = {
                "CURRENT_STATE": "INITIALIZATION",
                "pr_number": None,
                "paused": False,
                "bugs_details": []
            }
            
        # Get last successful release from history
        last_success = None
        if HISTORY_FILE.exists():
            try:
                with open(HISTORY_FILE, "r") as f:
                    history = json.load(f)
                    if history:
                        # Sort by merged_at descending
                        history.sort(key=lambda x: x.get("merged_at", ""), reverse=True)
                        last_success = history[0]
            except Exception:
                pass
                
        # Calculate next scheduled release
        # Standard: next Tuesday (weekly)
        import datetime
        today = datetime.date.today()
        # Find next Tuesday
        days_ahead = 1 - today.weekday() # Monday is 0, Tuesday is 1
        if days_ahead <= 0: # Target day has already happened this week
            days_ahead += 7
        next_tuesday = today + datetime.timedelta(days=days_ahead)
        
        response_data = {
            "state": state,
            "next_release_date": next_tuesday.strftime("%Y-%m-%d"),
            "last_successful_release": last_success.get("merged_at") if last_success else None,
            "last_successful_release_id": last_success.get("release_id") if last_success else None
        }
        
        self.send_json(response_data)

    def handle_get_history(self):
        history = []
        if HISTORY_FILE.exists():
            try:
                with open(HISTORY_FILE, "r") as f:
                    history = json.load(f)
            except Exception:
                pass
        self.send_json(history)

    def handle_get_aborted_history(self):
        aborted_file = BASE_DIR / "aborted_history.json"
        history = []
        if aborted_file.exists():
            try:
                with open(aborted_file, "r") as f:
                    history = json.load(f)
            except Exception:
                pass
        self.send_json(history)

    def handle_get_calendar(self):
        freezes = [
            {"name": "Spring Guard Window (RRC1)", "start": "2026-04-20", "end": "2026-04-24", "rrc": 1, "description": "Guarded - No feature releases."},
            {"name": "Autumn Guard Window (RRC2)", "start": "2026-10-16", "end": "2026-10-23", "rrc": 2, "description": "Guarded - Light week."},
            {"name": "Diwali Guard Window (RRC2)", "start": "2026-11-08", "end": "2026-11-10", "rrc": 2, "description": "Guarded - Light week."},
            {"name": "Thanksgiving Freeze (RRC1)", "start": "2026-11-20", "end": "2026-12-01", "rrc": 1, "description": "Guarded - Thanksgiving / Cyber Week Freeze."},
            {"name": "Pre-Holiday Guard (RRC1)", "start": "2026-12-16", "end": "2026-12-18", "rrc": 1, "description": "Guarded - Pre-Holiday Guard."},
            {"name": "Winter Holiday Chilled (RRC2)", "start": "2026-12-18", "end": "2026-12-23", "rrc": 2, "description": "Chilled - Strict Emergency Only."},
            {"name": "Winter Holiday Frozen (RRC3)", "start": "2026-12-23", "end": "2027-01-02", "rrc": 3, "description": "Frozen - TOTAL FREEZE."},
            {"name": "Post-Holiday Guard (RRC1)", "start": "2027-01-03", "end": "2027-01-03", "rrc": 1, "description": "Guarded - Gradual unfreeze."}
        ]
        self.send_json(freezes)

    def handle_toggle_pause(self):
        state = {}
        if STATE_FILE.exists():
            try:
                with open(STATE_FILE, "r") as f:
                    state = json.load(f)
            except Exception:
                pass
                
        if not state:
            state = {
                "CURRENT_STATE": "INITIALIZATION",
                "pr_number": None,
                "paused": False,
                "bugs_details": []
            }
            
        current_paused = state.get("paused", False)
        state["paused"] = not current_paused
        
        try:
            with open(STATE_FILE, "w") as f:
                json.dump(state, f, indent=4)
            self.send_json({"success": True, "paused": state["paused"]})
        except Exception as e:
            self.send_error(500, f"Failed to save state: {e}")

    def handle_start_release(self):
        # 1. Reset state to initialization
        import time
        initial_state = {
            "CURRENT_STATE": "INITIALIZATION",
            "pr_number": None,
            "rc_branch": None,
            "version_branch": None,
            "bugs": [],
            "bugs_details": [],
            "test_run_ids": [],
            "test_retries": 0,
            "on_call_ldap": "neelgoyal",
            "on_call_github": "Neelabh94",
            "pr_active_timestamp": None,
            "paused": False,
            "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "merged_at": None,
            "trigger_type": "adhoc",
            "version_pr_number": None,
            "backport_pr_number": None,
            "reviewer_assigned": False,
            "seen_comments": 0
        }
        
        try:
            with open(STATE_FILE, "w") as f:
                json.dump(initial_state, f, indent=4)
        except Exception as e:
            self.send_json({"success": False, "error": f"Failed to initialize state: {e}"})
            return
            
        # 2. Start main.py in background
        import subprocess
        log_file = BASE_DIR / "daemon.log"
        try:
            # Run daemon with unbuffered python and redirect logs
            with open(log_file, "w") as f_log:
                proc = subprocess.Popen(
                    [sys.executable, "-u", str(BASE_DIR / "main.py")],
                    stdout=f_log,
                    stderr=f_log,
                    cwd=str(BASE_DIR),
                    start_new_session=True
                )
            
            # Save daemon PID to state.json
            state = {}
            if STATE_FILE.exists():
                try:
                    with open(STATE_FILE, "r") as f:
                        state = json.load(f)
                except Exception:
                    pass
            state["daemon_pid"] = proc.pid
            with open(STATE_FILE, "w") as f:
                json.dump(state, f, indent=4)
                
            self.send_json({"success": True})
        except Exception as e:
            self.send_json({"success": False, "error": f"Failed to launch daemon: {e}"})

    def handle_stop_release(self):
        state = {}
        if STATE_FILE.exists():
            try:
                with open(STATE_FILE, "r") as f:
                    state = json.load(f)
            except Exception:
                pass
                
        # 1. Terminate daemon process if running
        pid = state.get("daemon_pid")
        if pid:
            import os
            import signal
            try:
                os.kill(pid, signal.SIGTERM)
                print(f"Terminated daemon process {pid}")
            except Exception as e:
                print(f"Could not terminate daemon process {pid}: {e}")
                
        # 2. Close open PRs on GitHub
        version_pr = state.get("version_pr_number")
        release_pr = state.get("pr_number")
        backport_pr = state.get("backport_pr_number")
        
        if version_pr or release_pr or backport_pr:
            try:
                from clients import GitHubClient
                github = GitHubClient()
                if version_pr:
                    github.close_pr(version_pr)
                if release_pr:
                    github.close_pr(release_pr)
                if backport_pr:
                    github.close_pr(backport_pr)
            except Exception as e:
                print(f"Error closing PRs: {e}")
                
        # 3. Save to aborted history
        if state.get("rc_branch") or state.get("created_at"):
            try:
                aborted_file = BASE_DIR / "aborted_history.json"
                aborted_history = []
                if aborted_file.exists():
                    try:
                        with open(aborted_file, 'r') as f:
                            aborted_history = json.load(f)
                    except Exception:
                        pass
                import time
                record = {
                    "release_id": state.get('rc_branch') or 'unknown',
                    "created_at": state.get("created_at"),
                    "aborted_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                    "last_state": state.get("CURRENT_STATE"),
                    "pr_number": state.get("pr_number") or state.get("version_pr_number"),
                    "on_call_ldap": state.get("on_call_ldap")
                }
                aborted_history.append(record)
                with open(aborted_file, 'w') as f:
                    json.dump(aborted_history, f, indent=4)
            except Exception as e:
                print(f"Failed to save aborted history: {e}")

        # 4. Reset state file
        try:
            default_state = {
                "CURRENT_STATE": "INITIALIZATION",
                "pr_number": None,
                "rc_branch": None,
                "version_branch": None,
                "bugs": [],
                "bugs_details": [],
                "test_run_ids": [],
                "test_retries": 0,
                "on_call_ldap": None,
                "on_call_github": None,
                "pr_active_timestamp": None,
                "paused": False,
                "created_at": None,
                "merged_at": None,
                "cancelled_from_state": None,
                "trigger_type": None,
                "version_pr_number": None,
                "backport_pr_number": None,
                "reviewer_assigned": False,
                "seen_comments": 0
            }
            with open(STATE_FILE, "w") as f:
                json.dump(default_state, f, indent=4)
            self.send_json({"success": True})
        except Exception as e:
            self.send_error(500, f"Failed to reset state: {e}")

    def handle_resume_release(self):
        state = {}
        if STATE_FILE.exists():
            try:
                with open(STATE_FILE, "r") as f:
                    state = json.load(f)
            except Exception:
                pass
                
        if state.get("CURRENT_STATE") != "MANUALLY_CANCELLED":
            self.send_json({"success": False, "error": "Release is not in manually cancelled state."})
            return
            
        # Restore original state
        original_state = state.get("cancelled_from_state") or "INITIALIZATION"
        
        # Reopen PR on Github based on which state it was cancelled from
        pr_number = None
        if original_state == "INITIALIZATION":
            pr_number = state.get("version_pr_number")
        elif original_state == "MERGE_AND_BACKPORT":
            pr_number = state.get("backport_pr_number")
        else:
            pr_number = state.get("pr_number")
            
        if pr_number:
            from clients import GitHubClient
            github = GitHubClient()
            github.reopen_pr(pr_number)
            
        state["CURRENT_STATE"] = original_state
        state["cancelled_from_state"] = None
        
        try:
            with open(STATE_FILE, "w") as f:
                json.dump(state, f, indent=4)
        except Exception as e:
            self.send_json({"success": False, "error": f"Failed to restore state: {e}"})
            return
            
        # Restart daemon
        import subprocess
        log_file = BASE_DIR / "daemon.log"
        try:
            with open(log_file, "w") as f_log:
                proc = subprocess.Popen(
                    [sys.executable, "-u", str(BASE_DIR / "main.py")],
                    stdout=f_log,
                    stderr=f_log,
                    cwd=str(BASE_DIR),
                    start_new_session=True
                )
            state["daemon_pid"] = proc.pid
            with open(STATE_FILE, "w") as f:
                json.dump(state, f, indent=4)
            self.send_json({"success": True})
        except Exception as e:
            self.send_json({"success": False, "error": f"Failed to restart daemon: {e}"})

    def send_json(self, data):
        content = json.dumps(data, indent=2).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(content)))
        self.send_header("Cache-Control", "no-cache")
        self.end_headers()
        self.wfile.write(content)

def run(server_class=http.server.HTTPServer, handler_class=ControlPlaneHandler):
    port = int(os.environ.get("ANTIGRAVITY_SIDECAR_WEB_PORT", 8000))
    server_address = ("", port)
    httpd = server_class(server_address, handler_class)
    print(f"Starting Control Plane Server on port {port}...")
    httpd.serve_forever()

if __name__ == "__main__":
    run()
