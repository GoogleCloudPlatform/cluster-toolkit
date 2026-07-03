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

import time
import schedule
import logging
from dotenv import load_dotenv

from state_manager import StateManager
from clients import (
    GitHubClient, OnCallClient, GeminiClient,
    BuganizerClient, TestRunnerClient, ChatClient, EmailClient
)

# Set up logging
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")

load_dotenv()

class ReleaseOrchestrator:
    def __init__(self):
        self.state_manager = StateManager()
        self.github = GitHubClient()
        self.on_call = OnCallClient()
        self.gemini = GeminiClient()
        self.buganizer = BuganizerClient()
        self.test_runner = TestRunnerClient()
        self.chat = ChatClient()
        self.email = EmailClient()

    def run_cycle(self):
        state = self.state_manager.read_state()
        if state.get("paused", False):
            logging.info("ReleaseOrchestrator is currently PAUSED. Skipping cycle.")
            return

        current_state = state.get("CURRENT_STATE")
        logging.info(f"Running cycle. Current State: {current_state}")

        if current_state == "INITIALIZATION":
            self.handle_initialization()
        elif current_state == "TRIAGE":
            self.handle_triage()
        elif current_state == "AWAITING_BUG_FIXES":
            self.handle_awaiting_bug_fixes()
        elif current_state == "TEST_EXECUTION_AND_MONITORING":
            self.handle_test_execution()
        elif current_state == "AWAITING_FINAL_REVIEW":
            self.handle_awaiting_final_review()
        elif current_state == "MERGE_AND_BACKPORT":
            self.handle_merge_and_backport()
        elif current_state == "MANUALLY_CANCELLED":
            self.handle_manually_cancelled()
        elif current_state == "DONE":
            import sys
            logging.info("Release complete. Archiving to history, resetting state back to INITIALIZATION, and exiting.")
            self.state_manager.archive_to_history()
            self.state_manager.reset_state()
            sys.exit(0)
        else:
            logging.error(f"Unknown state: {current_state}")

    def handle_initialization(self):
        logging.info("State 0: Initialization & Version Bump")
        state = self.state_manager.read_state()
        
        # Check if this is scheduled mode and we need to wait for release time
        trigger_type = state.get("trigger_type")
        if not trigger_type:
            # Running as scheduled daemon
            if not self.is_scheduled_release_time():
                return
            # Scheduled time reached! Set trigger type
            self.state_manager.update_state(trigger_type="scheduled")
            import time
            self.state_manager.update_state(created_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()))
            state = self.state_manager.read_state()

        version_pr_number = state.get("version_pr_number")
        
        if not version_pr_number:
            result = self.github.create_rc_and_version_branches()
            if not result:
                logging.error("Failed to create the Version PR. Blocking progress.")
                return # Block until fixed manually
            version_pr_number, rc_branch = result
            
            # Send Email notification upon release starting (both adhoc and scheduled)
            self.email.send_email(
                to_email="rahimkh@google.com",
                subject="[ReleaseBot] Release Process Started",
                body=f"Hi Rahim,\n\nThe release candidate process has been started.\n\nRelease Branch: {rc_branch}\nVersion PR: #{version_pr_number}\n\nYou can track the progress on the Control Plane dashboard at http://localhost:8000.\n\nBest regards,\nReleaseBot"
            )
            
            # 2. Identify On-call
            on_call_info = self.on_call.get_current_on_call()
            
            if version_pr_number:
                # Assign on-call to the version PR
                self.github.assign_reviewer(version_pr_number, on_call_info['github'])
                
            self.state_manager.update_state(
                version_pr_number=version_pr_number,
                rc_branch=rc_branch,
                on_call_ldap=on_call_info['ldap'],
                on_call_github=on_call_info['github']
            )
            
        # 3. Wait for Merge
        # Check if closed manually
        if self.github.is_pr_closed(version_pr_number):
            logging.warning(f"Version PR {version_pr_number} was manually closed/cancelled!")
            self.state_manager.update_state(cancelled_from_state="INITIALIZATION")
            self.state_manager.set_current_state("MANUALLY_CANCELLED")
            return

        if not self.github.is_pr_merged(version_pr_number):
            if self.github.is_pr_approved(version_pr_number):
                logging.info(f"Version PR {version_pr_number} is approved! Auto-merging...")
                self.github.merge_pr(version_pr_number)
            else:
                logging.info(f"Waiting for version PR {version_pr_number} to be approved and merged...")
            return # Block until merged
            
        logging.info(f"Version PR {version_pr_number} is merged! Opening main RC PR...")
        # 4. Open main rc -> main PR in DRAFT MODE
        state = self.state_manager.read_state()
        rc_branch = state.get("rc_branch") or "release-candidate"
        pr_number = self.github.open_draft_pr(rc_branch, "main", "Release Candidate")
        self.state_manager.update_state(pr_number=pr_number)
        
        self.state_manager.set_current_state("TRIAGE")

    def handle_triage(self):
        logging.info("State 1: Triage (Draft Mode)")
        pr_number = self.state_manager.read_state().get("pr_number")
        
        # 1. Fetch comments from draft PR
        comments = self.github.get_pr_comments(pr_number)
        
        # 2. Extract actionable bugs via Gemini
        bugs = self.gemini.triage_comments(comments, pr_number)
        
        # 3. Create Buganizer tickets
        on_call_ldap = self.state_manager.read_state().get("on_call_ldap")
        ticket_ids = []
        bugs_details = []
        for bug in bugs:
            tid = self.buganizer.create_ticket(bug['title'], bug['description'], bug['priority'], assignee=on_call_ldap)
            ticket_ids.append(tid)
            bugs_details.append({
                "ticket_id": tid,
                "title": bug['title'],
                "priority": bug['priority'],
                "assignee": on_call_ldap,
                "status": "NEW"
            })
            
        self.state_manager.update_state(
            bugs=ticket_ids,
            bugs_details=bugs_details,
            seen_comments=len(comments)
        )
        
        # 4. Notify via GChat
        if ticket_ids:
            self.chat.send_message(f"Triage complete. Created tickets: {ticket_ids}. Blocked until resolved.")
            
        self.state_manager.set_current_state("AWAITING_BUG_FIXES")

    def handle_awaiting_bug_fixes(self):
        logging.info("State 2: Awaiting Bug Fixes")
        state = self.state_manager.read_state()
        bugs_details = state.get("bugs_details", [])
        
        all_resolved = True
        updated_bugs = []
        for bug in bugs_details:
            tid = bug['ticket_id']
            status_info = self.buganizer.get_ticket_status(tid)
            bug['status'] = status_info['status']
            bug['priority'] = status_info['priority']
            updated_bugs.append(bug)
            
            if status_info['status'].upper() not in ['CLOSED', 'FIXED', 'VERIFIED'] and status_info['priority'] in ['P0', 'P1']:
                all_resolved = False
                
        self.state_manager.update_state(bugs_details=updated_bugs)
        
        if all_resolved:
            logging.info("All blocking bugs resolved.")
            self.state_manager.set_current_state("TEST_EXECUTION_AND_MONITORING")

    def handle_test_execution(self):
        logging.info("State 3: Test Execution and Monitoring")
        pr_number = self.state_manager.read_state().get("pr_number")
        run_id = self.test_runner.trigger_tests(pr_number)
        
        # Simulate polling the test runner...
        status = self.test_runner.check_test_status(run_id)
        if status == "SUCCESS":
            self.state_manager.set_current_state("AWAITING_FINAL_REVIEW")
        else:
            self.chat.send_message(f"Tests failed! Logs: {self.test_runner.get_failure_logs(run_id)}")

    def handle_awaiting_final_review(self):
        logging.info("State 4: Awaiting Final Review")
        state = self.state_manager.read_state()
        pr_number = state.get("pr_number")
        on_call_github = state.get("on_call_github")
        on_call_ldap = state.get("on_call_ldap")
        
        # 1 & 2. Activate PR and Assign Reviewers (only once)
        if not state.get("reviewer_assigned"):
            self.github.convert_pr_to_active(pr_number)
            self.github.assign_reviewer(pr_number, on_call_github)
            self.state_manager.update_state(reviewer_assigned=True)
        
        # 3. Check for new comments that might block approval
        comments = self.github.get_pr_comments(pr_number)
        seen_comments = state.get("seen_comments", 0)
        
        if len(comments) > seen_comments:
            new_comments = comments[seen_comments:]
            bugs = self.gemini.triage_comments(new_comments, pr_number)
            if bugs:
                ticket_ids = state.get("bugs", [])
                bugs_details = state.get("bugs_details", [])
                new_tickets = []
                for bug in bugs:
                    tid = self.buganizer.create_ticket(bug['title'], bug['description'], bug['priority'], assignee=on_call_ldap)
                    ticket_ids.append(tid)
                    new_tickets.append(tid)
                    bugs_details.append({
                        "ticket_id": tid,
                        "title": bug['title'],
                        "priority": bug['priority'],
                        "assignee": on_call_ldap,
                        "status": "NEW"
                    })
                
                self.state_manager.update_state(
                    bugs=ticket_ids,
                    bugs_details=bugs_details,
                    seen_comments=len(comments)
                )
                logging.info(f"New comments raised issues! Created tickets: {new_tickets}. Reverting to AWAITING_BUG_FIXES.")
                self.chat.send_message(f"New review comments raised issues! Created tickets: {new_tickets}. Blocked until resolved.")
                self.state_manager.set_current_state("AWAITING_BUG_FIXES")
                return
            else:
                logging.info(f"Analyzed {len(new_comments)} new comment(s) via Gemini. No release-blocking bugs found. Ignoring.")
                self.state_manager.update_state(seen_comments=len(comments))
        
        # Check if closed manually
        if self.github.is_pr_closed(pr_number):
            logging.warning(f"Release PR {pr_number} was manually closed/cancelled!")
            self.state_manager.update_state(cancelled_from_state="AWAITING_FINAL_REVIEW")
            self.state_manager.set_current_state("MANUALLY_CANCELLED")
            return

        # 4. Wait for real GitHub approval
        if self.github.is_pr_approved(pr_number):
            logging.info(f"PR {pr_number} has been officially APPROVED on GitHub!")
            self.chat.send_message(f"PR {pr_number} is approved and ready for merge.")
            self.state_manager.set_current_state("MERGE_AND_BACKPORT")
        else:
            logging.info(f"PR {pr_number} is NOT approved yet. Waiting...")
            # We stay in State 4 until the next poll cycle

    def handle_merge_and_backport(self):
        logging.info("State 5: Merge and Backport")
        state = self.state_manager.read_state()
        pr_number = state.get("pr_number")
        
        backport_pr_number = state.get("backport_pr_number")
        on_call_github = state.get("on_call_github")
        
        if not backport_pr_number:
            # Merge rc PR
            if not self.github.merge_pr(pr_number):
                logging.error(f"Failed to merge PR {pr_number}. Blocking backport creation.")
                return
            
            # Create backport PR
            backport_pr_number = self.github.create_backport_pr(pr_number)
            if not backport_pr_number:
                logging.error("Failed to create backport PR. Will retry.")
                return
            self.state_manager.update_state(backport_pr_number=backport_pr_number)
            
            # Assign reviewer
            self.github.assign_reviewer(backport_pr_number, on_call_github)
            
        # Check if closed manually
        if backport_pr_number and self.github.is_pr_closed(backport_pr_number):
            logging.warning(f"Backport PR {backport_pr_number} was manually closed/cancelled!")
            self.state_manager.update_state(cancelled_from_state="MERGE_AND_BACKPORT")
            self.state_manager.set_current_state("MANUALLY_CANCELLED")
            return

        # Check if approved
        if self.github.is_pr_approved(backport_pr_number):
            logging.info(f"Backport PR {backport_pr_number} is approved! Auto-merging...")
            self.github.merge_pr(backport_pr_number)
            self.chat.send_message("Release successfully merged and backported! 🎉")
            self.state_manager.set_current_state("DONE")
        else:
            logging.info(f"Waiting for backport PR {backport_pr_number} to be approved and merged...")
            return

    def handle_manually_cancelled(self):
        logging.info("State: MANUALLY_CANCELLED. Awaiting resume signal from Control Plane.")

    def is_scheduled_release_time(self):
        import datetime
        now = datetime.datetime.utcnow()
        is_tuesday = now.weekday() == 1
        is_target_time = now.hour == 9 and now.minute == 30
        is_frozen = self.is_date_frozen(now.date())
        
        logging.info(f"Checking scheduled release time. Tuesday: {is_tuesday}, Time matches 9:30 UTC: {is_target_time}, Frozen: {is_frozen}")
        if is_tuesday and is_target_time and not is_frozen:
            return True
        return False

    def is_date_frozen(self, check_date):
        import datetime
        freezes = [
            (datetime.date(2026, 4, 20), datetime.date(2026, 4, 24)),
            (datetime.date(2026, 10, 16), datetime.date(2026, 10, 23)),
            (datetime.date(2026, 11, 8), datetime.date(2026, 11, 10)),
            (datetime.date(2026, 11, 20), datetime.date(2026, 12, 1)),
            (datetime.date(2026, 12, 16), datetime.date(2026, 12, 18)),
            (datetime.date(2026, 12, 18), datetime.date(2026, 12, 23)),
            (datetime.date(2026, 12, 23), datetime.date(2027, 1, 2)),
            (datetime.date(2027, 1, 3), datetime.date(2027, 1, 3))
        ]
        for start, end in freezes:
            if check_date >= start and check_date <= end:
                return True
        return False


def main():
    orchestrator = ReleaseOrchestrator()
    
    # 0. Demonstrate dynamic on-call fetching
    fetched_on_call = orchestrator.on_call.fetch_on_call_from_api()
    logging.info(f"API fetched current active on-call: {fetched_on_call}")
    
    # Run once immediately
    orchestrator.run_cycle()
    
    # Then schedule every 2 seconds for the demo (instead of 5 minutes)
    schedule.every(2).seconds.do(orchestrator.run_cycle)
    
    # Schedule the inactive PR reminder (e.g. every 30 seconds for demo)
    schedule.every(30).seconds.do(orchestrator.github.run_inactive_pr_reminder)
    
    logging.info("Starting ReleaseBot polling daemon...")
    while True:
        schedule.run_pending()
        time.sleep(1)

if __name__ == "__main__":
    main()
