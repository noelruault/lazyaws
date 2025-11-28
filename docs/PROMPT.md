# PROMPT

> Replace SERVICE_NAME by some service
> Usage example: "Apply the template for SERVICE_NAME. Start at Phase 1."

Phase 1: Inventory (light)

- Identify SERVICE_NAME-specific pieces in main.go (state fields, loader functions, render functions, key handling branches, messages) and shared helpers (filters/search, history, status messages).
- Note shared deps needed: aws client, region/config, Vim state, history/nav, viewport helpers.

Phase 2: Define boundaries and scaffolding

- Create package layout: internal/ui/SERVICE_NAME (or internal/views/...), with:
  * State struct for service-local fields (lists, filters, selections, details).
  * Render functions (RenderList/RenderDetails or similar).
  * Loader entrypoints that accept *aws.Client and return messages/data.
  * Optional input handlers if delegating keys later.
- Add/update shared interfaces/helpers in internal/ui/shared (viewport, truncate, filters, status formatting) if needed.

Phase 3: Extract SERVICE_NAME first

- Move SERVICE_NAME state from model into SERVICE_NAME.State; embed/compose in model.
- Move render functions to the new package; call them from View.
- Move loaders/messages/handlers; keep message types centralized to avoid cycles.
- Keep key handling centralized initially; delegate gradually if desired.
- gofmt + go test ./... after changes.

Phase 4: Repeat for next services (if doing multiple)

- Apply the same pattern one service at a time; run gofmt + go test after each extraction.

Phase 5: Consolidate shared utilities

- Extract duplicated helpers (truncate, ensureVisible/getVisibleRange, search filters, status formatting) into internal/ui/shared and refactor callers.

Phase 6: Clean up

- Remove now-unused fields/functions from main.go.
- Update docs (README/TODO) to reflect the new structure.

Phase 7: Wire loaders/actions fully

- Replace placeholders with real AWS calls in internal/aws/SERVICE_NAME.go and state loaders.
- Ensure command/screen routing includes SERVICE_NAME (service selector, :services, :SERVICE_NAME commands, history nav).
- Smoke test navigation/rendering (go test ./..., go build && ./lazyaws basic flow).
