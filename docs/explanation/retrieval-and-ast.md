# Retrieval and AST evidence

Exact identifier/path/literal candidates, FTS5 lexical candidates,
Tree-sitter/compiler structure, and canonical claim traversal remain separate,
inspectable retrieval sources. The bounded hybrid planner fuses those sources
deterministically. An opt-in local vector projection runs only after those
routes are insufficient, or when `kb.query` requests `retrieval_mode:"vector"`.
Its score is a retrieval aid; returned records still resolve to canonical evidence.

Every factual result resolves to an active-generation entity, claim, source
revision, and evidence span. Unsupported dynamic routing, reflection,
registration, dependency injection, or computed protocol names produce
coverage diagnostics and `unknown`, not invented topology.
