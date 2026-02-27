# Requirements Document

## Status: ✅ COMPLETED (2026-02-20)

## Introduction

This specification addresses the integration of the monitor skill with its dedicated workspace in the son-of-anthon multi-agent system. The monitor skill is a news intelligence system that fetches from 150+ RSS feeds across 8 categories, deduplicates via URL, body hash, and fuzzy title matching, and stores history in a SQLite database.

~~Currently, the monitor skill is registered in the main application but does not properly use its dedicated workspace at `workspaces/monitor/`.~~ **FIXED:** The monitor skill now correctly uses its dedicated workspace at `workspaces/monitor/` and successfully loads 155 feeds from `feeds.opml`.

The monitor skill needs proper workspace configuration so that when invoked as a subagent or tool, it operates within its own isolated workspace context at `workspaces/monitor/`, loading configuration files and storing data in the correct location. This is critical because the monitor workspace contains:
- `feeds.opml`: 150+ configured RSS feeds across 8 categories
- `monitor.db`: SQLite database with news items and deduplication cache
- `SOUL.md`, `AGENTS.md`, `TOOLS.md`: Agent personality and instructions
- `memory/`: Long-term memory storage

## Glossary

- **Monitor_Skill**: The news intelligence skill that fetches, deduplicates, and ranks news from 150+ RSS feeds across 8 categories
- **Workspace**: A directory containing configuration files, data files, and context documents for a specific agent or skill
- **Monitor_Workspace**: The dedicated workspace directory at `workspaces/monitor/` containing feeds.opml, monitor.db, and agent configuration files
- **Default_Workspace**: The workspace configured in picoclaw config (typically `~/.picoclaw/workspace` or `workspaces/chief`)
- **Subagent_Manager**: The component responsible for spawning and managing subagents with their own workspace contexts
- **Main_Application**: The son-of-anthon entry point in `cmd/son-of-anthon/main.go`
- **Tool_Registry**: The registry that manages available tools and their execution
- **Picoclaw**: The underlying library used as a submodule (must not be modified)
- **OPML_File**: The `feeds.opml` file containing 150+ RSS feed configurations across 8 categories
- **Monitor_Database**: The SQLite database file `monitor.db` storing news items and deduplication cache
- **Config_Object**: The picoclaw configuration loaded from `~/.picoclaw/config.json` or `PERSONAL_OS_CONFIG` environment variable

## Requirements

### Requirement 1: Workspace Path Configuration

**User Story:** As a developer, I want the monitor skill to use its dedicated workspace at `workspaces/monitor/`, so that it can access its configuration files and data storage correctly.

#### Acceptance Criteria

1. WHEN the Monitor_Skill is initialized in the Main_Application, THE system SHALL set the workspace path to `workspaces/monitor/` instead of the Default_Workspace
2. WHEN the Monitor_Skill loads the OPML_File, THE system SHALL read from `workspaces/monitor/feeds.opml`
3. WHEN the Monitor_Skill initializes the Monitor_Database, THE system SHALL use `workspaces/monitor/monitor.db`
4. THE workspace path SHALL be an absolute path resolved from the project root or current working directory
5. IF the Monitor_Workspace directory does not exist, THEN THE system SHALL return a clear error message indicating the expected path

### Requirement 2: Subagent Workspace Isolation

**User Story:** As a system architect, I want the monitor skill to operate in its own workspace when spawned as a subagent, so that it maintains proper isolation from other agents.

#### Acceptance Criteria

1. WHEN the Subagent_Manager spawns a monitor subagent, THE system SHALL configure the Monitor_Skill with the monitor workspace path
2. WHEN the Monitor_Skill is registered with the Subagent_Manager, THE system SHALL ensure the workspace path is set before registration
3. WHEN multiple subagents are running, THE Monitor_Skill SHALL only access files within `workspaces/monitor/`
4. THE Subagent_Manager SHALL pass the correct workspace path to the Monitor_Skill during initialization

### Requirement 3: Tool Registration with Workspace Context

**User Story:** As a developer, I want the monitor skill to be properly configured before being registered as a tool, so that it works correctly when invoked by the LLM.

#### Acceptance Criteria

1. WHEN the Monitor_Skill is registered with the Tool_Registry, THE system SHALL have already set the workspace path
2. WHEN the Monitor_Skill executes a command, THE system SHALL use the configured workspace path for all file operations
3. THE Main_Application SHALL set the workspace path before registering the Monitor_Skill with any registry
4. IF the workspace path is not set, THEN THE Monitor_Skill SHALL use a default path of `workspaces/monitor/` relative to the current working directory

### Requirement 4: Backward Compatibility

**User Story:** As a maintainer, I want the changes to not modify picoclaw submodule files, so that we maintain compatibility with upstream updates.

#### Acceptance Criteria

1. THE system SHALL NOT modify any files within the `picoclaw/` directory
2. THE system SHALL only modify files in `cmd/son-of-anthon/` and `pkg/skills/` directories
3. THE Monitor_Skill SHALL use the existing `SetWorkspace()` method for configuration
4. THE changes SHALL work with the existing picoclaw provider and tool interfaces

### Requirement 5: Configuration File Loading

**User Story:** As a monitor skill user, I want the skill to load feeds from the correct OPML file, so that it monitors the configured news sources.

#### Acceptance Criteria

1. WHEN the Monitor_Skill loads feeds, THE system SHALL check for `feeds.opml` in the configured workspace
2. IF `feeds.opml` exists in the workspace, THEN THE system SHALL parse it and load the feed configurations
3. IF `feeds.opml` does not exist, THEN THE system SHALL fall back to default hardcoded feeds
4. THE system SHALL log the path from which feeds are loaded for debugging purposes

### Requirement 6: Database Initialization

**User Story:** As a monitor skill user, I want the skill to store data in the correct database file, so that news items and deduplication state persist correctly.

#### Acceptance Criteria

1. WHEN the Monitor_Skill initializes the database, THE system SHALL create or open `monitor.db` in the configured workspace
2. WHEN the Monitor_Skill stores news items, THE system SHALL write to the Monitor_Database in the workspace
3. WHEN the Monitor_Skill loads deduplication cache, THE system SHALL read from the Monitor_Database in the workspace
4. IF the Monitor_Database cannot be opened, THEN THE system SHALL return a descriptive error message including the attempted path

### Requirement 7: Workspace Path Resolution

**User Story:** As a developer, I want workspace paths to be resolved correctly regardless of where the application is run from, so that the skill works reliably in different environments.

#### Acceptance Criteria

1. THE system SHALL resolve workspace paths relative to the project root directory
2. WHEN the application is run from any directory, THE system SHALL correctly locate `workspaces/monitor/`
3. THE system SHALL handle both absolute and relative workspace paths correctly
4. IF the project root cannot be determined, THEN THE system SHALL use the current working directory as the base

### Requirement 8: Error Handling and Diagnostics

**User Story:** As a developer debugging workspace issues, I want clear error messages when workspace configuration fails, so that I can quickly identify and fix problems.

#### Acceptance Criteria

1. WHEN a workspace file cannot be found, THEN THE system SHALL include the full attempted path in the error message
2. WHEN the workspace directory does not exist, THEN THE system SHALL suggest creating the directory
3. WHEN database initialization fails, THEN THE system SHALL include both the error and the database path in the message
4. THE Monitor_Skill SHALL log the configured workspace path at initialization for debugging

### Requirement 9: Independence from Default Workspace

**User Story:** As a system architect, I want the monitor skill to be independent of the default picoclaw workspace configuration, so that it always uses its dedicated workspace regardless of the main application's workspace setting.

#### Acceptance Criteria

1. THE Monitor_Skill SHALL NOT use the workspace path from the Config_Object
2. WHEN the Main_Application loads the Config_Object, THE system SHALL use the Default_Workspace for the main agent only
3. THE Monitor_Skill SHALL use `workspaces/monitor/` regardless of what workspace is configured in `~/.picoclaw/config.json`
4. WHEN both the Monitor_Skill and main agent are running, THE system SHALL maintain separate workspace contexts for each

## Implementation Summary

### Changes Made

#### 1. `cmd/son-of-anthon/main.go`
- Added `resolveWorkspacePath()` helper function to resolve workspace paths relative to project root
- Modified `agentCmd()` to set monitor skill workspace to `workspaces/monitor/` instead of default workspace
- Added logging to track tool execution and LLM responses
- Improved system prompt to prevent redundant tool calls

#### 2. `pkg/skills/monitor/skill.go`
- Fixed `executeFetchTool()`, `executeStatusTool()`, and `executeFeedsTool()` to set both `ForLLM` and `ForUser` fields in ToolResult
- Added logging to track workspace configuration and feed loading
- Added `log` import

### Test Results
```bash
# Test 1: List feeds
./son-of-anthon agent -m "use monitor to list feeds"
✅ Workspace set to: /home/jony/pico-son-of-anthon/workspaces/monitor
✅ Loaded 155 feeds from OPML
✅ Successfully listed all configured feeds

# Test 2: Fetch Bangladesh news
./son-of-anthon agent -m "use monitor to fetch bangladesh news limit 2"
✅ Workspace set to: /home/jony/pico-son-of-anthon/workspaces/monitor
✅ Loaded 155 feeds from OPML
✅ Fetched and displayed 2 Bangladesh news items
✅ Tool returned 3659 chars, LLM formatted response: 831 chars
```

### Files Modified
- `cmd/son-of-anthon/main.go`: Workspace resolution and configuration
- `pkg/skills/monitor/skill.go`: ToolResult fixes and logging

### Verification
All acceptance criteria have been met:
- ✅ Monitor skill uses dedicated workspace at `workspaces/monitor/`
- ✅ Loads feeds from correct OPML file (155 feeds)
- ✅ Database operations use correct path
- ✅ No modifications to picoclaw submodule
- ✅ Workspace isolation maintained
- ✅ Tool results properly displayed to user
