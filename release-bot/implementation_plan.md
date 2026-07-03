# Implementation Plan: ReleaseBot (Autonomous Release Orchestrator)

**Project Goal:** Eliminate the manual toil of release management by building a Gemini-driven state machine that handles branch creation, bug triage, test monitoring, and nag-messaging, requiring humans *only* for final approvals and code fixes.

---

## 1. System Architecture & Tech Stack

For a 24-hour hackathon, we need to optimize for speed. We will build a **Python-based polling daemon**.

*   **Core Logic Engine:** Python 3.10+
*   **LLM Integration:** Direct API calls exclusively to **Gemini**.
*   **Hosting:** For the hackathon demo, this daemon can simply run locally on your workstation inside a `tmux` session, or as a background process.
*   **State Management:** Local JSON file to track the current state of a release.
*   **Integrations:**
    *   `PyGithub` (Python library for GitHub API)
    *   Buganizer API Client (internal)
    *   Google Chat Webhooks/API (for the "NagBot" feature)
    *   Internal Oncall API (`https://oncall.corp.google.com/cluster-toolkit`)

---

## 2. Required Accesses & Permissions (Pre-Hackathon Checklist)

You must secure these credentials *before* the clock starts:

1.  **GitHub Personal Access Token (PAT):**
    *   *Scopes required:* `repo`. 
    *   *Note for Demo:* Running this on a personal fork of the `cluster-toolkit` repo is the perfect, safe way to demonstrate the workflow without risking the upstream public repo.
2.  **Buganizer Service Account / Credentials:**
    *   *Scopes required:* Read/Write access to your component.
3.  **Gemini API Key / Internal Access:**
    *   *Purpose:* Parsing human/bot review comments into structured bug reports and analyzing test logs.
4.  **Google Chat Webhook URL:**
    *   *Purpose:* Pinging the team space or on-call engineers.
5.  **Test Triggering Credentials:** Ensure the daemon has the correct internal credentials to execute the logic found in the `babysit.ipynb` notebook and access Cloud Build logs.

---

## 3. The State Machine (Core Logic)

The Python daemon runs on a schedule (e.g., every 5 minutes). It checks the local JSON state file and executes logic based on the `CURRENT_STATE`.

### State 0: `INITIALIZATION` & `VERSION_BUMP`
*   **Trigger:** A cron schedule or a manual run of the script.
*   **Action:** 
    1.  **Execute script:** Call `/usr/local/google/home/neelgoyal/cluster-toolkit/tools/create-release-candidate.sh` to create the `rc` and `version` branches.
    2.  **Identify On-call:** Fetch JSON from `https://oncall.corp.google.com/cluster-toolkit`, extract LDAPs, and map to GitHub handles via `go/github`.
    3.  **Create Version PR:** Open PR: `version` -> `rc`. Assign the mapped on-call handles as reviewers.
    4.  **Wait for Merge:** Poll this PR until the on-call engineer approves and merges it. 
*   **Transition:** Once the `version` -> `rc` PR is merged, open the main release PR (`rc` -> `main`) in **DRAFT MODE** to prevent notification spam. Move to `STATE_1_TRIAGE`.

### State 1: `TRIAGE` (Draft Mode)
*   **Action:** 
    1. Fetch **all** comments left on the draft `rc` -> `main` PR (from the `gemini review` bot and any early human reviewers).
    2. Send combined comments to Gemini with the prompt: *"Extract actionable bugs from these comments. Output a JSON array. Criticality must be mapped strictly to P0 or P1."*
    3. Create Buganizer tickets as P0 or P1 based on the JSON output.
    4. **Notify via GChat:** Send message to the team space: *"I have triaged the draft PR review and created these internal Buganizer tickets: [Links]. Release testing is blocked until these are resolved or downgraded."*
*   **Transition:** Move to `STATE_2_AWAITING_BUG_FIXES`.

### State 2: `AWAITING_BUG_FIXES` (Draft Mode)
*   **Action:** 
    *   **Poll Buganizer API:** Check the linked tickets. A ticket is considered "resolved" for release purposes if it is marked as **Closed** OR if its priority is downgraded to **P2 or below** (non-blocking).
*   **Transition:** When all bugs meet the "resolved" criteria, move to `STATE_3_TEST_EXECUTION`.

### State 3: `TEST_EXECUTION_AND_MONITORING` (Draft Mode)
*   **Action:** 
    1.  **Trigger Tests:** Execute the automation logic extracted from the `babysit.ipynb` notebook to programmatically trigger the integration tests against the PR.
    2.  **Poll CI Status:** Monitor the test execution pipeline.
    3.  **Flake Handling & Retries:** Configure the bot to automatically retry a failed test up to **2 times** (max 3 runs total).
    4.  **Hard Failure Handling:** 
        *   If a test fails after exhausting its 2 retries, fetch the Cloud Build logs for that specific run.
        *   Parse the logs to extract the "triage agent summary".
        *   Create a new Buganizer ticket (P0/P1) using the extracted triage summary as the ticket description.
        *   **Notify via GChat:** *"Integration test [Test Name] failed after 2 retries. I have created a blocking ticket with the log summary: [Link]."*
        *   **Block State:** Poll the Buganizer API for this new ticket. Do not proceed until a human has reviewed and resolved this ticket (Closed or downgraded <= P2).
*   **Transition:** Once CI is fully Green (or all exhausted-retry tickets have been human-resolved), move to `STATE_4_AWAITING_FINAL_REVIEW`.

### State 4: `AWAITING_FINAL_REVIEW` (Active Mode)
*   **Action:** 
    1.  **Activate PR:** Take the `rc` -> `main` PR out of draft mode.
    2.  **Assign Reviewers:** Assign the on-call engineers for final approval.
    3.  *Nag Logic:* If PR is open for > 4 hours, send a **Google Chat** message: *"Hey @[oncall_ldap], Release PR #[ID] is fully tested, bugs are resolved, and it is waiting for your final approval."*
*   **Transition:** Once PR is approved by the on-call team, move to `STATE_5_MERGE_AND_BACKPORT`.

### State 5: `MERGE_AND_BACKPORT`
*   **Action:**
    *   Merge the `rc` PR into `main`.
    *   Generate Release Notes automatically.
    *   Create the backport PR (`main` -> `develop`) to sync hotfixes.
*   **Transition:** Move to `DONE`.

---

## 4. The System Prompt (The Persona)

When querying Gemini for triage, use this strict system prompt:

```text
You are 'ReleaseBot', an autonomous Principal Release Engineer. 
Your goal is to parse raw code review comments (from both humans and bots) and output strict, actionable JSON to automate bug filing.
Ignore minor stylistic nits. Only output bugs for structural, logic, or security issues that block a release.
All identified bugs MUST be classified as either P0 (critical blocker) or P1 (major issue). Do not create P2+ tickets for release blockers.
```


