package foley

import _ "embed"

// Skill is the agent-facing manual — foley.md at the repository root,
// embedded verbatim so an installed binary can materialize it anywhere
// (`foley skill > SKILL.md`). The file carries skill frontmatter on
// purpose: agents load it as-is; a change to foley.md ships in the
// next build with zero extra wiring.
//
//go:embed foley.md
var Skill string
