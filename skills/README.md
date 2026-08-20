# Cluster Toolkit AI Skills Directory

Welcome to the **Cluster Toolkit Skills Directory**. Skills are specialized, domain-specific instruction bundles and automated helper scripts designed for AI coding assistants (such as Jetski, Cider-J, and Gemini Code Assist). They equip AI agents with exact rules, translation parsers, blueprint templates, and CLI tools for Cluster Toolkit development, blueprint creation, and CLI migration workflows.

---

## 📖 Available Skills

| Skill Name | Description | Key Components |
| :--- | :--- | :--- |
| [`xpk_to_clustertoolkit`](xpk_to_clustertoolkit/SKILL.md) | Automates the migration of `xpk` CLI usages, scripts, and documentation to equivalent Cluster Toolkit (`gcluster`) blueprints and workload commands (`gcluster job submit`). | Deterministic Python parser (`parse_xpk_to_gcluster.py`), unit tests, TPU topology lookups, external migration guide reference. |

---

## 🛠️ How Skills Work

Each skill folder contains:

1. **`SKILL.md`**: The primary instruction document containing YAML frontmatter (`name`, `description`), workflow guidelines, and execution rules.
2. **`scripts/`**: Automated scripts and parsers used by the skill to process input files, convert configurations, or validate blueprints.
3. **`references/`**: Golden blueprint examples, machine type lookups, and reference documentation.
4. **`tests/`**: Unit tests verifying parser accuracy and skill rule compliance.

---

## 🚀 How to Use a Skill

When interacting with an AI assistant in the Cluster Toolkit repository:

- **Natural Language Invocation**: Ask the AI assistant to perform a task covered by a skill (e.g., *"Convert this XPK script to Cluster Toolkit"*). The assistant will automatically discover and invoke the relevant skill.
- **Explicit Tagging**: Mention the skill directly in your prompt using `@<skill_name>` (e.g., `@xpk_to_clustertoolkit Migrate docs/xpk_tutorial.md`).
