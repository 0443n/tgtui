package main

import _ "embed"

// bakedEnv is the contents of .env, compiled into the binary by a normal
// `go build` (go:embed, no linker flags). This is how tgtui carries its own
// Telegram credentials (api_id, api_hash only) so a built binary runs
// standalone. The proxy is deliberately not baked - see config.Load.
//
// Baked credentials are authoritative: config.Load uses them and does not read
// env vars or the config file for the id/hash when they are present. An empty
// or credential-less .env leaves this empty, and tgtui falls back to env/config.
//
// .env is gitignored, so the secrets live only in the local file and the binary
// you build, never in source. .env must exist for the build to compile; use an
// empty one (cp .env.example .env) for a non-baked build.
//
//go:embed .env
var bakedEnv string
