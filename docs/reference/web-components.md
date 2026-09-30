# Web projection components

`web/src/components.tsx` contains the shared read-only UI primitives. `Brand`,
`AppChrome`, `Panel`, and `StepRail` provide structure; buttons default to
`type="button"`; notices reserve assertive announcements for errors; dialogs
provide focus management and Escape dismissal.

The canvas in `web/src/main.tsx` renders canonical nodes and real edges from the
bounded projection API. Keyboard navigation, a selectable node list, visible
focus, and reduced-motion behaviour are part of its acceptance contract.
Visual activation represents selected evidence only.
