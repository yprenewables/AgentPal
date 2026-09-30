# Auth Profile Template Design

## Goal

Make the central admin Auth Profiles create and edit workflows template-driven. Administrators configure the generated profile templates and variable definitions in System Settings; profile forms then collect only those variables.

## Configuration

System Settings will contain only the template configuration:

- `profileTemplate`: a TOML template containing `${variable}` placeholders.
- `authTemplate`: a JSON object template whose string values may contain `${variable}` placeholders.
- `variables`: variable definitions with `name`, `label`, optional `default`, `required`, and `secret` fields.

Variable names are restricted to lowercase letters, digits, and underscores, starting with a letter or underscore. A template may only reference declared variables. Settings validation rejects malformed templates, unknown variables, and duplicate variable definitions.

For existing settings without templates, the server synthesizes the current provider, base URL, wire API, and environment key behavior. New settings use the same behavior as the default template.

## Form Behavior

The create and edit dialogs fetch the current system settings and render one input per variable. Defaults are initial values and remain editable. Variables without defaults are empty; required variables must be supplied before save. Secret variables use password inputs.

When editing an existing profile, non-secret values are populated from the stored profile data where possible. Secret values are never returned to the browser; an empty secret input preserves the existing stored value. The API key is therefore represented as a required secret variable without exposing its current value.

## Data Flow

1. Admin saves system settings.
2. Server validates the variable schema and both templates.
3. Admin opens create or edit and the page loads settings/profile detail.
4. The page submits variable values only.
5. Server merges defaults, validates required variables, preserves omitted secrets on edit, renders auth JSON and config TOML, and atomically replaces the profile directory.

## Error Handling and Security

The server is authoritative for validation and rendering; browser validation is only a usability aid. Secret values are accepted over the existing authenticated admin API but are omitted from profile list and detail responses. Template rendering rejects unsupported placeholder syntax and prevents arbitrary file paths or template execution.

## Testing

Add tests for default settings compatibility, variable validation, unknown placeholders, rendering, required variables, secret preservation on edit, and the admin create/edit HTTP flow. Run `go test ./...`, `git diff --check`, and verify the served `/agentpal` page after restarting the local node on port 28888.
