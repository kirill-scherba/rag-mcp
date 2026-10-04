# rag-mcp — Licensing of runtime dependencies

Date: 2026-10-04 · Context: the RAG is the knowledge backend for the planned
GUI. This note records what the RAG (and therefore the GUI) depends on at
runtime and whether it may ship inside a paid product.

## Go modules (shipped in the binary)

| Module | License | Note |
| --- | --- | --- |
| `github.com/kirill-scherba/keyvalembd` | BSD-3-Clause | storage + embeddings |
| `modernc.org/sqlite` (+ `libc`, `mem`, `mathutil`) | BSD-3-Clause | pure-Go SQLite |
| `github.com/mark3labs/mcp-go` | MIT | MCP |
| `github.com/spf13/cobra` | MIT | CLI |
| `github.com/kirill-scherba/s3lite` | BSD-3-Clause | storage interface |
| `github.com/kirill-scherba/anytext` | proprietary (first-party) | file → text, see its docs/LICENSING.md |

No copyleft — everything shipped is MIT or BSD. Since keyvalembd v0.6.x
dropped libSQL/go-libsql for `modernc.org/sqlite`, the whole RAG now builds
with **`CGO_ENABLED=0`** — a single static binary (~27 MB), which makes it easy
to bundle with the GUI.

## Ollama (embeddings — a required runtime dependency)

- **Engine: MIT** (`github.com/ollama/ollama/LICENSE`). May be bundled,
  modified and redistributed with a proprietary product; keep the copyright
  notice.
- **Installation is easy and end-user friendly:** a single self-contained
  binary per OS (official install script, Linux tarball + systemd, macOS
  app/pkg, Windows installer). On this machine it is `/usr/local/bin/ollama`
  with a systemd service. Bundling or requiring it are both practical.
- **Embedding model `embeddinggemma:latest` is Google Gemma.** It ships under
  the **Gemma Terms of Use** (not an OSI license): commercial use is allowed,
  subject to an acceptable-use policy; the terms must be passed through and
  included, and redistributing the weights has conditions. Downloading the
  model at first run (the normal Ollama flow) keeps the product from
  redistributing weights itself.
- **Answer model `deepseek-v4.1-flash:cloud` is an Ollama Cloud model** — a
  third-party hosted service, not redistributable, and requires network and an
  Ollama account. It is used only by `rag_query`; the GUI v1 deliberately does
  not use an LLM, so it does not depend on the cloud service.

## Verdict

Shipping the RAG (and a GUI on top of it) is fine for a proprietary product:

- bundle the Ollama **engine** (MIT) or require its installation;
- keep embeddings on Ollama with `embeddinggemma` (Gemma Terms — include the
  terms and obey acceptable use) downloaded at first run;
- keep the answer LLM optional and provider-selectable (`LLM_MODEL` /
  `--model`); a cloud model is a service dependency, not a distributed
  component.
