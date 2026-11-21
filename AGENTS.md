# Context & File Restrictions

- Never read, open, or inspect files in `assets/demos/` or `assets/img/` unless explicitly requested by the user.
- Exclude `assets/demos/` and `assets/img/` from file searches, directory traversals, and context gathering.
