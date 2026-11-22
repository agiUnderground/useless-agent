# Changelog

### v0.2.0

#### Features
* Redesigned the Settings -> Execution Engine tab with React-rendered status task boxes at the top and a centered, auto-scrolling vertical timeline below (executed items above, active item highlighted at the vertical center pointer, and planned items muted below, with manual-scroll pause and `FOLLOW LIVE` resume).
* Added dual-stack IPv4 and IPv6 support across backend, frontend, and Docker (including mixed IPv4/IPv6 sessions and automatic IPv4 fallback when IPv6 is disabled).
* Added LLM provider support for Google Gemini, OpenAI, Anthropic, Qwen, and MiniMax.
* Added dynamic model and thinking level selection, displaying `model · effort` on task cards.
* Added optional JSON configuration file support (`config.example.json`).
* Added persistent storage for settings and state.

#### Bugfixes
* Fixed real-time Execution Engine updates and instant session switching: fixed stale WebSocket handler closures, scoped incoming events by session, aborted stale `/execution-state` fetches on session switch, and fixed backend `/execution-state` JSON serialization (`Context`/`CancelFunc`) with live subtask/action snapshots.
* Restored toast error notifications for failed connections: entering an unreachable IP (or opening a task-card session whose backend is unreachable) now shows a bottom-right "Cannot connect" alert once per session, alongside the existing red chat-box flash.
* Fixed premature main-goal completion caused by JSON array truncation in `BreakGoalIntoSubtasks`, and added explicit main-goal verification and replanning (`IsMainGoalAchieved`, `ReplanSubtasks`).
* Fixed out-of-order WebSocket task status updates and queue state races that could leave newly started tasks stuck in `in-the-queue` or `in-progress`.
* Omitted unsupported `presence_penalty` parameter for the Gemini provider.
* Removed fixed `max-height` on the frontend tasks container.
* Cleaned up stale `/tmp/.X2-lock` and `/tmp/.X11-unix/X2` files on container startup so `Xvfb` restarts reliably.

#### Changes
* Refactored the execution engine (`Engine`) to use callback-driven subtask/task verification (triggering verification only when `stopIteration` is signaled, removing arbitrary sleep delays), guaranteed terminal status reporting, and context-propagated LLM calls.
* Refactored action execution to a registry-based executor with a compact JSON action format to reduce token usage while keeping human-readable logs and UI updates.
* Preferred X11/EWMH API for window detection and hardened the visual fallback.
* Optimized Docker builds with multi-stage parallel backend builds, BuildKit cache mounts (`apt`, Go/CGO, `npm`), fixed Go module layer caching, and added `frontend/.dockerignore`.
* Updated Go, npm, and Docker dependencies.
* Cleaned up `README.md` and moved release notes into `CHANGELOG.md`.

### v0.0.3
* User-assist feature works.
* Faster task cancellation.
* Extract css and js from the html file.
* Fixed routing of the line which connects chat and session window.
* Fixed media buttons overlay resizing.
* Tasks destined to the same session now queued.
* Use more API of the environment where it's possible.
* Break main.go into separate packages.
* Other small fixes.

### v0.0.2

* Manage multiple sessions at once.
* Fullscreen & maximize control elements for the video stream. Hotkeys F and M.
* Added a button to the task card to reopen a session window.
* MVP of the user-assist feature, only UI for now.
* Smart scrolling for the tasks section.
* Many small UI bugfixes and improvements.

![](/assets/img/multi-session-feature.png)
<p align="center">Multi-session</p>

![](/assets/img/user-assist-feature.png)
<p align="center">User-assist</p>

### v0.0.1

* Added basic tasks management and status indication.
* Hotkeys support.
* UI update.
* Video stream FPS selector.

![](/assets/img/tasks-management.png)
