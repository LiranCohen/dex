// Package frontend provides embedded frontend assets for the Dex server.
//
// The dist/ directory is populated by running `bun run build` in this directory.
// During CI releases, this is done automatically before building the Go binary.
//
// For local development, either:
//   - Run `bun run build` here first, OR
//   - Use `dex start --static ./frontend/dist` to serve from disk
package frontend

import "embed"

// Assets contains the built frontend files (index.html, assets/, etc.)
// This will fail to compile if dist/ doesn't exist - run `npm run build` first.
//
//go:embed dist/*
var Assets embed.FS
