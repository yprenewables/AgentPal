# AgentPal Authentication, Settings Refresh, and Backup Removal

## Goal

Make Authentication the first and default tab, keep Configuration after it, remove backup functionality and its implementation, and ensure the settings dialog always shows the latest saved values.

## Scope

- Set the initial tab to Authentication and order tabs as Authentication, Configuration.
- Remove the authentication backup tab and all supporting client/server API, archive helpers, backup-only types, and documentation of the removed feature.
- Remove local safety snapshots and rollback behavior from configuration application and profile activation, including result fields and UI messages that expose backup paths or rollback status.
- Make current bootstrap settings stateful at the app root. On successful save, replace it with the `Bootstrap` returned from `SaveSettings`; use that state for the header/menu and settings form.
- Preserve unrelated authentication profile CRUD, Central profile synchronization, and configuration apply behavior.
- Do not modify historical specs/plans. Update the current README to describe the remaining product behavior.

## Behavior

The settings dialog is initialized from the latest `Bootstrap` state each time it mounts. Saving persists settings through the existing backend call, updates the root state with the returned bootstrap, displays the existing restart/refresh notice, and closes the dialog. Failed saves leave the current state unchanged and keep the dialog available for correction.

Configuration and authentication profile changes no longer create local snapshots or attempt rollback. Errors still propagate to the existing error handling. Result contracts no longer include backup paths or rollback flags.

## Testing

- Go tests assert that configuration apply and profile activation do not create a backup directory and still apply successful changes.
- Contract checks ensure removed backup fields/types and APIs are not present in active code or generated bindings.
- Frontend build validates the settings state flow and tab changes.
- Repository searches confirm removed backup UI and persistence mechanisms have no active code/documentation references, excluding historical design records and the design history itself.
- Run all Go tests and `git diff --check`.
